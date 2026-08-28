package snt

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Segunda farmacia del dataset de pruebas. Existe para poder ejercitar un par
// de organizaciones SIN coleccion definida: PHARMACY -> PHARMACY no esta
// autorizado en ninguna direccion, de modo que ADR-006 no define coleccion para
// ese par.
const (
	farmaciaDosMSP = "FarmaciaDosMSP"
	farmaciaDosGLN = "7791234500062"
)

func transferFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
	registerOrg(t, stub, farmaciaDosMSP, farmaciaDosGLN, domain.AgentPharmacy)
	registerOrg(t, stub, "CentroMedicoMSP", "7791234500055", domain.AgentHealthcare)

	contract := new(SNTContract)
	_, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireNoError(t, err)
	return stub, contract
}

func withTransient(stub *mockStub, entries map[string]any) {
	stub.transient = map[string][]byte{}
	for key, value := range entries {
		raw, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		stub.transient[key] = raw
	}
}

func dispatchTransient(destino string) map[string]any {
	return map[string]any{
		transientDestinatario: destinatarioTransient{Destino: destino},
		transientCommercial: CommercialData{
			NumeroRemito: "R-0001-2026", NumeroFactura: "A-0001-00001234", Cantidad: 1,
		},
	}
}

// dispatchToDrugstore ejecuta el despacho LABORATORY -> DRUGSTORE que usan los
// tests de recepcion y rechazo.
func dispatchToDrugstore(t *testing.T, stub *mockStub, contract *SNTContract) {
	t.Helper()
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}
}

// --- Nombre de la coleccion del par (ADR-006, punto 1) ----------------------

// TestPairCollectionNameIsDirectionIndependent fija la propiedad que hace que
// la transferencia y la devolucion en sentido inverso resuelvan a la MISMA
// coleccion: el nombre se arma con los mspId ordenados lexicograficamente, no
// por origen -> destino.
func TestPairCollectionNameIsDirectionIndependent(t *testing.T) {
	forward := pairCollectionName(labMSP, drogueriaMSP)
	backward := pairCollectionName(drogueriaMSP, labMSP)

	if forward != backward {
		t.Fatalf("el nombre depende del sentido del flujo: %q vs %q", forward, backward)
	}
	if forward != "transfer_DrogueriaMSP_LabMSP" {
		t.Fatalf("nombre de coleccion = %q", forward)
	}
	if pairCollectionName(labMSP, farmaciaMSP) == forward {
		t.Fatal("pares distintos deben resolver a colecciones distintas")
	}
}

// --- Despacho (T02/T03) -----------------------------------------------------

func TestDispatchTransferHappyPath(t *testing.T) {
	stub, contract := transferFixture(t)
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))

	view, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	if view.Estado != domain.StateEnTransito {
		t.Fatalf("estado = %s, se esperaba EN_TRANSITO", view.Estado)
	}
	// CustodioActual NO cambia durante el transito: permanece en el emisor
	// hasta que la recepcion se confirma (ADR-004).
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodioActual = %q; durante el transito debe seguir siendo el emisor", view.CustodioActual)
	}

	// El destinatario declarado NO aparece en la vista publica: revela una
	// relacion emisor -> receptor que puede no consumarse (ADR-004).
	encoded, err := json.Marshal(view)
	requireNoError(t, err)
	if contains(string(encoded), drogueriaGLN) {
		t.Fatalf("la vista publica expone el destinatario declarado: %s", encoded)
	}

	// El registro de operacion vive en la coleccion privada del par, con el
	// ruleId y la schemaVersion de la matriz que autorizo el par.
	collection := pairCollectionName(labMSP, drogueriaMSP)
	op, found, err := readActiveTransferOperation(
		testContext(stub, labMSP, RoleOperator), collection, validGTIN, validSerial)
	requireNoError(t, err)
	if !found {
		t.Fatal("el despacho no escribio el registro de operacion activa")
	}
	if op.DestinatarioPendiente != "GLN:"+drogueriaGLN || op.Emisor != "GLN:"+labGLN {
		t.Fatalf("contrapartes del registro de operacion: %+v", op)
	}
	if op.RuleID != "LABORATORY_TO_DRUGSTORE" || op.SchemaVersion != "1.0.0" {
		t.Fatalf("el registro no persistio la regla aplicada: ruleId=%q schemaVersion=%q",
			op.RuleID, op.SchemaVersion)
	}
	if op.NumeroRemito != "R-0001-2026" || op.NumeroFactura != "A-0001-00001234" || op.Cantidad != 1 {
		t.Fatalf("datos documentales no persistidos: %+v", op)
	}
	if op.TxIDDespacho != stub.GetTxID() {
		t.Fatalf("txIdDespacho = %q", op.TxIDDespacho)
	}
}

