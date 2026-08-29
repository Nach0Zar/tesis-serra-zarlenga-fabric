package snt

import (
	"errors"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de VerifyUnit: la verificacion de autenticidad del adquirente que fija
// ADR-013 e implementa CC-7 (#61).

// verifyFixture deja una unidad recorrida por la cadena completa
// laboratorio -> drogueria, que es el estado desde el que el adquirente
// consulta en el caso de uso de la issue: la drogueria ya la recibio y la
// farmacia esta por adquirirla.
func verifyFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub, contract := transferFixture(t)

	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-recepcion"
	_, err = contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	return stub, contract
}

// checkByName evita que los tests dependan del indice de una comprobacion
// dentro del arreglo: si manana se agregara una, los asserts seguirian siendo
// sobre la comprobacion que dicen ser.
func checkByName(t *testing.T, verdict *UnitVerdict, name string) TraceCheck {
	t.Helper()
	for _, check := range verdict.Verificaciones {
		if check.Check == name {
			return check
		}
	}
	t.Fatalf("el veredicto no reporta la comprobacion %s: %+v", name, verdict.Verificaciones)
	return TraceCheck{}
}

// requireVerdict comprueba el veredicto completo: el booleano, el motivo y que
// la comprobacion que se dice fallada sea la que efectivamente figura como
// FALLO. Sin lo ultimo, un motivo correcto podria convivir con una checklist
// que reporta otra cosa.
func requireVerdict(t *testing.T, verdict *UnitVerdict, autentica bool, motivo, failedCheck string) {
	t.Helper()
	if verdict.Autentica != autentica {
		t.Fatalf("autentica = %v, se esperaba %v (motivo: %q)", verdict.Autentica, autentica, verdict.Motivo)
	}
	if verdict.Motivo != motivo {
		t.Fatalf("motivo = %q, se esperaba %q", verdict.Motivo, motivo)
	}
	if failedCheck == "" {
		return
	}
	if got := checkByName(t, verdict, failedCheck); got.Resultado != checkFailed {
		t.Fatalf("la comprobacion %s deberia figurar como %s y figura como %s",
			failedCheck, checkFailed, got.Resultado)
	}
}

// TestVerifyUnitHappyPath es el caso de uso que exige CC-7: la drogueria
// verifica una unidad antes de aceptar la recepcion y obtiene un veredicto
// afirmativo con el estado observado.
func TestVerifyUnitHappyPath(t *testing.T) {
	stub, contract := verifyFixture(t)

	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	requireVerdict(t, verdict, true, "", "")

	if verdict.Estado != domain.StateEnCustodia {
		t.Fatalf("estado = %s, se esperaba EN_CUSTODIA", verdict.Estado)
	}
	if len(verdict.Verificaciones) != 4 {
		t.Fatalf("la checklist de ADR-013 tiene cuatro comprobaciones y el veredicto reporta %d",
			len(verdict.Verificaciones))
	}
	for _, check := range verdict.Verificaciones {
		if check.Resultado != checkOK {
			t.Fatalf("con veredicto afirmativo toda comprobacion debe ser %s: %+v", checkOK, check)
		}
	}
}

