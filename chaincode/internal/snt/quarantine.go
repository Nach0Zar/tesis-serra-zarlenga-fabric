package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	opQuarantine        = "Quarantine"
	opReleaseQuarantine = "ReleaseQuarantine"
)

// Quarantine implementa T07, T08 y T09 de ADR-001: la suspension cautelar de
// una unidad ante una anomalia. Estado resultante: EN_CUARENTENA, que ADR-001
// declara bloqueante y no terminal -- conserva siete transiciones de salida, y
// por eso la unidad no queda inmovilizada para siempre.
//
// Suspende circulacion y dispensacion sin necesidad de una regla propia: el
// bloqueo lo produce ADR-001, porque ninguna transicion de despacho ni T06
// declara EN_CUARENTENA como estado de origen, y requireTransition las rechaza
// con INVALID_STATE_TRANSITION.
//
// Autorizacion (DES-6, "Evento extraordinario informado por custodio"): el
// custodio actual o la organizacion regulatoria. Durante EN_TRANSITO se suma el
// DESTINATARIO DECLARADO, que es la precision que ADR-001 enuncia para T09
// -- "se detecta anomalia durante el traslado o recepcion" -- y que el contrato
// incorporo en la version 2.6.0. Sin ella, un receptor que recibe mercaderia
// anomala solo podria aceptar la custodia y recien despues ponerla en
// cuarentena, o rechazar la transferencia entera: dos caminos que registran en
// el ledger algo distinto de lo que ocurrio.
//
// El destinatario declarado NO viaja en el request: se resuelve leyendo el
// registro de la operacion ACTIVA en la PDC del par, con el mismo criterio que
// ReceiveTransfer. Aceptarlo como argumento publico revelaria una relacion
// comercial no consumada (ADR-004) y dejaria la autorizacion a criterio de quien
// envia la propuesta.
func (c *SNTContract) Quarantine(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyQuarantineTransition(ctx, req, domain.EventPonerEnCuarentena, opQuarantine)
}

// ReleaseQuarantine implementa T10: el retorno al flujo normal cuando la
// anomalia se descarta. Estado resultante: EN_CUSTODIA.
//
// El destinatario declarado NO aparece entre los actores habilitados, y no es
// una omision: si la unidad entro en cuarentena desde EN_TRANSITO por T09, esa
// misma operacion cerro el registro de la transferencia (ADR-006, punto 4), de
// modo que al momento de liberar ya no existe destinatario declarado alguno. La
// unidad sale hacia EN_CUSTODIA del emisor, que es el custodio registrado.
func (c *SNTContract) ReleaseQuarantine(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyQuarantineTransition(ctx, req, domain.EventLiberarCuarentena, opReleaseQuarantine)
}

// applyQuarantineTransition concentra el flujo que comparten las dos
// operaciones. La diferencia entre ellas es el evento de ADR-001 que detonan;
// todo lo demas -- autorizacion, resolucion del actor logico, cierre del
// transito y escritura -- es identico, y duplicarlo invitaria a que una de las
// dos se olvidara de una pieza.
func applyQuarantineTransition(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
	event domain.Event,
	operation string,
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
func eventAllowsDeclaredRecipient(event domain.Event) bool {
	return event == domain.EventPonerEnCuarentena || event == domain.EventInformarVencimiento
}
