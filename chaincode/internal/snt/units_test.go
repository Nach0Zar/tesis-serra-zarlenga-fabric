package snt

import (
	"encoding/json"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// registerUnitFixture deja el registro sembrado con las organizaciones del
// dataset fundacional que los tests de T01 necesitan.
func registerUnitFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	registerOrg(t, stub, farmaciaMSP, farmaciaGLN, domain.AgentPharmacy)
	return stub, new(SNTContract)
}

func validRegisterUnitRequest() RegisterUnitRequest {
	return RegisterUnitRequest{
		GTIN:             validGTIN,
		NumeroSerie:      validSerial,
		Lote:             "L2026-014",
		FechaVencimiento: "2027-12-31",
	}
}

// --- Camino feliz -----------------------------------------------------------

func TestRegisterUnitHappyPath(t *testing.T) {
	stub, contract := registerUnitFixture(t)

	view, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireNoError(t, err)

	if view.Estado != domain.StateEnLaboratorio {
		t.Fatalf("estado inicial = %s, se esperaba EN_LABORATORIO (T01)", view.Estado)
	}
	// El custodio persistido es el identificador canonico resuelto desde el
	// registro, nunca el mspId (ADR-003, punto 4).
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodioActual = %q, se esperaba el GLN canonico del laboratorio", view.CustodioActual)
	}
	if view.CustodioActual == labMSP {
		t.Fatal("el mspId no debe persistirse como custodio")
	}
	// El timestamp sale de GetTxTimestamp(), nunca de time.Now()
	// (modelo-datos.md §3.5).
	if view.UltimaActualizacion != "2026-08-27T12:00:00Z" {
		t.Fatalf("ultimaActualizacion = %q; debe salir de GetTxTimestamp()", view.UltimaActualizacion)
	}
	if view.Lote != "L2026-014" || view.FechaVencimiento != "2027-12-31" {
		t.Fatalf("metadatos normativos no persistidos: %+v", view)
	}

	// La unidad queda bajo la clave compuesta GTIN+serie (modelo-datos.md §2.2).
	stored, err := readUnit(testContext(stub, labMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	if stored != MedicationUnit(*view) {
		t.Fatalf("el estado persistido difiere de la vista devuelta: %+v vs %+v", stored, *view)
	}
}

// TestRegisterUnitEmitsEvent cubre el criterio "emite evento de chaincode".
func TestRegisterUnitEmitsEvent(t *testing.T) {
	stub, contract := registerUnitFixture(t)

	_, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireNoError(t, err)

	payload, ok := stub.events[opRegisterUnit]
	if !ok {
		t.Fatal("RegisterUnit no emitio evento de chaincode")
	}
	var emitted MedicationUnit
	requireNoError(t, json.Unmarshal(payload, &emitted))
	if emitted.Estado != domain.StateEnLaboratorio || emitted.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("payload del evento inesperado: %+v", emitted)
	}
}

// --- Reglas de rechazo ------------------------------------------------------

// TestRegisterUnitRejectsDuplicate cubre UNIT_ALREADY_EXISTS: la unicidad que
// aplica el chaincode es la que exige la normativa, sobre la COMBINACION
// GTIN+serie (modelo-datos.md §2.1).
func TestRegisterUnitRejectsDuplicate(t *testing.T) {
	stub, contract := registerUnitFixture(t)
	ctx := testContext(stub, labMSP, RoleOperator)

	_, err := contract.RegisterUnit(ctx, validRegisterUnitRequest())
	requireNoError(t, err)

	_, err = contract.RegisterUnit(ctx, validRegisterUnitRequest())
	requireCode(t, err, cerr.UnitAlreadyExists)

	// Mismo numero de serie bajo otro GTIN NO es una colision: dos laboratorios
	// pueden generar el mismo string de serie para GTIN distintos.
	otherGTIN := validRegisterUnitRequest()
	otherGTIN.GTIN = "07791234500017"
	if !hasValidGS1CheckDigit(otherGTIN.GTIN) {
		t.Fatalf("el GTIN alternativo del test no es valido: %s", otherGTIN.GTIN)
	}
	_, err = contract.RegisterUnit(ctx, otherGTIN)
	requireNoError(t, err)
}