// TestVerifyUnitOnUnitInTransit cubre el momento REAL en que el adquirente
// consulta: la unidad viaja hacia el y todavia no la recibio.
//
// Es la diferencia sustantiva con VerifyTrace, y por eso tiene test propio: la
// checklist de ADR-011 exige estado DISPENSADO y responderia NO_DISPENSADA
// justamente aca, en el 100 % de las consultas legitimas del adquirente
// (ADR-013, alternativa B).
func TestVerifyUnitOnUnitInTransit(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	verdict, err := contract.VerifyUnit(
		testContext(stub, drogueriaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	requireVerdict(t, verdict, true, "", "")

	if verdict.Estado != domain.StateEnTransito {
		t.Fatalf("estado = %s, se esperaba EN_TRANSITO", verdict.Estado)
	}
}

// TestVerifyUnitNotFound: la inexistencia es un VEREDICTO, no un error de
// invocacion. Para quien consulta antes de adquirir es la respuesta mas
// importante que la operacion puede dar, y devolverla como UNIT_NOT_FOUND la
// haria indistinguible de una falla de la consulta.
func TestVerifyUnitNotFound(t *testing.T) {
	stub, contract := verifyFixture(t)

	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, "SERIE-INEXISTENTE")
	requireNoError(t, err)
	requireVerdict(t, verdict, false, verdictNotFound, checkExistence)

	if verdict.Estado != "" {
		t.Fatalf("una unidad inexistente no tiene estado observable: %q", verdict.Estado)
	}
	// Las comprobaciones posteriores no se evaluaron y deben decirlo.
	for _, name := range []string{checkUniqueness, checkCustodyChain, checkOperableState} {
		if got := checkByName(t, verdict, name); got.Resultado != checkNotEvaluated {
			t.Fatalf("%s deberia quedar en %s y quedo en %s", name, checkNotEvaluated, got.Resultado)
		}
	}
}

// TestVerifyUnitDetectsRecreatedKey cubre la comprobacion 2. Una segunda
// creacion de la clave exige un borrado previo: PutState sobre una clave
// existente es una actualizacion, no un alta.
func TestVerifyUnitDetectsRecreatedKey(t *testing.T) {
	stub, contract := verifyFixture(t)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	stub.txID = "tx-borrado"
	requireNoError(t, stub.DelState(key))
	stub.txID = "tx-recreacion"
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+drogueriaGLN)

	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	requireVerdict(t, verdict, false, verdictDuplicated, checkUniqueness)
}

// TestVerifyUnitDetectsInvalidSequence cubre la mitad "camino de estados" de la
// comprobacion 3, que ADR-013 comparte con la comprobacion 4 de ADR-011.
//
// El historial se fabrica salteando la maquina de estados a proposito: es la
// unica forma de producir una secuencia que el chaincode nunca escribiria, y es
// justamente lo que la comprobacion existe para detectar en un ledger que un
// tercero audita.
func TestVerifyUnitDetectsInvalidSequence(t *testing.T) {
	t.Run("salto no declarado por ADR-001", func(t *testing.T) {
		stub, contract := transferFixture(t)
		// EN_LABORATORIO -> EN_CUSTODIA no es una transicion de ADR-001: la
		// custodia solo se alcanza recibiendo una transferencia.
		stub.txID = "tx-salto"
		seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+labGLN)

		verdict, err := contract.VerifyUnit(
			testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
		requireNoError(t, err)
		requireVerdict(t, verdict, false, verdictInvalidSequence, checkCustodyChain)

		if got := checkByName(t, verdict, checkCustodyChain); got.Detalle == "" {
			t.Fatal("el detalle debe nombrar el par de estados que rompio la secuencia")
		}
	})

	t.Run("historial que no arranca en el estado inicial", func(t *testing.T) {
		stub := newMockStub()
		seedRegistry(t, stub)
		registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
		registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
		contract := new(SNTContract)

		// La unidad aparece directamente en EN_CUSTODIA, sin haber existido
		// nunca en EN_LABORATORIO.
		seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)

		verdict, err := contract.VerifyUnit(
			testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
		requireNoError(t, err)
		requireVerdict(t, verdict, false, verdictInvalidSequence, checkCustodyChain)
	})
}

