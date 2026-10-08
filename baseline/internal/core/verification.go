package core

import (
	"context"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
)

const (
	checkExistence        = "EXISTENCIA"
	checkUniqueness       = "UNICIDAD"
	checkCustodyChain     = "CADENA_CUSTODIA"
	checkOperableState    = "ESTADO_OPERABLE"
	checkDispensedState   = "ESTADO_DISPENSADO"
	checkDispenserAllowed = "DISPENSADOR_HABILITADO"
	checkStateSequence    = "SECUENCIA_ESTADOS"
	checkAuthorizedPairs  = "PARES_AUTORIZADOS"

	checkOK           = "OK"
	checkFailed       = "FALLO"
	checkNotEvaluated = "NO_EVALUADO"

	verdictNotFound              = "NO_ENCONTRADA"
	verdictDuplicated            = "UNIDAD_DUPLICADA"
	verdictInvalidSequence       = "SECUENCIA_INVALIDA"
	verdictTransferNotAuthorized = "TRANSFERENCIA_NO_AUTORIZADA"
	verdictBlockingState         = "ESTADO_BLOQUEANTE"
	verdictTerminalState         = "ESTADO_TERMINAL"
	verdictExpiredByDate         = "VENCIDO_POR_FECHA"
	verdictNotDispensed          = "NO_DISPENSADA"
	verdictInvalidDispenser      = "DISPENSADOR_INVALIDO"
)

type custodyChainResult struct {
	OK      bool
	Verdict string
	Detail  string
}