// TestDispatchTransferArmsTransitPolicy verifica que el despacho arme la
// politica de transito AND(emisor, receptor declarado) sobre la clave de la
// unidad, sin rama alternativa (ADR-007, punto 6.b).
func TestDispatchTransferArmsTransitPolicy(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)

	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 2 {
		t.Fatalf("la politica de transito debe exigir a las DOS partes; exige %v", orgs)
	}
	if orgs[0] != drogueriaMSP || orgs[1] != labMSP {
		t.Fatalf("politica de transito = %v, se esperaba AND(%s, %s)", orgs, labMSP, drogueriaMSP)
	}
	for _, org := range orgs {
		if org == anmatMSP {
			t.Fatal("ningun tercero puede sustituir a una de las partes del transito")
		}
	}
}

func TestDispatchTransferRejections(t *testing.T) {
	cases := []struct {
		name      string
		mspID     string
		role      string
		transient map[string]any
		setup     func(t *testing.T, stub *mockStub, contract *SNTContract)
		want      cerr.Code
	}{
		{
			name: "invocador no es el custodio actual", mspID: drogueriaMSP, role: RoleOperator,
			transient: dispatchTransient("GLN:" + farmaciaGLN),
			want:      cerr.UnauthorizedCustodian,
		},
		{
			name: "organizacion no registrada", mspID: "OrgFantasmaMSP", role: RoleOperator,
			transient: dispatchTransient("GLN:" + drogueriaGLN),
			want:      cerr.OrgNotRegistered,
		},
		{
			name: "rol no habilitado", mspID: labMSP, role: RoleAuditor,
			transient: dispatchTransient("GLN:" + drogueriaGLN),
			want:      cerr.UnauthorizedRole,
		},
		{
			name: "sin transient de destinatario", mspID: labMSP, role: RoleOperator,
			transient: map[string]any{transientCommercial: CommercialData{
				NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1}},
			want: cerr.InvalidRequest,
		},
		{
			name: "transient de destinatario sin campo destino", mspID: labMSP, role: RoleOperator,
			transient: map[string]any{
				transientDestinatario: destinatarioTransient{},
				transientCommercial:   CommercialData{NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1},
			},
			want: cerr.InvalidRequest,
		},
		{
			name: "sin transient comercial", mspID: labMSP, role: RoleOperator,
			transient: map[string]any{transientDestinatario: destinatarioTransient{Destino: "GLN:" + drogueriaGLN}},
			want:      cerr.InvalidRequest,
		},
		{
			name: "transient comercial incompleto", mspID: labMSP, role: RoleOperator,
			transient: map[string]any{
				transientDestinatario: destinatarioTransient{Destino: "GLN:" + drogueriaGLN},
				transientCommercial:   CommercialData{NumeroRemito: "R-1", Cantidad: 1},
			},
			want: cerr.InvalidRequest,
		},
		{
			name: "destino no registrado", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient("GLN:7791234500079"),
			want:      cerr.OrgNotRegistered,
		},
		{
			name: "destino con identificador mal formado", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient("7791234500024"),
			want:      cerr.OrgNotRegistered,
		},
		{
			name: "destino no custodial", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient(anmatMSP),
			want:      cerr.InvalidDestination,
		},
		{
			name: "destino igual al emisor", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient("GLN:" + labGLN),
			want:      cerr.InvalidDestination,
		},
		{
			name: "destino inactivo", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient("GLN:" + drogueriaGLN),
			setup: func(t *testing.T, stub *mockStub, contract *SNTContract) {
				_, err := contract.SetOrganizationActive(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					SetOrganizationActiveRequest{MSPID: drogueriaMSP, Active: false})
				requireNoError(t, err)
			},
			want: cerr.OrgInactive,
		},
		{
			name: "unidad ya despachada", mspID: labMSP, role: RoleOperator,
			transient: dispatchTransient("GLN:" + drogueriaGLN),
			setup: func(t *testing.T, stub *mockStub, contract *SNTContract) {
				dispatchToDrugstore(t, stub, contract)
			},
			want: cerr.InvalidStateTransition,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := transferFixture(t)
			if tc.setup != nil {
				tc.setup(t, stub, contract)
			}
			withTransient(stub, tc.transient)
			_, err := contract.DispatchTransfer(
				testContext(stub, tc.mspID, tc.role),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, tc.want)
		})
	}
}

