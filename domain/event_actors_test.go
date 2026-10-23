package domain

import (
	"reflect"
	"testing"
)

func TestEventActorCharactersAndSelection(t *testing.T) {
	tests := []struct {
		name       string
		state      State
		event      Event
		facts      EventActorFacts
		characters []Actor
		selected   Actor
		related    bool
	}{
		{
			name:  "laboratorio ajeno no puede ponerse en cuarentena",
			state: StateEnCustodia, event: EventPonerEnCuarentena,
			facts: EventActorFacts{Laboratory: true}, characters: []Actor{},
		},
		{
			name:  "laboratorio ajeno puede retirar",
			state: StateEnCustodia, event: EventRetirarMercado,
			facts:      EventActorFacts{Laboratory: true},
			characters: []Actor{ActorLaboratory}, selected: ActorLaboratory, related: true,
		},
		{
			name:  "laboratorio custodio elige laboratorio en T17",
			state: StateEnCustodia, event: EventRetirarMercado,
			facts:      EventActorFacts{Laboratory: true, CurrentCustodian: true},
			characters: []Actor{ActorLaboratory, ActorCurrentCustodian, ActorRecoveryOrDisposalAgent},
			selected:   ActorLaboratory, related: true,
		},
		{
			name:  "laboratorio custodio elige agente de recupero en T25",
			state: StateDevuelto, event: EventReingresarStock,
			facts:      EventActorFacts{Laboratory: true, CurrentCustodian: true},
			characters: []Actor{ActorLaboratory, ActorCurrentCustodian, ActorRecoveryOrDisposalAgent},
			selected:   ActorRecoveryOrDisposalAgent, related: true,
		},
		{
			name:  "destinatario declarado en T09",
			state: StateEnTransito, event: EventPonerEnCuarentena,
			facts:      EventActorFacts{DeclaredDestination: true},
			characters: []Actor{ActorDestinationAgent}, selected: ActorDestinationAgent, related: true,
		},
		{
			name:  "destinatario no actua fuera del transito",
			state: StateEnCustodia, event: EventPonerEnCuarentena,
			facts: EventActorFacts{DeclaredDestination: true}, characters: []Actor{},
		},
		{
			name:  "destinatario no participa en T14",
			state: StateEnTransito, event: EventInformarRobo,
			facts: EventActorFacts{DeclaredDestination: true}, characters: []Actor{},
		},
		{
			name:  "ANMAT no acumula caracteres",
			state: StateEnCustodia, event: EventRetirarMercado,
			facts:      EventActorFacts{Regulator: true, Laboratory: true, CurrentCustodian: true},
			characters: []Actor{ActorANMAT}, selected: ActorANMAT, related: true,
		},
		{
			name:  "custodio conserva relacion aunque no este habilitado",
			state: StateEnCustodia, event: EventRetirarMercado,
			facts:      EventActorFacts{CurrentCustodian: true},
			characters: []Actor{ActorCurrentCustodian, ActorRecoveryOrDisposalAgent},
			selected:   ActorCurrentCustodian, related: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			characters := EventActorCharacters(tt.state, tt.event, tt.facts)
			if !reflect.DeepEqual(characters, tt.characters) {
				t.Fatalf("characters = %v, want %v", characters, tt.characters)
			}
			actor, related := SelectEventActor(tt.state, tt.event, characters)
			if actor != tt.selected || related != tt.related {
				t.Fatalf("actor, related = %q, %t; want %q, %t", actor, related, tt.selected, tt.related)
			}
		})
	}
}
