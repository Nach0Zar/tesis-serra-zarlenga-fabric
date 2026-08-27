package domain

import "sync"

// Este archivo es la traduccion literal de la maquina de estados 1.0.0 de
// ADR-001 (docs/adr/001-maquina-estados-medicamento.md): catalogo de estados,
// eventos, actores logicos y la tabla completa de transiciones T01-T33.
//
// ADR-001 es la fuente de verdad y esta tabla no la interpreta: la reproduce.
// Toda implementacion -- chaincode y baseline -- debe rechazar cualquier
// transicion no listada aca ("Reglas de consumo" de ADR-001, y ADR-012
// seccion 4 del checklist de paridad).

// StateMachineVersion es la version de la maquina de ADR-001 que reproduce
// esta tabla.
const StateMachineVersion = "1.0.0"

// State es el estado canonico de una unidad trazable.
type State string

// Catalogo de estados de ADR-001, seccion "Estados".
const (
	StateEnLaboratorio   State = "EN_LABORATORIO"
	StateEnTransito      State = "EN_TRANSITO"
	StateEnCustodia      State = "EN_CUSTODIA"
	StateEnCuarentena    State = "EN_CUARENTENA"
	StateVencido         State = "VENCIDO"
	StateRobado          State = "ROBADO"
	StateExtraviado      State = "EXTRAVIADO"
	StateDeteriorado     State = "DETERIORADO"
	StateRetiradoMercado State = "RETIRADO_MERCADO"
	StateProhibido       State = "PROHIBIDO"
	StateDevuelto        State = "DEVUELTO"
	StateDispensado      State = "DISPENSADO"
	StateDispuestoFinal  State = "DISPUESTO_FINAL"
)

// Event es un evento de dominio que cambia el estado de una unidad. Los
// nombres son identificadores de dominio, no nombres de funciones publicas
// (ADR-001, "Reglas de consumo").
type Event string

// Catalogo de eventos de ADR-001, tabla de transiciones.
const (
	EventRegistrarUnidad            Event = "REGISTRAR_UNIDAD"
	EventDistribuirEslabonPosterior Event = "DISTRIBUIR_ESLABON_POSTERIOR"
	EventRecibirEnEstablecimiento   Event = "RECIBIR_EN_ESTABLECIMIENTO"
	EventDevolverProducto           Event = "DEVOLVER_PRODUCTO"
	EventDispensarPaciente          Event = "DISPENSAR_PACIENTE"
	EventPonerEnCuarentena          Event = "PONER_EN_CUARENTENA"
	EventLiberarCuarentena          Event = "LIBERAR_CUARENTENA"
	EventInformarVencimiento        Event = "INFORMAR_VENCIMIENTO"
	EventInformarRobo               Event = "INFORMAR_ROBO"
	EventInformarExtravio           Event = "INFORMAR_EXTRAVIO"
	EventInformarDeterioro          Event = "INFORMAR_DETERIORO"
	EventRetirarMercado             Event = "RETIRAR_MERCADO"
	EventProhibirProducto           Event = "PROHIBIR_PRODUCTO"
	EventReingresarStock            Event = "REINGRESAR_STOCK"
	EventDisponerFinal              Event = "DISPONER_FINAL"
)

// Actor es un rol logico de dominio de ADR-001. No representa una MSP ni una
// categoria de organizacion: su resolucion contra organizaciones Fabric la fija
// ADR-003, y ADR-009 punto 3 resuelve RECOVERY_OR_DISPOSAL_AGENT como el
// custodio actual registrado.
type Actor string

// Catalogo de actores logicos de ADR-001, seccion "Actores logicos".
const (
	ActorLaboratory              Actor = "LABORATORY"
	ActorCurrentCustodian        Actor = "CURRENT_CUSTODIAN"
	ActorDestinationAgent        Actor = "DESTINATION_AGENT"
	ActorDispensingAgent         Actor = "DISPENSING_AGENT"
	ActorANMAT                   Actor = "ANMAT"
	ActorRecoveryOrDisposalAgent Actor = "RECOVERY_OR_DISPOSAL_AGENT"
)

