package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de CC-9 (#112): QueryUnitsByState y el indice secundario UnitByState.

func queryState(t *testing.T, stub *mockStub, estado domain.State) []MedicationUnitView {
	t.Helper()
	contract := new(SNTContract)
	units, err := contract.QueryUnitsByState(
		testContext(stub, anmatMSP, RoleAuditor), string(estado))
	requireNoError(t, err)
	return units
}

// TestQueryUnitsByStateFindsRegisteredUnit: el alta ya deja la unidad
// enumerable, sin necesidad de ninguna transicion posterior.
func TestQueryUnitsByStateFindsRegisteredUnit(t *testing.T) {
	stub, _ := transferFixture(t)

	units := queryState(t, stub, domain.StateEnLaboratorio)
	if len(units) != 1 || units[0].NumeroSerie != validSerial {
		t.Fatalf("EN_LABORATORIO deberia tener la unidad del alta: %+v", units)
	}
	if len(queryState(t, stub, domain.StateEnCustodia)) != 0 {
		t.Fatal("EN_CUSTODIA deberia estar vacio")
	}
}

// TestUnitByStateIndexStaysSynchronized es el test que da sentido a la issue: un
// indice desincronizado hace que la consulta regulatoria MIENTA, y eso es peor
// que no tenerla.
//
// Recorre una cadena completa de transiciones y comprueba, en cada escalon, que
// la unidad aparezca UNICAMENTE bajo su estado vigente y que el estado anterior
// haya quedado vacio.
func TestUnitByStateIndexStaysSynchronized(t *testing.T) {
	stub, contract := transferFixture(t)

	requireOnlyIn := func(paso string, vigente domain.State, anteriores ...domain.State) {
		t.Helper()
		if units := queryState(t, stub, vigente); len(units) != 1 {
			t.Fatalf("%s: %s deberia tener exactamente la unidad, tiene %d", paso, vigente, len(units))
		}
		for _, previo := range anteriores {
			if units := queryState(t, stub, previo); len(units) != 0 {
				t.Fatalf("%s: %s deberia quedar vacio y tiene %d — el indice miente",
					paso, previo, len(units))
			}
		}
	}

	requireOnlyIn("alta", domain.StateEnLaboratorio)

	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}
	requireOnlyIn("despacho", domain.StateEnTransito, domain.StateEnLaboratorio)

	stub.txID = "tx-recepcion"
	_, err = contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	requireOnlyIn("recepcion", domain.StateEnCustodia,
		domain.StateEnLaboratorio, domain.StateEnTransito)

	stub.txID = "tx-cuarentena"
	_, err = contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "anomalia"})
	requireNoError(t, err)
	requireOnlyIn("cuarentena", domain.StateEnCuarentena,
		domain.StateEnLaboratorio, domain.StateEnTransito, domain.StateEnCustodia)

	stub.txID = "tx-liberacion"
	_, err = contract.ReleaseQuarantine(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "liberada"})
	requireNoError(t, err)
	requireOnlyIn("liberacion", domain.StateEnCustodia, domain.StateEnCuarentena)
}

// TestQueryUnitsByStateEnumeratesTheRegulatoryCase es el caso de uso que EXT-3
// (#29) no podia satisfacer: ANMAT pregunta que unidades estan en un estado de
// incidente, sin conocer sus seriales de antemano.
func TestQueryUnitsByStateEnumeratesTheRegulatoryCase(t *testing.T) {
	stub, contract := transferFixture(t)

	// Dos unidades mas del mismo GTIN, con seriales distintos.
	for _, serial := range []string{"SN-0002-ABCD", "SN-0003-ABCD"} {
		req := validRegisterUnitRequest()
		req.NumeroSerie = serial
		stub.txID = "tx-alta-" + serial
		_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
		requireNoError(t, err)
	}

	// Una sola se informa robada.
	stub.txID = "tx-robo"
	_, err := contract.ReportStolen(
		testContext(stub, labMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: "SN-0002-ABCD", Motivo: "robo en deposito"})
	requireNoError(t, err)

	robadas := queryState(t, stub, domain.StateRobado)
	if len(robadas) != 1 || robadas[0].NumeroSerie != "SN-0002-ABCD" {
		t.Fatalf("ROBADO deberia tener solo la unidad informada: %+v", robadas)
	}
	if len(queryState(t, stub, domain.StateEnLaboratorio)) != 2 {
		t.Fatal("EN_LABORATORIO deberia conservar las otras dos")
	}
}

// TestQueryUnitsByStateRejectsUnknownState: el catalogo de estados lo fija
// ADR-001 y la consulta valida contra el, no contra una lista propia.
func TestQueryUnitsByStateRejectsUnknownState(t *testing.T) {
	stub, contract := transferFixture(t)

	for _, estado := range []string{"", "INVENTADO", "en_custodia"} {
		_, err := contract.QueryUnitsByState(
			testContext(stub, anmatMSP, RoleAuditor), estado)
		requireCode(t, err, cerr.InvalidRequest)
	}
}

