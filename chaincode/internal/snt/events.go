package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// eventPrecondition expresa una precondicion que ADR-001 declara para una
// transicion concreta y que no se deriva del par (estado, actor). El motor la
// evalua entre la resolucion de la transicion y la primera escritura.
type eventPrecondition func(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	transition domain.Transition,
) error

// Motor comun de los eventos extraordinarios sobre una unidad (ADR-001,
// T07-T20). Cada operacion EXT aporta unicamente su evento; el flujo -- roles
// de DES-6, resolucion del actor logico contra el ledger, cierre del transito
// con las tres piezas de ADR-007 y escritura -- vive una sola vez aca.

// applyExtraordinaryEvent concentra el flujo que comparten TODOS los eventos
// extraordinarios sobre una unidad. La diferencia entre ellos es el evento de ADR-001 que detonan;
// todo lo demas -- autorizacion, resolucion del actor logico, cierre del
// transito y escritura -- es identico, y duplicarlo invitaria a que alguno se
// olvidara de una pieza. EXT-1 (#27) lo estrena y EXT-2 (#28) lo consume sin
// reescribirlo, con el mismo criterio con que CC-8 consumio el helper de cadena
// de custodia que dejo CC-7.
func applyExtraordinaryEvent(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
	event domain.Event,
	operation string,
	precondition eventPrecondition,
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
	// `motivo` es obligatorio con el mismo criterio que en RejectTransfer y
	// AuthorizeLabIntervention: un evento extraordinario deja un asiento
	// permanente en la traza y la causa regulatoria es parte del asiento. El
	// contrato lo acota a texto breve y neutro, sin datos personales, clinicos
	// ni comerciales.
	if req.Motivo == "" {
		return nil, invalidRequest("motivo es obligatorio para documentar la causa del evento")
	}

	unit, err := readUnit(ctx, req.GTIN, req.NumeroSerie)
	if err != nil {
		return nil, err
	}

	// El actor logico se DERIVA del estado del ledger, nunca del request: quien
	// invoca no declara en que caracter lo hace. requireTransition despues
	// comprueba que ADR-001 habilite a ese actor para la transicion observada.
	actor, err := resolveExtraordinaryEventActor(ctx, unit, invoker, event)
	if err != nil {
		return nil, err
	}
	transition, err := requireTransition(unit.Estado, event, actor)
	if err != nil {
		return nil, err
	}

	// Precondicion propia de la transicion, cuando ADR-001 declara una que va
	// mas alla del par (estado, actor). Se evalua DESPUES de resolver la
	// transicion porque varias operaciones tienen preconciones distintas segun
	// el estado de origen, y antes de cualquier escritura.
	if precondition != nil {
		if err := precondition(ctx, unit, transition); err != nil {
			return nil, err
		}
	}

	// T09 saca la unidad de EN_TRANSITO, y por lo tanto cierra el transito: sin
	// esto la unidad queda bajo la politica AND(emisor, receptor declarado) de
	// un despacho ya resuelto, es decir bloqueada de forma permanente
	// (ADR-007, punto 6.c). El helper lo compone con las otras dos piezas que
	// exige ADR-007 y que son faciles de olvidar por separado.
	if unit.Estado == domain.StateEnTransito {
		if err := CloseTransitForExtraordinaryEvent(ctx, unit, invoker, operation); err != nil {
			return nil, err
		}
	} else if invoker.Org.AgentType == domain.AgentRegulator {
		// Fuera del transito no hay registro de operacion que cerrar, pero el
		// coendoso regulatorio sigue haciendo falta cuando el evento lo inicia
		// ANMAT: su firma de creador acredita identidad, no ejecucion por un
		// peer suyo (ADR-007, punto 6.d).
		if err := writeUnitParticipationMarker(
			ctx, invoker.MSPID, operation, invoker.MSPID, unit.GTIN, unit.NumeroSerie); err != nil {
			return nil, err
		}
	}

	timestamp, err := txTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	// El custodio NO cambia. ADR-004 acopla custodia y estado: solo la recepcion
	// (T04) mueve CustodioActual, y un evento extraordinario no es una entrega.
	// Durante el transito el custodio registrado sigue siendo el emisor, de modo
	// que una unidad puesta en cuarentena por el destinatario declarado queda en
	// EN_CUARENTENA bajo custodia del emisor: es lo que efectivamente ocurrio.
	unit.Estado = transition.To
	unit.UltimaActualizacion = timestamp
	if _, err := putUnit(ctx, unit); err != nil {
		return nil, err
	}

	if err := emitUnitEvent(ctx, operation, unit); err != nil {
		return nil, err
	}

	view := MedicationUnitView(unit)
	return &view, nil
}

// requireExtraordinaryEventRole aplica la tabla de roles de DES-6: las
// organizaciones custodiales operan con `operator`, y la regulatoria con
// `regulatory-admin`, que es el rol al que DES-6 asigna "eventos regulatorios o
// extraordinarios". `auditor` no habilita escrituras.
//
// La condicion se deriva del agentType del registro, no del nombre de la MSP
// (ADR-003, ADR-010).
func requireExtraordinaryEventRole(invoker Invoker) error {
	if invoker.Org.AgentType == domain.AgentRegulator {
		return invoker.requireRole(RoleRegulatoryAdmin)
	}
	return invoker.requireRole(RoleOperator)
}