// Transition es una fila de la tabla de transiciones de ADR-001.
type Transition struct {
	// ID es el identificador de la fila (T01..T33).
	ID string
	// From son los estados de origen declarados. Vacio solo en T01, cuya
	// columna "Desde" es "Inicio".
	From []State
	// Event es el evento de dominio que dispara la transicion.
	Event Event
	// To es el estado resultante.
	To State
	// Actors es la columna "Actor habilitado" de ADR-001, que la propia ADR
	// declara fuente de verdad de quien puede detonar cada transicion: el
	// contrato DES-5 la refleja, no la restringe.
	Actors []Actor
}

// transitions reproduce, en orden, la tabla "Transiciones" de ADR-001.
var transitions = []Transition{
	{
		ID: "T01_REGISTER_UNIT", From: nil,
		Event: EventRegistrarUnidad, To: StateEnLaboratorio,
		Actors: []Actor{ActorLaboratory},
	},
	{
		ID: "T02_DISPATCH_TRANSFER_FROM_LAB", From: []State{StateEnLaboratorio},
		Event: EventDistribuirEslabonPosterior, To: StateEnTransito,
		Actors: []Actor{ActorCurrentCustodian},
	},
	{
		ID: "T03_DISPATCH_TRANSFER", From: []State{StateEnCustodia},
		Event: EventDistribuirEslabonPosterior, To: StateEnTransito,
		Actors: []Actor{ActorCurrentCustodian},
	},
	{
		ID: "T04_RECEIVE_TRANSFER", From: []State{StateEnTransito},
		Event: EventRecibirEnEstablecimiento, To: StateEnCustodia,
		Actors: []Actor{ActorDestinationAgent},
	},
	{
		ID: "T05_REJECT_OR_RETURN_IN_TRANSIT", From: []State{StateEnTransito},
		Event: EventDevolverProducto, To: StateDevuelto,
		Actors: []Actor{ActorDestinationAgent, ActorCurrentCustodian},
	},
	{
		ID: "T06_DISPENSE", From: []State{StateEnCustodia},
		Event: EventDispensarPaciente, To: StateDispensado,
		Actors: []Actor{ActorDispensingAgent},
	},
	{
		ID: "T07_QUARANTINE_FROM_LAB", From: []State{StateEnLaboratorio},
		Event: EventPonerEnCuarentena, To: StateEnCuarentena,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T08_QUARANTINE_FROM_CUSTODY", From: []State{StateEnCustodia},
		Event: EventPonerEnCuarentena, To: StateEnCuarentena,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T09_QUARANTINE_FROM_TRANSIT", From: []State{StateEnTransito},
		Event: EventPonerEnCuarentena, To: StateEnCuarentena,
		Actors: []Actor{ActorCurrentCustodian, ActorDestinationAgent, ActorANMAT},
	},
	{
		ID: "T10_RELEASE_QUARANTINE", From: []State{StateEnCuarentena},
		Event: EventLiberarCuarentena, To: StateEnCustodia,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T11_MARK_EXPIRED_FROM_LAB", From: []State{StateEnLaboratorio},
		Event: EventInformarVencimiento, To: StateVencido,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T12_MARK_EXPIRED_FROM_CUSTODY", From: []State{StateEnCustodia},
		Event: EventInformarVencimiento, To: StateVencido,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T13_MARK_EXPIRED_FROM_TRANSIT_OR_QUARANTINE", From: []State{StateEnTransito, StateEnCuarentena},
		Event: EventInformarVencimiento, To: StateVencido,
		Actors: []Actor{ActorCurrentCustodian, ActorDestinationAgent, ActorANMAT},
	},
	{
		ID:    "T14_REPORT_STOLEN",
		From:  []State{StateEnLaboratorio, StateEnTransito, StateEnCustodia, StateEnCuarentena, StateDevuelto},
		Event: EventInformarRobo, To: StateRobado,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID:    "T15_REPORT_LOST",
		From:  []State{StateEnLaboratorio, StateEnTransito, StateEnCustodia, StateEnCuarentena, StateDevuelto},
		Event: EventInformarExtravio, To: StateExtraviado,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID:    "T16_REPORT_DAMAGED",
		From:  []State{StateEnLaboratorio, StateEnTransito, StateEnCustodia, StateEnCuarentena, StateDevuelto},
		Event: EventInformarDeterioro, To: StateDeteriorado,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T17_MARK_WITHDRAWN_FROM_LAB", From: []State{StateEnLaboratorio},
		Event: EventRetirarMercado, To: StateRetiradoMercado,
		Actors: []Actor{ActorANMAT, ActorLaboratory},
	},
	{
		ID: "T18_MARK_WITHDRAWN_FROM_CUSTODY", From: []State{StateEnCustodia},
		Event: EventRetirarMercado, To: StateRetiradoMercado,
		Actors: []Actor{ActorANMAT, ActorLaboratory},
	},
	{
		ID:    "T19_MARK_WITHDRAWN_FROM_TRANSIT_QUARANTINE_OR_RETURN",
		From:  []State{StateEnTransito, StateEnCuarentena, StateDevuelto},
		Event: EventRetirarMercado, To: StateRetiradoMercado,
		Actors: []Actor{ActorANMAT, ActorLaboratory},
	},
	{
		ID: "T20_MARK_PROHIBITED",
		From: []State{
			StateEnLaboratorio, StateEnTransito, StateEnCustodia,
			StateEnCuarentena, StateDevuelto, StateRetiradoMercado,
		},
		Event: EventProhibirProducto, To: StateProhibido,
		Actors: []Actor{ActorANMAT},
	},
	{
		ID: "T21_RETURN_FROM_CUSTODY", From: []State{StateEnCustodia},
		Event: EventDevolverProducto, To: StateDevuelto,
		Actors: []Actor{ActorCurrentCustodian},
	},
	{
		ID: "T22_RETURN_FROM_QUARANTINE", From: []State{StateEnCuarentena},
		Event: EventDevolverProducto, To: StateDevuelto,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T23_RETURN_FROM_WITHDRAWN_OR_PROHIBITED", From: []State{StateRetiradoMercado, StateProhibido},
		Event: EventDevolverProducto, To: StateDevuelto,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T24_RETURN_FROM_EXPIRED", From: []State{StateVencido},
		Event: EventDevolverProducto, To: StateDevuelto,
		Actors: []Actor{ActorCurrentCustodian},
	},
	{
		ID: "T25_RESTOCK_RETURNED", From: []State{StateDevuelto},
		Event: EventReingresarStock, To: StateEnCustodia,
		Actors: []Actor{ActorRecoveryOrDisposalAgent},
	},
	{
		ID: "T26_RESTOCK_QUARANTINE", From: []State{StateEnCuarentena},
		Event: EventReingresarStock, To: StateEnCustodia,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T27_RESTOCK_WITHDRAWN", From: []State{StateRetiradoMercado},
		Event: EventReingresarStock, To: StateEnCustodia,
		Actors: []Actor{ActorANMAT, ActorLaboratory},
	},
	{
		ID: "T28_FINAL_DISPOSITION_FROM_EXPIRED", From: []State{StateVencido},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorRecoveryOrDisposalAgent},
	},
	{
		ID: "T29_FINAL_DISPOSITION_FROM_DAMAGED", From: []State{StateDeteriorado},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorRecoveryOrDisposalAgent},
	},
	{
		ID: "T30_FINAL_DISPOSITION_FROM_QUARANTINE", From: []State{StateEnCuarentena},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorCurrentCustodian, ActorANMAT},
	},
	{
		ID: "T31_FINAL_DISPOSITION_FROM_WITHDRAWN", From: []State{StateRetiradoMercado},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorANMAT, ActorLaboratory, ActorRecoveryOrDisposalAgent},
	},
	{
		ID: "T32_FINAL_DISPOSITION_FROM_PROHIBITED", From: []State{StateProhibido},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorANMAT, ActorRecoveryOrDisposalAgent},
	},
	{
		ID: "T33_FINAL_DISPOSITION_FROM_RETURNED", From: []State{StateDevuelto},
		Event: EventDisponerFinal, To: StateDispuestoFinal,
		Actors: []Actor{ActorRecoveryOrDisposalAgent},
	},
}

