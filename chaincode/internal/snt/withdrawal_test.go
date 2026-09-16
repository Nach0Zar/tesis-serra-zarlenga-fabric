package snt

import (
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-6 (#32): WithdrawFromMarket (T17-T19) y ProhibitProduct (T20).

func withdrawalRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "retiro dispuesto por desvio de calidad documentado",
	}
}

// authorizeWithdrawal emite la autorizacion de intervencion que el contrato
// exige a un laboratorio no custodio.
func authorizeWithdrawal(t *testing.T, stub *mockStub, laboratorio string) {
	t.Helper()
	contract := new(SNTContract)
	_, err := contract.AuthorizeLabIntervention(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		AuthorizeLabInterventionRequest{
			GTIN: validGTIN, NumeroSerie: validSerial,
			Laboratorio: laboratorio,
			Operacion:   LabOpWithdrawFromMarket,
			Motivo:      "habilitacion de retiro por desvio de calidad",
			ExpiraEn:    "2027-01-01T00:00:00Z",
		})
	requireNoError(t, err)
}

// TestWithdrawFromMarketIsNotOpenToCustodian fija lo que ADR-001 decide y que
// esta operacion no comparte con el resto de los eventos extraordinarios: el
// custodio actual NO es actor habilitado de T17-T19.
//
// Solo lo son ANMAT y el laboratorio titular, que es exactamente la potestad que
// el issue enuncia como "solo AnmatMSP (y laboratorio para retiro voluntario)".
// Una drogueria que custodia la unidad no puede retirarla del mercado: el retiro
// es una decision del titular del producto o de la autoridad, no de quien lo
// tiene en el deposito.
func TestWithdrawFromMarketIsNotOpenToCustodian(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.WithdrawFromMarket(
		testContext(stub, drogueriaMSP, RoleOperator), withdrawalRequest())
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestWithdrawFromMarketByRegulator cubre el camino regulatorio, con su
// marcador de participacion.
func TestWithdrawFromMarketByRegulator(t *testing.T) {
	stub, contract := verifyFixture(t)
	stub.txID = "tx-retiro-anmat"

	view, err := contract.WithdrawFromMarket(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateRetiradoMercado {
		t.Fatalf("estado = %s", view.Estado)
	}
	if domain.IsTerminalState(view.Estado) {
		t.Fatal("RETIRADO_MERCADO es bloqueante y no terminal: conserva T27 y T31")
	}
	requireRegulatoryMarker(t, stub, opWithdrawFromMarket)
}

// TestWithdrawFromMarketByTitularLaboratory es el caso que da sentido a esta
// operacion: el laboratorio ya despacho la unidad, NO es el custodio, y sigue
// siendo responsable del producto que puso en el mercado.
//
// Comprueba las tres piezas que exige ADR-007 punto 6.e: la autorizacion se
// consume, y se escriben los marcadores del laboratorio Y del regulador, que es
// lo que fuerza el endoso de tres organizaciones.
func TestWithdrawFromMarketByTitularLaboratory(t *testing.T) {
	stub, contract := verifyFixture(t)
	authorizeWithdrawal(t, stub, "GLN:"+labGLN)

	stub.txID = "tx-retiro-laboratorio"
	view, err := contract.WithdrawFromMarket(
		testContext(stub, labMSP, RoleOperator), withdrawalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateRetiradoMercado {
		t.Fatalf("estado = %s, se esperaba RETIRADO_MERCADO", view.Estado)
	}
	// La custodia NO se mueve: el retiro no es una entrega.
	if view.CustodioActual != "GLN:"+drogueriaGLN {
		t.Fatalf("custodio = %s, el retiro no mueve la custodia", view.CustodioActual)
	}

	// La autorizacion quedo CONSUMIDA: es de un solo uso.
	authorization, found, err := readLabIntervention(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)
	if !found || authorization.Estado != LabInterventionConsumed {
		t.Fatalf("la autorizacion deberia quedar CONSUMIDA: %+v", authorization)
	}
	if authorization.ConsumidaEn == "" {
		t.Fatal("la autorizacion consumida debe registrar consumidaEn")
	}

	// Los DOS marcadores: laboratorio designado y organizacion regulatoria.
	markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	if stub.privateData[implicitCollection(labMSP)][markerKey] == nil {
		t.Fatal("falta el marcador del laboratorio designado")
	}
	if stub.privateData[implicitCollection(anmatMSP)][markerKey] == nil {
		t.Fatal("falta el marcador de la organizacion regulatoria")
	}
}

// TestWithdrawFromMarketRequiresAuthorization recorre los cinco motivos por los
// que la autorizacion no habilita el retiro. Sin ellos, cualquier laboratorio
// registrado podria retirar del mercado la unidad de cualquier custodio.
func TestWithdrawFromMarketRequiresAuthorization(t *testing.T) {
	t.Run("sin autorizacion", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion de otro laboratorio", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		registerOrg(t, stub, "Lab2MSP", "7791234500079", domain.AgentLaboratory)
		authorizeWithdrawal(t, stub, "GLN:7791234500079")
		_, err := contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion ya consumida", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		authorizeWithdrawal(t, stub, "GLN:"+labGLN)
		_, err := contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireNoError(t, err)

		// Reingreso fuera de alcance (EXT-5); se comprueba la invariante
		// volviendo la unidad a EN_CUSTODIA por siembra.
		seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+drogueriaGLN)
		_, err = contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion revocada", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		authorizeWithdrawal(t, stub, "GLN:"+labGLN)
		_, err := contract.RevokeLabIntervention(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "revocada"})
		requireNoError(t, err)

		_, err = contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion vencida", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		authorizeWithdrawal(t, stub, "GLN:"+labGLN)
		// El reloj de la transaccion avanza mas alla de expiraEn.
		stub.timestamp = time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

		_, err := contract.WithdrawFromMarket(
			testContext(stub, labMSP, RoleOperator), withdrawalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})
}

// TestProhibitProductIsRegulatoryOnly es la potestad diferenciada que el trabajo
// debe demostrar: el retiro lo puede iniciar el titular, la prohibicion NO.
func TestProhibitProductIsRegulatoryOnly(t *testing.T) {
	for _, invocador := range []struct{ msp, rol string }{
		{drogueriaMSP, RoleOperator}, // custodio actual
		{labMSP, RoleOperator},       // laboratorio titular
		{anmatMSP, RoleAuditor},      // regulador sin rol de administracion
	} {
		t.Run(invocador.msp+"/"+invocador.rol, func(t *testing.T) {
			stub, contract := verifyFixture(t)
			_, err := contract.ProhibitProduct(
				testContext(stub, invocador.msp, invocador.rol), withdrawalRequest())
			requireCode(t, err, cerr.RegulatoryOnly)
		})
	}

	t.Run("ANMAT si puede", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		stub.txID = "tx-prohibicion"
		view, err := contract.ProhibitProduct(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
		requireNoError(t, err)
		if view.Estado != domain.StateProhibido {
			t.Fatalf("estado = %s, se esperaba PROHIBIDO", view.Estado)
		}
		requireRegulatoryMarker(t, stub, opProhibitProduct)
	})
}

// TestProhibitProductFromWithdrawn: retiro y prohibicion no son grados de lo
// mismo. ADR-001 declara T20 tambien desde RETIRADO_MERCADO, de modo que la
// autoridad puede prohibir un producto que el titular ya habia retirado.
func TestProhibitProductFromWithdrawn(t *testing.T) {
	stub, contract := verifyFixture(t)
	_, err := contract.WithdrawFromMarket(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
	requireNoError(t, err)

	view, err := contract.ProhibitProduct(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateProhibido {
		t.Fatalf("estado = %s, se esperaba PROHIBIDO", view.Estado)
	}
}

// TestWithdrawalBlocksOrdinaryOperations cubre el criterio "bloquea toda
// operacion salvo disposicion final": ADR-001 no declara despacho ni T06 desde
// RETIRADO_MERCADO ni desde PROHIBIDO, y conserva las salidas de resolucion.
func TestWithdrawalBlocksOrdinaryOperations(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		aplicar func(*SNTContract, *mockStub) error
	}{
		{"RETIRADO_MERCADO", func(c *SNTContract, s *mockStub) error {
			_, err := c.WithdrawFromMarket(testContext(s, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
			return err
		}},
		{"PROHIBIDO", func(c *SNTContract, s *mockStub) error {
			_, err := c.ProhibitProduct(testContext(s, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
			return err
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			stub, contract := verifyFixture(t)
			requireNoError(t, c.aplicar(contract, stub))

			withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
			_, err := contract.DispatchTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}

// TestWithdrawFromMarketByTitularLaboratoryFromLab cubre T17, que es el caso de
// uso PRINCIPAL de la operacion y que la primera version dejaba inalcanzable:
// el laboratorio retira voluntariamente su propio producto, con la unidad
// todavia EN_LABORATORIO y con el mismo laboratorio como custodio.
//
// El orden de resolucion del actor lo rompia: devolvia ActorCurrentCustodian y
// T17 solo admite ANMAT o LABORATORY. Tampoco necesita autorizacion de
// intervencion, porque el laboratorio ES el custodio.
func TestWithdrawFromMarketByTitularLaboratoryFromLab(t *testing.T) {
	stub, contract := transferFixture(t)

	view, err := contract.WithdrawFromMarket(
		testContext(stub, labMSP, RoleOperator), withdrawalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateRetiradoMercado {
		t.Fatalf("estado = %s, se esperaba RETIRADO_MERCADO", view.Estado)
	}
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodio = %s", view.CustodioActual)
	}
}

// TestWithdrawFromMarketInTransitExcludesTitularLaboratory fija la decision de
// DES-19 (#116), incorporada a ADR-001 en su revision 2: desde EN_TRANSITO el
// retiro queda reservado a ANMAT.
//
// El caso que fija es el mas incomodo de la decision y por eso se testea: el
// laboratorio de este escenario es el EMISOR de la transferencia, de modo que si
// es miembro de la coleccion del par y podria, tecnicamente, cerrar el registro
// de la operacion. Queda excluido igual, porque la columna "actor habilitado" de
// ADR-001 es por transicion y no admite la condicion "laboratorio que ademas es
// parte del par": introducirla seria agregar un actor logico que ninguna ADR
// declara. La potestad del titular sigue viva en T17, T18 y en el retiro desde
// EN_CUARENTENA o DEVUELTO.
//
// El test verifica ademas que el rechazo no deja rastro: la unidad sigue
// EN_TRANSITO y el registro de la operacion activa sigue abierto.
func TestWithdrawFromMarketInTransitExcludesTitularLaboratory(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-retiro-en-transito"
	_, err = contract.WithdrawFromMarket(
		testContext(stub, labMSP, RoleOperator), withdrawalRequest())
	requireCode(t, err, cerr.InvalidStateTransition)

	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	view, err := contract.ReadUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if view.Estado != domain.StateEnTransito {
		t.Fatalf("estado = %s, se esperaba EN_TRANSITO intacto", view.Estado)
	}
	_, found, err := readActiveTransferOperation(
		ctx, pairCollectionName(labMSP, drogueriaMSP), validGTIN, validSerial)
	requireNoError(t, err)
	if !found {
		t.Fatal("el retiro rechazado no debe cerrar el registro de la operacion activa")
	}
}

// TestWithdrawFromMarketInTransitByRegulator cubre el unico camino que ADR-001
// revision 2 deja abierto para T19 desde EN_TRANSITO: ANMAT es miembro de toda
// coleccion de par (ADR-006, punto 1), de modo que puede cerrar el registro de la
// operacion activa, cierre que ADR-007 punto 6.c exige para salir del transito.
//
// Verifica las tres consecuencias de ese cierre: el estado resultante, que el
// registro de la operacion queda cerrado y que la clave de la unidad vuelve a la
// politica de reposo del emisor (ADR-007, punto 6.a), porque la custodia no se
// movio -- solo T04 la mueve (ADR-004).
func TestWithdrawFromMarketInTransitByRegulator(t *testing.T) {
	stub, contract := verifyFixture(t)
	stub.txID = "tx-despacho-2"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-retiro-anmat-transito"
	view, err := contract.WithdrawFromMarket(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), withdrawalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateRetiradoMercado {
		t.Fatalf("estado = %s, se esperaba RETIRADO_MERCADO", view.Estado)
	}

	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	_, found, err := readActiveTransferOperation(
		ctx, pairCollectionName(drogueriaMSP, farmaciaMSP), validGTIN, validSerial)
	requireNoError(t, err)
	if found {
		t.Fatal("T19 desde EN_TRANSITO debe cerrar el registro de la operacion activa")
	}
	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != drogueriaMSP {
		t.Fatalf("politica de reposo = %v, se esperaba el emisor %s", orgs, drogueriaMSP)
	}
}
