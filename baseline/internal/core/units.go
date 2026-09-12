package core

import (
	"context"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
)

const (
	opRegisterUnit     = "RegisterUnit"
	opDispatchTransfer = "DispatchTransfer"
	opReceiveTransfer  = "ReceiveTransfer"
	opRejectTransfer   = "RejectTransfer"
	opDispense         = "Dispense"
)

type rowScanner interface{ Scan(dest ...any) error }

func scanUnit(row rowScanner) (MedicationUnit, error) {
	var unit MedicationUnit
	var expiry, updated time.Time
	if err := row.Scan(
		&unit.GTIN, &unit.NumeroSerie, &unit.Lote, &expiry,
		&unit.CustodioActual, &unit.Estado, &updated,
	); err != nil {
		return MedicationUnit{}, err
	}
	unit.FechaVencimiento = expiry.Format(time.DateOnly)
	unit.UltimaActualizacion = formatTimestamp(updated)
	return unit, nil
}

func readLockedUnit(ctx context.Context, tx pgx.Tx, gtin, serial string) (MedicationUnit, error) {
	unit, err := scanUnit(tx.QueryRow(ctx, `
		SELECT gtin, numero_serie, lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion
		FROM public.medication_units WHERE gtin=$1 AND numero_serie=$2 FOR UPDATE`, gtin, serial))
	if errors.Is(err, pgx.ErrNoRows) {
		return MedicationUnit{}, NewError(UnitNotFound, "la unidad %s/%s no existe", gtin, serial).WithDetails(map[string]any{"gtin": gtin, "numeroSerie": serial})
	}
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo leer la unidad")
	}
	return unit, nil
}

func updateUnit(ctx context.Context, tx pgx.Tx, unit MedicationUnit) error {
	_, err := tx.Exec(ctx, `
		UPDATE public.medication_units
		SET custodio_actual=$3, estado=$4, ultima_actualizacion=$5
		WHERE gtin=$1 AND numero_serie=$2`,
		unit.GTIN, unit.NumeroSerie, unit.CustodioActual, unit.Estado, unit.UltimaActualizacion)
	if err != nil {
		return internal(err, "no se pudo actualizar la unidad")
	}
	return nil
}

