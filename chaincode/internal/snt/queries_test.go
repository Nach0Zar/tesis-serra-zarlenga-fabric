package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// --- ReadUnit ---------------------------------------------------------------

func TestReadUnit(t *testing.T) {
	stub, contract := transferFixture(t)
	ctx := testContext(stub, farmaciaMSP, RoleOperator)

	view, err := contract.ReadUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if view.GTIN != validGTIN || view.NumeroSerie != validSerial {
		t.Fatalf("ReadUnit devolvio otra unidad: %+v", view)
	}
	if view.Estado != domain.StateEnLaboratorio || view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("estado publico inesperado: %+v", view)
	}
}

func TestReadUnitRejections(t *testing.T) {
	stub, contract := transferFixture(t)
	ctx := testContext(stub, farmaciaMSP, RoleOperator)

	_, err := contract.ReadUnit(ctx, validGTIN, "SN-9999-ZZZZ")
	requireCode(t, err, cerr.UnitNotFound)

	_, err = contract.ReadUnit(ctx, "07791234567890", validSerial)
	requireCode(t, err, cerr.InvalidRequest)

	_, err = contract.ReadUnit(ctx, validGTIN, "")
	requireCode(t, err, cerr.InvalidRequest)
}

// TestReadUnitIsOpenToChannelMembers deja fijado el supuesto de confianza que
// ADR-005 declara: el acceso de lectura al estado publico del canal no puede
// restringirse por chaincode, de modo que cualquier miembro del canal --
// incluido el financiador, cuya organizacion no participa de la operacion --
// puede leerlo. No es una omision: es una propiedad del modelo de canales de
// Fabric que ADR-002 establecio para todo miembro.
func TestReadUnitIsOpenToChannelMembers(t *testing.T) {
	stub, contract := transferFixture(t)
	_, err := contract.RegisterOrganization(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		RegisterOrganizationRequest{
			MSPID: financiadorMSP, ID: "INSSJP-PAMI", IDType: IDTypeREG,
			AgentType: domain.AgentFinancier, Active: true,
		})
	requireNoError(t, err)

	for _, mspID := range []string{financiadorMSP, anmatMSP, farmaciaDosMSP} {
		if _, err := contract.ReadUnit(testContext(stub, mspID, RoleAuditor), validGTIN, validSerial); err != nil {
			t.Fatalf("%s deberia poder leer el estado publico: %v", mspID, err)
		}
	}
}

// --- GetUnitHistory ---------------------------------------------------------