// VerifyUnit comprueba autenticidad sobre un snapshot de solo lectura.
func (s *Store) VerifyUnit(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
) (UnitVerdict, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return UnitVerdict{}, internal(err, "no se pudo iniciar la verificacion")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := resolveInvoker(ctx, tx, credential); err != nil {
		return UnitVerdict{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return UnitVerdict{}, err
	}
	verdict := UnitVerdict{Verificaciones: []TraceCheck{
		{Check: checkExistence, Resultado: checkNotEvaluated},
		{Check: checkUniqueness, Resultado: checkNotEvaluated},
		{Check: checkCustodyChain, Resultado: checkNotEvaluated},
		{Check: checkOperableState, Resultado: checkNotEvaluated},
	}}
	unit, found, err := readUnitForVerification(ctx, tx, gtin, serial)
	if err != nil {
		return UnitVerdict{}, err
	}
	if !found {
		verdict.fail(0, verdictNotFound, "la unidad no existe en el estado publico")
		return verdict, nil
	}
	verdict.Estado = unit.Estado
	verdict.pass(0, "")
	history, err := readHistoryForVerification(ctx, tx, gtin, serial)
	if err != nil {
		return UnitVerdict{}, err
	}
	if entry, deleted := firstDeletion(history); deleted {
		verdict.fail(1, verdictDuplicated,
			"el historial registra una eliminacion de la clave en la transaccion "+entry.TxID)
		return verdict, nil
	}
	verdict.pass(1, "")
	chain, err := verifyCustodyChain(ctx, tx, history)
	if err != nil {
		return UnitVerdict{}, err
	}
	if !chain.OK {
		verdict.fail(2, chain.Verdict, chain.Detail)
		return verdict, nil
	}
	verdict.pass(2, "")
	switch {
	case domain.IsTerminalState(unit.Estado):
		verdict.fail(3, verdictTerminalState, string(unit.Estado))
	case domain.IsBlockingState(unit.Estado):
		verdict.fail(3, verdictBlockingState, string(unit.Estado))
	default:
		expired, err := unitExpiredByDateAt(unit, s.now().UTC())
		if err != nil {
			return UnitVerdict{}, err
		}
		if expired {
			verdict.fail(3, verdictExpiredByDate,
				"la fecha de vencimiento "+unit.FechaVencimiento+
					" ya paso y el evento INFORMAR_VENCIMIENTO todavia no se registro")
			return verdict, nil
		}
		verdict.pass(3, string(unit.Estado))
		verdict.Autentica = true
	}
	return verdict, nil
}

// VerifyTrace comprueba legitimidad sobre un snapshot de solo lectura.
func (s *Store) VerifyTrace(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
) (TraceVerdict, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return TraceVerdict{}, internal(err, "no se pudo iniciar la verificacion")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := authorizeTraceVerification(ctx, tx, credential); err != nil {
		return TraceVerdict{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return TraceVerdict{}, err
	}
	verdict := TraceVerdict{Verificaciones: []TraceCheck{
		{Check: checkExistence, Resultado: checkNotEvaluated},
		{Check: checkDispensedState, Resultado: checkNotEvaluated},
		{Check: checkDispenserAllowed, Resultado: checkNotEvaluated},
		{Check: checkStateSequence, Resultado: checkNotEvaluated},
		{Check: checkAuthorizedPairs, Resultado: checkNotEvaluated},
	}}
	unit, found, err := readUnitForVerification(ctx, tx, gtin, serial)
	if err != nil {
		return TraceVerdict{}, err
	}
	if !found {
		verdict.fail(0, verdictNotFound, "la unidad no existe en el estado publico")
		return verdict, nil
	}
	verdict.pass(0, "")
	if unit.Estado != domain.StateDispensado {
		verdict.fail(1, verdictNotDispensed, string(unit.Estado))
		return verdict, nil
	}
	verdict.pass(1, string(unit.Estado))
	history, err := readHistoryForVerification(ctx, tx, gtin, serial)
	if err != nil {
		return TraceVerdict{}, err
	}
	dispenser, err := verifyDispenser(ctx, tx, history)
	if err != nil {
		return TraceVerdict{}, err
	}
	if !dispenser.OK {
		verdict.fail(2, dispenser.Verdict, dispenser.Detail)
		return verdict, nil
	}
	verdict.pass(2, dispenser.Detail)
	chain, err := verifyCustodyChain(ctx, tx, history)
	if err != nil {
		return TraceVerdict{}, err
	}
	switch {
	case chain.OK:
		verdict.pass(3, "")
		verdict.pass(4, "")
		verdict.Legitima = true
	case chain.Verdict == verdictInvalidSequence:
		verdict.fail(3, verdictInvalidSequence, chain.Detail)
	default:
		verdict.pass(3, "")
		verdict.fail(4, verdictTransferNotAuthorized, chain.Detail)
	}
	return verdict, nil
}

func authorizeTraceVerification(ctx context.Context, tx pgx.Tx, credential Credential) error {
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return err
	}
	if err := requireAgentType(invoker, domain.AgentFinancier, domain.AgentRegulator); err != nil {
		return err
	}
	if invoker.Org.AgentType == domain.AgentFinancier {
		return requireRole(invoker, RoleFinancierAuditor)
	}
	return requireRole(invoker, RoleAuditor, RoleRegulatoryAdmin)
}

func readUnitForVerification(
	ctx context.Context,
	tx pgx.Tx,
	gtin, serial string,
) (MedicationUnit, bool, error) {
	unit, err := scanUnit(tx.QueryRow(ctx, `
		SELECT gtin, numero_serie, lote, fecha_vencimiento, custodio_actual, estado, ultima_actualizacion
		FROM public.medication_units WHERE gtin=$1 AND numero_serie=$2`, gtin, serial))
	if errors.Is(err, pgx.ErrNoRows) {
		return MedicationUnit{}, false, nil
	}
	if err != nil {
		return MedicationUnit{}, false, internal(err, "no se pudo leer la unidad")
	}
	return unit, true, nil
}

func readHistoryForVerification(
	ctx context.Context,
	tx pgx.Tx,
	gtin, serial string,
) ([]HistoryEntry, error) {
	rows, err := tx.Query(ctx, `
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
		entries = append(entries, HistoryEntry{
			TxID: txID, Timestamp: formatTimestamp(eventTimestamp), Value: &unit,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err, "fallo el recorrido del historial")
	}
	if len(entries) == 0 {
		return nil, internal(errors.New("historial ausente"), "la unidad existe sin historial")
	}
	return entries, nil
}

func firstDeletion(history []HistoryEntry) (HistoryEntry, bool) {
	for _, entry := range history {
		if entry.IsDelete {
			return entry, true
		}
	}
	return HistoryEntry{}, false
}

func verifyDispenser(ctx context.Context, tx pgx.Tx, history []HistoryEntry) (custodyChainResult, error) {
	dispenser := ""
	for _, entry := range history {
		if entry.Value != nil && entry.Value.Estado == domain.StateDispensado {
			dispenser = entry.Value.CustodioActual
			break
		}
	}
	if dispenser == "" {
		return custodyChainResult{}, internal(errors.New("historial dispensado inconsistente"),
			"la unidad figura DISPENSADO y su historial no registra la entrada que lo produjo")
	}
	types, err := agentTypeByCanonicalID(ctx, tx)
	if err != nil {
		return custodyChainResult{}, err
	}
	agentType, ok := types[dispenser]
	if !ok {
		return custodyChainResult{}, unregisteredCustodian(dispenser)
	}
	if agentType != domain.AgentPharmacy && agentType != domain.AgentHealthcare {
		return custodyChainResult{
			Verdict: verdictInvalidDispenser,
			Detail: "el dispensador es " + string(agentType) +
				" y T06 solo habilita a PHARMACY o HEALTHCARE_FACILITY",
		}, nil
	}
	return custodyChainResult{OK: true, Detail: string(agentType)}, nil
}

func verifyCustodyChain(ctx context.Context, tx pgx.Tx, history []HistoryEntry) (custodyChainResult, error) {
	types, err := agentTypeByCanonicalID(ctx, tx)
	if err != nil {
		return custodyChainResult{}, err
	}
	result, err := verifyStateSequence(history, types)
	if err != nil || !result.OK {
		return result, err
	}
	return verifyAuthorizedPairs(history, types)
}

func verifyStateSequence(
	history []HistoryEntry,
	types map[string]domain.AgentType,
) (custodyChainResult, error) {
	var previous *MedicationUnit
	for _, entry := range history {
		if entry.Value == nil {
			continue
		}
		current := entry.Value
		if previous == nil {
			result, err := verifyChainOrigin(*current, types)
			if err != nil || !result.OK {
				return result, err
			}
			previous = current
			continue
		}
		if !domain.IsDeclaredStatePair(previous.Estado, current.Estado) {
			return custodyChainResult{
				Verdict: verdictInvalidSequence,
				Detail:  string(previous.Estado) + " -> " + string(current.Estado),
			}, nil
		}
		if result := verifyHandoverCoupling(*previous, *current); !result.OK {
			return result, nil
		}
		previous = current
	}
	return custodyChainResult{OK: true}, nil
}

func verifyHandoverCoupling(previous, current MedicationUnit) custodyChainResult {
	changed := current.CustodioActual != previous.CustodioActual
	isReception := previous.Estado == domain.StateEnTransito && current.Estado == domain.StateEnCustodia
	switch {
	case changed && !isReception:
		return custodyChainResult{
			Verdict: verdictInvalidSequence,
			Detail: "la custodia cambio en " + string(previous.Estado) + " -> " +
				string(current.Estado) + ", y solo la recepcion (T04) la mueve",
		}
	case !changed && isReception:
		return custodyChainResult{
			Verdict: verdictInvalidSequence,
			Detail:  "EN_TRANSITO -> EN_CUSTODIA sin cambio de custodio",
		}
	default:
		return custodyChainResult{OK: true}
	}
}

func verifyAuthorizedPairs(
	history []HistoryEntry,
	types map[string]domain.AgentType,
) (custodyChainResult, error) {
	var previous *MedicationUnit
	for _, entry := range history {
		if entry.Value == nil {
			continue
		}
		current := entry.Value
		if previous == nil {
			previous = current
			continue
		}
		if current.CustodioActual == previous.CustodioActual {
			previous = current
			continue
		}
		origin, ok := types[previous.CustodioActual]
		if !ok {
			return custodyChainResult{}, unregisteredCustodian(previous.CustodioActual)
		}
		destination, ok := types[current.CustodioActual]
		if !ok {
			return custodyChainResult{}, unregisteredCustodian(current.CustodioActual)
		}
		decision, err := domain.DecideTransfer(origin, destination)
		if err != nil {
			return custodyChainResult{}, internal(err, "no se pudo evaluar la matriz de transferencias")
		}
		if !decision.Allowed {
			return custodyChainResult{
				Verdict: verdictTransferNotAuthorized,
				Detail:  string(origin) + " -> " + string(destination),
			}, nil
		}
		previous = current
	}
	return custodyChainResult{OK: true}, nil
}

func verifyChainOrigin(
	first MedicationUnit,
	types map[string]domain.AgentType,
) (custodyChainResult, error) {
	if first.Estado != domain.InitialState {
		return custodyChainResult{
			Verdict: verdictInvalidSequence,
			Detail: "el historial arranca en " + string(first.Estado) +
				" y el unico estado inicial de ADR-001 es " + string(domain.InitialState),
		}, nil
	}
	agentType, ok := types[first.CustodioActual]
	if !ok {
		return custodyChainResult{}, unregisteredCustodian(first.CustodioActual)
	}
	if agentType != domain.AgentLaboratory {
		return custodyChainResult{
			Verdict: verdictInvalidSequence,
			Detail:  "el primer custodio es " + string(agentType) + " y T01 solo habilita a LABORATORY",
		}, nil
	}
	return custodyChainResult{OK: true}, nil
}

func agentTypeByCanonicalID(ctx context.Context, tx pgx.Tx) (map[string]domain.AgentType, error) {
	rows, err := tx.Query(ctx, `SELECT id_type, id, agent_type FROM public.organizations`)
	if err != nil {
		return nil, internal(err, "no se pudo leer el registro de organizaciones")
	}
	defer rows.Close()
	types := map[string]domain.AgentType{}
	for rows.Next() {
		var idType, id string
		var agentType domain.AgentType
		if err := rows.Scan(&idType, &id, &agentType); err != nil {
			return nil, internal(err, "no se pudo leer una organizacion")
		}
		types[idType+":"+id] = agentType
	}
	if err := rows.Err(); err != nil {
		return nil, internal(err, "fallo el recorrido del registro de organizaciones")
	}
	return types, nil
}

func unregisteredCustodian(canonicalID string) error {
	return NewError(OrgNotRegistered,
		"el custodio %s del historial no tiene entrada en el registro organizacion-establecimiento", canonicalID).
		WithDetails(map[string]any{"custodio": canonicalID})
}

func (verdict *UnitVerdict) pass(index int, detail string) {
	verdict.Verificaciones[index].Resultado = checkOK
	verdict.Verificaciones[index].Detalle = detail
}

func (verdict *UnitVerdict) fail(index int, reason, detail string) {
	verdict.Verificaciones[index].Resultado = checkFailed
	verdict.Verificaciones[index].Detalle = detail
	verdict.Motivo = reason
	verdict.Autentica = false
}

func (verdict *TraceVerdict) pass(index int, detail string) {
	verdict.Verificaciones[index].Resultado = checkOK
	verdict.Verificaciones[index].Detalle = detail
}

func (verdict *TraceVerdict) fail(index int, reason, detail string) {
	verdict.Verificaciones[index].Resultado = checkFailed
	verdict.Verificaciones[index].Detalle = detail
	verdict.Motivo = reason
	verdict.Legitima = false
}
