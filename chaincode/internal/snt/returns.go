package snt

import (
	"encoding/json"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const opReturnProduct = "ReturnProduct"

// devolucionTransient es el receptor declarado de una devolucion. Como todo
// identificador de contraparte, viaja por transient y nunca como argumento
// publico: revela una relacion comercial que el canal no tiene por que ver
// (ADR-004, ADR-009).
type devolucionTransient struct {
	Receptor string `json:"receptor"`
}

// ReturnProduct implementa T21, T22, T23 y T24 de ADR-001: la devolucion de una
// unidad a un eslabon anterior. Estado resultante: DEVUELTO.
//
// NO modifica CustodioActual, y es la decision central de ADR-009 (punto 1). La
// alternativa -- mover la custodia al receptor declarado -- se descarto
// expresamente porque violaria el principio de ADR-004 de que ningun cambio de
// custodia se asienta sin un acto propio del receptor. El retorno fisico no
// esta consumado cuando se declara la devolucion, y el ledger no debe afirmar
// que si. La limitacion queda registrada en docs/alcance-prototipo.md.
//
// El receptor declarado es OPCIONAL. Sin transient, la devolucion se asienta en
// el estado publico y no se escribe dato privado alguno ni se resuelve
// coleccion: una devolucion sin contraparte declarada es un caso legitimo, no
// una invocacion incompleta.
//
// Cuando el transient viene, se persiste en la clave PROPIA
// ReturnOp+[gtin, numeroSerie, txIdDevolucion] de la PDC del par (ADR-006,
// punto 4), y no en un TransferOp. No es un detalle de nomenclatura: una
// devolucion T21-T24 no nace de un despacho, no tiene txIdDespacho y no
// administra ciclo activo/cerrado, porque no espera la confirmacion de nadie.
// El registro es historico e inmutable, y una devolucion posterior de la misma
// unidad crea una clave nueva en lugar de sobreescribir la anterior.
//
// El rechazo en transito (T05) NO usa esta clave: cierra el TransferOp de su
// propio despacho. Las dos operaciones no bifurcan la semantica de la
// devolucion, se distinguen por EVIDENCIA -- la transicion de origen que queda
// en el historial y la presencia de un registro de operacion en la PDC solo en
// T05.
func (c *SNTContract) ReturnProduct(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireExtraordinaryEventRole(invoker); err != nil {
		return nil, err
	}
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}
	if req.Motivo == "" {
		return nil, invalidRequest("motivo es obligatorio para documentar la causa de la devolucion")
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	actor, err := resolveExtraordinaryEventActor(ctx, unit, invoker, domain.EventDevolverProducto)
	if err != nil {
		return nil, err
	}
	transition, err := requireTransition(unit.Estado, domain.EventDevolverProducto, actor)
	if err != nil {
		return nil, err
	}

	// El receptor se valida ANTES de resolver el nombre de la coleccion
	// (ADR-009, punto 2). El orden importa: la validacion 6 es la que garantiza
	// que la coleccion del par exista, y saltearla haria fallar la operacion con
	// un error de PLATAFORMA sobre una coleccion inexistente en lugar de con un
	// codigo de este contrato.
	receptor, declared, err := readDevolucionTransient(ctx)
	if err != nil {
		return nil, err
	}
	var receptorOrg OrganizationRecord
	if declared {
		receptorOrg, err = validateReturnReceiver(ctx, receptor, unit)
		if err != nil {
			return nil, err
		}
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	if invoker.Org.AgentType == domain.AgentRegulator {
		if err := writeUnitParticipationMarker(
			ctx, invoker.MSPID, opReturnProduct, invoker.MSPID, unit.GTIN, unit.NumeroSerie); err != nil {
			return nil, err
		}
	}
	if declared {
		if err := writeReturnOperation(ctx, unit, invoker, receptorOrg, req.Motivo, timestamp); err != nil {
			return nil, err
		}
	}

	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if _, err := putUnit(ctx, unit); err != nil {
		return nil, err
	}
	if err := emitUnitEvent(ctx, opReturnProduct, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

// readDevolucionTransient lee el receptor declarado opcional. Devuelve
// declared=false cuando el transient no viene, que es un caso valido.
func readDevolucionTransient(ctx contractapi.TransactionContextInterface) (string, bool, error) {
	raw, found, err := readTransient(ctx, transientDevolucion)
	if err != nil {
		return "", false, err
	}
	if !found || len(raw) == 0 {
		return "", false, nil
	}
	var payload devolucionTransient
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false, invalidRequest("el transient %q no es un objeto JSON valido", transientDevolucion)
	}
	if payload.Receptor == "" {
		return "", false, invalidRequest("el transient %q debe declarar el campo receptor", transientDevolucion)
	}
	return payload.Receptor, true, nil
}

// validateReturnReceiver aplica las SEIS validaciones de ADR-009 punto 2 en el
// orden que fija el contrato, cada una con su codigo propio. El orden no es
// estetico: cada paso presupone el anterior, y la sexta es la que hace que la
// coleccion del par exista.
func validateReturnReceiver(
	ctx contractapi.TransactionContextInterface,
	receptor string,
	unit MedicationUnit,
) (OrganizationRecord, error) {
	// 1. Forma canonica. A diferencia del destino de un despacho, aca NO se
	// admite declarar por mspId: ADR-003 reserva el mspId a la configuracion de
	// red y el contrato pide el identificador canonico del establecimiento.
	if _, _, err := parseCanonicalID(receptor); err != nil {
		return OrganizationRecord{}, invalidRequest(
			"el receptor %q no tiene la forma canonica GLN:<id> o CUFE:<id>", receptor)
	}

	// 2. Existe en el registro.
	org, err := lookupOrganizationByCanonicalID(ctx, receptor)
	if err != nil {
		return OrganizationRecord{}, err
	}

	// 3. Esta activo.
	if !org.Active {
		return OrganizationRecord{}, cerr.New(cerr.OrgInactive,
			"el receptor declarado %q esta registrado pero no habilitado", receptor).
			WithDetails(map[string]any{"receptor": receptor})
	}

	// 4. Su agentType es custodial: los no custodiales nunca reciben producto
	// fisico (ADR-010, punto 2).
	custodial, err := domain.IsCustodialAgentType(org.AgentType)
	if err != nil {
		return OrganizationRecord{}, cerr.Internal(err, "no se pudo consultar el catalogo de agentType")
	}
	if !custodial {
		return OrganizationRecord{}, cerr.New(cerr.InvalidDestination,
			"el agentType %s no puede recibir una devolucion", org.AgentType).
			WithDetails(map[string]any{"receptor": receptor, "agentType": string(org.AgentType)})
	}

	// 5. No es la propia organizacion declarante: una devolucion a uno mismo no
	// describe ningun movimiento.
	if org.CanonicalID() == unit.CustodioActual {
		return OrganizationRecord{}, cerr.New(cerr.InvalidDestination,
			"el receptor declarado es el propio custodio de la unidad").
			WithDetails(map[string]any{"receptor": receptor})
	}

	// 6. El par «receptor -> custodio declarante» esta autorizado por la matriz.
	// La direccion es la del flujo ORIGINAL: la devolucion va aguas arriba, de
	// modo que lo que debe estar autorizado es el envio que en su momento pudo
	// traer la unidad hasta el custodio actual. Es lo que garantiza que la
	// coleccion del par exista (ADR-006, punto 1).
	//
	// NO se exige que el receptor sea el proveedor REAL de esta unidad: ADR-009
	// punto 2 lo declara fuera de alcance de v1 bajo "Que no se exige en v1, y
	// por que".
	custodioOrg, err := lookupOrganizationByCanonicalID(ctx, unit.CustodioActual)
	if err != nil {
		return OrganizationRecord{}, err
	}
	decision, err := domain.DecideTransfer(org.AgentType, custodioOrg.AgentType)
	if err != nil {
		return OrganizationRecord{}, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed {
		return OrganizationRecord{}, cerr.New(cerr.TransferNotAuthorized,
			"la matriz no autoriza el par %s -> %s, de modo que no existe coleccion para ese par",
			org.AgentType, custodioOrg.AgentType).
			WithDetails(map[string]any{
				"receptor": receptor,
				"par":      string(org.AgentType) + " -> " + string(custodioOrg.AgentType),
			})
	}
	return org, nil
}

// writeReturnOperation persiste el registro historico de la devolucion en la
// clave propia ReturnOp de la PDC del par. Nunca se elimina ni se sobreescribe:
// una devolucion posterior de la misma unidad usa otro txIdDevolucion y crea
// una clave nueva.
func writeReturnOperation(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
	receptor OrganizationRecord,
	motivo, timestamp string,
) error {
	collection := pairCollectionName(invoker.MSPID, receptor.MSPID)
	key, err := returnOpKey(ctx.GetStub(), unit.GTIN, unit.NumeroSerie, ctx.GetStub().GetTxID())
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave del registro de devolucion")
	}
	record := ReturnOperation{
		GTIN:              unit.GTIN,
		NumeroSerie:       unit.NumeroSerie,
		TxIDDevolucion:    ctx.GetStub().GetTxID(),
		Declarante:        unit.CustodioActual,
		ReceptorDeclarado: receptor.CanonicalID(),
		Motivo:            motivo,
		DevueltaEn:        timestamp,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return cerr.Internal(err, "no se pudo serializar el registro de devolucion")
	}
	if err := ctx.GetStub().PutPrivateData(collection, key, payload); err != nil {
		return cerr.Internal(err, "no se pudo escribir el registro de devolucion")
	}
	return nil
}