// TestVerifyUnitDetectsUnauthorizedTransfer cubre la mitad "pares autorizados"
// de la comprobacion 3, que ADR-013 comparte con la comprobacion 5 de ADR-011.
//
// El par FARMACIA -> DROGUERIA esta explicitamente prohibido por la matriz de
// DES-3 (venta hacia un eslabon superior). El historial se fabrica con ese
// cambio de custodio para comprobar que la re-evaluacion contra la matriz lo
// detecta aunque el estado haya seguido un camino valido.
func TestVerifyUnitDetectsUnauthorizedTransfer(t *testing.T) {
	stub, contract := transferFixture(t)

	// EN_LABORATORIO (lab) -> EN_TRANSITO (lab) -> EN_CUSTODIA (farmacia):
	// camino de estados valido.
	stub.txID = "tx-transito"
	seedUnit(t, stub, domain.StateEnTransito, "GLN:"+labGLN)
	stub.txID = "tx-custodia-farmacia"
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
	// Y ahora la custodia salta a una drogueria: FARMACIA -> DRUGSTORE.
	stub.txID = "tx-transito-2"
	seedUnit(t, stub, domain.StateEnTransito, "GLN:"+farmaciaGLN)
	stub.txID = "tx-custodia-drogueria"
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+drogueriaGLN)

	verdict, err := contract.VerifyUnit(
		testContext(stub, drogueriaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	requireVerdict(t, verdict, false, verdictTransferNotAuthorized, checkCustodyChain)

	if got := checkByName(t, verdict, checkCustodyChain); got.Detalle != "PHARMACY -> DRUGSTORE" {
		t.Fatalf("detalle = %q, se esperaba el par observado", got.Detalle)
	}
}

// TestVerifyUnitRejectsBlockingAndTerminalStates cubre la comprobacion 4 sobre
// TODOS los estados no operables de ADR-001, y verifica la distincion que
// ADR-013 exige entre bloqueante y terminal: el primero puede resolverse, el
// segundo no, y para el adquirente son dos decisiones distintas.
func TestVerifyUnitRejectsBlockingAndTerminalStates(t *testing.T) {
	blocking := []domain.State{
		domain.StateVencido, domain.StateDeteriorado, domain.StateRetiradoMercado,
		domain.StateProhibido, domain.StateDevuelto, domain.StateEnCuarentena,
	}
	terminal := []domain.State{
		domain.StateDispensado, domain.StateRobado,
		domain.StateExtraviado, domain.StateDispuestoFinal,
	}

	// DISPUESTO_FINAL no es alcanzable directamente desde EN_CUSTODIA: ADR-001
	// solo lo declara como salida de PROHIBIDO (T32) y de otros bloqueantes. Se
	// lo alcanza por su camino real en lugar de saltear el caso, porque un
	// estado terminal sin cubrir es justamente el que un adquirente no debe
	// aceptar nunca.
	paths := map[domain.State][]domain.State{
		domain.StateDispuestoFinal: {domain.StateProhibido},
	}

	for _, tc := range []struct {
		states []domain.State
		want   string
	}{
		{blocking, verdictBlockingState},
		{terminal, verdictTerminalState},
	} {
		for _, state := range tc.states {
			t.Run(string(state), func(t *testing.T) {
				stub, contract := verifyFixture(t)
				// El camino se siembra respetando la tabla de ADR-001: si no lo
				// respetara, la comprobacion que fallaria seria la 3 (cadena de
				// custodia) y el caso probaria otra cosa que la que dice.
				previous := domain.StateEnCustodia
				for _, step := range append(append([]domain.State(nil), paths[state]...), state) {
					if !domain.IsDeclaredStatePair(previous, step) {
						t.Fatalf("la premisa del caso no se sostiene: ADR-001 no declara %s -> %s",
							previous, step)
					}
					stub.txID = "tx-" + string(step)
					seedUnit(t, stub, step, "GLN:"+drogueriaGLN)
					previous = step
				}

				verdict, err := contract.VerifyUnit(
					testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
				requireNoError(t, err)
				requireVerdict(t, verdict, false, tc.want, checkOperableState)

				if verdict.Estado != state {
					t.Fatalf("estado = %s, se esperaba %s", verdict.Estado, state)
				}
				if got := checkByName(t, verdict, checkOperableState); got.Detalle != string(state) {
					t.Fatalf("detalle = %q, se esperaba el estado observado", got.Detalle)
				}
				// La cadena de custodia es impecable: lo unico que falla es la
				// aptitud del estado actual.
				if got := checkByName(t, verdict, checkCustodyChain); got.Resultado != checkOK {
					t.Fatalf("la cadena deberia ser legitima y figura como %s", got.Resultado)
				}
			})
		}
	}
}

// TestVerifyUnitEvaluationOrder deja fijado el orden que ADR-013 declara. No es
// cosmetico: sobre una unidad que falla DOS comprobaciones, el motivo debe ser
// el de la PRIMERA en el orden declarado, porque es la que el adquirente tiene
// que resolver antes de mirar cualquier otra cosa.
func TestVerifyUnitEvaluationOrder(t *testing.T) {
	stub, contract := verifyFixture(t)

	// La unidad pasa a un estado terminal Y ademas se le rompe la unicidad.
	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	stub.txID = "tx-borrado"
	requireNoError(t, stub.DelState(key))
	stub.txID = "tx-recreacion"
	seedUnit(t, stub, domain.StateDispensado, "GLN:"+drogueriaGLN)

	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)

	// UNICIDAD precede a ESTADO_OPERABLE en la checklist.
	requireVerdict(t, verdict, false, verdictDuplicated, checkUniqueness)
	if got := checkByName(t, verdict, checkOperableState); got.Resultado != checkNotEvaluated {
		t.Fatalf("una comprobacion posterior a la que fallo debe quedar en %s y quedo en %s",
			checkNotEvaluated, got.Resultado)
	}
}

// TestVerifyUnitAuthorization fija las dos mitades de la decision de ADR-013:
// se exige registro y habilitacion, y NO se exige agentType ni rol.
func TestVerifyUnitAuthorization(t *testing.T) {
	t.Run("organizacion no registrada", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.VerifyUnit(
			testContext(stub, "OrgFantasmaMSP", RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.OrgNotRegistered)
	})

	t.Run("organizacion inhabilitada", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.SetOrganizationActive(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			SetOrganizationActiveRequest{MSPID: farmaciaMSP, Active: false})
		requireNoError(t, err)

		_, err = contract.VerifyUnit(
			testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.OrgInactive)
	})

	// La contracara, y el punto de la decision: la consulta NO se restringe por
	// agentType ni por rol. Restringirla seria una barrera aparente, porque la
	// misma informacion es alcanzable con ReadUnit y GetUnitHistory, que no
	// autorizan en absoluto (ADR-005).
	t.Run("habilitada para cualquier agentType y rol registrados", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			mspID string
			role  string
		}{
			{"laboratorio con rol operador", labMSP, RoleOperator},
			{"drogueria con rol operador", drogueriaMSP, RoleOperator},
			{"farmacia con rol auditor", farmaciaMSP, RoleAuditor},
			{"organizacion regulatoria", anmatMSP, RoleRegulatoryAdmin},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stub, contract := verifyFixture(t)
				verdict, err := contract.VerifyUnit(
					testContext(stub, tc.mspID, tc.role), validGTIN, validSerial)
				requireNoError(t, err)
				requireVerdict(t, verdict, true, "", "")
			})
		}
	})
}

