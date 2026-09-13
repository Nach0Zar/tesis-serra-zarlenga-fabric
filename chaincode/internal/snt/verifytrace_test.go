package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Tests de VerifyTrace: la verificacion de trazabilidad del financiador que fija
// ADR-011 e implementa CC-8 (#62).

const financiadorID = "REG-FIN-0001"

// registerFinancier da de alta al organismo financiador. No usa registerOrg
// porque un agentType no custodial exige idType=REG (ADR-010; el contrato lo
// valida en RegisterOrganization).
func registerFinancier(t *testing.T, stub *mockStub) {
	t.Helper()
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	_, err := contract.RegisterOrganization(ctx, RegisterOrganizationRequest{
		MSPID:     financiadorMSP,
		ID:        financiadorID,
		IDType:    IDTypeREG,
		AgentType: domain.AgentFinancier,
		Active:    true,
	})
	requireNoError(t, err)
}

// traceFixture deja una unidad DISPENSADA por el camino completo y legitimo
// lab -> drogueria -> farmacia -> dispensa, que es el unico escenario en el que
// el financiador tiene condicion de pago.
func traceFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub, contract := verifyFixture(t)
	registerFinancier(t, stub)

	stub.txID = "tx-despacho-farmacia"
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

	return stub, contract
}

func financierContext(stub *mockStub) contractapi.TransactionContextInterface {
	return testContext(stub, financiadorMSP, RoleFinancierAuditor)
}

// traceCheckByName evita que los asserts dependan del indice de una
// comprobacion dentro del arreglo.
func traceCheckByName(t *testing.T, verdict *TraceVerdict, name string) TraceCheck {
	t.Helper()
	for _, check := range verdict.Verificaciones {
		if check.Check == name {
			return check
		}
	}
	t.Fatalf("el veredicto no reporta la comprobacion %s: %+v", name, verdict.Verificaciones)
	return TraceCheck{}
}

// requireTraceVerdict comprueba el veredicto completo: el booleano, el motivo y
// que la comprobacion que se dice fallada figure efectivamente como FALLO.
func requireTraceVerdict(t *testing.T, verdict *TraceVerdict, legitima bool, motivo, failedCheck string) {
	t.Helper()
	if verdict.Legitima != legitima {
		t.Fatalf("legitima = %v, se esperaba %v (motivo: %q)", verdict.Legitima, legitima, verdict.Motivo)
	}
	if verdict.Motivo != motivo {
		t.Fatalf("motivo = %q, se esperaba %q", verdict.Motivo, motivo)
	}
	if failedCheck == "" {
		return
	}
	if got := traceCheckByName(t, verdict, failedCheck); got.Resultado != checkFailed {
		t.Fatalf("la comprobacion %s deberia figurar como %s y figura como %s",
			failedCheck, checkFailed, got.Resultado)
	}
}

// TestVerifyTraceHappyPath es el caso de uso de ADR-011: el financiador consulta
// una unidad efectivamente dispensada y obtiene la condicion de pago.
func TestVerifyTraceHappyPath(t *testing.T) {
	stub, contract := traceFixture(t)

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, true, "", "")

	if len(verdict.Verificaciones) != 5 {
		t.Fatalf("la checklist de ADR-011 tiene cinco comprobaciones y el veredicto reporta %d",
			len(verdict.Verificaciones))
	}
	for _, check := range verdict.Verificaciones {
		if check.Resultado != checkOK {
			t.Fatalf("con veredicto legitimo toda comprobacion debe ser %s: %+v", checkOK, check)
		}
	}
	if got := traceCheckByName(t, verdict, checkDispenserAllowed); got.Detalle != string(domain.AgentPharmacy) {
		t.Fatalf("el detalle del dispensador deberia nombrar su agentType, y es %q", got.Detalle)
	}
}

// TestVerifyTraceNotFound: la inexistencia es un veredicto, no un error de
// invocacion. Es la diferencia explicita que declara el contrato.
func TestVerifyTraceNotFound(t *testing.T) {
	stub, contract := traceFixture(t)

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, "SN-INEXISTENTE")
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictNotFound, checkExistence)

	for _, name := range []string{checkDispensedState, checkDispenserAllowed, checkStateSequence, checkAuthorizedPairs} {
		if got := traceCheckByName(t, verdict, name); got.Resultado != checkNotEvaluated {
			t.Fatalf("%s deberia quedar %s tras fallar la existencia y esta %s",
				name, checkNotEvaluated, got.Resultado)
		}
	}
}

// TestVerifyTraceOnUnitNotDispensed cubre la comprobacion 2 y la diferencia
// sustantiva con VerifyUnit: una unidad legitima que todavia no se dispenso no
// habilita el pago, y el detalle informa el estado observado para que el
// financiador distinga "todavia no" de "traza irregular".
func TestVerifyTraceOnUnitNotDispensed(t *testing.T) {
	stub, contract := verifyFixture(t)
	registerFinancier(t, stub)

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictNotDispensed, checkDispensedState)

	if got := traceCheckByName(t, verdict, checkDispensedState); got.Detalle != string(domain.StateEnCustodia) {
		t.Fatalf("el detalle deberia informar el estado observado, y es %q", got.Detalle)
	}
}

