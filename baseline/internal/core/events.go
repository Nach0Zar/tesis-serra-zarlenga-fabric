package core

import (
	"context"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
)

const (
	opQuarantine         = "Quarantine"
	opReleaseQuarantine  = "ReleaseQuarantine"
	opReportExpired      = "ReportExpired"
	opReportStolen       = "ReportStolen"
	opReportLost         = "ReportLost"
	opReportDamaged      = "ReportDamaged"
	opReturnProduct      = "ReturnProduct"
	opRestock            = "Restock"
	opWithdrawFromMarket = "WithdrawFromMarket"
	opProhibitProduct    = "ProhibitProduct"
	opFinalDisposition   = "FinalDisposition"
	transitionExpiredT13 = "T13_MARK_EXPIRED_FROM_TRANSIT_OR_QUARANTINE"
)

type eventPrecondition func(
	context.Context,
	pgx.Tx,
	MedicationUnit,
	Invoker,
	domain.Transition,
	time.Time,
) error

func (s *Store) Quarantine(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventPonerEnCuarentena, opQuarantine, false, nil)
}

func (s *Store) ReleaseQuarantine(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventLiberarCuarentena, opReleaseQuarantine, false, nil)
}

func (s *Store) ReportExpired(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventInformarVencimiento, opReportExpired, false, requireExpiredByDateInTransit)
}

func (s *Store) ReportStolen(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventInformarRobo, opReportStolen, false, nil)
}

func (s *Store) ReportLost(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventInformarExtravio, opReportLost, false, nil)
}

func (s *Store) ReportDamaged(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventInformarDeterioro, opReportDamaged, false, nil)
}

func (s *Store) Restock(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventReingresarStock, opRestock, false, restockPrecondition)
}

func (s *Store) WithdrawFromMarket(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventRetirarMercado, opWithdrawFromMarket, false, consumeWithdrawIntervention)
}

func (s *Store) ProhibitProduct(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventProhibirProducto, opProhibitProduct, true, nil)
}

func (s *Store) FinalDisposition(ctx context.Context, credential Credential, gtin, serial string, req UnitEventRequest) (MedicationUnit, error) {
	return s.applyExtraordinaryEvent(ctx, credential, gtin, serial, req, domain.EventDisponerFinal, opFinalDisposition, false, consumeFinalDispositionIntervention)
}