func TestVerifyUnitRejectsInvalidUnitRef(t *testing.T) {
	cases := map[string][2]string{
		"gtin vacio":            {"", validSerial},
		"serie vacia":           {validGTIN, ""},
		"gtin con digito malo":  {"07791234567890", validSerial},
		"gtin de largo erroneo": {"123", validSerial},
	}
	for name, ref := range cases {
		t.Run(name, func(t *testing.T) {
			stub, contract := verifyFixture(t)
			_, err := contract.VerifyUnit(
				testContext(stub, farmaciaMSP, RoleOperator), ref[0], ref[1])
			requireCode(t, err, cerr.InvalidRequest)
		})
	}
}

// TestVerifyUnitReadsNoPrivateData es la comprobacion estructural de la
// propiedad de confidencialidad que ADR-013 declara: el veredicto se computa
// solo sobre el estado minimo de trazabilidad de ADR-002, y la operacion no
// consulta ninguna coleccion privada.
//
// No se prueba leyendo el codigo sino inyectando una falla en TODA lectura
// privada: si VerifyUnit tocara una coleccion, fallaria. Es la diferencia entre
// afirmar la propiedad y verificarla.
func TestVerifyUnitReadsNoPrivateData(t *testing.T) {
	stub, contract := verifyFixture(t)
	boom := errors.New("lectura de coleccion privada no esperada")
	stub.failOn("GetPrivateData", boom)
	stub.failOn("GetPrivateDataHash", boom)

	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	requireVerdict(t, verdict, true, "", "")
}

