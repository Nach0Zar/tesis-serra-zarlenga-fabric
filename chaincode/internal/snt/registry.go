package snt

import (
	"encoding/json"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Nombres de operacion usados en los marcadores de participacion.
const (
	opInit                  = "Init"
	opRegisterOrganization  = "RegisterOrganization"
	opSetOrganizationActive = "SetOrganizationActive"
	opAuthorizeLabIntervent = "AuthorizeLabIntervention"
	opRevokeLabIntervention = "RevokeLabIntervention"
	opRegisterUnit          = "RegisterUnit"
)

// readOrganization lee una entrada del registro por mspId.
func readOrganization(ctx contractapi.TransactionContextInterface, mspID string) (OrganizationRecord, bool, error) {
	key, err := organizationKey(ctx.GetStub(), mspID)
	if err != nil {
		return OrganizationRecord{}, false, cerr.Internal(err, "no se pudo construir la clave del registro")
	}
	raw, err := ctx.GetStub().GetState(key)
	if err != nil {
		return OrganizationRecord{}, false, cerr.Internal(err, "no se pudo leer el registro organizacion-establecimiento")
	}
	if raw == nil {
		return OrganizationRecord{}, false, nil
	}

	var record OrganizationRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return OrganizationRecord{}, false, cerr.Internal(err, "entrada del registro corrupta")
	}
	return record, true, nil
}

// listOrganizations recorre el registro completo. El registro esta acotado por
// la cantidad de organizaciones de la red (siete en el dataset fundacional), de
// modo que el recorrido es barato y determinístico. No se usa en el camino de
// las operaciones de alto volumen.
func listOrganizations(ctx contractapi.TransactionContextInterface) ([]OrganizationRecord, error) {
	iterator, err := ctx.GetStub().GetStateByPartialCompositeKey(objectTypeOrganization, nil)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo recorrer el registro organizacion-establecimiento")
	}
	defer func() { _ = iterator.Close() }()

	var out []OrganizationRecord
	for iterator.HasNext() {
		kv, err := iterator.Next()
		if err != nil {
			return nil, cerr.Internal(err, "no se pudo leer una entrada del registro")
		}
		var record OrganizationRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			return nil, cerr.Internal(err, "entrada del registro corrupta")
		}
		out = append(out, record)
	}
	return out, nil
}

func putOrganization(ctx contractapi.TransactionContextInterface, record OrganizationRecord) (string, error) {
	key, err := organizationKey(ctx.GetStub(), record.MSPID)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo construir la clave del registro")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo serializar la entrada del registro")
	}
	if err := ctx.GetStub().PutState(key, payload); err != nil {
		return "", cerr.Internal(err, "no se pudo escribir la entrada del registro")
	}
	return key, nil
}