func TestRegisterUnitAuthorizationRejections(t *testing.T) {
	cases := []struct {
		name  string
		mspID string
		role  string
		want  cerr.Code
	}{
		{"organizacion no registrada", "OrgFantasmaMSP", RoleOperator, cerr.OrgNotRegistered},
		{"agentType no habilitado para T01", drogueriaMSP, RoleOperator, cerr.UnauthorizedAgentType},
		{"farmacia intentando el alta", farmaciaMSP, RoleOperator, cerr.UnauthorizedAgentType},
		{"organizacion regulatoria", anmatMSP, RoleRegulatoryAdmin, cerr.UnauthorizedAgentType},
		{"laboratorio sin atributo de rol", labMSP, "", cerr.UnauthorizedRole},
		{"laboratorio con rol auditor", labMSP, RoleAuditor, cerr.UnauthorizedRole},
		{"laboratorio con rol regulatorio", labMSP, RoleRegulatoryAdmin, cerr.UnauthorizedRole},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := registerUnitFixture(t)
			_, err := contract.RegisterUnit(
				testContext(stub, tc.mspID, tc.role), validRegisterUnitRequest())
			requireCode(t, err, tc.want)
		})
	}
}

// TestRegisterUnitRejectsInactiveLaboratory cubre el paso 2 de la regla de
// validacion de ADR-003: la organizacion debe existir Y estar habilitada.
func TestRegisterUnitRejectsInactiveLaboratory(t *testing.T) {
	stub, contract := registerUnitFixture(t)

	_, err := contract.SetOrganizationActive(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
	requireNoError(t, err)

	_, err = contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireCode(t, err, cerr.OrgInactive)
}

func TestRegisterUnitRejectsInvalidFormats(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*RegisterUnitRequest)
	}{
		{"GTIN con digito verificador invalido", func(r *RegisterUnitRequest) { r.GTIN = "07791234567890" }},
		{"GTIN de longitud incorrecta", func(r *RegisterUnitRequest) { r.GTIN = "0779123456789" }},
		{"GTIN vacio", func(r *RegisterUnitRequest) { r.GTIN = "" }},
		{"numero de serie vacio", func(r *RegisterUnitRequest) { r.NumeroSerie = "" }},
		{"numero de serie de mas de 20", func(r *RegisterUnitRequest) { r.NumeroSerie = "123456789012345678901" }},
		{"serie de 20 que abre con 779", func(r *RegisterUnitRequest) { r.NumeroSerie = "77912345678901234567" }},
		{"serie con caracter fuera del set GS1", func(r *RegisterUnitRequest) { r.NumeroSerie = "SN 0001" }},
		{"lote vacio", func(r *RegisterUnitRequest) { r.Lote = "" }},
		{"vencimiento vacio", func(r *RegisterUnitRequest) { r.FechaVencimiento = "" }},
		{"vencimiento en AAMMDD", func(r *RegisterUnitRequest) { r.FechaVencimiento = "271231" }},
		{"vencimiento no ISO 8601", func(r *RegisterUnitRequest) { r.FechaVencimiento = "31/12/2027" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := registerUnitFixture(t)
			req := validRegisterUnitRequest()
			tc.mutate(&req)
			_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
			requireCode(t, err, cerr.InvalidRequest)
		})
	}
}

// TestRegisterUnitRejectionsWriteNothing verifica que un alta rechazada no deje
// estado, politica de endoso ni marcador.
func TestRegisterUnitRejectionsWriteNothing(t *testing.T) {
	stub, contract := registerUnitFixture(t)
	stateBefore := len(stub.state)
	markersBefore := len(stub.privateData[implicitCollection(labMSP)])

	req := validRegisterUnitRequest()
	req.GTIN = "07791234567890"
	_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
	requireCode(t, err, cerr.InvalidRequest)

	if len(stub.state) != stateBefore {
		t.Fatal("un alta rechazada no debe escribir estado publico")
	}
	if len(stub.privateData[implicitCollection(labMSP)]) != markersBefore {
		t.Fatal("un alta rechazada no debe escribir marcador de participacion")
	}
	if len(stub.events) != 0 {
		t.Fatal("un alta rechazada no debe emitir evento")
	}
}

// --- Endoso de T01 (ADR-007, puntos 6.a, 6.g y 6.j) -------------------------

