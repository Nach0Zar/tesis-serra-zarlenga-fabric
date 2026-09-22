package core

import (
	"context"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AuthorizeLabIntervention emite o reemplaza la autorizacion vigente.
func (s *Store) AuthorizeLabIntervention(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
	req AuthorizeLabInterventionRequest,
) (LabInterventionView, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return LabInterventionView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	regulator, err := resolveRegulator(ctx, tx, credential)
	if err != nil {
		return LabInterventionView{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return LabInterventionView{}, err
	}
	if req.Motivo == "" {
		return LabInterventionView{}, NewError(InvalidRequest, "motivo es obligatorio")
	}
	if _, err := readLockedUnit(ctx, tx, gtin, serial); err != nil {
		return LabInterventionView{}, err
	}
	lab, err := lookupOrganizationByCanonicalID(ctx, tx, req.Laboratorio)
	if err != nil {
		return LabInterventionView{}, err
	}
	if !lab.Active {
		return LabInterventionView{}, NewError(OrgInactive,
			"el laboratorio designado %s esta registrado pero no habilitado", req.Laboratorio).
			WithDetails(map[string]any{"laboratorio": req.Laboratorio})
	}
	if lab.AgentType != domain.AgentLaboratory {
		return LabInterventionView{}, NewError(InvalidLabIntervention,
			"el establecimiento designado tiene agentType %s y no LABORATORY", lab.AgentType).
			WithDetails(map[string]any{"laboratorio": req.Laboratorio, "agentType": string(lab.AgentType)})
	}
	if !isKnownLabOperation(req.Operacion) {
		return LabInterventionView{}, NewError(InvalidLabIntervention,
			"la operacion %q esta fuera del catalogo de intervencion de laboratorio", req.Operacion).
			WithDetails(map[string]any{"operacion": string(req.Operacion)})
	}
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiraEn)
	if err != nil {
		return LabInterventionView{}, NewError(InvalidLabIntervention, "expiraEn debe estar en formato ISO 8601").
			WithDetails(map[string]any{"expiraEn": req.ExpiraEn})
	}
	now := s.now().UTC()
	if !expiresAt.After(now) {
		return LabInterventionView{}, NewError(InvalidLabIntervention,
			"expiraEn debe ser posterior al timestamp de la transaccion").
			WithDetails(map[string]any{"expiraEn": req.ExpiraEn, "txTimestamp": formatTimestamp(now)})
	}
	view := LabInterventionView{
		GTIN: gtin, NumeroSerie: serial, Laboratorio: lab.CanonicalID(),
		Operacion: req.Operacion, Motivo: req.Motivo,
		ExpiraEn: expiresAt.UTC().Format(time.RFC3339), Estado: LabInterventionActive,
		EmitidaPor: regulator.MSPID, EmitidaEn: formatTimestamp(now),
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.lab_interventions (
			gtin, numero_serie, laboratorio, operacion, motivo, expira_en,
			estado, emitida_por, emitida_en, consumida_en, revocada_en, motivo_revocacion
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULL,NULL,NULL)
		ON CONFLICT (gtin, numero_serie) DO UPDATE SET
			laboratorio=EXCLUDED.laboratorio, operacion=EXCLUDED.operacion,
			motivo=EXCLUDED.motivo, expira_en=EXCLUDED.expira_en,
			estado=EXCLUDED.estado, emitida_por=EXCLUDED.emitida_por,
			emitida_en=EXCLUDED.emitida_en, consumida_en=NULL,
			revocada_en=NULL, motivo_revocacion=NULL`,
		view.GTIN, view.NumeroSerie, view.Laboratorio, view.Operacion, view.Motivo,
		view.ExpiraEn, view.Estado, view.EmitidaPor, view.EmitidaEn)
	if err != nil {
		return LabInterventionView{}, internal(err, "no se pudo escribir la autorizacion de intervencion")
	}
	if err := commit(ctx, tx); err != nil {
		return LabInterventionView{}, err
	}
	return view, nil
}

// RevokeLabIntervention revoca la autorizacion vigente de una unidad.
func (s *Store) RevokeLabIntervention(
	ctx context.Context,
	credential Credential,
	gtin, serial string,
	req RevokeLabInterventionRequest,
) (LabInterventionView, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return LabInterventionView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := resolveRegulator(ctx, tx, credential); err != nil {
		return LabInterventionView{}, err
	}
	if err := validateUnitRef(gtin, serial); err != nil {
		return LabInterventionView{}, err
	}
	if req.Motivo == "" {
		return LabInterventionView{}, NewError(InvalidRequest, "motivo es obligatorio")
	}
	if _, err := readLockedUnit(ctx, tx, gtin, serial); err != nil {
		return LabInterventionView{}, err
	}
	view, found, err := readLabIntervention(ctx, tx, gtin, serial, true)
	if err != nil {
		return LabInterventionView{}, err
	}
	if !found {
		return LabInterventionView{}, NewError(LabInterventionNotFound,
			"no existe una autorizacion de intervencion para la unidad %s/%s", gtin, serial).
			WithDetails(map[string]any{"gtin": gtin, "numeroSerie": serial})
	}
	if view.Estado != LabInterventionActive {
		return LabInterventionView{}, NewError(LabInterventionNotActive,
			"la autorizacion ya esta en estado %s", view.Estado).
			WithDetails(map[string]any{"estado": string(view.Estado)})
	}
	now := s.now().UTC()
	view.Estado = LabInterventionRevoked
	view.RevocadaEn = formatTimestamp(now)
	view.MotivoRevocacion = req.Motivo
	_, err = tx.Exec(ctx, `
		UPDATE public.lab_interventions
		SET estado=$3, revocada_en=$4, motivo_revocacion=$5
		WHERE gtin=$1 AND numero_serie=$2`,
		gtin, serial, view.Estado, view.RevocadaEn, view.MotivoRevocacion)
	if err != nil {
		return LabInterventionView{}, internal(err, "no se pudo revocar la autorizacion de intervencion")
	}
	if err := commit(ctx, tx); err != nil {
		return LabInterventionView{}, err
	}
	return view, nil
}

func isKnownLabOperation(operation LabInterventionOperation) bool {
	switch operation {
	case LabOpWithdrawFromMarket, LabOpRestock, LabOpFinalDisposition:
		return true
	default:
		return false
	}
}

func readLabIntervention(
	ctx context.Context,
	tx pgx.Tx,
	gtin, serial string,
	forUpdate bool,
) (LabInterventionView, bool, error) {
	query := `
		SELECT gtin, numero_serie, laboratorio, operacion, motivo, expira_en,
		       estado, emitida_por, emitida_en, consumida_en, revocada_en, motivo_revocacion
		FROM public.lab_interventions WHERE gtin=$1 AND numero_serie=$2`
	if forUpdate {
		query += " FOR UPDATE"
	}
	var view LabInterventionView
	var expiresAt, issuedAt time.Time
	var consumedAt, revokedAt pgtype.Timestamptz
	var revocationReason pgtype.Text
	err := tx.QueryRow(ctx, query, gtin, serial).Scan(
		&view.GTIN, &view.NumeroSerie, &view.Laboratorio, &view.Operacion, &view.Motivo,
		&expiresAt, &view.Estado, &view.EmitidaPor, &issuedAt,
		&consumedAt, &revokedAt, &revocationReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LabInterventionView{}, false, nil
	}
	if err != nil {
		return LabInterventionView{}, false, internal(err, "no se pudo leer la autorizacion de intervencion")
	}
	view.ExpiraEn = expiresAt.UTC().Format(time.RFC3339)
	view.EmitidaEn = formatTimestamp(issuedAt)
	if consumedAt.Valid {
		view.ConsumidaEn = formatTimestamp(consumedAt.Time)
	}
	if revokedAt.Valid {
		view.RevocadaEn = formatTimestamp(revokedAt.Time)
	}
	if revocationReason.Valid {
		view.MotivoRevocacion = revocationReason.String
	}
	return view, true, nil
}

func consumeLabIntervention(
	ctx context.Context,
	tx pgx.Tx,
	unit MedicationUnit,
	invoker Invoker,
	now time.Time,
	expected LabInterventionOperation,
) error {
	if invoker.Org.AgentType != domain.AgentLaboratory || unit.CustodioActual == invoker.CanonicalID() {
		return nil
	}
	view, found, err := readLabIntervention(ctx, tx, unit.GTIN, unit.NumeroSerie, true)
	if err != nil {
		return err
	}
	if !found {
		return labInterventionRequired(unit, "no existe autorizacion de intervencion para la unidad")
	}
	if view.Laboratorio != invoker.CanonicalID() {
		return labInterventionRequired(unit, "la autorizacion vigente designa a otro laboratorio")
	}
	if view.Operacion != expected {
		return labInterventionRequired(unit, "la autorizacion vigente habilita la operacion "+string(view.Operacion))
	}
	if view.Estado != LabInterventionActive {
		return labInterventionRequired(unit, "la autorizacion esta en estado "+string(view.Estado))
	}
	expiresAt, err := time.Parse(time.RFC3339, view.ExpiraEn)
	if err != nil {
		return internal(err, "la autorizacion persistida tiene un expiraEn invalido")
	}
	if !expiresAt.After(now.UTC()) {
		return labInterventionRequired(unit, "la autorizacion vencio el "+view.ExpiraEn)
	}
	consumedAt := formatTimestamp(now)
	_, err = tx.Exec(ctx, `
		UPDATE public.lab_interventions SET estado=$3, consumida_en=$4
		WHERE gtin=$1 AND numero_serie=$2`,
		unit.GTIN, unit.NumeroSerie, LabInterventionConsumed, consumedAt)
	if err != nil {
		return internal(err, "no se pudo consumir la autorizacion de intervencion")
	}
	return nil
}

func labInterventionRequired(unit MedicationUnit, detail string) error {
	return NewError(LabInterventionRequired,
		"un laboratorio no custodio exige una autorizacion de intervencion ACTIVA y vigente: %s", detail).
		WithDetails(map[string]any{
			"gtin": unit.GTIN, "numeroSerie": unit.NumeroSerie, "custodioActual": unit.CustodioActual,
		})
}