// terminalStates son los estados terminales absolutos de ADR-001, seccion
// "Estados terminales": no admiten ninguna transicion de salida.
var terminalStates = map[State]struct{}{
	StateDispensado:     {},
	StateRobado:         {},
	StateExtraviado:     {},
	StateDispuestoFinal: {},
}

// blockingStates bloquean la circulacion ordinaria y la dispensacion pero no
// son terminales: conservan transiciones administrativas de resolucion
// (ADR-001, seccion "Estados terminales").
var blockingStates = map[State]struct{}{
	StateVencido:         {},
	StateDeteriorado:     {},
	StateRetiradoMercado: {},
	StateProhibido:       {},
	StateDevuelto:        {},
	StateEnCuarentena:    {},
}

type statePair struct {
	from State
	to   State
}

type transitionKey struct {
	from  State
	event Event
}

var (
	transitionOnce  sync.Once
	transitionIndex map[transitionKey]Transition
	statePairIndex  map[statePair]struct{}
	stateCatalog    map[State]struct{}
)

func buildStateIndexes() {
	transitionOnce.Do(func() {
		transitionIndex = make(map[transitionKey]Transition)
		statePairIndex = make(map[statePair]struct{})
		stateCatalog = map[State]struct{}{
			StateEnLaboratorio: {}, StateEnTransito: {}, StateEnCustodia: {},
			StateEnCuarentena: {}, StateVencido: {}, StateRobado: {},
			StateExtraviado: {}, StateDeteriorado: {}, StateRetiradoMercado: {},
			StateProhibido: {}, StateDevuelto: {}, StateDispensado: {},
			StateDispuestoFinal: {},
		}

		for _, t := range transitions {
			for _, from := range t.From {
				transitionIndex[transitionKey{from, t.Event}] = t
				statePairIndex[statePair{from, t.To}] = struct{}{}
			}
		}
	})
}