// TestVerifyTraceDetectsInvalidDispenser cubre la comprobacion 3: el estado dice
// DISPENSADO pero quien lo alcanzo no es un agente habilitado para T06.
//
// El historial se fabrica porque Dispense ya rechaza ese caso: la comprobacion
// existe justamente para RECOMPUTAR la regla desde el historial en lugar de
// confiar en que el chaincode la aplico al escribir, que es lo que la vuelve
// verificable por un tercero.
func TestVerifyTraceDetectsInvalidDispenser(t *testing.T) {
	stub, contract := verifyFixture(t)
	registerFinancier(t, stub)

	// La drogueria custodia la unidad y "la dispensa": DRUGSTORE no esta
	// habilitado para T06.
	stub.txID = "tx-dispensa-drogueria"
	seedUnit(t, stub, domain.StateDispensado, "GLN:"+drogueriaGLN)

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictInvalidDispenser, checkDispenserAllowed)

	if got := traceCheckByName(t, verdict, checkDispenserAllowed); got.Detalle == "" {
		t.Fatal("el detalle debe nombrar el agentType observado")
	}
	for _, name := range []string{checkStateSequence, checkAuthorizedPairs} {
		if got := traceCheckByName(t, verdict, name); got.Resultado != checkNotEvaluated {
			t.Fatalf("%s deberia quedar %s tras fallar el dispensador", name, checkNotEvaluated)
		}
	}
}

// TestVerifyTraceDetectsInvalidSequence cubre la comprobacion 4.
func TestVerifyTraceDetectsInvalidSequence(t *testing.T) {
	stub, contract := verifyFixture(t)
	registerFinancier(t, stub)

	// EN_CUSTODIA -> DISPENSADO es T06, pero la custodia salta de la drogueria
	// a la farmacia sin transito: rompe el acoplamiento de ADR-004.
	stub.txID = "tx-dispensa-sin-transito"
	seedUnit(t, stub, domain.StateDispensado, "GLN:"+farmaciaGLN)

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictInvalidSequence, checkStateSequence)

	if got := traceCheckByName(t, verdict, checkAuthorizedPairs); got.Resultado != checkNotEvaluated {
		t.Fatalf("PARES_AUTORIZADOS deberia quedar %s tras fallar la secuencia", checkNotEvaluated)
	}
}

// forgeChain fabrica un historial paso a paso. Cada paso es un snapshot
// (estado, custodio) escrito con su propia txID, que es exactamente la forma en
// que GetHistoryForKey lo devuelve.
func forgeChain(t *testing.T, stub *mockStub, steps []struct {
	estado   domain.State
	custodio string
}) {
	t.Helper()
	for index, step := range steps {
		stub.txID = "tx-forjada-" + string(rune('a'+index))
		seedUnit(t, stub, step.estado, step.custodio)
	}
}

// TestVerifyTraceDetectsUnauthorizedTransfer cubre la comprobacion 5 con las
// cuatro anteriores en verde: la unica regla violada es la matriz de DES-3.
//
// El par PHARMACY -> DRUGSTORE esta explicitamente prohibido (venta hacia un
// eslabon superior). La cadena vuelve despues a una farmacia para que la
// comprobacion 3 pase y el veredicto llegue efectivamente a la 5.
func TestVerifyTraceDetectsUnauthorizedTransfer(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
	registerFinancier(t, stub)
	contract := new(SNTContract)

	forgeChain(t, stub, []struct {
		estado   domain.State
		custodio string
	}{
		{domain.StateEnLaboratorio, "GLN:" + labGLN},
		{domain.StateEnTransito, "GLN:" + labGLN},
		{domain.StateEnCustodia, "GLN:" + farmaciaGLN}, // LABORATORY -> PHARMACY: autorizado
		{domain.StateEnTransito, "GLN:" + farmaciaGLN},
		{domain.StateEnCustodia, "GLN:" + drogueriaGLN}, // PHARMACY -> DRUGSTORE: PROHIBIDO
		{domain.StateEnTransito, "GLN:" + drogueriaGLN},
		{domain.StateEnCustodia, "GLN:" + farmaciaGLN}, // DRUGSTORE -> PHARMACY: autorizado
		{domain.StateDispensado, "GLN:" + farmaciaGLN},
	})

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictTransferNotAuthorized, checkAuthorizedPairs)

	if got := traceCheckByName(t, verdict, checkStateSequence); got.Resultado != checkOK {
		t.Fatalf("el camino de estados es valido y SECUENCIA_ESTADOS figura como %s", got.Resultado)
	}
	if got := traceCheckByName(t, verdict, checkAuthorizedPairs); got.Detalle != "PHARMACY -> DRUGSTORE" {
		t.Fatalf("detalle = %q, se esperaba el par observado", got.Detalle)
	}
}