// Init siembra la primera entrada REGULATOR del registro
// organizacion-establecimiento y resuelve el arranque en frio de ADR-010,
// punto 4: RegisterOrganization exige un regulador ya registrado, y esa primera
// entrada no puede haber sido registrada por nadie.
//
// Se invoca una sola vez, en la secuencia 1 del lifecycle, con --init-required
// y politica de endoso AND de todas las organizaciones fundacionales
// (ADR-007, punto 5).
//
// NO recibe argumentos, y en particular no recibe el mspId regulatorio:
// aceptarlo dejaria la identidad del regulador a criterio de quien envia la
// propuesta, y una politica multiparte prueba ejecucion coincidente, no
// aprobacion administrativa del valor (ADR-010, punto 4).
func (c *SNTContract) Init(ctx contractapi.TransactionContextInterface) (*OrganizationView, error) {
	regulator, err := manifest.Regulator()
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo resolver el regulador del manifiesto fundacional embebido")
	}

	// Condicion 1: el invocador es el mspId declarado como REGULATOR en el
	// manifiesto embebido.
	mspID, err := invokerMSPID(ctx)
	if err != nil {
		return nil, err
	}
	if mspID != regulator.MSPID {
		return nil, regulatoryOnly()
	}

	// Condicion 2: el invocador porta snt.role=regulatory-admin.
	role, err := invokerRole(ctx)
	if err != nil {
		return nil, err
	}
	if role != RoleRegulatoryAdmin {
		return nil, regulatoryOnly()
	}

	// Condicion 3: no existe todavia ninguna entrada REGULATOR. Init es
	// idempotente en el sentido de que reinvocarla falla: una reinvocacion no
	// puede sustituir al regulador (ADR-010, invariantes).
	existing, err := listOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	for _, org := range existing {
		if org.AgentType == domain.AgentRegulator {
			return nil, cerr.New(cerr.AlreadyInitialized,
				"el registro ya contiene la entrada REGULATOR %s", org.MSPID).
				WithDetails(map[string]any{"mspId": org.MSPID})
		}
	}

	record := OrganizationRecord{
		MSPID:     regulator.MSPID,
		ID:        regulator.ID,
		IDType:    regulator.IDType,
		AgentType: domain.AgentRegulator,
		Active:    true,
	}
	key, err := putOrganization(ctx, record)
	if err != nil {
		return nil, err
	}

	// Proteccion de la entrada regulatoria: la clave recibe, en la misma Init,
	// una SBE que exige a la organizacion regulatoria, de modo que ninguna otra
	// pueda modificarla despues del bootstrap (ADR-010, invariantes).
	if err := setKeyEndorsement(ctx, key, record.MSPID); err != nil {
		return nil, err
	}

	// Init no escribe marcador de participacion: se ejecuta bajo la politica
	// estricta AND de todas las organizaciones fundacionales, que ya exige a
	// todas (ADR-007, punto 6.g).
	view := OrganizationView(record)
	return &view, nil
}

// RegisterOrganization da de alta una entrada del registro
// organizacion-establecimiento (ADR-003, extendido por ADR-010).
//
// Endoso: el peer de la organizacion regulatoria BASTA. La primera escritura de
// la entrada se valida contra la politica de chaincode, que incluye a la
// organizacion regulatoria (ADR-007, punto 6.j), y el marcador de participacion
// es lo que exige efectivamente su endoso en esa primera transaccion.
func (c *SNTContract) RegisterOrganization(
	ctx contractapi.TransactionContextInterface,
	req RegisterOrganizationRequest,
) (*OrganizationView, error) {
	regulator, err := resolveRegulator(ctx)
	if err != nil {
		return nil, err
	}

	if req.MSPID == "" {
		return nil, invalidRequest("mspId es obligatorio")
	}
	if err := validateAgentTypeAndIDType(req.AgentType, req.IDType); err != nil {
		return nil, err
	}
	if err := validateOrganizationID(req.IDType, req.ID); err != nil {
		return nil, err
	}

	existing, err := listOrganizations(ctx)
	if err != nil {
		return nil, err
	}

	record := OrganizationRecord{
		MSPID:     req.MSPID,
		ID:        req.ID,
		IDType:    req.IDType,
		AgentType: req.AgentType,
		Active:    req.Active,
	}

	for _, org := range existing {
		if org.MSPID == req.MSPID {
			return nil, invalidRequest("la organizacion %s ya tiene entrada en el registro", req.MSPID).
				WithDetails(map[string]any{"mspId": req.MSPID})
		}
		// ADR-003: la relacion mspId <-> id es uno a uno.
		if org.CanonicalID() == record.CanonicalID() {
			return nil, invalidRequest("el identificador %s ya esta asignado a la organizacion %s",
				record.CanonicalID(), org.MSPID).
				WithDetails(map[string]any{"id": record.CanonicalID(), "mspId": org.MSPID})
		}
		// Invariante de unicidad del regulador (ADR-010, punto 4).
		if req.AgentType == domain.AgentRegulator && org.AgentType == domain.AgentRegulator && org.Active {
			return nil, invalidRequest("ya existe una entrada REGULATOR activa (%s)", org.MSPID).
				WithDetails(map[string]any{"mspId": org.MSPID})
		}
	}

	key, err := putOrganization(ctx, record)
	if err != nil {
		return nil, err
	}

	// SBE regulatoria para las modificaciones posteriores de la entrada.
	if err := setKeyEndorsement(ctx, key, regulator.MSPID); err != nil {
		return nil, err
	}

	// Marcador de la variante `Organizacion`: la operacion no recae sobre una
	// unidad y no tiene GTIN ni numero de serie (ADR-007, punto 6.g).
	if err := writeOrganizationParticipationMarker(
		ctx, regulator.MSPID, opRegisterOrganization, regulator.MSPID, record.MSPID); err != nil {
		return nil, err
	}

	view := OrganizationView(record)
	return &view, nil
}