// TestGetUnitHistoryReconstructsFullTrace es el criterio central de esta issue:
// demostrar la reconstruccion completa de la trazabilidad de una unidad. Recorre
// el flujo core -- alta, despacho, recepcion, despacho, recepcion y dispensacion
// -- y verifica que el historial devuelva la secuencia entera de estados y
// custodios, con el valor completo de la clave en cada punto.
func TestGetUnitHistoryReconstructsFullTrace(t *testing.T) {
	stub, contract := transferFixture(t)

	advance := func(txID string, fn func()) {
		stub.txID = txID
		fn()
		stub.transient = map[string][]byte{}
	}

	advance("tx-despacho-1", func() {
		withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
		_, err := contract.DispatchTransfer(
			testContext(stub, labMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
	})
	advance("tx-recepcion-1", func() {
		_, err := contract.ReceiveTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
	})
	advance("tx-despacho-2", func() {
		withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
		_, err := contract.DispatchTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
	})
	advance("tx-recepcion-2", func() {
		_, err := contract.ReceiveTransfer(
			testContext(stub, farmaciaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
	})
	advance("tx-dispensa", func() {
		_, err := contract.Dispense(
			testContext(stub, farmaciaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
	})

	history, err := contract.GetUnitHistory(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)

	expected := []struct {
		txID     string
		estado   domain.State
		custodio string
	}{
		{"tx-0000000000000000", domain.StateEnLaboratorio, "GLN:" + labGLN},
		{"tx-despacho-1", domain.StateEnTransito, "GLN:" + labGLN},
		{"tx-recepcion-1", domain.StateEnCustodia, "GLN:" + drogueriaGLN},
		{"tx-despacho-2", domain.StateEnTransito, "GLN:" + drogueriaGLN},
		{"tx-recepcion-2", domain.StateEnCustodia, "GLN:" + farmaciaGLN},
		{"tx-dispensa", domain.StateDispensado, "GLN:" + farmaciaGLN},
	}

	if len(history) != len(expected) {
		t.Fatalf("el historial tiene %d entradas y se esperaban %d", len(history), len(expected))
	}
	for i, want := range expected {
		got := history[i]
		if got.TxID != want.txID {
			t.Errorf("entrada %d: txId = %q, se esperaba %q", i, got.TxID, want.txID)
		}
		if got.Timestamp == "" {
			t.Errorf("entrada %d: sin timestamp", i)
		}
		if got.IsDelete {
			t.Errorf("entrada %d: marcada como borrado", i)
		}
		if got.Value == nil {
			t.Fatalf("entrada %d: sin el valor de la clave en ese punto", i)
		}
		if got.Value.Estado != want.estado || got.Value.CustodioActual != want.custodio {
			t.Errorf("entrada %d: estado=%s custodio=%s, se esperaba estado=%s custodio=%s",
				i, got.Value.Estado, got.Value.CustodioActual, want.estado, want.custodio)
		}
	}

	// La secuencia de estados observada debe ser un camino valido de la maquina
	// de ADR-001. Es la comprobacion 4 que ADR-011 recomputa para el veredicto
	// del financiador, y aca queda verificada sobre una traza real.
	if history[0].Value.Estado != domain.InitialState {
		t.Fatalf("el primer estado del historial es %s y no el estado inicial", history[0].Value.Estado)
	}
	for i := 1; i < len(history); i++ {
		from, to := history[i-1].Value.Estado, history[i].Value.Estado
		if !domain.IsDeclaredStatePair(from, to) {
			t.Errorf("el par %s -> %s no es una transicion declarada por ADR-001", from, to)
		}
	}
}

// TestGetUnitHistoryOnlyShowsConfirmedModifications documenta la semantica que
// GetHistoryForKey hereda de la plataforma y que ADR-011 declara como limite:
// los intentos rechazados no llegan al world state y no aparecen en el
// historial. La verificacion audita lo que ocurrio, no lo que se intento.
func TestGetUnitHistoryOnlyShowsConfirmedModifications(t *testing.T) {
	stub, contract := transferFixture(t)

	before, err := contract.GetUnitHistory(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)

	// Intento rechazado: una farmacia que no es la custodia intenta dispensar.
	stub.txID = "tx-rechazada"
	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	if err == nil {
		t.Fatal("el intento deberia haber sido rechazado")
	}

	after, err := contract.GetUnitHistory(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)

	if len(after) != len(before) {
		t.Fatalf("un intento rechazado agrego %d entradas al historial", len(after)-len(before))
	}
}

func TestGetUnitHistoryRejections(t *testing.T) {
	stub, contract := transferFixture(t)
	ctx := testContext(stub, anmatMSP, RoleAuditor)

	_, err := contract.GetUnitHistory(ctx, validGTIN, "SN-9999-ZZZZ")
	requireCode(t, err, cerr.UnitNotFound)

	_, err = contract.GetUnitHistory(ctx, "07791234567890", validSerial)
	requireCode(t, err, cerr.InvalidRequest)
}

// --- QueryUnitsByGTIN -------------------------------------------------------

func TestQueryUnitsByGTIN(t *testing.T) {
	stub, contract := transferFixture(t)
	labCtx := testContext(stub, labMSP, RoleOperator)

	// Dos unidades mas bajo el mismo GTIN.
	for _, serie := range []string{"SN-0002-ABCD", "SN-0003-ABCD"} {
		req := validRegisterUnitRequest()
		req.NumeroSerie = serie
		stub.txID = "tx-" + serie
		_, err := contract.RegisterUnit(labCtx, req)
		requireNoError(t, err)
	}

	// Y una bajo otro GTIN, que no debe aparecer en el resultado.
	otherGTIN := validRegisterUnitRequest()
	otherGTIN.GTIN = "07791234500017"
	stub.txID = "tx-otro-gtin"
	_, err := contract.RegisterUnit(labCtx, otherGTIN)
	requireNoError(t, err)

	units, err := contract.QueryUnitsByGTIN(labCtx, validGTIN)
	requireNoError(t, err)

	if len(units) != 3 {
		t.Fatalf("QueryUnitsByGTIN devolvio %d unidades, se esperaban 3", len(units))
	}
	for _, unit := range units {
		if unit.GTIN != validGTIN {
			t.Fatalf("el resultado incluye una unidad de otro GTIN: %s", unit.GTIN)
		}
	}

	// La consulta opera por clave compuesta parcial: recupera las unidades del
	// GTIN sin indice secundario, que es lo que permite que LevelDB alcance
	// (ADR-007, punto 2).
	other, err := contract.QueryUnitsByGTIN(labCtx, otherGTIN.GTIN)
	requireNoError(t, err)
	if len(other) != 1 || other[0].NumeroSerie != validSerial {
		t.Fatalf("la consulta del otro GTIN devolvio %+v", other)
	}
}

func TestQueryUnitsByGTINEmptyResult(t *testing.T) {
	stub, contract := transferFixture(t)

	units, err := contract.QueryUnitsByGTIN(testContext(stub, labMSP, RoleOperator), "07791234500017")
	requireNoError(t, err)
	if units == nil {
		t.Fatal("un resultado vacio debe ser una lista vacia, no nil")
	}
	if len(units) != 0 {
		t.Fatalf("se esperaba un resultado vacio y hay %d unidades", len(units))
	}
}

func TestQueryUnitsByGTINRejectsInvalidGTIN(t *testing.T) {
	stub, contract := transferFixture(t)
	ctx := testContext(stub, labMSP, RoleOperator)

	for _, gtin := range []string{"", "07791234567890", "0779123456789"} {
		_, err := contract.QueryUnitsByGTIN(ctx, gtin)
		requireCode(t, err, cerr.InvalidRequest)
	}
}

// TestQueryUnitsByGTINDoesNotLeakOtherObjectTypes verifica que la consulta por
// clave compuesta parcial quede acotada al tipo de objeto de las unidades y no
// alcance a las entradas del registro ni a las autorizaciones de intervencion,
// que conviven en el mismo world state.
func TestQueryUnitsByGTINDoesNotLeakOtherObjectTypes(t *testing.T) {
	stub, contract := transferFixture(t)

	units, err := contract.QueryUnitsByGTIN(testContext(stub, labMSP, RoleOperator), validGTIN)
	requireNoError(t, err)
	if len(units) != 1 {
		t.Fatalf("se esperaba 1 unidad y la consulta devolvio %d", len(units))
	}

	// El world state tiene ademas seis entradas del registro sembradas por el
	// fixture; ninguna debe aparecer en el resultado.
	orgs, err := listOrganizations(testContext(stub, anmatMSP, RoleRegulatoryAdmin))
	requireNoError(t, err)
	if len(orgs) < 5 {
		t.Fatalf("el fixture deberia haber sembrado el registro: %d entradas", len(orgs))
	}
}
