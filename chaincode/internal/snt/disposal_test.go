package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-8 (#63): FinalDisposition (T28 a T33).

func disposalRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "destruccion autorizada, Ley 24.051 de residuos peligrosos, acta 118/2026",
	}
}

// authorizeDisposal emite la autorizacion de intervencion con operacion
// FINAL_DISPOSITION, que es la que el contrato exige al laboratorio no custodio
// en T31.
func authorizeDisposal(t *testing.T, stub *mockStub, laboratorio string) {
	t.Helper()
	contract := new(SNTContract)
	_, err := contract.AuthorizeLabIntervention(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		AuthorizeLabInterventionRequest{
			GTIN: validGTIN, NumeroSerie: validSerial,
			Laboratorio: laboratorio,
			Operacion:   LabOpFinalDisposition,
			Motivo:      "disposicion final del lote retirado",
			ExpiraEn:    "2027-01-01T00:00:00Z",
		})
	requireNoError(t, err)
}

// TestFinalDispositionFromEveryOrigin recorre las SEIS filas de ADR-001 con el
// actor que cada una habilita. Es la cobertura que el issue pide ("tests de cada
// transicion de entrada") y no es redundante: la operacion tiene hasta tres
// actores distintos segun el origen.
func TestFinalDispositionFromEveryOrigin(t *testing.T) {
	custodio := "GLN:" + drogueriaGLN

	casos := []struct {
		nombre string
		estado domain.State
		msp    string
		rol    string
		setup  func(t *testing.T, stub *mockStub)
	}{
		// T28, T29 y T33 habilitan UNICAMENTE a RECOVERY_OR_DISPOSAL_AGENT, que
		// ADR-009 punto 3 resuelve como el custodio actual registrado.
		{"T28 desde VENCIDO", domain.StateVencido, drogueriaMSP, RoleOperator, nil},
		{"T29 desde DETERIORADO", domain.StateDeteriorado, drogueriaMSP, RoleOperator, nil},
		{"T33 desde DEVUELTO", domain.StateDevuelto, drogueriaMSP, RoleOperator, nil},

		// T30: custodio actual o ANMAT.
		{"T30 desde EN_CUARENTENA, custodio", domain.StateEnCuarentena, drogueriaMSP, RoleOperator, nil},
		{"T30 desde EN_CUARENTENA, ANMAT", domain.StateEnCuarentena, anmatMSP, RoleRegulatoryAdmin, nil},

		// T31: ANMAT, LABORATORY o el agente de recupero.
		{"T31 desde RETIRADO_MERCADO, ANMAT", domain.StateRetiradoMercado, anmatMSP, RoleRegulatoryAdmin, nil},
		{"T31 desde RETIRADO_MERCADO, custodio", domain.StateRetiradoMercado, drogueriaMSP, RoleOperator, nil},
		{
			"T31 desde RETIRADO_MERCADO, laboratorio titular no custodio",
			domain.StateRetiradoMercado, labMSP, RoleOperator,
			func(t *testing.T, stub *mockStub) { authorizeDisposal(t, stub, "GLN:"+labGLN) },
		},

		// T32: ANMAT o el agente de recupero. El laboratorio titular NO figura.
		{"T32 desde PROHIBIDO, ANMAT", domain.StateProhibido, anmatMSP, RoleRegulatoryAdmin, nil},
		{"T32 desde PROHIBIDO, custodio", domain.StateProhibido, drogueriaMSP, RoleOperator, nil},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			seedUnit(t, stub, caso.estado, custodio)
			if caso.setup != nil {
				caso.setup(t, stub)
			}

			stub.txID = "tx-disposicion"
			view, err := contract.FinalDisposition(
				testContext(stub, caso.msp, caso.rol), disposalRequest())
			requireNoError(t, err)
			if view.Estado != domain.StateDispuestoFinal {
				t.Fatalf("estado = %s, se esperaba DISPUESTO_FINAL", view.Estado)
			}
			if !domain.IsTerminalState(view.Estado) {
				t.Fatal("DISPUESTO_FINAL debe ser terminal")
			}
			// La custodia no se mueve: el custodio registrado al momento de la
			// disposicion queda en la traza como responsable de haberla
			// ejecutado, que es lo que la normativa de residuos peligrosos
			// necesita poder atribuir.
			if view.CustodioActual != custodio {
				t.Fatalf("custodio = %s, la disposicion no mueve la custodia", view.CustodioActual)
			}
		})
	}
}

