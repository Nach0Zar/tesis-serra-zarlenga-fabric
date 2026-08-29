package snt

import (
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Verificaciones de legitimidad sobre el historial de una unidad.
//
// Este archivo tiene DOS consumidores y por eso separa lo compartido de lo
// propio: CC-7 (#61) implementa VerifyUnit, la verificacion del adquirente que
// fija ADR-013, y CC-8 (#62) implementara VerifyTrace, la del financiador que
// fija ADR-011. Las dos checklists coinciden en la nocion de "cadena de
// custodia legitima" -- camino de estados valido contra la tabla de ADR-001 y
// pares de transferencia autorizados contra la matriz embebida de ADR-008 --,
// y ADR-013 (Decision, comprobacion 3) obliga a implementarla UNA SOLA VEZ.
//
// El motivo es el mismo por el que ADR-008 impone una matriz unica para
// chaincode y baseline: dos implementaciones de la misma regla divergen en
// silencio, y el dia que divergieran el adquirente y el financiador darian
// veredictos distintos sobre la misma unidad.

// Nombres de las comprobaciones que reporta el campo `verificaciones` de un
// veredicto (ADR-013, "Salida: veredicto estructurado").
const (
	checkExistence     = "EXISTENCIA"
	checkUniqueness    = "UNICIDAD"
	checkCustodyChain  = "CADENA_CUSTODIA"
	checkOperableState = "ESTADO_OPERABLE"
)

// Resultados posibles de una comprobacion individual.
const (
	checkOK           = "OK"
	checkFailed       = "FALLO"
	checkNotEvaluated = "NO_EVALUADO"
)

// Veredictos nombrados. Los dos primeros son propios del adquirente (ADR-013);
// SECUENCIA_INVALIDA y TRANSFERENCIA_NO_AUTORIZADA los COMPARTE con ADR-011 y
// tienen identica semantica en ambas operaciones, porque salen de la misma
// comprobacion.
const (
	verdictNotFound              = "NO_ENCONTRADA"
	verdictDuplicated            = "UNIDAD_DUPLICADA"
	verdictInvalidSequence       = "SECUENCIA_INVALIDA"
	verdictTransferNotAuthorized = "TRANSFERENCIA_NO_AUTORIZADA"
	verdictBlockingState         = "ESTADO_BLOQUEANTE"
	verdictTerminalState         = "ESTADO_TERMINAL"

	// VENCIDO_POR_FECHA tiene veredicto propio y no se reporta como
	// ESTADO_BLOQUEANTE porque exige del adquirente una accion distinta: ante un
	// estado bloqueante el ledger ya registro la causa y hay un proceso en
	// curso; aca el ledger todavia no registro nada, el adquirente esta
	// descubriendo una condicion no informada y corresponde ademas detonar
	// ReportExpired (T13). Reportarlo como ESTADO_BLOQUEANTE afirmaria que el
	// ledger bloquea la unidad cuando el ledger no dice nada de ella.
	verdictExpiredByDate = "VENCIDO_POR_FECHA"
)

// VerifyUnit implementa la verificacion de autenticidad del adquirente
// (ADR-013): cuatro comprobaciones determinísticas evaluadas EN ORDEN sobre el
// estado publico y el historial de la unidad.
//
// No se confunde con VerifyTrace aunque compartan dos comprobaciones. La
// checklist de ADR-011 exige que la unidad este DISPENSADO y devuelve
// NO_DISPENSADA en cualquier otro caso: es correcta para el financiador, cuya
// condicion de pago nace de una dispensa ya ocurrida, e inservible para el
// adquirente, que consulta ANTES de aceptar la custodia -- cuando la unidad
// esta justamente en EN_TRANSITO o EN_CUSTODIA.
//
// Autorizacion: invocador registrado y habilitado, sin exigir agentType ni
// snt.role (ADR-013, "Autorizacion"). El consultante es cualquier eslabon que
// este por adquirir, y restringirlo seria una barrera APARENTE: la misma
// informacion es alcanzable con ReadUnit y GetUnitHistory, que no autorizan en
// absoluto porque ADR-005 declara que la lectura del estado publico no es
// restringible por chaincode.
//
// Confidencialidad: el veredicto se computa exclusivamente sobre el estado
// minimo de trazabilidad, que ADR-002 declara de visibilidad amplia dentro del
// canal. Esta operacion NO lee ninguna coleccion privada -- ni el registro de
// operacion de ADR-004 ni ningun marcador --, de modo que no puede filtrar
// informacion comercial: es una propiedad estructural, no una promesa.
//
// La inexistencia de la unidad NO es un error sino el veredicto NO_ENCONTRADA:
// para quien consulta antes de adquirir es una respuesta legitima -- y la mas
// importante --, no una falla de invocacion.
func (c *SNTContract) VerifyUnit(
	ctx contractapi.TransactionContextInterface,
	gtin string,
	numeroSerie string,
) (*UnitVerdict, error) {
	if _, err := resolveInvoker(ctx); err != nil {
		return nil, err
	}
	if err := validateUnitRef(gtin, numeroSerie); err != nil {
		return nil, err
	}

	verdict := &UnitVerdict{
		Verificaciones: []TraceCheck{
			{Check: checkExistence, Resultado: checkNotEvaluated},
			{Check: checkUniqueness, Resultado: checkNotEvaluated},
			{Check: checkCustodyChain, Resultado: checkNotEvaluated},
			{Check: checkOperableState, Resultado: checkNotEvaluated},
		},
	}

	// 1. Existencia. Responde "¿es original?" en el unico sentido que el ledger
	// puede acreditar: el serial fue dado de alta por un laboratorio via T01 y
	// no es un codigo inventado.
	unit, err := readUnit(ctx, gtin, numeroSerie)
	if err != nil {
		if code, ok := cerr.CodeOf(err); ok && code == cerr.UnitNotFound {
			verdict.fail(0, verdictNotFound, "la unidad no existe en el estado publico")
			return verdict, nil
		}
		return nil, err
	}
	verdict.Estado = unit.Estado
	verdict.pass(0, "")

	history, err := readUnitHistory(ctx, gtin, numeroSerie)
	if err != nil {
		return nil, err
	}

	// 2. Unicidad, recomputada desde el historial. Es redundante con dos
	// invariantes ya vigentes -- la clave compuesta es unica y RegisterUnit
	// rechaza el alta repetida con UNIT_ALREADY_EXISTS -- y se conserva igual,
	// porque una comprobacion recomputada es EVIDENCIA de la invariante y no
	// una afirmacion sobre ella (ADR-013, comprobacion 2).
	//
	// Una segunda creacion de la clave exige un borrado previo: PutState sobre
	// una clave existente es una actualizacion, no un alta. Por eso basta con
	// que el historial no registre ninguna eliminacion.
	if entry, deleted := firstDeletion(history); deleted {
		verdict.fail(1, verdictDuplicated,
			"el historial registra una eliminacion de la clave en la transaccion "+entry.TxID)
		return verdict, nil
	}
	verdict.pass(1, "")

	// 3. Cadena de custodia legitima: comprobaciones 4 y 5 de ADR-011,
	// compartidas con VerifyTrace.
	chain, err := verifyCustodyChain(ctx, history)
	if err != nil {
		return nil, err
	}
	if !chain.OK {
		verdict.fail(2, chain.Verdict, chain.Detail)
		return verdict, nil
	}
	verdict.pass(2, "")

	// 4. Aptitud para operar. Es la ultima porque es la unica que puede fallar
	// sobre una unidad de traza impecable, que es el caso mas frecuente del uso
	// real: producto legitimo, retirado del mercado o vencido.
	//
	// Bloqueante y terminal son veredictos distintos y no es cosmetico: un
	// estado bloqueante puede resolverse -- ADR-001 les conserva transiciones de
	// salida -- y uno terminal no. Para el adquirente son dos decisiones
	// distintas: rechazar a la espera de una resolucion, o rechazar en firme.
	//
	// El vencimiento POR FECHA es una tercera condicion y no un caso del estado
	// bloqueante. El paso del tiempo no ejecuta transacciones: VENCIDO se
	// alcanza por T11/T12/T13, que alguien tiene que invocar, y hasta entonces
	// una unidad cuya fecha ya paso sigue registrada como EN_CUSTODIA o
	// EN_TRANSITO. Mirar solo el estado la declararia apta, que es el peor
	// resultado posible de esta operacion: decirle a quien esta por adquirir que
	// un producto vencido es apto, con la fecha que lo desmiente en el mismo
	// estado publico que la verificacion ya esta leyendo (ADR-013,
	// comprobacion 4; ADR-002 lo enuncia como el caso testigo de la visibilidad
	// del estado minimo, y ADR-001 lo trata como condicion independiente en la
	// precondicion de T06).
	switch {
	case domain.IsTerminalState(unit.Estado):
		verdict.fail(3, verdictTerminalState, string(unit.Estado))
	case domain.IsBlockingState(unit.Estado):
		verdict.fail(3, verdictBlockingState, string(unit.Estado))
	default:
		expired, err := unitExpiredByDate(ctx, unit)
		if err != nil {
			return nil, err
		}
		if expired {
			verdict.fail(3, verdictExpiredByDate,
				"la fecha de vencimiento "+unit.FechaVencimiento+
					" ya paso y el evento INFORMAR_VENCIMIENTO todavia no se registro")
			return verdict, nil
		}
		verdict.pass(3, string(unit.Estado))
		verdict.Autentica = true
	}
	return verdict, nil
}

// unitExpiredByDate informa si la fecha de vencimiento de la unidad ya paso al
// momento de la transaccion.
//
// `fechaVencimiento` es una fecha YYYY-MM-DD (modelo-datos.md §3.2) y se
// interpreta como el ULTIMO DIA OPERABLE, conforme el uso corriente de la fecha
// de vencimiento de un medicamento: con fechaVencimiento 2026-08-28, una
// consulta del 28 la considera apta y una del 29 vencida.
//
// El instante sale SIEMPRE de GetTxTimestamp() y nunca del reloj local: el
// reloj local da un valor distinto en cada peer endosante y, ademas de romper
// el determinismo, volveria el veredicto irreproducible para un auditor. Es el
// mismo criterio que ADR-007 punto 6.f fija para el vencimiento de las
// autorizaciones de intervencion.
func unitExpiredByDate(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
) (bool, error) {
	if unit.FechaVencimiento == "" {
		// RegisterUnit la exige, de modo que solo un estado corrupto llegaria
		// aca. No se inventa un veredicto por eso: la unidad no esta vencida
		// segun un dato que no existe.
		return false, nil
	}
	expiry, err := time.Parse(expirationDateForm, unit.FechaVencimiento)
	if err != nil {
		return false, cerr.Internal(err, "la fecha de vencimiento persistida no es una fecha valida")
	}
	now, err := txTime(ctx)
	if err != nil {
		return false, err
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.After(expiry), nil
}

// pass y fail mantienen la invariante de la forma del veredicto: `motivo` es el
// veredicto nombrado de la PRIMERA comprobacion que falla, y las posteriores
// quedan en NO_EVALUADO. Concentrarlo aca evita que cada rama la reconstruya.
func (v *UnitVerdict) pass(index int, detalle string) {
	v.Verificaciones[index].Resultado = checkOK
	v.Verificaciones[index].Detalle = detalle
}

func (v *UnitVerdict) fail(index int, motivo, detalle string) {
	v.Verificaciones[index].Resultado = checkFailed
	v.Verificaciones[index].Detalle = detalle
	v.Motivo = motivo
	v.Autentica = false
}

// firstDeletion devuelve la primera entrada de borrado del historial, si la hay.
func firstDeletion(history []UnitHistoryEntry) (UnitHistoryEntry, bool) {
	for _, entry := range history {
		if entry.IsDelete {
			return entry, true
		}
	}
	return UnitHistoryEntry{}, false
}

// custodyChainResult es el resultado de la comprobacion de cadena de custodia.
// Verdict lleva el veredicto nombrado que corresponda cuando OK es false.
type custodyChainResult struct {
	OK      bool
	Verdict string
	Detail  string
}

// verifyCustodyChain materializa las comprobaciones 4 y 5 de ADR-011 sobre el
// historial de una unidad, y es el helper COMPARTIDO que ADR-013 obliga a tener:
// lo consume VerifyUnit (CC-7) y debe consumirlo VerifyTrace (CC-8) en lugar de
// reimplementarlo.
//
//   - Camino de estados: la secuencia observada arranca en el estado inicial de
//     ADR-001 y cada par consecutivo es una transicion declarada en su tabla.
//   - Pares de transferencia: cada cambio de CustodioActual corresponde a un par
//     (agentType origen -> agentType destino) autorizado por la matriz embebida
//     de ADR-008, resolviendo el agentType de cada custodio contra el registro.
//
// Las dos se recomputan desde el historial en lugar de confiar en que el
// chaincode las valido al escribir: eso es lo que las vuelve verificables por un
// tercero, que es el punto entero de la operacion.
//
// Un custodio que no resuelve contra el registro NO se convierte en veredicto de
// negocio: se propaga como ORG_NOT_REGISTERED. El registro no borra entradas --
// SetOrganizationActive solo cambia `active` --, de modo que esa situacion no es
// alcanzable por ningun camino soportado y darle un veredicto seria simular que
// se contemplo una corrupcion del estado.
func verifyCustodyChain(
	ctx contractapi.TransactionContextInterface,
	history []UnitHistoryEntry,
) (custodyChainResult, error) {
	agentTypes, err := agentTypeByCanonicalID(ctx)
	if err != nil {
		return custodyChainResult{}, err
	}

	var previous *MedicationUnit
	for _, entry := range history {
		if entry.Value == nil {
			continue
		}
		current := entry.Value

		if previous == nil {
			result, err := verifyChainOrigin(*current, agentTypes)
			if err != nil || !result.OK {
				return result, err
			}
			previous = current
			continue
		}

		// Invariante 3: TODA escritura de la clave debe corresponder a una
		// transicion declarada, incluidas las que dejan el estado igual. ADR-001
		// no declara transiciones sobre si mismas, de modo que una segunda
		// escritura con el mismo estado no corresponde a ninguna transicion.
		// Saltearlas -- como hacia la version anterior de esta funcion -- deja
		// pasar una transferencia consumada sin transito: EN_CUSTODIA/A ->
		// EN_CUSTODIA/B ni siquiera llegaba a examinarse.
		if !domain.IsDeclaredStatePair(previous.Estado, current.Estado) {
			return custodyChainResult{
				OK:      false,
				Verdict: verdictInvalidSequence,
				Detail:  string(previous.Estado) + " -> " + string(current.Estado),
			}, nil
		}

		result, err := verifyCustodyHandover(*previous, *current, agentTypes)
		if err != nil || !result.OK {
			return result, err
		}
		previous = current
	}

	return custodyChainResult{OK: true}, nil
}

// verifyChainOrigin comprueba las dos invariantes del primer snapshot: la
// unidad nace en EN_LABORATORIO y su primer custodio es un LABORATORY.
//
// La segunda no es redundante con la primera: T01 habilita unicamente al actor
// LABORATORY y RegisterUnit persiste al laboratorio invocante como custodio, de
// modo que un historial que arrancara en EN_LABORATORIO bajo custodia de una
// farmacia describe un alta que ADR-001 no habilita.
func verifyChainOrigin(
	first MedicationUnit,
	agentTypes map[string]domain.AgentType,
) (custodyChainResult, error) {
	if first.Estado != domain.InitialState {
		return custodyChainResult{
			OK:      false,
			Verdict: verdictInvalidSequence,
			Detail: "el historial arranca en " + string(first.Estado) +
				" y el unico estado inicial de ADR-001 es " + string(domain.InitialState),
		}, nil
	}
	agentType, ok := agentTypes[first.CustodioActual]
	if !ok {
		return custodyChainResult{}, unregisteredCustodian(first.CustodioActual)
	}
	if agentType != domain.AgentLaboratory {
		return custodyChainResult{
			OK:      false,
			Verdict: verdictInvalidSequence,
			Detail: "el primer custodio es " + string(agentType) +
				" y T01 solo habilita a LABORATORY",
		}, nil
	}
	return custodyChainResult{OK: true}, nil
}

// verifyCustodyHandover comprueba el ACOPLAMIENTO entre estado y custodia que
// fija ADR-004, y que ni ADR-001 ni la matriz de ADR-008 expresan por separado.
//
// La regla es una equivalencia, no dos condiciones sueltas: `CustodioActual`
// cambia si y solo si la transicion observada es EN_TRANSITO -> EN_CUSTODIA
// (T04). El despacho (T02/T03) lleva la unidad a EN_TRANSITO SIN mover la
// custodia, y ninguna otra transicion de ADR-001 la mueve: ADR-009 (punto 1) lo
// confirma para las cuatro vias hacia DEVUELTO y descarta expresamente la
// alternativa que la cambiaba, porque "viola el principio establecido por
// ADR-004 de que ningun cambio de custodia se asienta sin un acto propio del
// receptor".
//
// Verificar las dos proyecciones por separado -- estados validos por un lado,
// pares autorizados por otro -- deja pasar historiales que violan el
// acoplamiento aunque ambas proyecciones sean validas. El caso testigo es
// EN_LABORATORIO/laboratorio -> EN_TRANSITO/drogueria: T02 es una transicion
// declarada y LABORATORY -> DRUGSTORE esta autorizado, pero durante el transito
// la custodia registrada todavia es la del laboratorio.
func verifyCustodyHandover(
	previous, current MedicationUnit,
	agentTypes map[string]domain.AgentType,
) (custodyChainResult, error) {
	changed := current.CustodioActual != previous.CustodioActual
	isReception := previous.Estado == domain.StateEnTransito &&
		current.Estado == domain.StateEnCustodia

	switch {
	case changed && !isReception:
		return custodyChainResult{
			OK:      false,
			Verdict: verdictInvalidSequence,
			Detail: "la custodia cambio en " + string(previous.Estado) + " -> " +
				string(current.Estado) + ", y solo la recepcion (T04) la mueve",
		}, nil
	case !changed && isReception:
		// La contracara: DispatchTransfer rechaza que el destino sea la propia
		// organizacion emisora, de modo que una recepcion siempre mueve la
		// custodia. Una que no la mueve no es una recepcion.
		return custodyChainResult{
			OK:      false,
			Verdict: verdictInvalidSequence,
			Detail:  "EN_TRANSITO -> EN_CUSTODIA sin cambio de custodio",
		}, nil
	case !changed:
		return custodyChainResult{OK: true}, nil
	}

	origin, ok := agentTypes[previous.CustodioActual]
	if !ok {
		return custodyChainResult{}, unregisteredCustodian(previous.CustodioActual)
	}
	destination, ok := agentTypes[current.CustodioActual]
	if !ok {
		return custodyChainResult{}, unregisteredCustodian(current.CustodioActual)
	}
	decision, err := domain.DecideTransfer(origin, destination)
	if err != nil {
		return custodyChainResult{}, cerr.Internal(err, "no se pudo evaluar la matriz de transferencias")
	}
	if !decision.Allowed {
		return custodyChainResult{
			OK:      false,
			Verdict: verdictTransferNotAuthorized,
			Detail:  string(origin) + " -> " + string(destination),
		}, nil
	}
	return custodyChainResult{OK: true}, nil
}

// agentTypeByCanonicalID indexa el registro por identificador canonico, que es
// como el estado publico de una unidad nombra a su custodio (ADR-003, punto 4:
// nunca se persiste el mspId). Se arma una sola vez por verificacion en lugar de
// recorrer el registro por cada cambio de custodio del historial.
func agentTypeByCanonicalID(
	ctx contractapi.TransactionContextInterface,
) (map[string]domain.AgentType, error) {
	orgs, err := listOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]domain.AgentType, len(orgs))
	for _, org := range orgs {
		index[org.CanonicalID()] = org.AgentType
	}
	return index, nil
}

func unregisteredCustodian(canonicalID string) error {
	return cerr.New(cerr.OrgNotRegistered,
		"el custodio %s del historial no tiene entrada en el registro organizacion-establecimiento",
		canonicalID).
		WithDetails(map[string]any{"custodio": canonicalID})
}