// Transitions devuelve una copia de la tabla de transiciones de ADR-001.
func Transitions() []Transition {
	out := make([]Transition, len(transitions))
	copy(out, transitions)
	return out
}

// IsKnownState indica si el valor pertenece al catalogo de estados de ADR-001.
func IsKnownState(s State) bool {
	buildStateIndexes()
	_, ok := stateCatalog[s]
	return ok
}

// IsTerminalState indica si el estado es terminal absoluto: ADR-001 no declara
// ninguna transicion de salida y la logica del chaincode debe rechazar todo
// intento con INVALID_STATE_TRANSITION (ADR-007, punto 6.h).
func IsTerminalState(s State) bool {
	_, ok := terminalStates[s]
	return ok
}

// IsBlockingState indica si el estado bloquea la circulacion ordinaria y la
// dispensacion sin ser terminal.
func IsBlockingState(s State) bool {
	_, ok := blockingStates[s]
	return ok
}

// LookupTransition devuelve la transicion declarada por ADR-001 para el par
// (estado de origen, evento). El segundo valor es false cuando la combinacion
// no esta declarada, que es exactamente el caso que chaincode y baseline deben
// rechazar.
//
// El par (from, event) identifica una unica fila de la tabla; la unicidad esta
// verificada por test.
func LookupTransition(from State, event Event) (Transition, bool) {
	buildStateIndexes()
	t, ok := transitionIndex[transitionKey{from, event}]
	return t, ok
}

// LookupInitialTransition devuelve la transicion de alta (T01), cuya columna
// "Desde" de ADR-001 es "Inicio" y por lo tanto no tiene estado de origen.
func LookupInitialTransition(event Event) (Transition, bool) {
	for _, t := range transitions {
		if len(t.From) == 0 && t.Event == event {
			return t, true
		}
	}
	return Transition{}, false
}

// IsDeclaredStatePair indica si existe alguna transicion declarada de ADR-001
// que lleve de `from` a `to`. Es la comprobacion 4 de ADR-011 (camino de
// estados valido), que recomputa la secuencia observada en el historial contra
// esta misma tabla.
func IsDeclaredStatePair(from, to State) bool {
	buildStateIndexes()
	_, ok := statePairIndex[statePair{from, to}]
	return ok
}

// InitialState es el unico estado inicial de la maquina: el resultado de T01.
// ADR-011 comprobacion 4 exige que toda traza legitima comience aca.
const InitialState = StateEnLaboratorio

// AllowsActor indica si el actor logico figura en la columna "Actor habilitado"
// de la transicion.
func (t Transition) AllowsActor(a Actor) bool {
	for _, actor := range t.Actors {
		if actor == a {
			return true
		}
	}
	return false
}