// TestQueryUnitsByStateOnEmptyState devuelve lista vacia y no error: un estado
// sin unidades es una respuesta legitima de la consulta.
func TestQueryUnitsByStateOnEmptyState(t *testing.T) {
	stub, _ := transferFixture(t)

	for _, estado := range []domain.State{
		domain.StateDispensado, domain.StateProhibido, domain.StateDispuestoFinal,
	} {
		if units := queryState(t, stub, estado); len(units) != 0 {
			t.Fatalf("%s deberia estar vacio: %+v", estado, units)
		}
	}
}

// TestQueryUnitsByStateNeedsNoAuthorization fija la decision de autorizacion:
// abierta, igual que el resto de las lecturas. Restringirla seria una barrera
// aparente, porque el universo es enumerable con QueryUnitsByGTIN.
func TestQueryUnitsByStateNeedsNoAuthorization(t *testing.T) {
	stub, contract := transferFixture(t)

	for _, invocador := range []struct{ msp, rol string }{
		{labMSP, RoleOperator},
		{drogueriaMSP, RoleOperator},
		{anmatMSP, RoleAuditor},
	} {
		_, err := contract.QueryUnitsByState(
			testContext(stub, invocador.msp, invocador.rol), string(domain.StateEnLaboratorio))
		requireNoError(t, err)
	}
}

// TestSplitCompositeKeyIsInverseOfCreate verifica que la descomposicion de
// claves compuestas del mock sea la inversa exacta de shim.CreateCompositeKey,
// que es la real.
//
// Importa porque QueryUnitsByState depende de esa descomposicion para recuperar
// el GTIN y el serial desde la clave del indice. Si el mock divergiera del
// formato de Fabric, la consulta pasaria los tests y fallaria en la red.
func TestSplitCompositeKeyIsInverseOfCreate(t *testing.T) {
	stub := newMockStub()
	casos := [][]string{
		{string(domain.StateEnCustodia), validGTIN, validSerial},
		{string(domain.StateRobado), validGTIN, "SN-CON-GUIONES-9"},
		{string(domain.StateDispuestoFinal), "07791234567898", "A"},
	}
	for _, attrs := range casos {
		key, err := stub.CreateCompositeKey(objectTypeUnitByState, attrs)
		requireNoError(t, err)
		objectType, parts, err := stub.SplitCompositeKey(key)
		requireNoError(t, err)
		if objectType != objectTypeUnitByState {
			t.Fatalf("objectType = %q", objectType)
		}
		if len(parts) != len(attrs) {
			t.Fatalf("componentes = %v, se esperaban %v", parts, attrs)
		}
		for i := range attrs {
			if parts[i] != attrs[i] {
				t.Fatalf("componente %d = %q, se esperaba %q", i, parts[i], attrs[i])
			}
		}
	}
}

// TestQueryUnitsByStateCoversEveryDeclaredState recorre el catalogo COMPLETO de
// ADR-001 y demuestra dos cosas por cada uno de los trece estados: que la
// consulta lo acepta -- ninguno cae en INVALID_REQUEST -- y que el indice
// UnitByState se mantiene tambien para el, devolviendo la unidad sembrada.
//
// El catalogo NO se repite aca: sale de domain.States(), y
// TestStateCatalogMatchesADR001 lo contrasta contra el Markdown de ADR-001. Con
// una lista propia, agregar un estado a la ADR dejaria este test pasando sobre
// trece de catorce; derivandolo, el estado nuevo entra solo.
//
// Los casos con resultados y vacios de los tests anteriores cubrian ocho de los
// trece: faltaban VENCIDO, EXTRAVIADO, DETERIORADO, RETIRADO_MERCADO y
// DEVUELTO, precisamente los estados a los que llegan los eventos
// extraordinarios, que son el caso de auditoria que motivo la operacion.
func TestQueryUnitsByStateCoversEveryDeclaredState(t *testing.T) {
	catalog := domain.States()
	if len(catalog) == 0 {
		t.Fatal("el catalogo de estados de ADR-001 no puede estar vacio")
	}

	for _, estado := range catalog {
		t.Run(string(estado), func(t *testing.T) {
			stub, contract := transferFixture(t)
			seedUnit(t, stub, estado, "GLN:"+drogueriaGLN)

			units, err := contract.QueryUnitsByState(
				testContext(stub, anmatMSP, RoleAuditor), string(estado))
			requireNoError(t, err)

			if len(units) != 1 {
				t.Fatalf("%s devolvio %d unidades, se esperaba la sembrada", estado, len(units))
			}
			if units[0].Estado != estado {
				t.Fatalf("la unidad devuelta esta en %s y se consulto %s", units[0].Estado, estado)
			}
			if units[0].NumeroSerie != validSerial {
				t.Fatalf("numeroSerie = %s", units[0].NumeroSerie)
			}
		})
	}
}
