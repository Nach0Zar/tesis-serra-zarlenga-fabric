package core

import (
	"context"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
)

type transferOperation struct {
	TxIDDespacho          string
	Emisor                string
	DestinatarioPendiente string
	RuleID                string
	SchemaVersion         string
}

func readActiveTransfer(ctx context.Context, tx pgx.Tx, gtin, serial string) (transferOperation, bool, error) {
	var operation transferOperation
	err := tx.QueryRow(ctx, `
		SELECT tx_id_despacho, emisor, destinatario_pendiente, rule_id, schema_version
		FROM public.transfer_operations
		WHERE gtin=$1 AND numero_serie=$2 AND estado='ACTIVA'`, gtin, serial).
		Scan(&operation.TxIDDespacho, &operation.Emisor, &operation.DestinatarioPendiente, &operation.RuleID, &operation.SchemaVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return transferOperation{}, false, nil
	}
	if err != nil {
		return transferOperation{}, false, internal(err, "no se pudo leer la operacion activa")
	}
	return operation, true, nil
}

func (s *Store) Dispatch(ctx context.Context, credential Credential, gtin, serial string, req DispatchRequest) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := requireRole(invoker, RoleOperator); err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if unit.CustodioActual != invoker.CanonicalID() {
		return MedicationUnit{}, NewError(UnauthorizedCustodian, "el invocador no es el custodio actual de la unidad").WithDetails(map[string]any{"custodioActual": unit.CustodioActual})
	}
	transition, err := requireTransition(unit.Estado, domain.EventDistribuirEslabonPosterior, domain.ActorCurrentCustodian)
	if err != nil {
		return MedicationUnit{}, err
	}
	if req.Destino == "" {
		return MedicationUnit{}, NewError(InvalidRequest, "destino es obligatorio")
	}
	destination, err := resolveDestination(ctx, tx, req.Destino)
	if err != nil {
		return MedicationUnit{}, err
	}
	if destination.MSPID == invoker.MSPID {
		return MedicationUnit{}, NewError(InvalidDestination, "el destino declarado es la propia organizacion emisora").WithDetails(map[string]any{"destino": req.Destino})
	}
	decision, err := domain.DecideTransfer(invoker.Org.AgentType, destination.AgentType)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed {
		return MedicationUnit{}, NewError(TransferNotAuthorized, "el par %s -> %s no esta autorizado", invoker.Org.AgentType, destination.AgentType).WithDetails(map[string]any{
			"origen": string(invoker.Org.AgentType), "destino": string(destination.AgentType), "razon": decision.Reason,
		})
	}
	commercial := CommercialData{NumeroRemito: req.NumeroRemito, NumeroFactura: req.NumeroFactura, Cantidad: req.Cantidad}
	if err := validateCommercial(commercial); err != nil {
		return MedicationUnit{}, err
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	timestamp := formatTimestamp(s.now())
	_, err = tx.Exec(ctx, `
		INSERT INTO public.transfer_operations (
			gtin, numero_serie, tx_id_despacho, emisor, destinatario_pendiente,
			numero_remito, numero_factura, cantidad, rule_id, schema_version, despachada_en, estado
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ACTIVA')`,
		gtin, serial, txID, invoker.CanonicalID(), destination.CanonicalID(),
		req.NumeroRemito, req.NumeroFactura, req.Cantidad, decision.RuleID, decision.SchemaVersion, timestamp)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo registrar el despacho")
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := appendUnitEvent(ctx, tx, txID, opDispatchTransfer, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func (s *Store) Receive(ctx context.Context, credential Credential, gtin, serial string, commercial *CommercialData) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := requireRole(invoker, RoleOperator); err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if unit.Estado != domain.StateEnTransito {
		return MedicationUnit{}, NewError(NotInTransit, "la unidad no esta en EN_TRANSITO").WithDetails(map[string]any{"estado": string(unit.Estado)})
	}
	transition, err := requireTransition(unit.Estado, domain.EventRecibirEnEstablecimiento, domain.ActorDestinationAgent)
	if err != nil {
		return MedicationUnit{}, err
	}
	operation, found, err := readActiveTransfer(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if !found {
		return MedicationUnit{}, internal(errors.New("operacion activa ausente"), "unidad en transito inconsistente")
	}
	if operation.DestinatarioPendiente != invoker.CanonicalID() {
		return MedicationUnit{}, NewError(ReceiverMismatch, "el invocador no coincide con el destinatario declarado de la operacion activa")
	}
	emitter, err := lookupOrganizationByCanonicalID(ctx, tx, operation.Emisor)
	if err != nil {
		return MedicationUnit{}, err
	}
	decision, err := domain.DecideTransfer(emitter.AgentType, invoker.Org.AgentType)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed || decision.RuleID != operation.RuleID || decision.SchemaVersion != operation.SchemaVersion {
		return MedicationUnit{}, NewError(TransferNotAuthorized, "la matriz no coincide con la regla que autorizo el despacho").WithDetails(map[string]any{
			"ruleIdDespacho": operation.RuleID, "schemaVersionDespacho": operation.SchemaVersion,
			"ruleIdReceptor": decision.RuleID, "schemaVersionReceptor": decision.SchemaVersion,
		})
	}
	if commercial != nil {
		if commercial.NumeroRemito == "" && commercial.NumeroFactura == "" && commercial.Cantidad == 0 {
			commercial = nil
		} else if err := validateCommercial(*commercial); err != nil {
			return MedicationUnit{}, err
		}
	}
	timestamp := formatTimestamp(s.now())
	var remito, factura any
	var cantidad any
	if commercial != nil {
		remito, factura, cantidad = commercial.NumeroRemito, commercial.NumeroFactura, commercial.Cantidad
	}
	_, err = tx.Exec(ctx, `
		UPDATE public.transfer_operations SET estado='CERRADA', cerrada_en=$4,
		motivo_cierre='RECEPCION', recepcion_numero_remito=$5,
		recepcion_numero_factura=$6, recepcion_cantidad=$7
		WHERE gtin=$1 AND numero_serie=$2 AND tx_id_despacho=$3 AND estado='ACTIVA'`,
		gtin, serial, operation.TxIDDespacho, timestamp, remito, factura, cantidad)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo cerrar la transferencia recibida")
	}
	unit.CustodioActual = invoker.CanonicalID()
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	if err := appendUnitEvent(ctx, tx, txID, opReceiveTransfer, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func (s *Store) Reject(ctx context.Context, credential Credential, gtin, serial string, req RejectRequest) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := requireRole(invoker, RoleOperator); err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	if req.Motivo == "" {
		return MedicationUnit{}, NewError(InvalidRequest, "motivo es obligatorio: el rechazo debe documentar su causa")
	}
	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if unit.Estado != domain.StateEnTransito {
		return MedicationUnit{}, NewError(NotInTransit, "la unidad no esta en EN_TRANSITO").WithDetails(map[string]any{"estado": string(unit.Estado)})
	}
	operation, found, err := readActiveTransfer(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if !found {
		return MedicationUnit{}, internal(errors.New("operacion activa ausente"), "unidad en transito inconsistente")
	}
	isEmitter := unit.CustodioActual == invoker.CanonicalID()
	actor := domain.ActorDestinationAgent
	if isEmitter {
		actor = domain.ActorCurrentCustodian
	}
	if !isEmitter && operation.DestinatarioPendiente != invoker.CanonicalID() {
		return MedicationUnit{}, NewError(ReceiverMismatch, "el invocador no es el emisor ni el destinatario declarado")
	}
	transition, err := requireTransition(unit.Estado, domain.EventDevolverProducto, actor)
	if err != nil {
		return MedicationUnit{}, err
	}
	timestamp := formatTimestamp(s.now())
	_, err = tx.Exec(ctx, `
		UPDATE public.transfer_operations SET estado='CERRADA', cerrada_en=$4, motivo_cierre='RECHAZO'
		WHERE gtin=$1 AND numero_serie=$2 AND tx_id_despacho=$3 AND estado='ACTIVA'`,
		gtin, serial, operation.TxIDDespacho, timestamp)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo cerrar la transferencia rechazada")
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	if err := appendUnitEvent(ctx, tx, txID, opRejectTransfer, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func (s *Store) Dispense(ctx context.Context, credential Credential, gtin, serial string) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := requireAgentType(invoker, domain.AgentPharmacy, domain.AgentHealthcare); err != nil {
		return MedicationUnit{}, err
	}
	if err := requireRole(invoker, RoleOperator); err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	if unit.CustodioActual != invoker.CanonicalID() {
		return MedicationUnit{}, NewError(UnauthorizedCustodian, "el invocador no es el custodio actual de la unidad").WithDetails(map[string]any{"custodioActual": unit.CustodioActual})
	}
	transition, err := requireTransition(unit.Estado, domain.EventDispensarPaciente, domain.ActorDispensingAgent)
	if err != nil {
		return MedicationUnit{}, err
	}
	expiry, err := time.Parse(time.DateOnly, unit.FechaVencimiento)
	if err != nil {
		return MedicationUnit{}, internal(err, "fecha de vencimiento persistida invalida")
	}
	now := s.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if today.After(expiry) {
		return MedicationUnit{}, NewError(InvalidStateTransition, "la unidad no es dispensable: su fecha de vencimiento ya paso").WithDetails(map[string]any{
			"estado": string(unit.Estado), "fechaVencimiento": unit.FechaVencimiento, "causa": "VENCIDO_POR_FECHA",
		})
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = formatTimestamp(now)
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	if err := appendUnitEvent(ctx, tx, txID, opDispense, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}