// TestDispatchTransferRejectsUnauthorizedPair es la validacion normativa
// distribuida: el par origen -> destino sale de la matriz de DES-3, no de
// condicionales del chaincode.
func TestDispatchTransferRejectsUnauthorizedPair(t *testing.T) {
	cases := []struct {
		name      string
		emitter   string
		emitterID string
		destino   string
		razon     string
	}{
		{
			// Prohibicion explicita: venta hacia un eslabon superior.
			name: "farmacia hacia drogueria", emitter: farmaciaMSP, emitterID: farmaciaGLN,
			destino: "GLN:" + drogueriaGLN, razon: "PHARMACY_TO_UPSTREAM_AGENT",
		},
		{
			// Par no declarado: rechazo por defaultDecision.
			name: "drogueria hacia laboratorio", emitter: drogueriaMSP, emitterID: drogueriaGLN,
			destino: "GLN:" + labGLN, razon: domain.DefaultDenyReason,
		},
		{
			name: "farmacia hacia farmacia", emitter: farmaciaMSP, emitterID: farmaciaGLN,
			destino: "GLN:" + farmaciaDosGLN, razon: domain.DefaultDenyReason,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := transferFixture(t)
			// La unidad se coloca bajo custodia del emisor del caso.
			seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+tc.emitterID)
			withTransient(stub, dispatchTransient(tc.destino))

			_, err := contract.DispatchTransfer(
				testContext(stub, tc.emitter, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, cerr.TransferNotAuthorized)

			parsed, ok := cerr.Parse(err)
			if !ok || parsed.Details["razon"] != tc.razon {
				t.Fatalf("razon de rechazo = %v, se esperaba %q", parsed.Details["razon"], tc.razon)
			}
		})
	}
}

// TestDispatchTransferAcceptsDestinationByMSPID cubre que el contrato admita
// declarar el destino por mspId ademas de por identificador canonico.
func TestDispatchTransferAcceptsDestinationByMSPID(t *testing.T) {
	stub, contract := transferFixture(t)
	withTransient(stub, dispatchTransient(drogueriaMSP))

	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	collection := pairCollectionName(labMSP, drogueriaMSP)
	op, found, err := readActiveTransferOperation(
		testContext(stub, labMSP, RoleOperator), collection, validGTIN, validSerial)
	requireNoError(t, err)
	if !found || op.DestinatarioPendiente != "GLN:"+drogueriaGLN {
		t.Fatalf("el destino declarado por mspId no se persistio como identificador canonico: %+v", op)
	}
}

// --- Recepcion (T04) --------------------------------------------------------

func TestReceiveTransferHappyPath(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	view, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	if view.Estado != domain.StateEnCustodia {
		t.Fatalf("estado = %s, se esperaba EN_CUSTODIA", view.Estado)
	}
	if view.CustodioActual != "GLN:"+drogueriaGLN {
		t.Fatalf("custodioActual = %q, se esperaba el receptor", view.CustodioActual)
	}
	if view.UltimaActualizacion != "2026-08-27T12:00:00Z" {
		t.Fatalf("ultimaActualizacion = %q; debe salir de GetTxTimestamp()", view.UltimaActualizacion)
	}

	// Ciclo de vida activo/cerrado: la clave activa se elimina y el registro
	// historico se conserva (ADR-004, regla 4; ADR-006, punto 4).
	ctx := testContext(stub, drogueriaMSP, RoleOperator)
	collection := pairCollectionName(labMSP, drogueriaMSP)
	if _, found, err := readActiveTransferOperation(ctx, collection, validGTIN, validSerial); err != nil || found {
		t.Fatal("la clave del registro de operacion activa deberia haberse eliminado")
	}

	histKey, err := transferOpKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	raw := stub.privateData[collection][histKey]
	if raw == nil {
		t.Fatal("la recepcion no conservo el registro historico de la operacion")
	}
	var historical TransferOperation
	requireNoError(t, json.Unmarshal(raw, &historical))
	if historical.MotivoCierre != closureReception || historical.CerradaEn == "" {
		t.Fatalf("registro historico sin cierre documentado: %+v", historical)
	}
}

// TestReceiveTransferRestoresRestingPolicy cubre la primera de las tres salidas
// de EN_TRANSITO (ADR-007, punto 6.c): el custodio registrado pasa a ser el
// receptor, y la politica vuelve a exigir solo a su organizacion.
func TestReceiveTransferRestoresRestingPolicy(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	_, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != drogueriaMSP {
		t.Fatalf("politica de reposo tras la recepcion = %v, se esperaba unicamente %s", orgs, drogueriaMSP)
	}
}

