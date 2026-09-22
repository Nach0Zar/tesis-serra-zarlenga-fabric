package domain

// EventActorFacts son los hechos que cada SUT resuelve desde su propio estado.
// No contienen identidad, credenciales ni datos de almacenamiento.
type EventActorFacts struct {
	Regulator           bool
	Laboratory          bool
	CurrentCustodian    bool
	DeclaredDestination bool
}

// EventAdmitsActor indica si alguna fila de ADR-001 habilita al actor para el
// evento. Esta pregunta es distinta de la habilitacion en el estado observado.
func EventAdmitsActor(event Event, actor Actor) bool {
	for _, transition := range transitions {
		if transition.Event == event && transition.AllowsActor(actor) {
			return true
		}
	}
	return false
}

// EventActorCharacters enumera los caracteres que el invocador reune sobre
// esta unidad, en orden de especificidad. El custodio conserva ambos caracteres
// de ADR-009 punto 3 incluso si este evento no los admite.
func EventActorCharacters(state State, event Event, facts EventActorFacts) []Actor {
	if facts.Regulator {
		return []Actor{ActorANMAT}
	}
	characters := []Actor{}
	if facts.Laboratory && EventAdmitsActor(event, ActorLaboratory) {
		characters = append(characters, ActorLaboratory)
	}
	if facts.CurrentCustodian {
		characters = append(characters, ActorCurrentCustodian, ActorRecoveryOrDisposalAgent)
	}
	if state == StateEnTransito && facts.DeclaredDestination && EventAdmitsActor(event, ActorDestinationAgent) {
		characters = append(characters, ActorDestinationAgent)
	}
	return characters
}

// SelectEventActor elige el primer caracter habilitado por la fila concreta.
// Si no hay fila o ninguno procede, devuelve el primer caracter para que el
// consumidor emita INVALID_STATE_TRANSITION. El segundo valor es false solo
// cuando el invocador no reune ningun caracter (UNAUTHORIZED_CUSTODIAN).
func SelectEventActor(state State, event Event, characters []Actor) (Actor, bool) {
	if len(characters) == 0 {
		return "", false
	}
	if transition, declared := LookupTransition(state, event); declared {
		for _, actor := range characters {
			if transition.AllowsActor(actor) {
				return actor, true
			}
		}
	}
	return characters[0], true
}