// TestRegisterUnitSetsRestingEndorsementPolicy verifica que el alta fije la
// politica de reposo de la clave de la unidad en la organizacion del custodio.
func TestRegisterUnitSetsRestingEndorsementPolicy(t *testing.T) {
	stub, contract := registerUnitFixture(t)

	_, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireNoError(t, err)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)

	policy := stub.validation[key]
	if len(policy) == 0 {
		t.Fatal("RegisterUnit no fijo la politica de endoso por clave de la unidad")
	}

	orgs := endorsingOrganizations(t, policy)
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo = %v, se esperaba unicamente %s", orgs, labMSP)
	}
	// Ninguna politica de clave de unidad admite a la organizacion regulatoria
	// como rama alternativa: habilitaria el endoso unilateral del regulador en
	// las operaciones ordinarias (ADR-007, punto 6.a).
	for _, org := range orgs {
		if org == anmatMSP {
			t.Fatal("la politica de reposo no debe incluir a la organizacion regulatoria")
		}
	}
}

// TestRegisterUnitWritesLaboratoryMarker es el criterio de CC-2 sobre el endoso
// de T01: la clave de la unidad no existe todavia y por eso el marcador en la
// coleccion implicita del laboratorio es lo unico que hace que la plataforma
// exija su endoso en esa primera escritura.
func TestRegisterUnitWritesLaboratoryMarker(t *testing.T) {
	stub, contract := registerUnitFixture(t)

	_, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
	requireNoError(t, err)

	wantKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)

	raw := stub.privateData[implicitCollection(labMSP)][wantKey]
	if raw == nil {
		t.Fatal("RegisterUnit no escribio el marcador en la coleccion implicita del laboratorio")
	}

	var marker participationMarker
	requireNoError(t, json.Unmarshal(raw, &marker))
	if marker.Operacion != opRegisterUnit || marker.MSPID != labMSP {
		t.Fatalf("contenido del marcador inesperado: %+v", marker)
	}
	// El contenido lo calcula el chaincode y es determinístico: no viaja por
	// transient ni depende de nada del cliente.
	if marker.Timestamp != "2026-08-27T12:00:00Z" {
		t.Fatalf("timestamp del marcador = %s; debe salir de GetTxTimestamp()", marker.Timestamp)
	}

	// El marcador va SOLO a la coleccion del laboratorio: escribirlo tambien en
	// la del regulador convertiria el alta en una operacion multiparte con
	// ANMAT como coendosante obligatorio (ADR-006, alternativa A, objecion 2).
	// La coleccion regulatoria ya tiene los marcadores de las altas registrales
	// del fixture; lo que no debe haber es uno de esta operacion.
	for _, raw := range stub.privateData[implicitCollection(anmatMSP)] {
		var regulatoryMarker participationMarker
		if err := json.Unmarshal(raw, &regulatoryMarker); err != nil {
			continue
		}
		if regulatoryMarker.Operacion == opRegisterUnit {
			t.Fatal("RegisterUnit no debe escribir marcador en la coleccion implicita regulatoria")
		}
	}
}

// TestRegisterUnitMarkersDoNotSerialize fija la propiedad que evita el conflicto
// MVCC en la operacion de mayor volumen del prototipo: dos altas distintas
// escriben claves de marcador distintas, sin clave compartida por organizacion.
func TestRegisterUnitMarkersDoNotSerialize(t *testing.T) {
	stub, contract := registerUnitFixture(t)
	ctx := testContext(stub, labMSP, RoleOperator)

	first := validRegisterUnitRequest()
	_, err := contract.RegisterUnit(ctx, first)
	requireNoError(t, err)

	// Segunda alta, en otra transaccion.
	stub.txID = "tx-0000000000000001"
	second := validRegisterUnitRequest()
	second.NumeroSerie = "SN-0002-ABCD"
	_, err = contract.RegisterUnit(ctx, second)
	requireNoError(t, err)

	markers := stub.privateData[implicitCollection(labMSP)]
	if len(markers) != 2 {
		t.Fatalf("se esperaban 2 marcadores independientes y hay %d: una clave compartida "+
			"por organizacion serializaria las altas por conflicto MVCC", len(markers))
	}
}
