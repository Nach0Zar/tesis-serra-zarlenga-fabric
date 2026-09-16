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
// presenta el invocador, en el orden en que las alternativas son excluyentes.
//
// El destinatario declarado solo se considera cuando la unidad esta en
// EN_TRANSITO y el evento es uno de los dos que ADR-001 le habilita. NO se
// extiende a T14-T16 (robo, extravio, deterioro), que ADR-001 reserva al
// custodio o a ANMAT aunque la unidad este en transito: son hechos sobre los
// que el destinatario declarado no tiene conocimiento propio mientras la unidad
// no este en su poder.
//
// Un invocador que no es ninguno de los tres recibe UNAUTHORIZED_CUSTODIAN, y no
// un rechazo de transicion: el problema no es que ADR-001 no declare la
// transicion, es que quien la pide no tiene caracter para pedirla.
func resolveExtraordinaryEventActor(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	invoker Invoker,
	event domain.Event,
) (domain.Actor, error) {
	if invoker.Org.AgentType == domain.AgentRegulator {
		return domain.ActorANMAT, nil
	}
	// El LABORATORIO titular se resuelve ANTES del caso generico de custodio, y
	// el orden es una correccion, no una preferencia. ADR-001 habilita el retiro
	// unicamente a ANMAT y a LABORATORY: si el laboratorio tambien es el
	// custodio -- el caso normal de T17, con la unidad todavia EN_LABORATORIO --
	// devolver ActorCurrentCustodian lo hacia rechazar por requireTransition, y
	// con eso el retiro VOLUNTARIO, que es el caso de uso principal de la
	// operacion, quedaba inalcanzable.
	//
	// Esta resolucion es por EVENTO y no por transicion, de modo que un
	// laboratorio queda resuelto como LABORATORY tambien cuando la unidad esta
	// EN_TRANSITO; el rechazo lo produce entonces requireTransition, porque
	// ADR-001 revision 2 reserva ese origen a ANMAT (DES-19). Esa division es
	// deliberada: quien puede pedir la operacion lo decide el caracter del
	// invocador, y en que estados procede lo decide la tabla de ADR-001, que es
	// la unica fuente de esa regla.
	//
	// La lista de eventos es explicita por la misma razon que la del
	// destinatario declarado: la habilitacion es una decision de ADR-001 por
	// transicion. Para cualquier otro evento un laboratorio cae al caso de
	// custodio, como corresponde.
	if invoker.Org.AgentType == domain.AgentLaboratory && eventAllowsTitularLaboratory(event) {
		return domain.ActorLaboratory, nil
	}

	if unit.CustodioActual == invoker.CanonicalID() {
		return domain.ActorCurrentCustodian, nil
	}

	if unit.Estado == domain.StateEnTransito && eventAllowsDeclaredRecipient(event) {
		op, _, found, err := findActiveTransferOperation(ctx, unit, invoker)
		if err != nil {
			return "", err
		}
		if found && op.DestinatarioPendiente == invoker.CanonicalID() {
			return domain.ActorDestinationAgent, nil
		}
	}

	return "", cerr.New(cerr.UnauthorizedCustodian,
		"el invocador no es el custodio actual, el destinatario declarado ni la organizacion regulatoria").
		WithDetails(map[string]any{"custodioActual": unit.CustodioActual, "estado": string(unit.Estado)})
}

// eventAllowsDeclaredRecipient enumera los dos eventos que ADR-001 habilita al
// destinatario declarado durante el transito: T09 y T13. Es una lista explicita
// y no una propiedad derivada, porque la habilitacion es una decision de ADR-001
// por transicion y no una regla general sobre los eventos extraordinarios.
// eventAllowsTitularLaboratory enumera los eventos que ADR-001 abre al
// LABORATORIO titular aunque no sea el custodio actual. Es una lista explicita y
// no una propiedad derivada, por la misma razon que la del destinatario
// declarado: la habilitacion es una decision de ADR-001 por transicion.
//
// REINGRESAR_STOCK (T27) y DISPONER_FINAL (T31) tambien lo habilitan, y se
// agregan cuando EXT-5 (#31) y EXT-8 (#63) implementen esas operaciones.
func eventAllowsTitularLaboratory(event domain.Event) bool {
	return event == domain.EventRetirarMercado
}

func eventAllowsDeclaredRecipient(event domain.Event) bool {
	return event == domain.EventPonerEnCuarentena || event == domain.EventInformarVencimiento
}
