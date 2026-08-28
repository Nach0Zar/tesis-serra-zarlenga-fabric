package domain

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestTransitionTableMatchesADR001 contrasta la tabla de este paquete contra la
// tabla Markdown de docs/adr/001-maquina-estados-medicamento.md. ADR-001 es la
// fuente de verdad; este paquete la reproduce, y el test impide que las dos
// diverjan en silencio.
func TestTransitionTableMatchesADR001(t *testing.T) {
	raw, err := os.ReadFile("../docs/adr/001-maquina-estados-medicamento.md")
	if err != nil {
		t.Fatalf("no se pudo leer ADR-001: %v", err)
	}

	rowRE := regexp.MustCompile(`(?m)^\| ` + "`" + `(T\d\d_[A-Z_]+)` + "`" + ` \|(.*)$`)
	matches := rowRE.FindAllStringSubmatch(string(raw), -1)
	if len(matches) != len(transitions) {
		t.Fatalf("ADR-001 declara %d transiciones y el paquete %d", len(matches), len(transitions))
	}

	byID := make(map[string]Transition, len(transitions))
	for _, tr := range transitions {
		byID[tr.ID] = tr
	}

	for _, match := range matches {
		id := match[1]
		tr, ok := byID[id]
		if !ok {
			t.Errorf("ADR-001 declara %s y el paquete no lo tiene", id)
			continue
		}

		// Columnas de la tabla de ADR-001 despues del ID:
		// Desde | Evento | Hacia | Actor habilitado | Precondiciones.
		cols := strings.Split(match[2], "|")
		if len(cols) < 5 {
			t.Fatalf("fila %s de ADR-001 mal formada", id)
		}
		from, event, to, actors := cleanCell(cols[0]), cleanCell(cols[1]), cleanCell(cols[2]), cleanCell(cols[3])

		if got := strings.Join(statesToStrings(tr.From), " o "); !equivalentList(from, got) {
			t.Errorf("%s: estados de origen ADR-001 %q vs paquete %q", id, from, got)
		}
		if string(tr.Event) != event {
			t.Errorf("%s: evento ADR-001 %q vs paquete %q", id, event, tr.Event)
		}
		if string(tr.To) != to {
			t.Errorf("%s: estado destino ADR-001 %q vs paquete %q", id, to, tr.To)
		}
		// La columna "Actor habilitado" no es documental: ADR-001 ("Reglas de
		// consumo") la declara fuente de verdad de quien puede detonar cada
		// transicion, y de ella dependen las decisiones de autorizacion y de
		// endoso posteriores. Si el paquete la reprodujera mal, el contrato
		// habilitaria a un actor que la ADR no habilita -- o al reves -- sin
		// que nada fallara.
		if got := strings.Join(actorsToStrings(tr.Actors), " o "); !equivalentList(actors, got) {
			t.Errorf("%s: actores habilitados ADR-001 %q vs paquete %q", id, actors, got)
		}
	}
}

func cleanCell(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "`", ""))
}

func statesToStrings(states []State) []string {
	if len(states) == 0 {
		return []string{"Inicio"}
	}
	out := make([]string, len(states))
	for i, s := range states {
		out[i] = string(s)
	}
	return out
}

func actorsToStrings(actors []Actor) []string {
	out := make([]string, len(actors))
	for i, a := range actors {
		out[i] = string(a)
	}
	return out
}

