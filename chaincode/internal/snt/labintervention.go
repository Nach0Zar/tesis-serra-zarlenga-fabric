package snt

import (
	"encoding/json"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// AuthorizeLabIntervention autoriza a un laboratorio titular a ejecutar UNA
// operacion extraordinaria sobre una unidad bajo custodia de un tercero.
//
// Existe porque el par de endosos que DES-6 exige para ese caso no puede
// imponerse con SBE sobre la clave de la unidad: la politica de una clave se
// evalua contra el estado previo y no puede condicionarse a la operacion
// intentada (ADR-007, punto 6.e).
func (c *SNTContract) AuthorizeLabIntervention(
	ctx contractapi.TransactionContextInterface,
	req AuthorizeLabInterventionRequest,
) (*LabInterventionView, error) {
	regulator, err := resolveRegulator(ctx)
	if err != nil {
		return nil, err
	}

	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}
	if req.Motivo == "" {
		return nil, invalidRequest("motivo es obligatorio")
	}

	if _, err := readUnit(ctx, req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	// El laboratorio designado debe estar registrado, activo y ser LABORATORY.
	lab, err := findOrganizationByCanonicalID(ctx, req.Laboratorio)
	if err != nil {
		return nil, err
	}
	if lab.AgentType != domain.AgentLaboratory {
		return nil, cerr.New(cerr.InvalidLabIntervention,
			"el establecimiento designado tiene agentType %s y no LABORATORY", lab.AgentType).
			WithDetails(map[string]any{"laboratorio": req.Laboratorio, "agentType": string(lab.AgentType)})
	}

	if !isKnownLabOperation(req.Operacion) {
		return nil, cerr.New(cerr.InvalidLabIntervention,
			"la operacion %q esta fuera del catalogo de intervencion de laboratorio", req.Operacion).
			WithDetails(map[string]any{"operacion": string(req.Operacion)})
	}

	// expiraEn debe ser ISO 8601 y POSTERIOR al timestamp de la transaccion.
	// Es exactamente esta validacion la que hace imposible "revocar" una
	// autorizacion reemitiendola vencida, y por eso existe
	// RevokeLabIntervention (ADR-007, punto 6.f).
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiraEn)
	if err != nil {
		return nil, cerr.New(cerr.InvalidLabIntervention,
			"expiraEn debe estar en formato ISO 8601").
			WithDetails(map[string]any{"expiraEn": req.ExpiraEn})
	}
	now, err := txTime(ctx)
	if err != nil {
		return nil, err
	}
	if !expiresAt.After(now) {
		return nil, cerr.New(cerr.InvalidLabIntervention,
			"expiraEn debe ser posterior al timestamp de la transaccion").
			WithDetails(map[string]any{"expiraEn": req.ExpiraEn, "txTimestamp": now.Format(time.RFC3339)})
	}

	emitidaEn, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	authorization := LabInterventionView{
		GTIN:        req.GTIN,
		NumeroSerie: req.NumeroSerie,
		Laboratorio: lab.CanonicalID(),
		Operacion:   req.Operacion,
		Motivo:      req.Motivo,
		ExpiraEn:    expiresAt.UTC().Format(time.RFC3339),
		Estado:      LabInterventionActiva,
		EmitidaPor:  regulator.MSPID,
		EmitidaEn:   emitidaEn,
	}

	// Una autorizacion nueva sobre la misma unidad REEMPLAZA a la anterior,
	// cualquiera sea su estado. La clave es una por unidad y GetHistoryForKey
	// conserva la secuencia completa (contrato DES-5).
	key, err := putLabIntervention(ctx, authorization)
	if err != nil {
		return nil, err
	}

	// SBE de la organizacion regulatoria SOLAMENTE, no conjunta con el
	// laboratorio: si lo fuera, reemplazar una autorizacion vencida por otra
	// dirigida a un laboratorio distinto exigiria el endoso del laboratorio
	// anterior, y bastaria con que se negara o perdiera su peer para dejar la
	// unidad impedida de forma permanente (ADR-007, punto 6.f).
	if err := setKeyEndorsement(ctx, key, regulator.MSPID); err != nil {
		return nil, err
	}

	// Marcador regulatorio: fuerza el endoso de su peer ya en esta primera
	// escritura de la clave, donde SBE todavia no puede aplicarse
	// (ADR-007, puntos 6.g y 6.j).
	if err := writeUnitParticipationMarker(
		ctx, regulator.MSPID, opAuthorizeLabIntervent, regulator.MSPID, req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	return &authorization, nil
}

// RevokeLabIntervention deja sin efecto una autorizacion antes de su
// vencimiento. Existe porque un campo de vigencia no es un mecanismo de
// revocacion: expiraEn no borra la clave, y reemitir la autorizacion vencida es
// imposible por la validacion de AuthorizeLabIntervention (ADR-007, punto 6.f).
func (c *SNTContract) RevokeLabIntervention(
	ctx contractapi.TransactionContextInterface,
	req RevokeLabInterventionRequest,
) (*LabInterventionView, error) {
	regulator, err := resolveRegulator(ctx)
	if err != nil {
		return nil, err
	}

	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}
	if req.Motivo == "" {
		return nil, invalidRequest("motivo es obligatorio")
	}

	if _, err := readUnit(ctx, req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	authorization, found, err := readLabIntervention(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, cerr.New(cerr.LabInterventionNotFound,
			"no existe una autorizacion de intervencion para la unidad %s/%s", req.GTIN, req.NumeroSerie).
			WithDetails(map[string]any{"gtin": req.GTIN, "numeroSerie": req.NumeroSerie})
	}

	// La revocacion no es idempotente y no reabre una autorizacion cerrada.
	// Una autorizacion VENCIDA pero ACTIVA si puede revocarse: cierra el
	// registro de forma explicita en lugar de dejarlo en un estado que solo el
	// timestamp desambigua (contrato DES-5).
	if authorization.Estado != LabInterventionActiva {
		return nil, cerr.New(cerr.LabInterventionNotActive,
			"la autorizacion ya esta en estado %s", authorization.Estado).
			WithDetails(map[string]any{"estado": string(authorization.Estado)})
	}

	revocadaEn, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	authorization.Estado = LabInterventionRevoked
	authorization.RevocadaEn = revocadaEn
	authorization.MotivoRevocacion = req.Motivo

	// Se CONSERVA la clave y su SBE en lugar de borrarla: la traza de emision y
	// revocacion es evidencia auditable de un acto administrativo, y borrarla
	// devolveria la proxima autorizacion sobre esa unidad a la ventana de
	// creacion de una clave nueva (ADR-007, punto 6.f).
	if _, err := putLabIntervention(ctx, authorization); err != nil {
		return nil, err
	}

	if err := writeUnitParticipationMarker(
		ctx, regulator.MSPID, opRevokeLabIntervention, regulator.MSPID, req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	return &authorization, nil
}

func isKnownLabOperation(op LabInterventionOperation) bool {
	switch op {
	case LabOpWithdrawFromMarket, LabOpRestock, LabOpFinalDisposition:
		return true
	default:
		return false
	}
}

func readLabIntervention(
	ctx contractapi.TransactionContextInterface,
	gtin, numeroSerie string,
) (LabInterventionView, bool, error) {
	key, err := labInterventionKey(ctx.GetStub(), gtin, numeroSerie)
	if err != nil {
		return LabInterventionView{}, false, cerr.Internal(err, "no se pudo construir la clave de autorizacion")
	}
	raw, err := ctx.GetStub().GetState(key)
	if err != nil {
		return LabInterventionView{}, false, cerr.Internal(err, "no se pudo leer la autorizacion de intervencion")
	}
	if raw == nil {
		return LabInterventionView{}, false, nil
	}
	var authorization LabInterventionView
	if err := json.Unmarshal(raw, &authorization); err != nil {
		return LabInterventionView{}, false, cerr.Internal(err, "autorizacion de intervencion corrupta")
	}
	return authorization, true, nil
}

func putLabIntervention(
	ctx contractapi.TransactionContextInterface,
	authorization LabInterventionView,
) (string, error) {
	key, err := labInterventionKey(ctx.GetStub(), authorization.GTIN, authorization.NumeroSerie)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo construir la clave de autorizacion")
	}
	payload, err := json.Marshal(authorization)
	if err != nil {
		return "", cerr.Internal(err, "no se pudo serializar la autorizacion de intervencion")
	}
	if err := ctx.GetStub().PutState(key, payload); err != nil {
		return "", cerr.Internal(err, "no se pudo escribir la autorizacion de intervencion")
	}
	return key, nil
}

// findOrganizationByCanonicalID resuelve una organizacion por su identificador
// canonico `<idType>:<id>` y aplica las validaciones de existencia y
// habilitacion que comparten el destino de un despacho (ADR-003) y el receptor
// declarado de una devolucion (ADR-009, punto 2, validaciones 1 a 3).
func findOrganizationByCanonicalID(
	ctx contractapi.TransactionContextInterface,
	canonicalID string,
) (OrganizationRecord, error) {
	if _, _, err := parseCanonicalID(canonicalID); err != nil {
		return OrganizationRecord{}, err
	}

	all, err := listOrganizations(ctx)
	if err != nil {
		return OrganizationRecord{}, err
	}
	for _, org := range all {
		if org.CanonicalID() != canonicalID {
			continue
		}
		if !org.Active {
			return OrganizationRecord{}, cerr.New(cerr.OrgInactive,
				"la organizacion %s esta registrada pero no habilitada", canonicalID).
				WithDetails(map[string]any{"id": canonicalID})
		}
		return org, nil
	}

	return OrganizationRecord{}, cerr.New(cerr.OrgNotRegistered,
		"el identificador %s no tiene entrada en el registro organizacion-establecimiento", canonicalID).
		WithDetails(map[string]any{"id": canonicalID})
}