// SetOrganizationActive cambia la habilitacion de una organizacion.
//
// La baja se modela actualizando `active`, nunca alterando historiales de
// custodia ya persistidos (ADR-003, "Consecuencias").
func (c *SNTContract) SetOrganizationActive(
	ctx contractapi.TransactionContextInterface,
	req SetOrganizationActiveRequest,
) (*OrganizationView, error) {
	regulator, err := resolveRegulator(ctx)
	if err != nil {
		return nil, err
	}

	if req.MSPID == "" {
		return nil, invalidRequest("mspId es obligatorio")
	}

	record, found, err := readOrganization(ctx, req.MSPID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, cerr.New(cerr.OrgNotRegistered,
			"la organizacion %s no tiene entrada en el registro", req.MSPID).
			WithDetails(map[string]any{"mspId": req.MSPID})
	}

	// Invariante: la red no puede quedar sin autoridad capaz de administrar el
	// registro (ADR-010, punto 4).
	if record.AgentType == domain.AgentRegulator && record.Active && !req.Active {
		if err := ensureAnotherActiveRegulator(ctx, req.MSPID); err != nil {
			return nil, err
		}
	}

	record.Active = req.Active
	key, err := putOrganization(ctx, record)
	if err != nil {
		return nil, err
	}

	if err := setKeyEndorsement(ctx, key, regulator.MSPID); err != nil {
		return nil, err
	}
	if err := writeOrganizationParticipationMarker(
		ctx, regulator.MSPID, opSetOrganizationActive, regulator.MSPID, record.MSPID); err != nil {
		return nil, err
	}

	view := OrganizationView(record)
	return &view, nil
}

func ensureAnotherActiveRegulator(ctx contractapi.TransactionContextInterface, excludeMSPID string) error {
	all, err := listOrganizations(ctx)
	if err != nil {
		return err
	}
	for _, org := range all {
		if org.MSPID == excludeMSPID {
			continue
		}
		if org.AgentType == domain.AgentRegulator && org.Active {
			return nil
		}
	}
	return cerr.New(cerr.LastActiveRegulator,
		"no puede desactivarse la unica entrada REGULATOR activa del registro").
		WithDetails(map[string]any{"mspId": excludeMSPID})
}

// validateAgentTypeAndIDType aplica el catalogo de valores admitidos y la
// coherencia entre ambos: idType=REG solo es valido con un agentType no
// custodial, y viceversa (contrato DES-5, RegisterOrganization).
func validateAgentTypeAndIDType(agentType domain.AgentType, idType string) error {
	custodial, err := domain.IsCustodialAgentType(agentType)
	if err != nil {
		return cerr.Internal(err, "no se pudo consultar el catalogo de agentType")
	}
	nonCustodial := agentType == domain.AgentRegulator || agentType == domain.AgentFinancier

	if !custodial && !nonCustodial {
		return invalidRequest("agentType %q fuera del catalogo", agentType).
			WithDetails(map[string]any{"agentType": string(agentType)})
	}

	switch idType {
	case IDTypeGLN, IDTypeCUFE:
		if !custodial {
			return invalidRequest("idType %s solo es valido con un agentType custodial", idType).
				WithDetails(map[string]any{"idType": idType, "agentType": string(agentType)})
		}
	case IDTypeREG:
		if !nonCustodial {
			return invalidRequest("idType %s solo es valido con un agentType no custodial", IDTypeREG).
				WithDetails(map[string]any{"idType": idType, "agentType": string(agentType)})
		}
	default:
		return invalidRequest("idType %q fuera del catalogo (%s, %s, %s)",
			idType, IDTypeGLN, IDTypeCUFE, IDTypeREG).
			WithDetails(map[string]any{"idType": idType})
	}

	return nil
}