// equivalentList compara dos enumeraciones de una celda de ADR-001 -- estados
// de origen o actores habilitados -- sin depender del separador que use la
// redaccion, que mezcla ", " y " o " en la misma celda, ni del orden.
func equivalentList(adr, pkg string) bool {
	norm := func(s string) []string {
		fields := strings.Fields(strings.ReplaceAll(s, ",", " "))
		out := make([]string, 0, len(fields))
		for _, f := range fields {
			if f == "o" {
				continue
			}
			out = append(out, f)
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(norm(adr), norm(pkg))
}

// TestTransitionKeyUniqueness verifica el supuesto sobre el que se apoya
// LookupTransition: el par (estado de origen, evento) identifica una unica fila
// de la tabla de ADR-001.
func TestTransitionKeyUniqueness(t *testing.T) {
	seen := make(map[transitionKey]string)
	for _, tr := range transitions {
		for _, from := range tr.From {
			key := transitionKey{from, tr.Event}
			if prev, dup := seen[key]; dup {
				t.Fatalf("el par (%s, %s) esta declarado por %s y por %s", from, tr.Event, prev, tr.ID)
			}
			seen[key] = tr.ID
		}
	}
}

// TestTerminalStatesHaveNoOutgoingTransition materializa la seccion "Estados
// terminales" de ADR-001 y el punto 6.h de ADR-007.
func TestTerminalStatesHaveNoOutgoingTransition(t *testing.T) {
	for _, tr := range transitions {
		for _, from := range tr.From {
			if IsTerminalState(from) {
				t.Errorf("%s declara salida desde el estado terminal %s", tr.ID, from)
			}
		}
	}

	expected := []State{StateDispensado, StateRobado, StateExtraviado, StateDispuestoFinal}
	for _, s := range expected {
		if !IsTerminalState(s) {
			t.Errorf("%s deberia ser terminal", s)
		}
	}
}

// TestBlockingStatesAreNotTerminal deja fijado que PROHIBIDO, RETIRADO_MERCADO,
// VENCIDO, DETERIORADO, DEVUELTO y EN_CUARENTENA bloquean pero conservan
// transiciones de resolucion. Corrige el error que ADR-007 punto 6.h registra
// de una version anterior, que listaba PROHIBIDO como terminal.
func TestBlockingStatesAreNotTerminal(t *testing.T) {
	blocking := []State{
		StateVencido, StateDeteriorado, StateRetiradoMercado,
		StateProhibido, StateDevuelto, StateEnCuarentena,
	}
	for _, s := range blocking {
		if !IsBlockingState(s) {
			t.Errorf("%s deberia ser bloqueante", s)
		}
		if IsTerminalState(s) {
			t.Errorf("%s no es terminal segun ADR-001", s)
		}

		hasExit := false
		for _, tr := range transitions {
			for _, from := range tr.From {
				if from == s {
					hasExit = true
				}
			}
		}
		if !hasExit {
			t.Errorf("%s es bloqueante no terminal pero no tiene transicion de salida", s)
		}
	}
}

func TestLookupTransition(t *testing.T) {
	cases := []struct {
		from  State
		event Event
		want  string
		ok    bool
	}{
		{StateEnLaboratorio, EventDistribuirEslabonPosterior, "T02_DISPATCH_TRANSFER_FROM_LAB", true},
		{StateEnCustodia, EventDistribuirEslabonPosterior, "T03_DISPATCH_TRANSFER", true},
		{StateEnTransito, EventRecibirEnEstablecimiento, "T04_RECEIVE_TRANSFER", true},
		{StateEnTransito, EventDevolverProducto, "T05_REJECT_OR_RETURN_IN_TRANSIT", true},
		{StateEnCustodia, EventDispensarPaciente, "T06_DISPENSE", true},
		{StateEnCustodia, EventDevolverProducto, "T21_RETURN_FROM_CUSTODY", true},
		{StateDevuelto, EventReingresarStock, "T25_RESTOCK_RETURNED", true},
		// Transiciones que ADR-001 no declara: el chaincode debe rechazarlas.
		{StateEnLaboratorio, EventDispensarPaciente, "", false},
		{StateDispensado, EventDistribuirEslabonPosterior, "", false},
		{StateEnTransito, EventDispensarPaciente, "", false},
		{StateProhibido, EventDispensarPaciente, "", false},
		{StateVencido, EventReingresarStock, "", false},
	}

	for _, tc := range cases {
		tr, ok := LookupTransition(tc.from, tc.event)
		if ok != tc.ok {
			t.Errorf("LookupTransition(%s, %s) ok = %v, se esperaba %v", tc.from, tc.event, ok, tc.ok)
			continue
		}
		if ok && tr.ID != tc.want {
			t.Errorf("LookupTransition(%s, %s) = %s, se esperaba %s", tc.from, tc.event, tr.ID, tc.want)
		}
	}
}

func TestLookupInitialTransition(t *testing.T) {
	tr, ok := LookupInitialTransition(EventRegistrarUnidad)
	if !ok {
		t.Fatal("T01 deberia resolverse como transicion inicial")
	}
	if tr.ID != "T01_REGISTER_UNIT" || tr.To != StateEnLaboratorio {
		t.Fatalf("T01 resolvio a %s -> %s", tr.ID, tr.To)
	}
	if !tr.AllowsActor(ActorLaboratory) {
		t.Error("T01 debe habilitar al actor LABORATORY")
	}
	if _, ok := LookupInitialTransition(EventDispensarPaciente); ok {
		t.Error("DISPENSAR_PACIENTE no es una transicion inicial")
	}
}

// TestIsDeclaredStatePair cubre la comprobacion 4 de ADR-011: recomputar el
// camino de estados observado en el historial contra la tabla de ADR-001.
func TestIsDeclaredStatePair(t *testing.T) {
	valid := [][2]State{
		{StateEnLaboratorio, StateEnTransito},
		{StateEnTransito, StateEnCustodia},
		{StateEnCustodia, StateDispensado},
		{StateEnCuarentena, StateEnCustodia},
		{StateRetiradoMercado, StateEnCustodia},
	}
	for _, pair := range valid {
		if !IsDeclaredStatePair(pair[0], pair[1]) {
			t.Errorf("%s -> %s deberia ser un par declarado", pair[0], pair[1])
		}
	}

	invalid := [][2]State{
		{StateEnLaboratorio, StateDispensado},
		{StateEnLaboratorio, StateEnCustodia},
		{StateDispensado, StateEnCustodia},
		{StateEnTransito, StateDispensado},
	}
	for _, pair := range invalid {
		if IsDeclaredStatePair(pair[0], pair[1]) {
			t.Errorf("%s -> %s no esta declarado por ADR-001", pair[0], pair[1])
		}
	}
}

func TestIsKnownState(t *testing.T) {
	if !IsKnownState(StateEnCustodia) {
		t.Error("EN_CUSTODIA pertenece al catalogo")
	}
	if IsKnownState("ESTADO_INVENTADO") {
		t.Error("un estado fuera del catalogo no debe reconocerse")
	}
	if InitialState != StateEnLaboratorio {
		t.Errorf("el estado inicial de ADR-001 es EN_LABORATORIO, no %s", InitialState)
	}
}