// TestVerifyUnitPlatformFailures verifica que las fallas de plataforma se
// tipifiquen como INTERNAL_ERROR y no se disfracen de veredicto: un veredicto
// negativo afirma algo sobre la unidad, y afirmarlo porque el ledger no
// respondio seria mentir sobre el producto.
func TestVerifyUnitPlatformFailures(t *testing.T) {
	for _, method := range []string{"GetState", "GetHistoryForKey"} {
		t.Run(method, func(t *testing.T) {
			stub, contract := verifyFixture(t)
			stub.failOn(method, errors.New("fallo simulado de la plataforma"))
			_, err := contract.VerifyUnit(
				testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
			requireCode(t, err, cerr.InternalError)
		})
	}
}

// TestVerifyCustodyChainIsSharedWithVerifyTrace deja constancia ejecutable de la
// obligacion que ADR-013 le impone a CC-8 (#62): las comprobaciones 4 y 5 de
// ADR-011 estan implementadas aca y deben CONSUMIRSE, no reescribirse.
//
// El test ejercita el helper directamente -- no a traves de VerifyUnit -- para
// que quede claro que es una pieza con contrato propio y no un detalle interno
// de la operacion del adquirente.
func TestVerifyCustodyChainIsSharedWithVerifyTrace(t *testing.T) {
	stub, contract := verifyFixture(t)
	ctx := testContext(stub, farmaciaMSP, RoleOperator)

	// Una unidad ya dispensada: el caso que VerifyTrace va a auditar. La cadena
	// de custodia es legitima y el helper debe decirlo, sin que el estado
	// terminal lo afecte -- la aptitud del estado es comprobacion 4 de ADR-013 y
	// no forma parte de la cadena.
	stub.txID = "tx-despacho-a-farmacia"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}
	stub.txID = "tx-recepcion-farmacia"
	_, err = contract.ReceiveTransfer(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.txID = "tx-dispensa"
	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	history, err := readUnitHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)

	result, err := verifyCustodyChain(ctx, history)
	requireNoError(t, err)
	if !result.OK {
		t.Fatalf("la cadena LABORATORY -> DRUGSTORE -> PHARMACY es legitima: %+v", result)
	}
}

// TestVerifyCustodyChainPropagatesUnregisteredCustodian fija la decision de no
// convertir un custodio irresoluble en veredicto de negocio. El registro no
// borra entradas -- SetOrganizationActive solo cambia `active` --, de modo que
// la situacion no es alcanzable por ningun camino soportado, y darle un
// veredicto seria simular que se contemplo una corrupcion del estado.
func TestVerifyCustodyChainPropagatesUnregisteredCustodian(t *testing.T) {
	stub, contract := verifyFixture(t)
	_ = contract

	// Se fabrica un historial cuyo custodio no existe en el registro.
	stub.txID = "tx-transito-fantasma"
	seedUnit(t, stub, domain.StateEnTransito, "GLN:"+drogueriaGLN)
	stub.txID = "tx-custodia-fantasma"
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:7791234599999")

	ctx := testContext(stub, farmaciaMSP, RoleOperator)
	history, err := readUnitHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)

	_, err = verifyCustodyChain(ctx, history)
	requireCode(t, err, cerr.OrgNotRegistered)
}
