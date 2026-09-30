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
		// El motivo de un caso de uso real cita la norma y el acto; el chaincode
		// solo exige que no este vacio (ver TestFinalDispositionRequiresMotivo).
		Motivo: "destruccion autorizada, Ley 24.051 Anexo I Y3, acta 118/2026",
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

// TestDisposedUnitCannotLeaveItsState es el criterio "ninguna operacion
// posterior es valida" del issue, con la precision que el review de #118 exigio:
// la garantia de ADR-001 es que ninguna TRANSICION declara DISPUESTO_FINAL como
// estado de origen, y por lo tanto la unidad no puede salir de ese estado. NO es
// que toda escritura publica devuelva el mismo codigo.
//
// La version anterior de este test recorria trece operaciones y afirmaba
// INVALID_STATE_TRANSITION para todas, omitiendo justamente las cuatro que
// devuelven otra cosa. Ahora estan las diecisiete escrituras publicas sobre la
// unidad, cada una con lo que realmente hace, y la invariante que se comprueba
// en todas es la unica que ADR-001 garantiza: el estado no se mueve.
//
// Los codigos distintos no son un defecto: cada operacion valida lo suyo antes
// de llegar a la maquina de estados, y el codigo que devuelve describe la
// primera condicion que falla.
func TestDisposedUnitCannotLeaveItsState(t *testing.T) {
	// `custodio` no es decorativo: una operacion cuya comprobacion de identidad
	// no pasa nunca llega a requireTransition, y el test estaria comprobando el
	// rechazo equivocado. Dispense es el caso: con la unidad en custodia de la
	// drogueria, la farmacia recibe UNAUTHORIZED_CUSTODIAN y el estado terminal
	// no se ejercita. Es el mismo error que EXT-2 tuvo que corregir.
	//
	// `codigo` vacio significa que la operacion NO falla: es el caso de
	// AuthorizeLabIntervention, comentado abajo.
	operaciones := []struct {
		nombre   string
		custodio string
		codigo   cerr.Code
		invocar  func(*SNTContract, *mockStub) error
	}{
		// Las trece que llegan a la maquina de estados.
		{nombre: "DispatchTransfer", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
				_, err := c.DispatchTransfer(
					testContext(stub, drogueriaMSP, RoleOperator),
					DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			}},
		{nombre: "Dispense", custodio: "GLN:" + farmaciaGLN, codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.Dispense(
					testContext(stub, farmaciaMSP, RoleOperator),
					UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			}},
		{nombre: "Quarantine", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.Quarantine(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "ReleaseQuarantine", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReleaseQuarantine(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "ReportExpired", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReportExpired(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "ReportStolen", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReportStolen(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "ReportLost", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReportLost(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "ReportDamaged", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReportDamaged(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "WithdrawFromMarket", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.WithdrawFromMarket(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
				return err
			}},
		{nombre: "ProhibitProduct", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ProhibitProduct(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin), disposalRequest())
				return err
			}},
		{nombre: "ReturnProduct", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReturnProduct(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "Restock", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.Restock(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},
		{nombre: "FinalDisposition", codigo: cerr.InvalidStateTransition,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.FinalDisposition(testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
				return err
			}},

		// Las cuatro que NO llegan a la maquina de estados, y que la version
		// anterior de este test omitia.
		//
		// ReceiveTransfer y RejectTransfer comprueban primero que la unidad este
		// EN_TRANSITO: una unidad dispuesta no lo esta, y NOT_IN_TRANSIT describe
		// esa condicion mejor que un rechazo de transicion.
		{nombre: "ReceiveTransfer", custodio: "GLN:" + farmaciaGLN, codigo: cerr.NotInTransit,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.ReceiveTransfer(
					testContext(stub, farmaciaMSP, RoleOperator),
					UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			}},
		{nombre: "RejectTransfer", custodio: "GLN:" + farmaciaGLN, codigo: cerr.NotInTransit,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.RejectTransfer(
					testContext(stub, farmaciaMSP, RoleOperator), disposalRequest())
				return err
			}},
		// RegisterUnit no intenta una transicion: intenta crear la clave, que ya
		// existe. UNIT_ALREADY_EXISTS es lo que corresponde, y ademas es lo que
		// impide reciclar una unidad dispuesta registrandola de nuevo.
		{nombre: "RegisterUnit", codigo: cerr.UnitAlreadyExists,
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.RegisterUnit(
					testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
				return err
			}},
		// AuthorizeLabIntervention NO falla, y queda documentado como limite del
		// prototipo en el contrato: la autoridad puede emitir una autorizacion de
		// intervencion sobre una unidad ya dispuesta. Es inocua -- ejercerla
		// exige una transicion que ADR-001 no declara, de modo que la
		// autorizacion nunca puede consumirse -- pero deja un asiento que
		// promete algo irrealizable. Bloquearla seria una regla nueva que ninguna
		// ADR decide, y EXT-8 no la inventa.
		{nombre: "AuthorizeLabIntervention", codigo: "",
			invocar: func(c *SNTContract, stub *mockStub) error {
				_, err := c.AuthorizeLabIntervention(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					AuthorizeLabInterventionRequest{
						GTIN: validGTIN, NumeroSerie: validSerial,
						Laboratorio: "GLN:" + labGLN,
						Operacion:   LabOpFinalDisposition,
						Motivo:      "autorizacion sobre unidad ya dispuesta",
						ExpiraEn:    "2027-01-01T00:00:00Z",
					})
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
			if caso.codigo == "" {
				requireNoError(t, err)
			} else {
				requireCode(t, err, caso.codigo)
			}

			// La invariante que ADR-001 SI garantiza, y la unica que vale para
			// las diecisiete: el estado no se mueve.
			view, readErr := contract.ReadUnit(
				testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
			requireNoError(t, readErr)
			if view.Estado != domain.StateDispuestoFinal {
				t.Fatalf("estado = %s: DISPUESTO_FINAL es terminal y no admite salida", view.Estado)
			}
		})
	}
}

// TestDisposedUnitRejectsTransitionsFromTheStateMachine complementa al anterior
// sobre las operaciones que SI llegan a la maquina de estados: comprueba que su
// rechazo provenga del estado terminal y no de otra validacion que devuelva el
// mismo codigo. Sin esta asercion, un UNAUTHORIZED_CUSTODIAN mal escrito o un
// INVALID_STATE_TRANSITION de otra causa pasarian por cobertura del criterio.
func TestDisposedUnitRejectsTransitionsFromTheStateMachine(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateDispuestoFinal, "GLN:"+drogueriaGLN)

	_, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
	requireCode(t, err, cerr.InvalidStateTransition)

	parsed, ok := cerr.Parse(err)
	if !ok || parsed.Details["estado"] != string(domain.StateDispuestoFinal) {
		t.Fatalf("el rechazo debe provenir del estado terminal: %v", err)
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

// TestFinalDispositionRequiresMotivo comprueba lo unico que el chaincode puede
// comprobar sobre el motivo: que no este vacio.
//
// El review de #118 señalo con razon que este test NO demuestra trazabilidad
// normativa: acepta cualquier texto. Y no puede demostrarla -- exigir que el
// motivo cite la Ley 24.051 seria una condicion de rechazo nueva que ningun ADR
// decide, y un texto libre no es verificable por el chaincode. La referencia
// normativa (Ley 24.051, Anexo I, categoria Y3, con URL oficial) queda donde
// corresponde: declarada en el contrato y en el godoc de la operacion, que son
// artefactos versionados, y no afirmada por un fixture de test.
func TestFinalDispositionRequiresMotivo(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateVencido, "GLN:"+drogueriaGLN)

	_, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)
}

// TestDisposedUnitIsReadableByRegulator cubre la auditoria POR UNIDAD: partiendo
// de un GTIN y un numero de serie, la autoridad lee estado e historial sin
// autorizacion alguna, porque las lecturas publicas del canal no son
// restringibles (ADR-005).
//
// El alcance GLOBAL -- enumerar el conjunto -- lo cubre
// TestDisposedUnitsAreEnumerableByRegulator, abajo. Los dos hacen falta: este
// prueba que la traza de una unidad conocida es legible, y el otro que la
// autoridad puede DESCUBRIR el conjunto sin conocer las unidades de antemano,
// que es lo que el criterio de #63 pide.
func TestDisposedUnitIsReadableByRegulator(t *testing.T) {
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

// TestDisposedUnitsAreEnumerableByRegulator cierra el criterio de #63 en su
// forma global: "la ANMAT puede auditar TODAS las disposiciones finales".
//
// Hasta el merge de CC-9 (#112) esto no era demostrable -- no habia consulta por
// estado -- y el criterio estaba registrado como pendiente. Con
// QueryUnitsByState en develop si lo es, y el review de #118 tuvo razon en
// pedirlo: el supuesto de que faltaba dejo de ser cierto al rebasar.
//
// Lo que el test verifica no es solo que la consulta responda: es que
// FinalDisposition MANTENGA el indice UnitByState. La operacion no lo toca de
// forma explicita -- escribe por putUnit, que es el unico camino de escritura y
// el que sostiene el indice --, y justamente por eso conviene probarlo: si
// alguna operacion futura escribiera por fuera de putUnit, el indice quedaria
// desincronizado y la autoridad enumeraria un conjunto incompleto sin que nada
// mas fallara.
//
// Se disponen DOS unidades y se deja una tercera en otro estado, de modo que el
// test distingue "devuelve todo" de "devuelve lo que corresponde".
func TestDisposedUnitsAreEnumerableByRegulator(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	custodio := "GLN:" + drogueriaGLN

	// La primera unidad es la del fixture: se dispone desde VENCIDO (T28).
	seedUnit(t, stub, domain.StateVencido, custodio)
	stub.txID = "tx-disposicion-1"
	_, err := contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator), disposalRequest())
	requireNoError(t, err)

	// La segunda, con otro numero de serie, desde DEVUELTO (T33).
	const segundaSerie = "SN-0002-ABCD"
	seedUnitWithSerial(t, stub, segundaSerie, domain.StateDevuelto, custodio)
	stub.txID = "tx-disposicion-2"
	_, err = contract.FinalDisposition(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: segundaSerie, Motivo: disposalRequest().Motivo})
	requireNoError(t, err)

	// La tercera NO se dispone: queda EN_CUARENTENA para comprobar que la
	// consulta discrimina por estado y no devuelve el universo.
	const terceraSerie = "SN-0003-ABCD"
	seedUnitWithSerial(t, stub, terceraSerie, domain.StateEnCuarentena, custodio)

	dispuestas := queryState(t, stub, domain.StateDispuestoFinal)
	if len(dispuestas) != 2 {
		t.Fatalf("la autoridad enumero %d unidades dispuestas, se esperaban 2: %+v", len(dispuestas), dispuestas)
	}
	series := map[string]bool{}
	for _, unidad := range dispuestas {
		if unidad.Estado != domain.StateDispuestoFinal {
			t.Fatalf("la consulta devolvio una unidad en %s", unidad.Estado)
		}
		series[unidad.NumeroSerie] = true
	}
	if !series[validSerial] || !series[segundaSerie] {
		t.Fatalf("faltan unidades dispuestas en la enumeracion: %+v", series)
	}
	if series[terceraSerie] {
		t.Fatal("la unidad EN_CUARENTENA no debe figurar entre las dispuestas")
	}

	// Y la que quedo fuera sigue enumerable en SU estado: la disposicion de las
	// otras dos no rompio el resto del indice.
	if cuarentena := queryState(t, stub, domain.StateEnCuarentena); len(cuarentena) != 1 {
		t.Fatalf("EN_CUARENTENA enumero %d unidades, se esperaba 1", len(cuarentena))
	}
}