// resolveExtraordinaryEventActor decide con que actor logico de ADR-001 se
// presenta el invocador. Los caracteres NO son excluyentes -- el laboratorio
// titular que ademas custodia la unidad reune dos --, de modo que la eleccion la
// hace la tabla: se toma el primer caracter que la fila de ADR-001 del estado de
// origen observado habilita.
//
// De ahi sale, sin regla propia, que el destinatario declarado pueda poner en
// cuarentena (T09) e informar vencimiento (T13) durante el transito pero no
// informar robo, extravio ni deterioro (T14-T16): esas tres filas no lo listan.
// ADR-001 lo decide asi porque son hechos sobre los que el destinatario
// declarado no tiene conocimiento propio mientras la unidad no este en su poder.
//
// Los dos rechazos posibles no son intercambiables:
//
//   - UNAUTHORIZED_CUSTODIAN cuando el invocador no reune NINGUN caracter sobre
//     la unidad. El problema no es que ADR-001 no declare la transicion, es que
//     quien la pide no tiene caracter para pedirla.
//   - INVALID_STATE_TRANSITION cuando reune alguno pero la fila del estado
//     observado no lo habilita. Ahi el problema no es quien pide, es donde: el
//     mismo invocador podria ejecutar la operacion desde otro estado de origen.
//     Lo emite requireTransition, nombrando al caracter rechazado.
func resolveExtraordinaryEventActor(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
	event domain.Event,
) (domain.Actor, error) {
	transition, declared := domain.LookupTransition(unit.Estado, event)

	characters, err := invokerCharacters(ctx, unit, invoker, transition, declared)
	if err != nil {
		return "", err
	}
	if len(characters) == 0 {
		return "", cerr.New(cerr.UnauthorizedCustodian,
			"el invocador no es el custodio actual, el destinatario declarado, "+
				"el laboratorio titular ni la organizacion regulatoria").
			WithDetails(map[string]any{"custodioActual": unit.CustodioActual, "estado": string(unit.Estado)})
	}

	if declared {
		for _, actor := range characters {
			if transition.AllowsActor(actor) {
				return actor, nil
			}
		}
	}

	// El invocador tiene caracter, pero ADR-001 no lo habilita en este estado de
	// origen. Se devuelve el primero para que requireTransition emita el rechazo
	// de transicion nombrandolo: el problema no es quien pide, es donde.
	return characters[0], nil
}

// invokerCharacters enumera los caracteres de ADR-001 que el invocador reune
// sobre ESTA unidad, en orden de especificidad. Un mismo invocador puede reunir
// varios a la vez -- el laboratorio titular que ademas custodia la unidad es el
// caso normal de T17 --, y cual de ellos aplica no lo decide el codigo: lo
// decide la columna "actor habilitado" de la fila de ADR-001 que corresponde al
// estado de origen observado.
//
// Derivar la habilitacion de la tabla, y no de una lista de eventos escrita a
// mano, es una correccion que EXT-5 forzo. Con una lista por EVENTO,
// REINGRESAR_STOCK habria resuelto a LABORATORY tambien en T25 y T26, que
// ADR-001 reserva al custodio -- el mismo defecto que EXT-6 tuvo que arreglar
// en sentido inverso para T17. La tabla ya expresa la regla por transicion, que
// es la granularidad en la que ADR-001 la decide; la lista era una copia de esa
// informacion condenada a divergir.
//
// La lectura de la PDC para resolver al destinatario declarado se hace solo
// cuando la transicion observada lo habilita: es un acceso a datos privados y no
// corresponde ejecutarlo para averiguar algo que la tabla ya niega.
func invokerCharacters(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
	transition domain.Transition,
	declared bool,
) ([]domain.Actor, error) {
	// La organizacion regulatoria no acumula caracteres: no custodia unidades
	// (ADR-010, punto 1) ni es titular de producto alguno.
	if invoker.Org.AgentType == domain.AgentRegulator {
		return []domain.Actor{domain.ActorANMAT}, nil
	}

	var characters []domain.Actor
	if invoker.Org.AgentType == domain.AgentLaboratory {
		characters = append(characters, domain.ActorLaboratory)
	}
	if unit.CustodioActual == invoker.CanonicalID() {
		// ADR-009 punto 3 resuelve RECOVERY_OR_DISPOSAL_AGENT como el custodio
		// actual registrado con rol operator: es el mismo invocador bajo otro
		// nombre, y por eso los dos caracteres salen de la misma condicion. El
		// rol lo exige requireExtraordinaryEventRole aguas arriba.
		characters = append(characters, domain.ActorCurrentCustodian, domain.ActorRecoveryOrDisposalAgent)
	}
	if declared && unit.Estado == domain.StateEnTransito &&
		transition.AllowsActor(domain.ActorDestinationAgent) {
		op, _, found, err := findActiveTransferOperation(ctx, unit, invoker)
		if err != nil {
			return nil, err
		}
		if found && op.DestinatarioPendiente == invoker.CanonicalID() {
			characters = append(characters, domain.ActorDestinationAgent)
		}
	}
	return characters, nil
}
