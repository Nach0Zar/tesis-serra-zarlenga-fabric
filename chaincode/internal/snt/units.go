package snt

import (
	"encoding/json"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// RegisterUnit implementa T01 de ADR-001: el alta de una unidad serializada por
// el laboratorio que la libera al circuito trazado. Estado resultante:
// EN_LABORATORIO.
//
// Autorizacion (DES-6): organizacion activa con agentType=LABORATORY y
// snt.role=operator, resuelta por el registro organizacion-establecimiento y
// nunca contra el literal "LabMSP" (ADR-003, ADR-010).
//
// Alcance v1: solo LABORATORY ejecuta el registro inicial. Que una drogueria
// pueda ser eslabon de origen es un caso relevado pero excluido, y su
// incorporacion exigiria extender T01/ADR-001 y la autorizacion de DES-6
// (docs/alcance-prototipo.md).
//
// Endoso (ADR-007, puntos 6.a, 6.g y 6.j): la clave de la unidad no existe
// todavia, de modo que su primera escritura no puede quedar cubierta por endoso
// basado en estado. La operacion escribe ademas un marcador de participacion en
// la coleccion implicita del laboratorio invocante, cuya politica de endoso
// pertenece a esa organizacion y rige desde el despliegue: con eso el endoso de
// su peer queda exigido por la plataforma ya en esta transaccion. El laboratorio
// queda **necesario**, no exclusivo — la politica de chaincode admite
// endosantes adicionales, que solo agregan ejecuciones coincidentes de la misma
// logica determinística.
func (c *SNTContract) RegisterUnit(
	ctx contractapi.TransactionContextInterface,
	req RegisterUnitRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := invoker.requireAgentType(domain.AgentLaboratory); err != nil {
		return nil, err
	}
	if err := invoker.requireRole(RoleOperator); err != nil {
		return nil, err
	}

	if err := validateRegisterUnitRequest(req); err != nil {
		return nil, err
	}

	// T01 es la unica transicion de ADR-001 cuya columna "Desde" es "Inicio".
	// Se resuelve contra la tabla en lugar de asumir el estado resultante, para
	// que un cambio de la maquina no quede en dos lugares.
	transition, ok := domain.LookupInitialTransition(domain.EventRegistrarUnidad)
	if !ok || !transition.AllowsActor(domain.ActorLaboratory) {
		return nil, cerr.New(cerr.InvalidStateTransition,
			"la maquina de estados no declara el alta de unidades para el actor LABORATORY")
	}

	key, err := medicationUnitKey(ctx.GetStub(), req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo construir la clave de la unidad")
	}
	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo verificar la existencia de la unidad")
	}
	if existing != nil {
		return nil, cerr.New(cerr.UnitAlreadyExists,
			"la unidad %s/%s ya esta registrada", req.GTIN, req.NumeroSerie).
			WithDetails(map[string]any{"gtin": req.GTIN, "numeroSerie": req.NumeroSerie})
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	unit := MedicationUnit{
		GTIN:             req.GTIN,
		NumeroSerie:      req.NumeroSerie,
		Lote:             req.Lote,
		FechaVencimiento: req.FechaVencimiento,
		// El custodio persistido es el identificador canonico GLN/CUFE resuelto
		// desde el registro, nunca el mspId ni un atributo de certificado
		// (ADR-003, punto 4).
		CustodioActual:      invoker.CanonicalID(),
		Estado:              transition.To,
		UltimaActualizacion: timestamp,
	}
	if _, err := putUnit(ctx, unit); err != nil {
		return nil, err
	}

	// Politica de reposo de la clave: la organizacion del custodio actual, SIN
	// rama alternativa. Ninguna politica de clave de unidad admite a la
	// organizacion regulatoria como rama disyuntiva: habilitaria el endoso
	// unilateral del regulador en las operaciones ordinarias, porque la politica
	// es de la clave y no de la funcion (ADR-007, punto 6.a).
	if err := setKeyEndorsement(ctx, key, invoker.MSPID); err != nil {
		return nil, err
	}

	// Marcador de participacion del laboratorio invocante. Su clave es unica por
	// transaccion (txId ultimo), de modo que las 50.000 altas del dataset del
	// protocolo de medicion no se serializan por conflicto MVCC — que es lo que
	// haria un contador o una clave compartida por organizacion protegida con su
	// SBE (ADR-007, punto 6.g, alternativa descartada).
	if err := writeUnitParticipationMarker(
		ctx, invoker.MSPID, opRegisterUnit, invoker.MSPID, unit.GTIN, unit.NumeroSerie); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, opRegisterUnit, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

func validateRegisterUnitRequest(req RegisterUnitRequest) error {
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return err
	}
	if req.Lote == "" {
		return invalidRequest("el lote es obligatorio")
	}
	return validateExpirationDate(req.FechaVencimiento)
}

// emitUnitEvent publica un evento de chaincode por cada escritura sobre una
// unidad. El nombre del evento es el de la operacion del contrato y el payload
// es la vista publica resultante.
//
// El contrato DES-5 no define eventos -- fija la superficie invocable, no el
// canal de notificacion --, de modo que este esquema es una convencion de
// implementacion que NET-8 (#64, listener de ANMAT) debera consumir o revisar.
// El payload no agrega nada al canal: es exactamente el estado publico que la
// transaccion acaba de escribir.
func emitUnitEvent(ctx contractapi.TransactionContextInterface, operation string, unit MedicationUnit) error {
	payload, err := json.Marshal(unit)
	if err != nil {
		return cerr.Internal(err, "no se pudo serializar el payload del evento")
	}
	if err := ctx.GetStub().SetEvent(operation, payload); err != nil {
		return cerr.Internal(err, "no se pudo emitir el evento de chaincode")
	}
	return nil
}