// TestFinalDispositionIsNotOpenToRegulatorFromCustodialOrigins fija el detalle
// mas filoso de estas seis filas: T28, T29 y T33 habilitan unicamente al agente
// de recupero, que es el custodio actual. ANMAT no es actor de ninguna de las
// tres, y no es una omision: la unidad esta fisicamente en poder de su custodio
// y es el quien ejecuta y documenta la destruccion.
func TestFinalDispositionIsNotOpenToRegulatorFromCustodialOrigins(t *testing.T) {
	for _, estado := range []domain.State{
		domain.StateVencido, domain.StateDeteriorado, domain.StateDevuelto,
	} {
		t.Run(string(estado), func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			seedUnit(t, stub, estado, "GLN:"+drogueriaGLN)

			_, err := contract.FinalDisposition(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}

// TestFinalDispositionFromProhibitedExcludesTitularLaboratory es coherente con
// T20: la prohibicion es una potestad exclusivamente regulatoria, y su cierre no
// queda en manos del titular del producto. ADR-001 no lista LABORATORY en T32,
// a diferencia de T31.
//
// El laboratorio de este caso tiene autorizacion de intervencion VALIDA y
// vigente, y el rechazo llega igual: es de la TABLA y no de la autorizacion.
func TestFinalDispositionFromProhibitedExcludesTitularLaboratory(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateProhibido, "GLN:"+drogueriaGLN)
	authorizeDisposal(t, stub, "GLN:"+labGLN)

	_, err := contract.FinalDisposition(
		testContext(stub, labMSP, RoleOperator), disposalRequest())
	requireCode(t, err, cerr.InvalidStateTransition)

	// La autorizacion NO se consumio: el rechazo ocurre antes de la
	// precondicion, en la resolucion de la transicion.
	authorization, found, err := readLabIntervention(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)
	if !found || authorization.Estado != LabInterventionActiva {
		t.Fatalf("un rechazo de transicion no debe consumir la autorizacion: %+v", authorization)
	}
}

// TestFinalDispositionAsymmetryWithRestock lee juntas T31 y T27, que dicen algo
// que ninguna de las dos dice sola: desde RETIRADO_MERCADO el custodio PUEDE
// disponer finalmente la unidad pero NO puede reingresarla a stock.
//
// Destruir lo retirado es una salida siempre admisible; devolverlo a la
// circulacion es una decision que solo el titular o la autoridad pueden tomar.
// Si alguna de las dos filas cambiara, este test lo expone.
func TestFinalDispositionAsymmetryWithRestock(t *testing.T) {
	custodio := "GLN:" + drogueriaGLN

	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateRetiradoMercado, custodio)
	_, err := contract.Restock(
		testContext(stub, drogueriaMSP, RoleOperator), restockRequest())
	requireCode(t, err, cerr.InvalidStateTransition)

	stub, contract = labInterventionFixture(t)
	seedUnit(t, stub, domain.StateRetiradoMercado, custodio)
	view, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateDispuestoFinal {
		t.Fatalf("estado = %s, se esperaba DISPUESTO_FINAL", view.Estado)
	}
}

// TestDisposedUnitBlocksEveryOperation es el criterio "ninguna operacion
// posterior es valida" del issue, y se comprueba sobre TODAS las operaciones de
// escritura sobre una unidad, no sobre una muestra.
//
// El bloqueo no tiene regla propia: ADR-001 no declara ninguna fila con
// DISPUESTO_FINAL como estado de origen, y requireTransition las rechaza todas.
func TestDisposedUnitBlocksEveryOperation(t *testing.T) {
	// `custodio` no es decorativo: una operacion cuya comprobacion de identidad
	// no pasa nunca llega a requireTransition, y el test estaria comprobando el
	// rechazo equivocado. Dispense es el caso: con la unidad en custodia de la
	// drogueria, la farmacia recibe UNAUTHORIZED_CUSTODIAN y el estado terminal
	// no se ejercita. Es el mismo error que EXT-2 tuvo que corregir.
	operaciones := []struct {
		nombre   string
		custodio string
		invocar  func(*SNTContract, *mockStub) error
	}{
		{nombre: "DispatchTransfer", invocar: func(c *SNTContract, stub *mockStub) error {
			withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
			_, err := c.DispatchTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		}},
		{nombre: "Dispense", custodio: "GLN:" + farmaciaGLN, invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.Dispense(
				testContext(stub, farmaciaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		}},
		{nombre: "Quarantine", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.Quarantine(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "ReleaseQuarantine", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReleaseQuarantine(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "ReportExpired", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReportExpired(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "ReportStolen", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReportStolen(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "ReportLost", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReportLost(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "ReportDamaged", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReportDamaged(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "WithdrawFromMarket", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.WithdrawFromMarket(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
			return err
		}},
		{nombre: "ProhibitProduct", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ProhibitProduct(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
			return err
		}},
		{nombre: "ReturnProduct", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.ReturnProduct(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "Restock", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.Restock(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
		{nombre: "FinalDisposition", invocar: func(c *SNTContract, stub *mockStub) error {
			_, err := c.FinalDisposition(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
			return err
		}},
	}

	for _, caso := range operaciones {
		t.Run(caso.nombre, func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
			custodio := caso.custodio
			if custodio == "" {
				custodio = "GLN:" + drogueriaGLN
			}
			seedUnit(t, stub, domain.StateDispuestoFinal, custodio)

			err := caso.invocar(contract, stub)
			if err == nil {
				t.Fatal("DISPUESTO_FINAL es terminal: la operacion no debe tener exito")
			}
			requireCode(t, err, cerr.InvalidStateTransition)

			// El codigo solo no alcanza: se comprueba que el rechazo venga de la
			// maquina de estados sobre el estado terminal y no de otra
			// validacion que devuelva el mismo codigo.
			parsed, ok := cerr.Parse(err)
			if !ok || parsed.Details["estado"] != string(domain.StateDispuestoFinal) {
				t.Fatalf("el rechazo debe provenir del estado terminal: %v", err)
			}
		})
	}
}

// TestFinalDispositionRequiresDispositionAuthorization comprueba que la
// autorizacion de intervencion es POR OPERACION. La distincion importa mas aca
// que en las otras dos operaciones que la admiten, porque la disposicion final
// es irreversible: una autorizacion emitida para retirar del mercado no puede
// habilitar la destruccion de la unidad.
func TestFinalDispositionRequiresDispositionAuthorization(t *testing.T) {
	t.Run("sin autorizacion", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)

		_, err := contract.FinalDisposition(
			testContext(stub, labMSP, RoleOperator), disposalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion de otra operacion", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)
		authorizeWithdrawal(t, stub, "GLN:"+labGLN)

		_, err := contract.FinalDisposition(
			testContext(stub, labMSP, RoleOperator), disposalRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion consumida con los dos marcadores", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)
		authorizeDisposal(t, stub, "GLN:"+labGLN)

		stub.txID = "tx-disposicion-laboratorio"
		_, err := contract.FinalDisposition(
			testContext(stub, labMSP, RoleOperator), disposalRequest())
		requireNoError(t, err)

		authorization, found, err := readLabIntervention(
			testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
		requireNoError(t, err)
		if !found || authorization.Estado != LabInterventionConsumed {
			t.Fatalf("la autorizacion deberia quedar CONSUMIDA: %+v", authorization)
		}

		markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
		requireNoError(t, err)
		if stub.privateData[implicitCollection(labMSP)][markerKey] == nil {
			t.Fatal("falta el marcador del laboratorio designado")
		}
		if stub.privateData[implicitCollection(anmatMSP)][markerKey] == nil {
			t.Fatal("falta el marcador de la organizacion regulatoria")
		}
	})
}

// TestFinalDispositionByRegulatorWritesMarker: el coendoso regulatorio no se
// apoya en la firma de creador (ADR-007, punto 6.d).
func TestFinalDispositionByRegulatorWritesMarker(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateProhibido, "GLN:"+drogueriaGLN)

	stub.txID = "tx-disposicion-anmat"
	_, err := contract.FinalDisposition(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
	requireNoError(t, err)
	requireRegulatoryMarker(t, stub, opFinalDisposition)
}

// TestFinalDispositionRequiresMotivo: el motivo es donde viaja la causa
// regulatoria del asiento, incluida la referencia a la normativa de residuos
// peligrosos que el issue pide documentar. El contrato no tiene un campo
// dedicado y agregarlo seria un cambio MINOR que una issue de implementacion no
// puede hacer.
func TestFinalDispositionRequiresMotivo(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateVencido, "GLN:"+drogueriaGLN)

	_, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)
}

// TestFinalDispositionIsAuditableByRegulator cubre el criterio "la ANMAT puede
// auditar todas las disposiciones finales". No necesita una operacion nueva: las
// lecturas publicas del canal no son restringibles (ADR-005), de modo que la
// autoridad lee el estado y el historial de cualquier unidad.
func TestFinalDispositionIsAuditableByRegulator(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateVencido, "GLN:"+drogueriaGLN)

	stub.txID = "tx-disposicion-auditable"
	_, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
	requireNoError(t, err)

	ctx := testContext(stub, anmatMSP, RoleAuditor)
	view, err := contract.ReadUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if view.Estado != domain.StateDispuestoFinal {
		t.Fatalf("estado leido = %s", view.Estado)
	}
	history, err := contract.GetUnitHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if len(history) == 0 {
		t.Fatal("la autoridad debe poder leer el historial de la disposicion")
	}
}