// TestReceiveTransferStoresReceptionDocumentation cubre el transient opcional
// `commercial` de la recepcion.
func TestReceiveTransferStoresReceptionDocumentation(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	withTransient(stub, map[string]any{
		transientCommercial: CommercialData{
			NumeroRemito: "R-0001-2026", NumeroFactura: "A-0001-00001234", Cantidad: 1,
		},
	})
	_, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	collection := pairCollectionName(labMSP, drogueriaMSP)
	histKey, err := transferOpKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	var historical TransferOperation
	requireNoError(t, json.Unmarshal(stub.privateData[collection][histKey], &historical))
	if historical.Recepcion == nil || historical.Recepcion.NumeroRemito != "R-0001-2026" {
		t.Fatalf("la confirmacion documental de recepcion no se conservo: %+v", historical.Recepcion)
	}
}

func TestReceiveTransferRejections(t *testing.T) {
	t.Run("unidad que no esta en transito", func(t *testing.T) {
		stub, contract := transferFixture(t)
		_, err := contract.ReceiveTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.NotInTransit)
	})

	// Idempotencia por rechazo de estado: una segunda recepcion sobre una unidad
	// que ya salio de EN_TRANSITO falla con NOT_IN_TRANSIT; no hay deduplicacion
	// por identificador de operacion (docs/alcance-prototipo.md).
	t.Run("segunda recepcion de la misma unidad", func(t *testing.T) {
		stub, contract := transferFixture(t)
		dispatchToDrugstore(t, stub, contract)
		ctx := testContext(stub, drogueriaMSP, RoleOperator)
		req := UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}

		_, err := contract.ReceiveTransfer(ctx, req)
		requireNoError(t, err)
		_, err = contract.ReceiveTransfer(ctx, req)
		requireCode(t, err, cerr.NotInTransit)
	})

	// El invocador no puede ser el destinatario declarado porque no existe
	// relacion de transferencia autorizada con el emisor en ninguna direccion:
	// ADR-006 no define coleccion para ese par.
	t.Run("invocador sin par autorizado con el emisor", func(t *testing.T) {
		stub, contract := transferFixture(t)
		seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
		withTransient(stub, dispatchTransient("GLN:7791234500055"))
		_, err := contract.DispatchTransfer(
			testContext(stub, farmaciaMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
		stub.transient = map[string][]byte{}

		_, err = contract.ReceiveTransfer(
			testContext(stub, farmaciaDosMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.ReceiverMismatch)
	})

	// El caso dificil: el invocador SI tiene coleccion definida con el emisor
	// -- LABORATORY -> PHARMACY esta autorizado --, pero la operacion activa
	// vive en la coleccion del par Lab/Drogueria. Mirando solo el contenido
	// privado, la ausencia de la clave es indistinguible de una diseminacion
	// pendiente; el hash publico la vuelve concluyente y el contrato exige
	// RECEIVER_MISMATCH, no un INTERNAL_ERROR reintentable.
	t.Run("invocador con par autorizado pero ajeno a la operacion activa", func(t *testing.T) {
		stub, contract := transferFixture(t)
		dispatchToDrugstore(t, stub, contract)

		if !pairCollectionNameExistsForTest(t, stub, labMSP, farmaciaMSP) {
			t.Fatal("el caso exige que ADR-006 defina coleccion para el par laboratorio/farmacia")
		}

		_, err := contract.ReceiveTransfer(
			testContext(stub, farmaciaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.ReceiverMismatch)

		// Y no debe ofrecerse como reintentable: reintentar no lo va a arreglar.
		parsed, ok := cerr.Parse(err)
		if !ok {
			t.Fatalf("error sin el formato del contrato: %v", err)
		}
		if parsed.Details["reintentable"] == true {
			t.Fatalf("un receptor equivocado no debe marcarse como reintentable: %+v", parsed.Details)
		}
	})

	t.Run("rol no habilitado", func(t *testing.T) {
		stub, contract := transferFixture(t)
		dispatchToDrugstore(t, stub, contract)
		_, err := contract.ReceiveTransfer(
			testContext(stub, drogueriaMSP, RoleAuditor),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.UnauthorizedRole)
	})
}

// pairCollectionNameExistsForTest confirma que la premisa del caso anterior se
// sostiene: si la matriz dejara de autorizar LABORATORY -> PHARMACY, el test
// pasaria por el camino equivocado y dejaria de probar lo que dice probar.
func pairCollectionNameExistsForTest(t *testing.T, stub *mockStub, a, b string) bool {
	t.Helper()
	ctx := testContext(stub, a, RoleOperator)
	orgA, foundA, err := readOrganization(ctx, a)
	requireNoError(t, err)
	orgB, foundB, err := readOrganization(ctx, b)
	requireNoError(t, err)
	if !foundA || !foundB {
		t.Fatalf("las organizaciones %s y %s deben estar registradas", a, b)
	}
	exists, err := pairCollectionExists(orgA, orgB)
	requireNoError(t, err)
	return exists
}

// TestReceiveTransferRetriesOnUndisseminatedPrivateData cubre la falla
// TRANSITORIA de ADR-006 punto 1: con requiredPeerCount 1, el peer del receptor
// puede no tener todavia el registro de la operacion. Debe distinguirse de un
// rechazo por regla de negocio y no contabilizarse como rechazo esperado en la
// medicion.
func TestReceiveTransferRetriesOnUndisseminatedPrivateData(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	// Se simula que el dato privado todavia no llego al peer del receptor.
	collection := pairCollectionName(labMSP, drogueriaMSP)
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	// Se oculta SOLO el contenido: el hash sigue en el estado publico, que es
	// exactamente lo que ve un peer miembro al que el bloque privado todavia no
	// llego. Si se borrara tambien el hash, el chaincode concluiria -- con
	// razon -- que no hay operacion y devolveria RECEIVER_MISMATCH.
	stored := stub.hidePrivateData(collection, activeKey)

	_, err = contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})

	parsed, ok := cerr.Parse(err)
	if !ok {
		t.Fatalf("error sin el formato del contrato: %v", err)
	}
	if parsed.Code != cerr.InternalError {
		t.Fatalf("codigo = %s; la falla transitoria no debe confundirse con un rechazo de negocio", parsed.Code)
	}
	if parsed.Details["reintentable"] != true {
		t.Fatalf("la falla transitoria debe marcarse como reintentable: %+v", parsed.Details)
	}
	if parsed.Details["causa"] != "PRIVATE_DATA_NOT_DISSEMINATED" {
		t.Fatalf("causa = %v", parsed.Details["causa"])
	}
	// El diagnostico llega por el camino REAL de Fabric -- la lectura privada
	// que falla contra un hash confirmado --, no por una lectura que devolvio
	// vacio. La causa subyacente lo acredita: si el chaincode volviera a
	// apoyarse en un (nil, nil) inexistente en la plataforma, este detalle no
	// estaria.
	if cause, _ := parsed.Details["causaSubyacente"].(string); !strings.Contains(cause, errPvtdataNotAvailable) {
		t.Fatalf("causaSubyacente = %q; se esperaba el error de lectura privada de Fabric", cause)
	}

	// Tras la reconciliacion, el reintento tiene exito.
	stub.privateData[collection][activeKey] = stored
	_, err = contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
}

// TestMockStubReproducesFabricPrivateDataSemantics fija la fidelidad del doble
// de prueba, porque de ella depende que el test anterior pruebe algo.
//
// Fabric no se comporta como un mapa: cuando el hash publico de una clave esta
// confirmado y el contenido privado todavia no llego a este peer, GetPrivateData
// NO devuelve (nil, nil), falla. Si el mock devolviera vacio, el chaincode
// podria depender de un camino que en la red real nunca se recorre y los tests
// de la condicion transitoria pasarian igual.
func TestMockStubReproducesFabricPrivateDataSemantics(t *testing.T) {
	stub := newMockStub()
	const collection, key = "transfer_A_B", "clave"

	// Clave inexistente: ni contenido ni hash. Fabric devuelve vacio sin error.
	value, err := stub.GetPrivateData(collection, key)
	if err != nil || value != nil {
		t.Fatalf("clave inexistente: (%v, %v); se esperaba (nil, nil)", value, err)
	}

	requireNoError(t, stub.PutPrivateData(collection, key, []byte(`{"ok":true}`)))
	value, err = stub.GetPrivateData(collection, key)
	requireNoError(t, err)
	if string(value) != `{"ok":true}` {
		t.Fatalf("lectura tras la escritura = %q", value)
	}

	// Diseminacion pendiente: se va el contenido y queda el hash.
	stored := stub.hidePrivateData(collection, key)
	hash, err := stub.GetPrivateDataHash(collection, key)
	requireNoError(t, err)
	if len(hash) == 0 {
		t.Fatal("el hash publico debe sobrevivir a la diseminacion pendiente")
	}
	if _, err := stub.GetPrivateData(collection, key); err == nil {
		t.Fatal("con hash confirmado y contenido ausente, Fabric falla la lectura privada")
	} else if !strings.Contains(err.Error(), errPvtdataNotAvailable) {
		t.Fatalf("mensaje de error = %q", err.Error())
	}

	// Reconciliacion: vuelve el contenido y la lectura se normaliza.
	requireNoError(t, stub.PutPrivateData(collection, key, stored))
	value, err = stub.GetPrivateData(collection, key)
	requireNoError(t, err)
	if string(value) != string(stored) {
		t.Fatalf("lectura tras la reconciliacion = %q", value)
	}

	// Cierre de la operacion: DelPrivateData borra contenido Y hash, de modo que
	// una operacion cerrada no puede confundirse con una diseminacion pendiente.
	requireNoError(t, stub.DelPrivateData(collection, key))
	value, err = stub.GetPrivateData(collection, key)
	if err != nil || value != nil {
		t.Fatalf("clave eliminada: (%v, %v); se esperaba (nil, nil)", value, err)
	}
}

// TestRejectTransferByEmitterRetriesOnUndisseminatedPrivateData cubre la misma
// condicion transitoria en el OTRO camino de lectura: el del emisor, que no
// conoce al destinatario declarado y recorre sus colecciones candidatas con
// findActiveTransferOperation.
//
// Ahi la confusion seria peor que un INTERNAL_ERROR generico: una coleccion con
// hash confirmado cuyo contenido no llego no debe descartarse como "no hay nada
// aca" y hacer que el recorrido termine acusando una inconsistencia del ledger.
func TestRejectTransferByEmitterRetriesOnUndisseminatedPrivateData(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	collection := pairCollectionName(labMSP, drogueriaMSP)
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	stored := stub.hidePrivateData(collection, activeKey)

	_, err = contract.RejectTransfer(
		testContext(stub, labMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Error de entrega."})

	parsed, ok := cerr.Parse(err)
	if !ok {
		t.Fatalf("error sin el formato del contrato: %v", err)
	}
	if parsed.Code != cerr.InternalError {
		t.Fatalf("codigo = %s", parsed.Code)
	}
	if parsed.Details["reintentable"] != true || parsed.Details["causa"] != "PRIVATE_DATA_NOT_DISSEMINATED" {
		t.Fatalf("la falla transitoria del emisor no quedo tipificada: %+v; mensaje: %s",
			parsed.Details, parsed.Message)
	}
	if parsed.Details["coleccion"] != collection {
		t.Fatalf("coleccion = %v, se esperaba %s", parsed.Details["coleccion"], collection)
	}

	// Tras la reconciliacion, el reintento tiene exito.
	requireNoError(t, stub.PutPrivateData(collection, activeKey, stored))
	_, err = contract.RejectTransfer(
		testContext(stub, labMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Error de entrega."})
	requireNoError(t, err)
}

// TestReceiveTransferDetectsDivergentMatrix es la comprobacion cruzada de
// ADR-008 punto 5: el despacho lo endosa solo el emisor, de modo que una matriz
// divergente en ese peer autorizaria un par que ninguna otra organizacion
// contrasto. El peer del receptor re-evalua el par contra SU matriz y lo
// contrasta con el ruleId y la schemaVersion persistidos.
func TestReceiveTransferDetectsDivergentMatrix(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	// Se altera el registro para simular un despacho endosado por un peer cuya
	// matriz embebida difiere de la del receptor.
	ctx := testContext(stub, drogueriaMSP, RoleOperator)
	collection := pairCollectionName(labMSP, drogueriaMSP)
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)

	var op TransferOperation
	requireNoError(t, json.Unmarshal(stub.privateData[collection][activeKey], &op))
	op.RuleID = "REGLA_DE_UNA_MATRIZ_ALTERADA"
	altered, err := json.Marshal(op)
	requireNoError(t, err)
	stub.privateData[collection][activeKey] = altered

	_, err = contract.ReceiveTransfer(ctx, UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.TransferNotAuthorized)

	// La misma comprobacion detecta una schemaVersion divergente.
	op.RuleID = "LABORATORY_TO_DRUGSTORE"
	op.SchemaVersion = "9.9.9"
	altered, err = json.Marshal(op)
	requireNoError(t, err)
	stub.privateData[collection][activeKey] = altered

	_, err = contract.ReceiveTransfer(ctx, UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.TransferNotAuthorized)
}

// --- Rechazo (T05) ----------------------------------------------------------

func TestRejectTransferByDeclaredReceiver(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	view, err := contract.RejectTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Inconsistencia documental."})
	requireNoError(t, err)

	if view.Estado != domain.StateDevuelto {
		t.Fatalf("estado = %s, se esperaba DEVUELTO", view.Estado)
	}
	// La devolucion es un evento unico que no modifica la custodia registrada
	// (ADR-004; ADR-009, punto 1).
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodioActual = %q; el rechazo no revierte ni cambia la custodia", view.CustodioActual)
	}

	collection := pairCollectionName(labMSP, drogueriaMSP)
	histKey, err := transferOpKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	var historical TransferOperation
	requireNoError(t, json.Unmarshal(stub.privateData[collection][histKey], &historical))
	if historical.MotivoCierre != closureRejection {
		t.Fatalf("motivo de cierre = %q", historical.MotivoCierre)
	}
}

// TestRejectTransferByEmitter cubre el camino en que el emisor rechaza: no
// conoce al destinatario declarado sin leer el registro, porque es un dato
// privado que no esta en el estado publico.
func TestRejectTransferByEmitter(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	view, err := contract.RejectTransfer(
		testContext(stub, labMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Error de entrega."})
	requireNoError(t, err)

	if view.Estado != domain.StateDevuelto || view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("estado o custodia inesperados: %+v", view)
	}

	collection := pairCollectionName(labMSP, drogueriaMSP)
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	if stub.privateData[collection][activeKey] != nil {
		t.Fatal("el rechazo del emisor no cerro el registro de operacion activa")
	}
}

// TestRejectTransferRestoresRestingPolicyToEmitter cubre la segunda de las tres
// salidas de EN_TRANSITO: la custodia permanece en el emisor, de modo que la
// politica de reposo debe volver a exigir a SU organizacion (ADR-007, 6.c).
func TestRejectTransferRestoresRestingPolicyToEmitter(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	_, err := contract.RejectTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Rechazo en recepcion."})
	requireNoError(t, err)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo tras el rechazo = %v, se esperaba unicamente el emisor %s", orgs, labMSP)
	}
}

func TestRejectTransferRejections(t *testing.T) {
	t.Run("unidad que no esta en transito", func(t *testing.T) {
		stub, contract := transferFixture(t)
		_, err := contract.RejectTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
		requireCode(t, err, cerr.NotInTransit)
	})

	t.Run("motivo ausente", func(t *testing.T) {
		stub, contract := transferFixture(t)
		dispatchToDrugstore(t, stub, contract)
		_, err := contract.RejectTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("invocador ajeno a la operacion", func(t *testing.T) {
		stub, contract := transferFixture(t)
		seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
		withTransient(stub, dispatchTransient("GLN:7791234500055"))
		_, err := contract.DispatchTransfer(
			testContext(stub, farmaciaMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
		stub.transient = map[string][]byte{}

		_, err = contract.RejectTransfer(
			testContext(stub, farmaciaDosMSP, RoleOperator),
			UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
		requireCode(t, err, cerr.ReceiverMismatch)
	})
}

// --- Tercera salida de EN_TRANSITO ------------------------------------------

// TestExtraordinaryExitClosesOperationAndRestoresPolicy cubre la tercera salida
// de EN_TRANSITO (ADR-007, punto 6.c): un evento extraordinario que cierra el
// transito conserva la custodia en el emisor y debe restaurar su politica de
// reposo.
//
// Las operaciones de evento extraordinario las implementan las issues EXT; lo
// que esta issue entrega y prueba es el mecanismo reutilizable — el cierre del
// registro de operacion y la restauracion de la politica —, porque omitir la
// restauracion dejaria la unidad bloqueada de forma permanente bajo una
// politica que exige al receptor de un despacho ya resuelto.
func TestExtraordinaryExitClosesOperationAndRestoresPolicy(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	regulator, err := resolveInvoker(ctx)
	requireNoError(t, err)
	unit, err := readUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)

	collection := pairCollectionName(labMSP, drogueriaMSP)
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	op, found, err := readActiveTransferOperation(ctx, collection, validGTIN, validSerial)
	requireNoError(t, err)
	if !found {
		t.Fatal("el despacho deberia haber dejado una operacion activa")
	}

	requireNoError(t, CloseTransitForExtraordinaryEvent(ctx, unit, regulator, "Quarantine"))

	// (1) Marcador regulatorio. Es lo que somete la transaccion a la politica de
	// endoso de la coleccion implicita de ANMAT y convierte su participacion en
	// un coendoso real de peer, en lugar de apoyarla en la firma de creador
	// (ADR-007, punto 6.d).
	marker := stub.privateData[implicitCollection(anmatMSP)]
	markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	raw := marker[markerKey]
	if raw == nil {
		t.Fatal("el cierre extraordinario iniciado por el regulador debe escribir su marcador de participacion")
	}
	var decoded participationMarker
	requireNoError(t, json.Unmarshal(raw, &decoded))
	if decoded.Operacion != "Quarantine" || decoded.MSPID != anmatMSP {
		t.Fatalf("marcador = %+v", decoded)
	}

	// (2) Cierre del registro: se conserva el historico y desaparece la clave
	// activa, tambien del estado publico.
	if stub.privateData[collection][activeKey] != nil {
		t.Fatal("el cierre extraordinario debe eliminar la clave de operacion activa")
	}
	histKey, err := transferOpKey(stub, validGTIN, validSerial, op.TxIDDespacho)
	requireNoError(t, err)
	if stub.privateData[collection][histKey] == nil {
		t.Fatal("el cierre extraordinario debe conservar el registro historico")
	}
	var historical TransferOperation
	requireNoError(t, json.Unmarshal(stub.privateData[collection][histKey], &historical))
	if historical.MotivoCierre != closureExtraordinary {
		t.Fatalf("motivo de cierre = %q", historical.MotivoCierre)
	}

	// (3) Politica de reposo hacia el EMISOR: el transito no se consumo, de modo
	// que la custodia registrada sigue siendo la suya.
	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo tras el cierre extraordinario = %v, se esperaba %s", orgs, labMSP)
	}
}

// TestExtraordinaryExitByCustodianWritesNoRegulatoryMarker es la contracara del
// test anterior: el marcador regulatorio se escribe SOLO cuando el evento lo
// inicia la organizacion regulatoria (ADR-007, punto 6.d, "cuando el invocador
// es efectivamente la organizacion regulatoria"). Escribirlo siempre convertiria
// a ANMAT en coendosante obligatoria de eventos que no inicio, que es
// exactamente lo que DES-6 prohibe.
func TestExtraordinaryExitByCustodianWritesNoRegulatoryMarker(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)

	ctx := testContext(stub, labMSP, RoleOperator)
	emitter, err := resolveInvoker(ctx)
	requireNoError(t, err)
	unit, err := readUnit(ctx, validGTIN, validSerial)
	requireNoError(t, err)

	requireNoError(t, CloseTransitForExtraordinaryEvent(ctx, unit, emitter, "ReportDamaged"))

	// La coleccion implicita regulatoria no esta vacia -- el alta de cada
	// organizacion del fixture dejo su marcador de la variante `Organizacion` --,
	// de modo que la asercion es sobre la clave de ESTE evento: la variante
	// `Unidad` con el txId de esta transaccion.
	markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	if stub.privateData[implicitCollection(anmatMSP)][markerKey] != nil {
		t.Fatal("un evento que no inicia el regulador no debe escribir su marcador")
	}
	activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	if stub.privateData[pairCollectionName(labMSP, drogueriaMSP)][activeKey] != nil {
		t.Fatal("el cierre debe eliminar la clave de operacion activa igualmente")
	}
}

// --- Flujo completo ---------------------------------------------------------

// TestTransferIsTwoTransactions deja constancia del modelo de ADR-004: el
// cambio de custodia exige dos transacciones con agentes detonantes distintos,
// y el estado intermedio EN_TRANSITO es observable entre ambas.
func TestTransferIsTwoTransactions(t *testing.T) {
	stub, contract := transferFixture(t)

	afterDispatch := func() MedicationUnit {
		withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
		view, err := contract.DispatchTransfer(
			testContext(stub, labMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
		stub.transient = map[string][]byte{}
		return MedicationUnit(*view)
	}()

	if afterDispatch.Estado != domain.StateEnTransito || afterDispatch.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("el despacho no dejo la unidad en transito bajo el emisor: %+v", afterDispatch)
	}

	view, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	if view.Estado != domain.StateEnCustodia || view.CustodioActual != "GLN:"+drogueriaGLN {
		t.Fatalf("la recepcion no confirmo el cambio de custodia: %+v", view)
	}

	// La unidad recibida puede volver a despacharse: un despacho posterior crea
	// un registro de operacion NUEVO, con clave nueva (ADR-004, regla 5).
	stub.txID = "tx-0000000000000002"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err = contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
