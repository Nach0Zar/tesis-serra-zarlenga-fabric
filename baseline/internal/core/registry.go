package core

import (
	"context"
	"errors"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
)

// lockOrganizationRegistry serializa las mutaciones del registro. Esto hace
// atómicas las invariantes que abarcan más de una fila (regulador único activo
// y prohibición de desactivar el último regulador).
func lockOrganizationRegistry(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `LOCK TABLE public.organizations IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return internal(err, "no se pudo bloquear el registro de organizaciones")
	}
	return nil
}

func (s *Store) RegisterOrganization(ctx context.Context, credential Credential, req RegisterOrganizationRequest) (Organization, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockOrganizationRegistry(ctx, tx); err != nil {
		return Organization{}, err
	}
	if _, err := resolveRegulator(ctx, tx, credential); err != nil {
		return Organization{}, err
	}
	if err := validateOrganization(req); err != nil {
		return Organization{}, err
	}
	var conflictingMSPID string
	err = tx.QueryRow(ctx, `
		SELECT msp_id FROM public.organizations
		WHERE msp_id=$1 OR (id_type=$2 AND id=$3) LIMIT 1`, req.MSPID, req.IDType, req.ID).Scan(&conflictingMSPID)
	if err == nil {
		return Organization{}, NewError(InvalidRequest, "la organizacion o el identificador canonico ya esta registrado").WithDetails(map[string]any{"mspId": conflictingMSPID})
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, internal(err, "no se pudo comprobar la unicidad de la organizacion")
	}
	if req.AgentType == domain.AgentRegulator && req.Active {
		var activeMSPID string
		err = tx.QueryRow(ctx, `SELECT msp_id FROM public.organizations WHERE agent_type='REGULATOR' AND active LIMIT 1`).Scan(&activeMSPID)
		if err == nil {
			return Organization{}, NewError(InvalidRequest, "ya existe una entrada REGULATOR activa (%s)", activeMSPID).WithDetails(map[string]any{"mspId": activeMSPID})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, internal(err, "no se pudo comprobar la unicidad del regulador")
		}
	}
	org := Organization{MSPID: req.MSPID, ID: req.ID, IDType: req.IDType, AgentType: req.AgentType, Active: req.Active}
	_, err = tx.Exec(ctx, `INSERT INTO public.organizations (msp_id,id,id_type,agent_type,active) VALUES ($1,$2,$3,$4,$5)`,
		org.MSPID, org.ID, org.IDType, org.AgentType, org.Active)
	if err != nil {
		if isUniqueViolation(err) {
			return Organization{}, NewError(InvalidRequest, "la organizacion o el identificador canonico ya esta registrado")
		}
		return Organization{}, internal(err, "no se pudo registrar la organizacion")
	}
	if err := commit(ctx, tx); err != nil {
		return Organization{}, err
	}
	return org, nil
}

func (s *Store) SetOrganizationActive(ctx context.Context, credential Credential, mspID string, active bool) (Organization, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockOrganizationRegistry(ctx, tx); err != nil {
		return Organization{}, err
	}
	if _, err := resolveRegulator(ctx, tx, credential); err != nil {
		return Organization{}, err
	}
	if mspID == "" {
		return Organization{}, NewError(InvalidRequest, "mspId es obligatorio")
	}
	org, found, err := readOrganization(ctx, tx, mspID)
	if err != nil {
		return Organization{}, err
	}
	if !found {
		return Organization{}, NewError(OrgNotRegistered, "la organizacion %s no tiene entrada en el registro", mspID).WithDetails(map[string]any{"mspId": mspID})
	}
	if org.AgentType == domain.AgentRegulator && org.Active && !active {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.organizations WHERE agent_type='REGULATOR' AND active AND msp_id<>$1`, mspID).Scan(&count); err != nil {
			return Organization{}, internal(err, "no se pudo comprobar el ultimo regulador activo")
		}
		if count == 0 {
			return Organization{}, NewError(LastActiveRegulator, "no puede desactivarse la unica entrada REGULATOR activa").WithDetails(map[string]any{"mspId": mspID})
		}
	}
	org.Active = active
	if _, err := tx.Exec(ctx, `UPDATE public.organizations SET active=$2 WHERE msp_id=$1`, mspID, active); err != nil {
		return Organization{}, internal(err, "no se pudo actualizar la organizacion")
	}
	if err := commit(ctx, tx); err != nil {
		return Organization{}, err
	}
	return org, nil
}
