package snt

import (
	"encoding/json"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest"
)

// --- Init: bootstrap regulatorio (ADR-010, punto 4) ------------------------

func TestInitSeedsRegulatorFromEmbeddedManifest(t *testing.T) {
	stub := newMockStub()
	contract := new(SNTContract)

	regulator, err := manifest.Regulator()
	requireNoError(t, err)

	view, err := contract.Init(testContext(stub, regulator.MSPID, RoleRegulatoryAdmin))
	requireNoError(t, err)

	if view.MSPID != regulator.MSPID {
		t.Fatalf("Init sembro %s, se esperaba %s", view.MSPID, regulator.MSPID)
	}
	if view.AgentType != domain.AgentRegulator || view.IDType != IDTypeREG || !view.Active {
		t.Fatalf("la entrada sembrada no es un REGULATOR activo con idType REG: %+v", view)
	}

	// La entrada queda protegida por SBE de la organizacion regulatoria, de modo
	// que ninguna otra pueda modificarla despues del bootstrap.
	key, err := organizationKey(stub, regulator.MSPID)
	requireNoError(t, err)
	if len(stub.validation[key]) == 0 {
		t.Fatal("Init no fijo la politica de endoso por clave de la entrada REGULATOR")
	}
}

// TestInitRejectsNonManifestRegulator cubre la condicion 1 de ADR-010 punto 4:
// el mspId regulatorio no viaja como argumento, se resuelve contra el
// manifiesto embebido.
func TestInitRejectsNonManifestRegulator(t *testing.T) {
	stub := newMockStub()
	contract := new(SNTContract)

	_, err := contract.Init(testContext(stub, labMSP, RoleRegulatoryAdmin))
	requireCode(t, err, cerr.RegulatoryOnly)

	if len(stub.state) != 0 {
		t.Fatal("una Init rechazada no debe escribir estado")
	}
}

// TestInitRejectsMissingRegulatoryRole cubre la condicion 2.
func TestInitRejectsMissingRegulatoryRole(t *testing.T) {
	stub := newMockStub()
	contract := new(SNTContract)

	for _, role := range []string{"", RoleOperator, RoleAuditor, RoleFinancierAuditor} {
		_, err := contract.Init(testContext(stub, anmatMSP, role))
		requireCode(t, err, cerr.RegulatoryOnly)
	}
}

// TestInitIsNotReinvocable cubre la condicion 3: reinvocar falla, de modo que
// una segunda Init no puede sustituir al regulador.
func TestInitIsNotReinvocable(t *testing.T) {
	stub := newMockStub()
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.Init(ctx)
	requireNoError(t, err)

	_, err = contract.Init(ctx)
	requireCode(t, err, cerr.AlreadyInitialized)
}

// --- RegisterOrganization ---------------------------------------------------

func TestRegisterOrganizationHappyPath(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	contract := new(SNTContract)

	view, err := contract.RegisterOrganization(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: IDTypeGLN,
			AgentType: domain.AgentPharmacy, Active: true,
		})
	requireNoError(t, err)

	if view.CanonicalID() != "GLN:"+farmaciaGLN {
		t.Fatalf("identificador canonico = %s", view.CanonicalID())
	}

	stored, found, err := readOrganization(testContext(stub, anmatMSP, RoleRegulatoryAdmin), farmaciaMSP)
	requireNoError(t, err)
	if !found || stored.AgentType != domain.AgentPharmacy {
		t.Fatalf("la entrada no quedo persistida: %+v", stored)
	}
}

// TestRegisterOrganizationIsRegulatoryOnly verifica que la autorizacion se
// derive del registro (agentType=REGULATOR) y no de un literal de MSP.
func TestRegisterOrganizationIsRegulatoryOnly(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	contract := new(SNTContract)

	req := RegisterOrganizationRequest{
		MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: IDTypeGLN,
		AgentType: domain.AgentPharmacy, Active: true,
	}

	cases := []struct {
		name  string
		mspID string
		role  string
	}{
		{"organizacion custodial con rol operador", labMSP, RoleOperator},
		{"organizacion custodial con rol regulatorio", labMSP, RoleRegulatoryAdmin},
		{"regulador sin el rol requerido", anmatMSP, RoleOperator},
		{"regulador sin atributo de rol", anmatMSP, ""},
		{"organizacion no registrada", "OrgFantasmaMSP", RoleRegulatoryAdmin},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contract.RegisterOrganization(testContext(stub, tc.mspID, tc.role), req)
			requireCode(t, err, cerr.RegulatoryOnly)
		})
	}
}