func (s *Store) applyExtraordinaryEvent(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
	req UnitEventRequest,
	event domain.Event,
	operation string,
	regulatorOnly bool,
	precondition eventPrecondition,
) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var invoker Invoker
	if regulatorOnly {
		invoker, err = resolveRegulator(ctx, tx, credential)
	} else {
		invoker, err = resolveInvoker(ctx, tx, credential)
		if err == nil {
			err = requireExtraordinaryEventRole(invoker)
		}
	}
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	if req.Motivo == "" {
		return MedicationUnit{}, NewError(InvalidRequest, "motivo es obligatorio para documentar la causa del evento")
	}

	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	actor, err := resolveExtraordinaryEventActor(ctx, tx, unit, invoker, event)
	if err != nil {
		return MedicationUnit{}, err
	}
	transition, err := requireTransition(unit.Estado, event, actor)
	if err != nil {
		return MedicationUnit{}, err
	}
	now := s.now().UTC()
	if precondition != nil {
		if err := precondition(ctx, tx, unit, invoker, transition, now); err != nil {
			return MedicationUnit{}, err
		}
	}
	timestamp := formatTimestamp(now)
	if unit.Estado == domain.StateEnTransito {
		if err := closeActiveTransferForExtraordinaryEvent(ctx, tx, unit, timestamp); err != nil {
			return MedicationUnit{}, err
		}
	}
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := appendUnitEvent(ctx, tx, txID, operation, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func requireExtraordinaryEventRole(invoker Invoker) error {
	if invoker.Org.AgentType == domain.AgentRegulator {
		return requireRole(invoker, RoleRegulatoryAdmin)
	}
	return requireRole(invoker, RoleOperator)
}

func resolveExtraordinaryEventActor(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	event domain.Event,
) (domain.Actor, error) {
	transition, declared := domain.LookupTransition(unit.Estado, event)
	characters, err := invokerCharacters(ctx, tx, unit, invoker, event)
	if err != nil {
		return "", err
	}
	if len(characters) == 0 {
		return "", NewError(UnauthorizedCustodian,
			"el invocador no es el custodio actual, el destinatario declarado, el laboratorio titular ni la organizacion regulatoria").
			WithDetails(map[string]any{"custodioActual": unit.CustodioActual, "estado": string(unit.Estado)})
	}
	if declared {
		for _, actor := range characters {
			if transition.AllowsActor(actor) {
				return actor, nil
			}
		}
	}
	return characters[0], nil
}

func invokerCharacters(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	event domain.Event,
) ([]domain.Actor, error) {
	if invoker.Org.AgentType == domain.AgentRegulator {
		return []domain.Actor{domain.ActorANMAT}, nil
	}
	characters := []domain.Actor{}
	if invoker.Org.AgentType == domain.AgentLaboratory && eventAdmitsActor(event, domain.ActorLaboratory) {
		characters = append(characters, domain.ActorLaboratory)
	}
	if unit.CustodioActual == invoker.CanonicalID() {
		characters = append(characters, domain.ActorCurrentCustodian, domain.ActorRecoveryOrDisposalAgent)
	}
	if unit.Estado == domain.StateEnTransito && eventAdmitsActor(event, domain.ActorDestinationAgent) {
		operation, found, err := readActiveTransfer(ctx, tx, unit.GTIN, unit.NumeroSerie)
		if err != nil {
			return nil, err
		}
		if found && operation.DestinatarioPendiente == invoker.CanonicalID() {
			characters = append(characters, domain.ActorDestinationAgent)
		}
	}
	return characters, nil
}

func eventAdmitsActor(event domain.Event, actor domain.Actor) bool {
	for _, transition := range domain.Transitions() {
		if transition.Event == event && transition.AllowsActor(actor) {
			return true
		}
	}
	return false
}

func closeActiveTransferForExtraordinaryEvent(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	timestamp string,
) error {
	operation, found, err := readActiveTransfer(ctx, tx, unit.GTIN, unit.NumeroSerie)
	if err != nil {
		return err
	}
	if !found {
		return internal(errors.New("operacion activa ausente"), "unidad en transito inconsistente")
	}
	command, err := tx.Exec(ctx, `
		UPDATE public.transfer_operations
		SET estado='CERRADA', cerrada_en=$4, motivo_cierre='EVENTO_EXTRAORDINARIO'
		WHERE gtin=$1 AND numero_serie=$2 AND tx_id_despacho=$3 AND estado='ACTIVA'`,
		unit.GTIN, unit.NumeroSerie, operation.TxIDDespacho, timestamp)
	if err != nil {
		return internal(err, "no se pudo cerrar la transferencia por evento extraordinario")
	}
	if command.RowsAffected() != 1 {
		return internal(errors.New("la operacion activa cambio concurrentemente"), "no se pudo cerrar el transito")
	}
	return nil
}

func requireExpiredByDateInTransit(
	_ context.Context,
	_ pgx.Tx,
	unit MedicationUnit,
	_ Invoker,
	transition domain.Transition,
	now time.Time,
) error {
	if transition.ID != transitionExpiredT13 {
		return nil
	}
	expired, err := unitExpiredByDateAt(unit, now)
	if err != nil {
		return err
	}
	if expired {
		return nil
	}
	return NewError(InvalidStateTransition,
		"T13 exige que la fecha de vencimiento %s ya haya sido alcanzada", unit.FechaVencimiento).
		WithDetails(map[string]any{
			"estado": string(unit.Estado), "fechaVencimiento": unit.FechaVencimiento, "transicion": transition.ID,
		})
}

func restockPrecondition(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	_ domain.Transition,
	now time.Time,
) error {
	expired, err := unitExpiredByDateAt(unit, now)
	if err != nil {
		return err
	}
	if expired {
		return NewError(InvalidStateTransition,
			"el reingreso a stock exige una unidad apta y la fecha de vencimiento %s ya fue alcanzada", unit.FechaVencimiento).
			WithDetails(map[string]any{
				"estado": string(unit.Estado), "fechaVencimiento": unit.FechaVencimiento, "causa": "VENCIDO_POR_FECHA",
			})
	}
	return consumeLabIntervention(ctx, tx, unit, invoker, now, LabOpRestock)
}

func consumeWithdrawIntervention(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	_ domain.Transition,
	now time.Time,
) error {
	return consumeLabIntervention(ctx, tx, unit, invoker, now, LabOpWithdrawFromMarket)
}

func consumeFinalDispositionIntervention(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	_ domain.Transition,
	now time.Time,
) error {
	return consumeLabIntervention(ctx, tx, unit, invoker, now, LabOpFinalDisposition)
}

func unitExpiredByDateAt(unit MedicationUnit, now time.Time) (bool, error) {
	if unit.FechaVencimiento == "" {
		return false, nil
	}
	expiry, err := time.Parse(time.DateOnly, unit.FechaVencimiento)
	if err != nil {
		return false, internal(err, "la fecha de vencimiento persistida no es una fecha valida")
	}
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.After(expiry), nil
}

func (s *Store) ReturnProduct(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
	req ReturnProductRequest,
) (MedicationUnit, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MedicationUnit{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		return MedicationUnit{}, err
	}
	if err := requireExtraordinaryEventRole(invoker); err != nil {
		return MedicationUnit{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return MedicationUnit{}, err
	}
	if req.Motivo == "" {
		return MedicationUnit{}, NewError(InvalidRequest, "motivo es obligatorio para documentar la causa de la devolucion")
	}
	unit, err := readLockedUnit(ctx, tx, gtin, serial)
	if err != nil {
		return MedicationUnit{}, err
	}
	actor, err := resolveExtraordinaryEventActor(ctx, tx, unit, invoker, domain.EventDevolverProducto)
	if err != nil {
		return MedicationUnit{}, err
	}
	transition, err := requireTransition(unit.Estado, domain.EventDevolverProducto, actor)
	if err != nil {
		return MedicationUnit{}, err
	}

	var receiver any
	if req.Receptor != "" {
		custodian, err := lookupOrganizationByCanonicalID(ctx, tx, unit.CustodioActual)
		if err != nil {
			return MedicationUnit{}, err
		}
		receiverOrg, err := validateReturnReceiver(ctx, tx, req.Receptor, custodian)
		if err != nil {
			return MedicationUnit{}, err
		}
		receiver = receiverOrg.CanonicalID()
	}
	now := s.now().UTC()
	timestamp := formatTimestamp(now)
	txID, err := s.newID()
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo generar el identificador de transaccion")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.return_operations
		(gtin, numero_serie, tx_id_devolucion, receptor_declarado, motivo, event_timestamp)
		VALUES ($1,$2,$3,$4,$5,$6)`, gtin, serial, txID, receiver, req.Motivo, timestamp)
	if err != nil {
		return MedicationUnit{}, internal(err, "no se pudo registrar la devolucion")
	}
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if err := updateUnit(ctx, tx, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := appendUnitEvent(ctx, tx, txID, opReturnProduct, invoker.MSPID, unit); err != nil {
		return MedicationUnit{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return MedicationUnit{}, err
	}
	return unit, nil
}

func validateReturnReceiver(
	ctx context.Context,
	tx pgx.Tx,
	receiver string,
	custodian Organization,
) (Organization, error) {
	if _, _, err := parseCanonicalID(receiver); err != nil {
		return Organization{}, NewError(InvalidRequest,
			"el receptor %q no tiene la forma canonica GLN:<id> o CUFE:<id>", receiver)
	}
	org, err := lookupOrganizationByCanonicalID(ctx, tx, receiver)
	if err != nil {
		return Organization{}, err
	}
	if !org.Active {
		return Organization{}, NewError(OrgInactive,
			"el receptor declarado %q esta registrado pero no habilitado", receiver).
			WithDetails(map[string]any{"receptor": receiver})
	}
	custodial, err := domain.IsCustodialAgentType(org.AgentType)
	if err != nil {
		return Organization{}, internal(err, "no se pudo consultar el catalogo de agentType")
	}
	if !custodial {
		return Organization{}, NewError(InvalidDestination,
			"el agentType %s no puede recibir una devolucion", org.AgentType).
			WithDetails(map[string]any{"receptor": receiver, "agentType": string(org.AgentType)})
	}
	if org.CanonicalID() == custodian.CanonicalID() {
		return Organization{}, NewError(InvalidDestination, "el receptor declarado es el propio custodio de la unidad").
			WithDetails(map[string]any{"receptor": receiver})
	}
	decision, err := domain.DecideTransfer(org.AgentType, custodian.AgentType)
	if err != nil {
		return Organization{}, internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed {
		return Organization{}, NewError(TransferNotAuthorized,
			"la matriz no autoriza el par %s -> %s", org.AgentType, custodian.AgentType).
			WithDetails(map[string]any{
				"receptor": receiver, "par": string(org.AgentType) + " -> " + string(custodian.AgentType),
			})
	}
	return org, nil
}