// appendUnitEvent se invoca mientras la transaccion conserva el lock de la
// fila medication_units. Por eso event_sequence es un orden total por unidad
// que coincide con el orden de confirmacion, aun con timestamps iguales.
func appendUnitEvent(ctx context.Context, tx pgx.Tx, txID, operation, invokerMSPID string, unit MedicationUnit) error {
	var sequence int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_sequence), 0) + 1
		FROM public.unit_events WHERE gtin=$1 AND numero_serie=$2`, unit.GTIN, unit.NumeroSerie).Scan(&sequence); err != nil {
		return internal(err, "no se pudo asignar el orden del evento")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO public.unit_events (
			gtin, numero_serie, tx_id, event_timestamp, event_sequence,
			operation, invoker_msp_id, lote, fecha_vencimiento,
			custodio_actual, estado, ultima_actualizacion
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		unit.GTIN, unit.NumeroSerie, txID, unit.UltimaActualizacion, sequence,
		operation, invokerMSPID, unit.Lote, unit.FechaVencimiento,
		unit.CustodioActual, unit.Estado, unit.UltimaActualizacion)
	if err != nil {
		return internal(err, "no se pudo agregar el evento de la unidad")
	}
	return nil
}

func (s *Store) registerUnitInTx(
	ctx context.Context,
	tx pgx.Tx,
	invoker Invoker,
	req RegisterUnitRequest,
	timestamp string,
) (MedicationUnit, error) {
	transition, ok := domain.LookupInitialTransition(domain.EventRegistrarUnidad)
	if !ok || !transition.AllowsActor(domain.ActorLaboratory) {
		return MedicationUnit{}, NewError(InvalidStateTransition, "la maquina de estados no declara el alta para LABORATORY")
	}
	if err := requireAgentType(invoker, domain.AgentLaboratory); err != nil {
		return MedicationUnit{}, err
	}
	if err := requireRole(invoker, RoleOperator); err != nil {
		return MedicationUnit{}, err
	}
	if err := ValidateRegisterUnitRequest(req); err != nil {
		return MedicationUnit{}, err
	}
	unit := MedicationUnit{
		GTIN: req.GTIN, NumeroSerie: req.NumeroSerie, Lote: req.Lote,
		FechaVencimiento: req.FechaVencimiento, CustodioActual: invoker.CanonicalID(),
		Estado: transition.To, UltimaActualizacion: timestamp,
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO public.medication_units
		(gtin, numero_serie, lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		unit.GTIN, unit.NumeroSerie, unit.Lote, unit.FechaVencimiento,
		unit.CustodioActual, unit.Estado, unit.UltimaActualizacion)
	if err != nil {
		if isUniqueViolation(err) {
			return MedicationUnit{}, NewError(UnitAlreadyExists, "la unidad %s/%s ya esta registrada", req.GTIN, req.NumeroSerie)
		}
		return MedicationUnit{}, internal(err, "no se pudo registrar la unidad")
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	if err := appendUnitEvent(ctx, tx, txID, opRegisterUnit, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func (s *Store) RegisterUnit(ctx context.Context, credential Credential, req RegisterUnitRequest) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	unit, err := s.registerUnitInTx(ctx, tx, invoker, req, formatTimestamp(s.now()))
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func (s *Store) ReadUnit(ctx context.Context, gtin, serial string) (MedicationUnit, error) {
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	unit, err := scanUnit(s.pool.QueryRow(ctx, `
		SELECT gtin, numero_serie, lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion
		FROM public.medication_units WHERE gtin=$1 AND numero_serie=$2`, gtin, serial))
	if errors.Is(err, pgx.ErrNoRows) {
		return MedicationUnit{}, NewError(UnitNotFound, "la unidad %s/%s no existe", gtin, serial).WithDetails(map[string]any{"gtin": gtin, "numeroSerie": serial})
	}
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo leer la unidad")
	}
	return unit, nil
}

func (s *Store) QueryUnitsByGTIN(ctx context.Context, gtin string) ([]MedicationUnit, error) {
	if err := validateGTIN(gtin); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT gtin, numero_serie, lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion
		FROM public.medication_units WHERE gtin=$1 ORDER BY numero_serie`, gtin)
	if err != nil {
		return nil, internal(err, "no se pudo consultar las unidades del GTIN")
	}
	defer rows.Close()
	units := []MedicationUnit{}
	for rows.Next() {
		unit, err := scanUnit(rows)
		if err != nil {
			return nil, internal(err, "no se pudo leer una unidad del resultado")
		}
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err, "fallo el recorrido de unidades")
	}
	return units, nil
}

func (s *Store) GetUnitHistory(ctx context.Context, gtin, serial string) ([]HistoryEntry, error) {
	if err := validateUnitRef(gtin, serial); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT tx_id, event_timestamp, gtin, numero_serie, lote, fecha_vencimiento,
		       custodio_actual, estado, ultima_actualizacion
		FROM public.unit_events WHERE gtin=$1 AND numero_serie=$2 ORDER BY event_sequence`, gtin, serial)
	if err != nil {
		return nil, internal(err, "no se pudo leer el historial")
	}
	defer rows.Close()
	entries := []HistoryEntry{}
	for rows.Next() {
		var txID string
		var eventTimestamp, expiry, updated time.Time
		unit := MedicationUnit{}
		if err := rows.Scan(&txID, &eventTimestamp, &unit.GTIN, &unit.NumeroSerie, &unit.Lote,
			&expiry, &unit.CustodioActual, &unit.Estado, &updated); err != nil {
			return nil, internal(err, "no se pudo leer una entrada del historial")
		}
		unit.FechaVencimiento = expiry.Format(time.DateOnly)
		unit.UltimaActualizacion = formatTimestamp(updated)
		entries = append(entries, HistoryEntry{TxID: txID, Timestamp: formatTimestamp(eventTimestamp), IsDelete: false, Value: &unit})
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err, "fallo el recorrido del historial")
	}
	if len(entries) == 0 {
		return nil, NewError(UnitNotFound, "la unidad %s/%s no existe", gtin, serial).WithDetails(map[string]any{"gtin": gtin, "numeroSerie": serial})
	}
	return entries, nil
}