func TestRegisterOrganizationRejectsInvalidRequests(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	cases := []struct {
		name string
		req  RegisterOrganizationRequest
	}{
		{"mspId vacio", RegisterOrganizationRequest{
			ID: farmaciaGLN, IDType: IDTypeGLN, AgentType: domain.AgentPharmacy}},
		{"idType fuera del catalogo", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: "DNI", AgentType: domain.AgentPharmacy}},
		{"agentType fuera del catalogo", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: IDTypeGLN, AgentType: "MAYORISTA"}},
		{"digito verificador GLN invalido", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: "7791234500049", IDType: IDTypeGLN, AgentType: domain.AgentPharmacy}},
		{"GLN de longitud incorrecta", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: "779123450004", IDType: IDTypeGLN, AgentType: domain.AgentPharmacy}},
		{"idType REG con agentType custodial", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: "FARMACIA", IDType: IDTypeREG, AgentType: domain.AgentPharmacy}},
		{"idType GLN con agentType no custodial", RegisterOrganizationRequest{
			MSPID: financiadorMSP, ID: farmaciaGLN, IDType: IDTypeGLN, AgentType: domain.AgentFinancier}},
		{"mspId duplicado", RegisterOrganizationRequest{
			MSPID: labMSP, ID: farmaciaGLN, IDType: IDTypeGLN, AgentType: domain.AgentPharmacy}},
		{"identificador canonico duplicado", RegisterOrganizationRequest{
			MSPID: farmaciaMSP, ID: labGLN, IDType: IDTypeGLN, AgentType: domain.AgentPharmacy}},
		{"segunda entrada REGULATOR", RegisterOrganizationRequest{
			MSPID: "OtroAnmatMSP", ID: "ANMAT-2", IDType: IDTypeREG, AgentType: domain.AgentRegulator}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contract.RegisterOrganization(ctx, tc.req)
			requireCode(t, err, cerr.InvalidRequest)
		})
	}
}

// TestRegisterOrganizationAcceptsFinancier cubre el soporte nativo de multiples
// financiadores de ADR-010, punto 3.
func TestRegisterOrganizationAcceptsFinancier(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	contract := new(SNTContract)

	view, err := contract.RegisterOrganization(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		RegisterOrganizationRequest{
			MSPID: financiadorMSP, ID: "INSSJP-PAMI", IDType: IDTypeREG,
			AgentType: domain.AgentFinancier, Active: true,
		})
	requireNoError(t, err)
	if view.CanonicalID() != "REG:INSSJP-PAMI" {
		t.Fatalf("identificador canonico del financiador = %s", view.CanonicalID())
	}
}

// --- SetOrganizationActive --------------------------------------------------

