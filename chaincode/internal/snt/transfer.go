package snt

import (
	"encoding/json"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// La transferencia son DOS transacciones de chaincode separadas -- una invocada
// por el emisor y otra por el receptor -- mas el rechazo, y no una operacion
// atomica (ADR-004). La Disposicion ANMAT 3683/2011, articulo 8, lista
// distribucion y recepcion como movimientos logisticos distinguibles, cada uno
// con su propio agente detonante.

// Nombres de operacion usados en eventos y marcadores.
const (
	opDispatchTransfer = "DispatchTransfer"
	opReceiveTransfer  = "ReceiveTransfer"
	opRejectTransfer   = "RejectTransfer"
)

// DispatchTransfer implementa T02 (desde EN_LABORATORIO) y T03 (desde
// EN_CUSTODIA). Estado resultante: EN_TRANSITO.
//
// `CustodioActual` NO cambia durante el transito: permanece en el emisor hasta
// que la recepcion se confirma (ADR-004). El identificador del destinatario
// declarado NO viaja como argumento publico -- revela una relacion emisor ->
// receptor que puede no consumarse -- sino exclusivamente por el campo
// transient, y se persiste en la coleccion privada del par.
//
// Endoso: peer del custodio actual (emisor). La operacion fija ademas sobre la
// clave de la unidad la politica de transito AND(emisor, receptor declarado),
// que rige las escrituras posteriores (ADR-007, punto 6.b).
func (c *SNTContract) DispatchTransfer(
	ctx contractapi.TransactionContextInterface,
	req DispatchTransferRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := invoker.requireRole(RoleOperator); err != nil {
		return nil, err
	}
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if unit.CustodioActual != invoker.CanonicalID() {
		return nil, cerr.New(cerr.UnauthorizedCustodian,
			"el invocador no es el custodio actual de la unidad").
			WithDetails(map[string]any{"custodioActual": unit.CustodioActual})
	}

	// La aptitud del estado la decide ADR-001; la matriz solo decide si el par
	// origen-destino es admisible.
	transition, err := requireTransition(unit.Estado, domain.EventDistribuirEslabonPosterior, domain.ActorCurrentCustodian)
	if err != nil {
		return nil, err
	}

	destino, err := readDestinatarioTransient(ctx)
	if err != nil {
		return nil, err
	}
	destination, err := resolveTransferCounterparty(ctx, destino)
	if err != nil {
		return nil, err
	}
	if destination.MSPID == invoker.MSPID {
		return nil, cerr.New(cerr.InvalidDestination,
			"el destino declarado es la propia organizacion emisora").
			WithDetails(map[string]any{"destino": destino})
	}

	// Fuente unica de verdad del par origen -> destino: la matriz embebida del
	// paquete compartido, sin ifs duplicados (ADR-008, punto 2).
	decision, err := domain.DecideTransfer(invoker.Org.AgentType, destination.AgentType)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed {
		return nil, cerr.New(cerr.TransferNotAuthorized,
			"el par %s -> %s no esta autorizado por la matriz de transferencias",
			invoker.Org.AgentType, destination.AgentType).
			WithDetails(map[string]any{
				"origen":  string(invoker.Org.AgentType),
				"destino": string(destination.AgentType),
				"razon":   decision.Reason,
			})
	}

	commercial, err := readCommercialTransient(ctx, true)
	if err != nil {
		return nil, err
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	collection := pairCollectionName(invoker.MSPID, destination.MSPID)
	operation := TransferOperation{
		GTIN:                  unit.GTIN,
		NumeroSerie:           unit.NumeroSerie,
		TxIDDespacho:          ctx.GetStub().GetTxID(),
		Emisor:                invoker.CanonicalID(),
		DestinatarioPendiente: destination.CanonicalID(),
		RuleID:                decision.RuleID,
		SchemaVersion:         decision.SchemaVersion,
		NumeroRemito:          commercial.NumeroRemito,
		NumeroFactura:         commercial.NumeroFactura,
		Cantidad:              commercial.Cantidad,
		DespachadaEn:          timestamp,
	}
	if err := putActiveTransferOperation(ctx, collection, operation); err != nil {
		return nil, err
	}

	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	key, err := putUnit(ctx, unit)
	if err != nil {
		return nil, err
	}

	// Politica de transito: AND(emisor, receptor declarado), SIN rama
	// alternativa. Es el caso en que SBE si puede expresar un requisito
	// multiparte, porque el despacho deja declarada la contraparte en el estado.
	// Ningun tercero -- tampoco la organizacion regulatoria -- puede sustituir a
	// una de las partes (ADR-007, punto 6.b).
	if err := setKeyEndorsement(ctx, key, invoker.MSPID, destination.MSPID); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, opDispatchTransfer, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

// ReceiveTransfer implementa T04. Estado resultante: EN_CUSTODIA, con
// `CustodioActual` actualizado al receptor.
//
// Solo puede invocarla la organizacion que figura como destinatario declarado
// en el registro de la operacion ACTIVA -- la creada por el ultimo despacho,
// mientras la unidad permanece en EN_TRANSITO --, nunca contra registros de
// operaciones cerradas (ADR-004, "Ciclo de vida del registro de operacion").
//
// Endoso: AND(org emisora, org receptora declarada), impuesto por la politica
// de la clave que fijo el despacho.
func (c *SNTContract) ReceiveTransfer(
	ctx contractapi.TransactionContextInterface,
	req UnitRefRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := invoker.requireRole(RoleOperator); err != nil {
		return nil, err
	}
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if unit.Estado != domain.StateEnTransito {
		return nil, notInTransit(unit)
	}
	transition, err := requireTransition(unit.Estado, domain.EventRecibirEnEstablecimiento, domain.ActorDestinationAgent)
	if err != nil {
		return nil, err
	}

	// La contraparte se deriva del custodio publico -- que durante el transito
	// sigue siendo el emisor --, de modo que la recepcion resuelve la coleccion
	// con una sola lectura. Es el camino que mide DES-7 y no debe recorrer
	// colecciones candidatas.
	emitter, err := lookupOrganizationByCanonicalID(ctx, unit.CustodioActual)
	if err != nil {
		return nil, err
	}

	// Antes de resolver el nombre de la coleccion hay que comprobar que ADR-006
	// la defina para este par: sin esta guarda el chaincode intentaria leer una
	// coleccion inexistente y fallaria con un error interno de la plataforma en
	// lugar de con un codigo del contrato (ADR-009, punto 2). Si el par no esta
	// autorizado en ninguna direccion, el invocador no puede ser el destinatario
	// declarado de esta operacion.
	exists, err := pairCollectionExists(emitter, invoker.Org)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, cerr.New(cerr.ReceiverMismatch,
			"el invocador no puede ser el destinatario declarado: no existe relacion de transferencia autorizada con el emisor").
			WithDetails(map[string]any{
				"origen":  string(emitter.AgentType),
				"destino": string(invoker.Org.AgentType),
			})
	}

	collection := pairCollectionName(emitter.MSPID, invoker.MSPID)
	operation, found, err := readActiveTransferOperation(ctx, collection, unit.GTIN, unit.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if !found {
		// Que la clave no sea legible admite dos causas opuestas, y el contrato
		// les asigna codigos distintos: o el contenido todavia no se disemino a
		// este peer (transitorio, reintentable), o en esta coleccion nunca hubo
		// operacion porque el invocador no es el destinatario declarado
		// (RECEIVER_MISMATCH). El hash publico las separa: existe desde que el
		// despacho se confirma, en todos los peers, sin depender de la
		// diseminacion del contenido.
		written, err := activeTransferOperationIsWritten(ctx, collection, unit.GTIN, unit.NumeroSerie)
		if err != nil {
			return nil, err
		}
		if !written {
			return nil, receiverMismatchForPair(collection)
		}
		return nil, errPrivateDataNotDisseminated(unit.GTIN, unit.NumeroSerie, collection)
	}
	if operation.DestinatarioPendiente != invoker.CanonicalID() {
		return nil, cerr.New(cerr.ReceiverMismatch,
			"el invocador no coincide con el destinatario declarado de la operacion activa")
	}

	// Comprobacion cruzada de ADR-008, punto 5: el peer del receptor re-evalua
	// el par contra SU PROPIA matriz embebida y lo contrasta con el ruleId y la
	// schemaVersion que persistio el despacho.
	//
	// El despacho lo endosa solo el emisor, de modo que una matriz divergente en
	// ese peer autorizaria un par que ninguna otra organizacion contrasto. Con
	// esta comprobacion, ningun cambio de custodia se consuma sin que dos
	// binarios independientes hayan coincidido en que el par estaba autorizado.
	decision, err := domain.DecideTransfer(emitter.AgentType, invoker.Org.AgentType)
	if err != nil {
		return nil, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed || decision.RuleID != operation.RuleID || decision.SchemaVersion != operation.SchemaVersion {
		return nil, cerr.New(cerr.TransferNotAuthorized,
			"la matriz de este peer no coincide con la regla que autorizo el despacho").
			WithDetails(map[string]any{
				"ruleIdDespacho":        operation.RuleID,
				"schemaVersionDespacho": operation.SchemaVersion,
				"ruleIdReceptor":        decision.RuleID,
				"schemaVersionReceptor": decision.SchemaVersion,
			})
	}

	recepcion, err := readOptionalCommercialTransient(ctx)
	if err != nil {
		return nil, err
	}
	if err := closeTransferOperation(ctx, collection, operation, closureReception, recepcion); err != nil {
		return nil, err
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	unit.CustodioActual = invoker.CanonicalID()
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	key, err := putUnit(ctx, unit)
	if err != nil {
		return nil, err
	}

	// Restauracion de la politica de reposo: el custodio registrado pasa a ser
	// el receptor.
	if err := restoreRestingEndorsement(ctx, key, invoker.MSPID); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, opReceiveTransfer, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

// RejectTransfer implementa T05. Estado resultante: DEVUELTO.
//
// La invoca el destinatario declarado o el emisor, con la causa documentada.
// `CustodioActual` permanece en el emisor: la devolucion es un evento unico que
// no modifica la custodia registrada, ni en T05 ni en T21-T24 (ADR-009, punto 1).
// T05 no registra que el retorno fisico al remitente haya ocurrido.
func (c *SNTContract) RejectTransfer(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	invoker, err := resolveInvoker(ctx)
	if err != nil {
		return nil, err
	}
	if err := invoker.requireRole(RoleOperator); err != nil {
		return nil, err
	}
	if err := validateUnitRef(req.GTIN, req.NumeroSerie); err != nil {
		return nil, err
	}
	if req.Motivo == "" {
		return nil, invalidRequest("motivo es obligatorio: el rechazo debe documentar su causa")
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}
	if unit.Estado != domain.StateEnTransito {
		return nil, notInTransit(unit)
	}

	isEmitter := unit.CustodioActual == invoker.CanonicalID()
	actor := domain.ActorDestinationAgent
	if isEmitter {
		actor = domain.ActorCurrentCustodian
	}
	transition, err := requireTransition(unit.Estado, domain.EventDevolverProducto, actor)
	if err != nil {
		return nil, err
	}

	var (
		operation  TransferOperation
		collection string
	)
	if isEmitter {
		// El emisor no conoce al destinatario declarado sin leer el registro:
		// es un dato privado que, por decision de ADR-004, no esta en el estado
		// publico. Hay que buscarlo entre las colecciones de las que es miembro.
		var found, written bool
		operation, collection, found, written, err = findActiveTransferOperation(ctx, unit, invoker)
		if err != nil {
			return nil, err
		}
		if !written {
			// La unidad esta en EN_TRANSITO y el invocador es su custodio, de
			// modo que por construccion existe una operacion activa en alguna
			// de sus colecciones (ADR-004, regla 2). Que no haya hash en
			// ninguna es una inconsistencia del ledger, no un caso de negocio.
			return nil, cerr.New(cerr.InternalError,
				"la unidad %s/%s esta en EN_TRANSITO sin registro de operacion activa",
				unit.GTIN, unit.NumeroSerie)
		}
		if !found {
			return nil, errPrivateDataNotDisseminated(unit.GTIN, unit.NumeroSerie, collection)
		}
	} else {
		emitter, err := lookupOrganizationByCanonicalID(ctx, unit.CustodioActual)
		if err != nil {
			return nil, err
		}
		exists, err := pairCollectionExists(emitter, invoker.Org)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, cerr.New(cerr.ReceiverMismatch,
				"el invocador no es ni el emisor ni un destinatario declarado posible de esta operacion")
		}
		collection = pairCollectionName(emitter.MSPID, invoker.MSPID)
		var found bool
		operation, found, err = readActiveTransferOperation(ctx, collection, unit.GTIN, unit.NumeroSerie)
		if err != nil {
			return nil, err
		}
		if !found {
			written, err := activeTransferOperationIsWritten(ctx, collection, unit.GTIN, unit.NumeroSerie)
			if err != nil {
				return nil, err
			}
			if !written {
				return nil, receiverMismatchForPair(collection)
			}
			return nil, errPrivateDataNotDisseminated(unit.GTIN, unit.NumeroSerie, collection)
		}
		if operation.DestinatarioPendiente != invoker.CanonicalID() {
			return nil, cerr.New(cerr.ReceiverMismatch,
				"el invocador no es ni el emisor ni el destinatario declarado de la operacion activa")
		}
	}

	if err := closeTransferOperation(ctx, collection, operation, closureRejection, nil); err != nil {
		return nil, err
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	// La custodia permanece en el emisor (ADR-004; ADR-009, punto 1).
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	key, err := putUnit(ctx, unit)
	if err != nil {
		return nil, err
	}

	// Restauracion de la politica de reposo hacia el EMISOR, que sigue siendo
	// el custodio registrado.
	emitterMSPID, err := mspIDForCanonicalID(ctx, unit.CustodioActual)
	if err != nil {
		return nil, err
	}
	if err := restoreRestingEndorsement(ctx, key, emitterMSPID); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, opRejectTransfer, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

// receiverMismatchForPair es el rechazo DEFINITIVO por destinatario: el ledger
// publico no registra ninguna operacion activa de esta unidad en la coleccion
// del par, de modo que el invocador no es el destinatario declarado. No es
// reintentable.
func receiverMismatchForPair(collection string) error {
	return cerr.New(cerr.ReceiverMismatch,
		"el invocador no es el destinatario declarado: no hay operacion activa de esta unidad entre ambas organizaciones").
		WithDetails(map[string]any{"coleccion": collection})
}

// CloseTransitForExtraordinaryEvent compone la salida de EN_TRANSITO por un
// evento extraordinario (T09, T13-T16) en las TRES piezas que ADR-007 exige, de
// modo que las issues EXT no tengan que recomponerlas ni puedan olvidarse de
// una:
//
//  1. el marcador de participacion en la coleccion implicita de la organizacion
//     regulatoria, cuando es ella quien inicia el evento (punto 6.d). La firma
//     de creador acredita identidad e iniciativa pero NO es un endoso de peer;
//     el marcador es lo que somete la transaccion a la politica de esa
//     coleccion y convierte la participacion del regulador en coendoso real.
//     Es el segundo uso del marcador —exigir el endoso de una organizacion que
//     no es titular de la clave escrita—, distinto del que cierra la ventana de
//     creacion de una clave publica nueva (punto 6.g);
//  2. el cierre del registro de operacion, que escribe el historico y elimina
//     la clave activa (ADR-004, regla 4; ADR-006, punto 4);
//  3. la restauracion de la politica de reposo hacia el EMISOR, que sigue
//     siendo el custodio registrado porque el transito no llego a consumarse
//     (punto 6.c).
//
// Omitir (3) dejaria la unidad bajo una politica que exige al receptor de un
// despacho ya resuelto: bloqueo permanente. Omitir (1) haria que el evento
// regulatorio se apoyara solo en la firma de creador, que es el error que la
// version anterior de ADR-007 corrigio.
//
// No cambia el estado ni el custodio de la unidad: eso lo hace la operacion EXT
// que la invoca, conforme la transicion de ADR-001 que corresponda.
func CloseTransitForExtraordinaryEvent(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
	operation string,
) error {
	op, collection, found, written, err := findActiveTransferOperation(ctx, unit, invoker)
	if err != nil {
		return err
	}
	if !written {
		return cerr.New(cerr.InternalError,
			"la unidad %s/%s esta en EN_TRANSITO sin registro de operacion activa",
			unit.GTIN, unit.NumeroSerie)
	}
	if !found {
		return errPrivateDataNotDisseminated(unit.GTIN, unit.NumeroSerie, collection)
	}

	if invoker.Org.AgentType == domain.AgentRegulator {
		if err := writeUnitParticipationMarker(
			ctx, invoker.MSPID, operation, invoker.MSPID, unit.GTIN, unit.NumeroSerie); err != nil {
			return err
		}
	}

	if err := closeTransferOperation(ctx, collection, op, closureExtraordinary, nil); err != nil {
		return err
	}

	key, err := medicationUnitKey(ctx.GetStub(), unit.GTIN, unit.NumeroSerie)
	if err != nil {
		return cerr.Internal(err, "no se pudo construir la clave de la unidad")
	}
	emitterMSPID, err := mspIDForCanonicalID(ctx, unit.CustodioActual)
	if err != nil {
		return err
	}
	return restoreRestingEndorsement(ctx, key, emitterMSPID)
}

// restoreRestingEndorsement devuelve la clave de la unidad a la politica de
// reposo: la organizacion del custodio registrado, sin rama alternativa
// (ADR-007, punto 6.a).
//
// Debe ejecutarse en TODA salida de EN_TRANSITO -- recepcion, rechazo y evento
// extraordinario que cierre el transito (ADR-007, punto 6.c). Omitirla dejaria
// la unidad bajo una politica que exige al receptor de un despacho ya resuelto:
// bloqueo permanente de las operaciones del custodio. Es requisito de
// correccion, no optimizacion.
func restoreRestingEndorsement(ctx contractapi.TransactionContextInterface, key, custodianMSPID string) error {
	return setKeyEndorsement(ctx, key, custodianMSPID)
}

// requireTransition resuelve la transicion de ADR-001 para el par (estado de
// origen, evento) y verifica que el actor logico este habilitado. La columna
// "Actor habilitado" de ADR-001 es la fuente de verdad; el contrato la refleja,
// no la restringe.
func requireTransition(from domain.State, event domain.Event, actor domain.Actor) (domain.Transition, error) {
	transition, ok := domain.LookupTransition(from, event)
	if !ok {
		return domain.Transition{}, cerr.New(cerr.InvalidStateTransition,
			"la maquina de estados no declara el evento %s desde el estado %s", event, from).
			WithDetails(map[string]any{"estado": string(from), "evento": string(event)})
	}
	if !transition.AllowsActor(actor) {
		return domain.Transition{}, cerr.New(cerr.InvalidStateTransition,
			"la transicion %s no habilita al actor %s", transition.ID, actor).
			WithDetails(map[string]any{"transicion": transition.ID, "actor": string(actor)})
	}
	return transition, nil
}

func notInTransit(unit MedicationUnit) error {
	return cerr.New(cerr.NotInTransit,
		"la unidad no esta en EN_TRANSITO (estado actual: %s)", unit.Estado).
		WithDetails(map[string]any{"estado": string(unit.Estado)})
}

// readDestinatarioTransient lee el destino declarado del campo transient. El
// identificador NUNCA viaja como argumento publico: los argumentos ordinarios
// quedan registrados en la transaccion visible del canal, y solo el transient
// queda excluido de ella (ADR-004).
func readDestinatarioTransient(ctx contractapi.TransactionContextInterface) (string, error) {
	raw, found, err := readTransient(ctx, transientDestinatario)
	if err != nil {
		return "", err
	}
	if !found || len(raw) == 0 {
		return "", invalidRequest("el despacho exige el destino declarado en el transient %q", transientDestinatario)
	}
	var payload destinatarioTransient
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", invalidRequest("el transient %q no es un objeto JSON valido", transientDestinatario)
	}
	if payload.Destino == "" {
		return "", invalidRequest("el transient %q debe declarar el campo destino", transientDestinatario)
	}
	return payload.Destino, nil
}

func readCommercialTransient(ctx contractapi.TransactionContextInterface, required bool) (CommercialData, error) {
	data, err := readOptionalCommercialTransient(ctx)
	if err != nil {
		return CommercialData{}, err
	}
	if data == nil {
		if required {
			return CommercialData{}, invalidRequest(
				"el despacho exige los datos documentales en el transient %q (remito, factura y cantidad)",
				transientCommercial)
		}
		return CommercialData{}, nil
	}
	if required && (data.NumeroRemito == "" || data.NumeroFactura == "" || data.Cantidad < 1) {
		return CommercialData{}, invalidRequest(
			"el transient %q debe incluir numeroRemito, numeroFactura y una cantidad mayor a cero",
			transientCommercial)
	}
	return *data, nil
}

func readOptionalCommercialTransient(ctx contractapi.TransactionContextInterface) (*CommercialData, error) {
	raw, found, err := readTransient(ctx, transientCommercial)
	if err != nil {
		return nil, err
	}
	if !found || len(raw) == 0 {
		return nil, nil
	}
	var data CommercialData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, invalidRequest("el transient %q no es un objeto JSON valido", transientCommercial)
	}
	return &data, nil
}

// resolveTransferCounterparty resuelve la contraparte declarada de una
// transferencia. El contrato admite declararla por identificador canonico o por
// mspId; en ambos casos se valida contra el registro (ADR-003).
//
// Los agentType no custodiales nunca son origen ni destino de una transferencia
// (ADR-010, punto 2), y el chaincode lo valida estructuralmente.
func resolveTransferCounterparty(
	ctx contractapi.TransactionContextInterface,
	declared string,
) (OrganizationRecord, error) {
	var (
		org OrganizationRecord
		err error
	)
	if _, _, parseErr := parseCanonicalID(declared); parseErr == nil {
		org, err = lookupOrganizationByCanonicalID(ctx, declared)
	} else {
		var found bool
		org, found, err = readOrganization(ctx, declared)
		if err == nil && !found {
			err = cerr.New(cerr.OrgNotRegistered,
				"el destino %q no tiene entrada en el registro organizacion-establecimiento", declared).
				WithDetails(map[string]any{"destino": declared})
		}
	}
	if err != nil {
		return OrganizationRecord{}, err
	}

	if !org.Active {
		return OrganizationRecord{}, cerr.New(cerr.OrgInactive,
			"el destino declarado %q esta registrado pero no habilitado", declared).
			WithDetails(map[string]any{"destino": declared})
	}

	custodial, err := domain.IsCustodialAgentType(org.AgentType)
	if err != nil {
		return OrganizationRecord{}, cerr.Internal(err, "no se pudo consultar el catalogo de agentType")
	}
	if !custodial {
		return OrganizationRecord{}, cerr.New(cerr.InvalidDestination,
			"el agentType %s no puede ser destino de una transferencia", org.AgentType).
			WithDetails(map[string]any{"destino": declared, "agentType": string(org.AgentType)})
	}
	return org, nil
}

// lookupOrganizationByCanonicalID resuelve una entrada del registro por su
// identificador canonico, sin exigir que este habilitada: la habilitacion es
// una regla de negocio que cada operacion aplica donde corresponde.
func lookupOrganizationByCanonicalID(
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
		if org.CanonicalID() == canonicalID {
			return org, nil
		}
	}
	return OrganizationRecord{}, cerr.New(cerr.OrgNotRegistered,
		"el identificador %s no tiene entrada en el registro organizacion-establecimiento", canonicalID).
		WithDetails(map[string]any{"id": canonicalID})
}

// mspIDForCanonicalID traduce un identificador canonico al mspId de su
// organizacion, para fijar politicas de endoso por clave.
func mspIDForCanonicalID(ctx contractapi.TransactionContextInterface, canonicalID string) (string, error) {
	org, err := lookupOrganizationByCanonicalID(ctx, canonicalID)
	if err != nil {
		return "", err
	}
	return org.MSPID, nil
}
