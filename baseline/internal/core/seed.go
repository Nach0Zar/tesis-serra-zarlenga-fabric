package core

import (
	"context"
	"fmt"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// SeedRegistration contiene exclusivamente el alta que forma el snapshot
// inicial. Las operaciones de workload del bundle se ejecutan en EVAL-3.
type SeedRegistration struct {
	InvokerMSPID string
	Request      RegisterUnitRequest
}

// SeedResult resume la carga confirmada sin exponer credenciales ni datos del
// workload posterior al snapshot.
type SeedResult struct {
	Organizations int
	Units         int
}

// SeedSnapshot carga el registro fundacional y las unidades iniciales en una
// unica transaccion. Una baseline con cualquier dato de dominio se rechaza para
// impedir que una repeticion mezcle snapshots.
func (s *Store) SeedSnapshot(
	ctx context.Context,
	organizations []Organization,
	registrations []SeedRegistration,
) (SeedResult, error) {
	byMSPID, err := validateSeedInput(organizations, registrations)
	if err != nil {
		return SeedResult{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return SeedResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		LOCK TABLE public.lab_interventions,
		           public.transfer_operations,
		           public.return_operations,
		           public.unit_events,
		           public.medication_units,
		           public.organizations
		IN ACCESS EXCLUSIVE MODE`); err != nil {
		return SeedResult{}, internal(err, "no se pudieron bloquear las tablas de la baseline para el seed")
	}

	var organizationsCount, unitsCount, eventsCount int64
	var transfersCount, returnsCount, interventionsCount int64
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM public.organizations),
			(SELECT count(*) FROM public.medication_units),
			(SELECT count(*) FROM public.unit_events),
			(SELECT count(*) FROM public.transfer_operations),
			(SELECT count(*) FROM public.return_operations),
			(SELECT count(*) FROM public.lab_interventions)`).Scan(
		&organizationsCount, &unitsCount, &eventsCount,
		&transfersCount, &returnsCount, &interventionsCount,
	); err != nil {
		return SeedResult{}, internal(err, "no se pudo comprobar que la baseline estuviera vacia")
	}
	if organizationsCount+unitsCount+eventsCount+transfersCount+returnsCount+interventionsCount != 0 {
		return SeedResult{}, NewError(AlreadyInitialized, "el seed exige una baseline sin datos de dominio")
	}

	for _, org := range organizations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO public.organizations (msp_id,id,id_type,agent_type,active)
			VALUES ($1,$2,$3,$4,$5)`,
			org.MSPID, org.ID, org.IDType, org.AgentType, org.Active,
		); err != nil {
			return SeedResult{}, internal(err, fmt.Sprintf("no se pudo cargar la organizacion %s", org.MSPID))
		}
	}

	timestamp := formatTimestamp(s.now())
	for index, registration := range registrations {
		org := byMSPID[registration.InvokerMSPID]
		invoker := Invoker{MSPID: org.MSPID, Org: org, Role: RoleOperator}
		if _, err := s.registerUnitInTx(ctx, tx, invoker, registration.Request, timestamp); err != nil {
			return SeedResult{}, fmt.Errorf("registrar unidad de secuencia %d: %w", index+1, err)
		}
	}

	if err := commit(ctx, tx); err != nil {
		return SeedResult{}, err
	}
	return SeedResult{Organizations: len(organizations), Units: len(registrations)}, nil
}

func validateSeedInput(organizations []Organization, registrations []SeedRegistration) (map[string]Organization, error) {
	if len(organizations) == 0 {
		return nil, fmt.Errorf("el seed no contiene organizaciones")
	}
	if len(registrations) == 0 {
		return nil, fmt.Errorf("el seed no contiene unidades")
	}

	byMSPID := make(map[string]Organization, len(organizations))
	canonicalIDs := make(map[string]struct{}, len(organizations))
	activeRegulators := 0
	for _, org := range organizations {
		request := RegisterOrganizationRequest{
			MSPID: org.MSPID, ID: org.ID, IDType: org.IDType,
			AgentType: org.AgentType, Active: org.Active,
		}
		if err := validateOrganization(request); err != nil {
			return nil, fmt.Errorf("organizacion %s invalida: %w", org.MSPID, err)
		}
		if _, exists := byMSPID[org.MSPID]; exists {
			return nil, fmt.Errorf("mspId duplicado en el seed: %s", org.MSPID)
		}
		canonicalID := org.CanonicalID()
		if _, exists := canonicalIDs[canonicalID]; exists {
			return nil, fmt.Errorf("identificador canonico duplicado en el seed: %s", canonicalID)
		}
		byMSPID[org.MSPID] = org
		canonicalIDs[canonicalID] = struct{}{}
		if org.AgentType == domain.AgentRegulator && org.Active {
			activeRegulators++
		}
	}
	if activeRegulators != 1 {
		return nil, fmt.Errorf("el seed debe contener exactamente un REGULATOR activo")
	}

	units := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		org, found := byMSPID[registration.InvokerMSPID]
		if !found || !org.Active || org.AgentType != domain.AgentLaboratory {
			return nil, fmt.Errorf("el invocador %s no es un LABORATORY activo del manifiesto", registration.InvokerMSPID)
		}
		if err := ValidateRegisterUnitRequest(registration.Request); err != nil {
			return nil, fmt.Errorf("unidad %s/%s invalida: %w", registration.Request.GTIN, registration.Request.NumeroSerie, err)
		}
		key := registration.Request.GTIN + "\x00" + registration.Request.NumeroSerie
		if _, exists := units[key]; exists {
			return nil, fmt.Errorf("unidad duplicada en el seed: %s/%s", registration.Request.GTIN, registration.Request.NumeroSerie)
		}
		units[key] = struct{}{}
	}
	return byMSPID, nil
}