func TestSetOrganizationActive(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	view, err := contract.SetOrganizationActive(ctx, SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
	requireNoError(t, err)
	if view.Active {
		t.Fatal("la organizacion deberia haber quedado inactiva")
	}

	// La baja se modela con `active`, sin alterar el resto de la entrada.
	if view.ID != labGLN || view.AgentType != domain.AgentLaboratory {
		t.Fatalf("la baja altero la identidad de la entrada: %+v", view)
	}
}

func TestSetOrganizationActiveRejections(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	contract := new(SNTContract)
	regulatorCtx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	t.Run("organizacion inexistente", func(t *testing.T) {
		_, err := contract.SetOrganizationActive(regulatorCtx,
			SetOrganizationActiveRequest{MSPID: "OrgFantasmaMSP", Active: false})
		requireCode(t, err, cerr.OrgNotRegistered)
	})

	t.Run("mspId vacio", func(t *testing.T) {
		_, err := contract.SetOrganizationActive(regulatorCtx,
			SetOrganizationActiveRequest{Active: false})
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("invocador no regulatorio", func(t *testing.T) {
		_, err := contract.SetOrganizationActive(testContext(stub, labMSP, RoleOperator),
			SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
		requireCode(t, err, cerr.RegulatoryOnly)
	})

	// Invariante de ADR-010: la red no puede quedar sin autoridad capaz de
	// administrar el registro.
	t.Run("ultimo regulador activo", func(t *testing.T) {
		_, err := contract.SetOrganizationActive(regulatorCtx,
			SetOrganizationActiveRequest{MSPID: anmatMSP, Active: false})
		requireCode(t, err, cerr.LastActiveRegulator)
	})
}

// --- Resolucion de identidad (ADR-003, ADR-010) -----------------------------

func TestResolveInvokerRejectsUnregisteredAndInactive(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)

	_, err := resolveInvoker(testContext(stub, "OrgFantasmaMSP", RoleOperator))
	requireCode(t, err, cerr.OrgNotRegistered)

	contract := new(SNTContract)
	_, err = contract.SetOrganizationActive(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
	requireNoError(t, err)

	_, err = resolveInvoker(testContext(stub, labMSP, RoleOperator))
	requireCode(t, err, cerr.OrgInactive)
}

func TestInvokerRoleAndAgentTypeChecks(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)

	invoker, err := resolveInvoker(testContext(stub, labMSP, RoleOperator))
	requireNoError(t, err)

	requireNoError(t, invoker.requireRole(RoleOperator))
	requireCode(t, invoker.requireRole(RoleRegulatoryAdmin), cerr.UnauthorizedRole)

	requireNoError(t, invoker.requireAgentType(domain.AgentLaboratory))
	requireCode(t, invoker.requireAgentType(domain.AgentPharmacy, domain.AgentHealthcare), cerr.UnauthorizedAgentType)

	if invoker.CanonicalID() != "GLN:"+labGLN {
		t.Fatalf("identificador canonico del invocador = %s", invoker.CanonicalID())
	}
}

// --- Marcadores de participacion (ADR-007, punto 6) -------------------------

// TestRegistryOperationsWriteOrganizationMarker verifica que las operaciones
// del registro escriban el marcador en la variante `Organizacion`, que es la
// unica construible cuando la operacion no recae sobre una unidad.
func TestRegistryOperationsWriteOrganizationMarker(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	contract := new(SNTContract)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.RegisterOrganization(ctx, RegisterOrganizationRequest{
		MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: IDTypeGLN,
		AgentType: domain.AgentPharmacy, Active: true,
	})
	requireNoError(t, err)

	wantKey, err := organizationParticipationKey(stub, farmaciaMSP, stub.GetTxID())
	requireNoError(t, err)

	raw := stub.privateData[implicitCollection(anmatMSP)][wantKey]
	if raw == nil {
		t.Fatal("RegisterOrganization no escribio el marcador en la coleccion implicita regulatoria")
	}

	var marker participationMarker
	requireNoError(t, json.Unmarshal(raw, &marker))
	if marker.Operacion != opRegisterOrganization || marker.MSPID != anmatMSP {
		t.Fatalf("contenido del marcador inesperado: %+v", marker)
	}
	// El contenido debe ser determinístico: sale de GetTxTimestamp, no del reloj.
	if marker.Timestamp != "2026-08-27T12:00:00Z" {
		t.Fatalf("timestamp del marcador = %s; debe salir de GetTxTimestamp()", marker.Timestamp)
	}
}

// TestParticipationMarkerKeyPutsTxIDLast fija la propiedad de la que depende
// que las 50.000 altas del dataset de medicion no se serialicen por conflicto
// MVCC: la clave del marcador es unica por transaccion (ADR-007, punto 6.g).
func TestParticipationMarkerKeyPutsTxIDLast(t *testing.T) {
	stub := newMockStub()

	unitKey, err := unitParticipationKey(stub, validGTIN, validSerial, "tx-A")
	requireNoError(t, err)
	otherTx, err := unitParticipationKey(stub, validGTIN, validSerial, "tx-B")
	requireNoError(t, err)
	if unitKey == otherTx {
		t.Fatal("dos transacciones sobre la misma unidad producen la misma clave de marcador")
	}

	// El prefijo hasta el ultimo componente debe ser comun, para que la clave
	// siga siendo consultable por clave compuesta parcial.
	prefix, err := stub.CreateCompositeKey(objectTypeParticipation,
		[]string{participationTargetUnit, validGTIN, validSerial})
	requireNoError(t, err)
	prefix = prefix[:len(prefix)-1]
	if len(unitKey) <= len(prefix) || unitKey[:len(prefix)] != prefix {
		t.Fatalf("la clave del marcador no conserva el prefijo consultable %q", prefix)
	}

	orgKey, err := organizationParticipationKey(stub, farmaciaMSP, "tx-A")
	requireNoError(t, err)
	if orgKey == unitKey {
		t.Fatal("las dos variantes del marcador deben producir claves distintas")
	}
}

// TestPublicKeyCreationWritesMarker es la invariante verificable de ADR-007,
// punto 6.j: TODA operacion que escriba una clave publica nueva escribe tambien
// el marcador de la organizacion responsable de esa escritura.
//
// Mientras se cumpla, la politica de chaincode OR(custodiales, regulatoria) no
// es una frontera de seguridad. Una operacion futura que cree una clave publica
// sin marcador reabriria una ventana de creacion sin dueno.
//
// Hoy son exactamente tres: RegisterUnit (laboratorio invocante),
// RegisterOrganization (regulador) y AuthorizeLabIntervention (regulador).
func TestPublicKeyCreationWritesMarker(t *testing.T) {
	cases := []struct {
		name          string
		responsable   string
		run           func(t *testing.T, stub *mockStub)
		markerOwnerOp string
	}{
		{
			// Unica de las tres cuya organizacion responsable NO es la
			// regulatoria: la clave de la unidad la crea el laboratorio.
			name:        "RegisterUnit",
			responsable: labMSP,
			run: func(t *testing.T, stub *mockStub) {
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
				contract := new(SNTContract)
				_, err := contract.RegisterUnit(
					testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
				requireNoError(t, err)
			},
			markerOwnerOp: opRegisterUnit,
		},
		{
			name:        "RegisterOrganization",
			responsable: anmatMSP,
			run: func(t *testing.T, stub *mockStub) {
				contract := new(SNTContract)
				_, err := contract.RegisterOrganization(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					RegisterOrganizationRequest{
						MSPID: farmaciaMSP, ID: farmaciaGLN, IDType: IDTypeGLN,
						AgentType: domain.AgentPharmacy, Active: true,
					})
				requireNoError(t, err)
			},
			markerOwnerOp: opRegisterOrganization,
		},
		{
			name:        "AuthorizeLabIntervention",
			responsable: anmatMSP,
			run: func(t *testing.T, stub *mockStub) {
				registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
				seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+drogueriaGLN)
				contract := new(SNTContract)
				_, err := contract.AuthorizeLabIntervention(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					AuthorizeLabInterventionRequest{
						GTIN: validGTIN, NumeroSerie: validSerial,
						Laboratorio: "GLN:" + labGLN,
						Operacion:   LabOpWithdrawFromMarket,
						Motivo:      "Retiro de lote, expediente ANMAT 1234/2026.",
						ExpiraEn:    "2026-09-30T00:00:00Z",
					})
				requireNoError(t, err)
			},
			markerOwnerOp: opAuthorizeLabIntervent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newMockStub()
			seedRegistry(t, stub)
			tc.run(t, stub)

			collection := stub.privateData[implicitCollection(tc.responsable)]
			found := false
			for _, raw := range collection {
				var marker participationMarker
				if err := json.Unmarshal(raw, &marker); err != nil {
					continue
				}
				if marker.Operacion == tc.markerOwnerOp {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s creo una clave publica sin escribir el marcador de %s",
					tc.name, tc.responsable)
			}
		})
	}
}