// TestVerifyTraceEvaluationOrder fija la regla de ADR-011 que hace reproducible
// al veredicto: `motivo` nombra la primera comprobacion que falla EN EL ORDEN
// DECLARADO, no la primera violacion que aparece recorriendo el historial.
//
// Es el test que discrimina entre las dos implementaciones posibles. El
// historial tiene las dos violaciones, y la del par aparece ANTES por posicion:
//
//	indice 4: PHARMACY -> DRUGSTORE, prohibido por la matriz   (comprobacion 5)
//	indice 5: EN_CUSTODIA -> EN_LABORATORIO, no declarada      (comprobacion 4)
//
// Una sola pasada intercalada devolveria TRANSFERENCIA_NO_AUTORIZADA. ADR-011
// exige SECUENCIA_INVALIDA, porque la comprobacion 4 se evalua antes que la 5 y
// falla.
func TestVerifyTraceEvaluationOrder(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
	registerFinancier(t, stub)
	contract := new(SNTContract)

	forgeChain(t, stub, []struct {
		estado   domain.State
		custodio string
	}{
		{domain.StateEnLaboratorio, "GLN:" + labGLN},
		{domain.StateEnTransito, "GLN:" + labGLN},
		{domain.StateEnCustodia, "GLN:" + farmaciaGLN},
		{domain.StateEnTransito, "GLN:" + farmaciaGLN},
		{domain.StateEnCustodia, "GLN:" + drogueriaGLN},    // par prohibido (idx 4)
		{domain.StateEnLaboratorio, "GLN:" + drogueriaGLN}, // secuencia invalida (idx 5)
		{domain.StateEnTransito, "GLN:" + drogueriaGLN},
		{domain.StateEnCustodia, "GLN:" + farmaciaGLN},
		{domain.StateDispensado, "GLN:" + farmaciaGLN},
	})

	verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
	requireNoError(t, err)
	requireTraceVerdict(t, verdict, false, verdictInvalidSequence, checkStateSequence)

	if got := traceCheckByName(t, verdict, checkAuthorizedPairs); got.Resultado != checkNotEvaluated {
		t.Fatalf("PARES_AUTORIZADOS deberia quedar %s tras fallar la secuencia y esta %s",
			checkNotEvaluated, got.Resultado)
	}
}

// TestVerifyTraceAuthorization cubre la autorizacion de ADR-011, que a
// diferencia de VerifyUnit SI restringe: el financiador no es un eslabon de la
// cadena y su consulta no es la lectura del estado publico que ADR-005 declara
// no restringible.
func TestVerifyTraceAuthorization(t *testing.T) {
	t.Run("financiador con financier-auditor", func(t *testing.T) {
		stub, contract := traceFixture(t)
		verdict, err := contract.VerifyTrace(financierContext(stub), validGTIN, validSerial)
		requireNoError(t, err)
		requireTraceVerdict(t, verdict, true, "", "")
	})

	t.Run("ANMAT con auditor", func(t *testing.T) {
		stub, contract := traceFixture(t)
		verdict, err := contract.VerifyTrace(
			testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
		requireNoError(t, err)
		requireTraceVerdict(t, verdict, true, "", "")
	})

	t.Run("ANMAT con regulatory-admin", func(t *testing.T) {
		stub, contract := traceFixture(t)
		verdict, err := contract.VerifyTrace(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin), validGTIN, validSerial)
		requireNoError(t, err)
		requireTraceVerdict(t, verdict, true, "", "")
	})

	// El agentType se comprueba antes que el rol: la farmacia esta registrada y
	// habilitada, y lo que la excluye es su tipo, no su atributo.
	t.Run("eslabon custodial", func(t *testing.T) {
		stub, contract := traceFixture(t)
		_, err := contract.VerifyTrace(
			testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.UnauthorizedAgentType)
	})

	t.Run("financiador con rol equivocado", func(t *testing.T) {
		stub, contract := traceFixture(t)
		_, err := contract.VerifyTrace(
			testContext(stub, financiadorMSP, RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.UnauthorizedRole)
	})

	t.Run("ANMAT con rol operativo", func(t *testing.T) {
		stub, contract := traceFixture(t)
		_, err := contract.VerifyTrace(
			testContext(stub, anmatMSP, RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.UnauthorizedRole)
	})

	t.Run("organizacion no registrada", func(t *testing.T) {
		stub, contract := traceFixture(t)
		_, err := contract.VerifyTrace(
			testContext(stub, "DesconocidaMSP", RoleFinancierAuditor), validGTIN, validSerial)
		requireCode(t, err, cerr.OrgNotRegistered)
	})
}

// TestVerifyTraceRejectsInvalidRequest: la referencia de unidad se valida con el
// mismo criterio que el resto del contrato, y una unidad mal identificada es un
// error de invocacion, no un veredicto.
func TestVerifyTraceRejectsInvalidRequest(t *testing.T) {
	stub, contract := traceFixture(t)

	_, err := contract.VerifyTrace(financierContext(stub), "", validSerial)
	requireCode(t, err, cerr.InvalidRequest)
}
