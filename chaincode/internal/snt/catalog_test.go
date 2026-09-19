package snt

import (
	"errors"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Este archivo es el inventario que exige CC-6 (#19): un test por cada camino
// feliz y un test por CADA regla de rechazo. Las reglas puntuales ya estan
// cubiertas por los tests de su operacion; lo que agrega este archivo es la
// garantia de COMPLETITUD -- que ningun codigo del catalogo del contrato quede
// sin un escenario que lo produzca, y que ninguna operacion implementada quede
// sin su camino feliz.
//
// Es material de defensa: la norma codificada y probada.

// --- Inventario de caminos felices ------------------------------------------

// TestHappyPathInventory ejercita, de punta a punta, el camino feliz de cada
// operacion implementada. Falla si una operacion implementada deja de tener uno.
func TestHappyPathInventory(t *testing.T) {
	stub := newMockStub()
	contract := new(SNTContract)

	// Bootstrap regulatorio (ADR-010, punto 4).
	if _, err := contract.Init(testContext(stub, anmatMSP, RoleRegulatoryAdmin)); err != nil {
		t.Fatalf("Init: %v", err)
	}

	regulatorCtx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	// Seed del registro (paso f del bootstrap de ADR-007).
	for _, org := range []struct {
		mspID     string
		gln       string
		agentType domain.AgentType
	}{
		{labMSP, labGLN, domain.AgentLaboratory},
		{drogueriaMSP, drogueriaGLN, domain.AgentDrugstore},
		{farmaciaMSP, farmaciaGLN, domain.AgentPharmacy},
	} {
		if _, err := contract.RegisterOrganization(regulatorCtx, RegisterOrganizationRequest{
			MSPID: org.mspID, ID: org.gln, IDType: IDTypeGLN,
			AgentType: org.agentType, Active: true,
		}); err != nil {
			t.Fatalf("RegisterOrganization(%s): %v", org.mspID, err)
		}
	}

	if _, err := contract.SetOrganizationActive(regulatorCtx,
		SetOrganizationActiveRequest{MSPID: farmaciaMSP, Active: true}); err != nil {
		t.Fatalf("SetOrganizationActive: %v", err)
	}

	// T01.
	if _, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest()); err != nil {
		t.Fatalf("RegisterUnit: %v", err)
	}

	// T02 -> T04: laboratorio -> drogueria.
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	if _, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("DispatchTransfer: %v", err)
	}
	stub.transient = map[string][]byte{}
	stub.txID = "tx-recepcion"
	if _, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("ReceiveTransfer: %v", err)
	}

	// T03 -> T05: drogueria -> farmacia, rechazado en recepcion.
	stub.txID = "tx-despacho-2"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	if _, err := contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("DispatchTransfer (segundo): %v", err)
	}
	stub.transient = map[string][]byte{}
	stub.txID = "tx-rechazo"
	if _, err := contract.RejectTransfer(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Inconsistencia documental."}); err != nil {
		t.Fatalf("RejectTransfer: %v", err)
	}

	// T06 sobre otra unidad, ya en custodia de la farmacia.
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
	stub.txID = "tx-dispensa"
	if _, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("Dispense: %v", err)
	}

	// Autorizacion de intervencion de laboratorio y su revocacion.
	if _, err := contract.AuthorizeLabIntervention(regulatorCtx, validAuthorizationRequest()); err != nil {
		t.Fatalf("AuthorizeLabIntervention: %v", err)
	}
	if _, err := contract.RevokeLabIntervention(regulatorCtx, RevokeLabInterventionRequest{
		GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Cierre administrativo.",
	}); err != nil {
		t.Fatalf("RevokeLabIntervention: %v", err)
	}

	// Lecturas.
	if _, err := contract.ReadUnit(regulatorCtx, validGTIN, validSerial); err != nil {
		t.Fatalf("ReadUnit: %v", err)
	}
	if _, err := contract.GetUnitHistory(regulatorCtx, validGTIN, validSerial); err != nil {
		t.Fatalf("GetUnitHistory: %v", err)
	}
	if _, err := contract.QueryUnitsByGTIN(regulatorCtx, validGTIN); err != nil {
		t.Fatalf("QueryUnitsByGTIN: %v", err)
	}
	if _, err := contract.VerifyUnit(regulatorCtx, validGTIN, validSerial); err != nil {
		t.Fatalf("VerifyUnit: %v", err)
	}
}

// --- Completitud del catalogo de errores ------------------------------------

// TestErrorCatalogIsCovered exige un escenario por CADA codigo del catalogo del
// contrato que las operaciones ya implementadas puedan producir. Los codigos
// que todavia no son alcanzables se declaran con la issue que los habilitara,
// de modo que la tabla nunca queda muda sobre uno de ellos.
func TestErrorCatalogIsCovered(t *testing.T) {
	scenarios := map[cerr.Code]func(t *testing.T) error{
		cerr.InvalidRequest: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			req := validRegisterUnitRequest()
			req.Lote = ""
			_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
			return err
		},
		cerr.UnitNotFound: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.ReadUnit(testContext(stub, labMSP, RoleOperator), validGTIN, "SN-9999-ZZZZ")
			return err
		},
		cerr.UnitAlreadyExists: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RegisterUnit(
				testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
			return err
		},
		cerr.InvalidStateTransition: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			// La farmacia es la custodia, de modo que la unica regla que puede
			// fallar es la aptitud del estado: EN_LABORATORIO no admite T06.
			seedUnit(t, stub, domain.StateEnLaboratorio, "GLN:"+farmaciaGLN)
			_, err := contract.Dispense(
				testContext(stub, farmaciaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		cerr.UnauthorizedCustodian: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
			_, err := contract.DispatchTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		cerr.UnauthorizedRole: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RegisterUnit(
				testContext(stub, labMSP, RoleAuditor), validRegisterUnitRequest())
			return err
		},
		cerr.UnauthorizedAgentType: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RegisterUnit(
				testContext(stub, drogueriaMSP, RoleOperator), validRegisterUnitRequest())
			return err
		},
		cerr.OrgNotRegistered: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RegisterUnit(
				testContext(stub, "OrgFantasmaMSP", RoleOperator), validRegisterUnitRequest())
			return err
		},
		cerr.OrgInactive: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.SetOrganizationActive(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin),
				SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
			requireNoError(t, err)
			_, err = contract.RegisterUnit(
				testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
			return err
		},
		cerr.TransferNotAuthorized: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+farmaciaGLN)
			withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
			_, err := contract.DispatchTransfer(
				testContext(stub, farmaciaMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		cerr.InvalidDestination: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			withTransient(stub, dispatchTransient(anmatMSP))
			_, err := contract.DispatchTransfer(
				testContext(stub, labMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		cerr.NotInTransit: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.ReceiveTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			return err
		},
		cerr.ReceiverMismatch: func(t *testing.T) error {
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
			return err
		},
		cerr.RegulatoryOnly: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RegisterOrganization(
				testContext(stub, labMSP, RoleOperator),
				RegisterOrganizationRequest{
					MSPID: "OtraMSP", ID: farmaciaDosGLN, IDType: IDTypeGLN,
					AgentType: domain.AgentPharmacy, Active: true,
				})
			return err
		},
		cerr.LastActiveRegulator: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.SetOrganizationActive(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin),
				SetOrganizationActiveRequest{MSPID: anmatMSP, Active: false})
			return err
		},
		cerr.AlreadyInitialized: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.Init(testContext(stub, anmatMSP, RoleRegulatoryAdmin))
			return err
		},
		cerr.InvalidLabIntervention: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			req := validAuthorizationRequest()
			req.Operacion = "DISPENSE"
			_, err := contract.AuthorizeLabIntervention(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin), req)
			return err
		},
		cerr.LabInterventionNotFound: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			_, err := contract.RevokeLabIntervention(
				testContext(stub, anmatMSP, RoleRegulatoryAdmin),
				RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
			return err
		},
		cerr.LabInterventionNotActive: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			regulatorCtx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
			_, err := contract.AuthorizeLabIntervention(regulatorCtx, validAuthorizationRequest())
			requireNoError(t, err)
			req := RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"}
			_, err = contract.RevokeLabIntervention(regulatorCtx, req)
			requireNoError(t, err)
			_, err = contract.RevokeLabIntervention(regulatorCtx, req)
			return err
		},
		cerr.InternalError: func(t *testing.T) error {
			stub, contract := transferFixture(t)
			stub.failOn("GetState", errors.New("fallo simulado del ledger"))
			_, err := contract.ReadUnit(testContext(stub, labMSP, RoleOperator), validGTIN, validSerial)
			return err
		},
	}

	// Codigos del catalogo que las operaciones implementadas todavia no pueden
	// producir, con la issue que los habilitara.
	pending := map[cerr.Code]string{
		cerr.LabInterventionRequired: "EXT-6 (#32), EXT-5 (#31) y EXT-8 (#63): lo produce la " +
			"intervencion de un laboratorio no custodio sin autorizacion ACTIVA y vigente",
	}

	catalog := []cerr.Code{
		cerr.InvalidRequest, cerr.UnitNotFound, cerr.UnitAlreadyExists,
		cerr.InvalidStateTransition, cerr.UnauthorizedCustodian, cerr.UnauthorizedRole,
		cerr.UnauthorizedAgentType, cerr.OrgNotRegistered, cerr.OrgInactive,
		cerr.TransferNotAuthorized, cerr.InvalidDestination, cerr.NotInTransit,
		cerr.ReceiverMismatch, cerr.RegulatoryOnly, cerr.LastActiveRegulator,
		cerr.AlreadyInitialized, cerr.InvalidLabIntervention, cerr.LabInterventionNotFound,
		cerr.LabInterventionNotActive, cerr.LabInterventionRequired, cerr.InternalError,
	}

	for _, code := range catalog {
		scenario, covered := scenarios[code]
		if !covered {
			if _, declared := pending[code]; declared {
				t.Logf("%s todavia no es alcanzable: %s", code, pending[code])
				continue
			}
			t.Errorf("el codigo %s no tiene escenario ni esta declarado como pendiente", code)
			continue
		}
		t.Run(string(code), func(t *testing.T) {
			requireCode(t, scenario(t), code)
		})
	}
}

// --- Fallas de plataforma ---------------------------------------------------

// TestPlatformFailuresAreTypedAsInternal verifica que toda falla de la API del
// stub -- del ledger, de los datos privados o de la politica de endoso -- se
// devuelva como INTERNAL_ERROR y no como una regla de negocio. La distincion
// importa para la evidencia: NET-6 debe separar el rechazo por plataforma del
// rechazo por logica del contrato.
func TestPlatformFailuresAreTypedAsInternal(t *testing.T) {
	boom := errors.New("fallo simulado de la plataforma")

	cases := []struct {
		name   string
		method string
		run    func(t *testing.T, stub *mockStub, contract *SNTContract) error
	}{
		{
			name: "lectura del world state", method: "GetState",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				_, err := contract.ReadUnit(testContext(stub, labMSP, RoleOperator), validGTIN, validSerial)
				return err
			},
		},
		{
			name: "escritura del world state", method: "PutState",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				req := validRegisterUnitRequest()
				req.NumeroSerie = "SN-0009-ABCD"
				_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
				return err
			},
		},
		{
			name: "recorrido del registro", method: "GetStateByPartialCompositeKey",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				_, err := contract.RegisterOrganization(
					testContext(stub, anmatMSP, RoleRegulatoryAdmin),
					RegisterOrganizationRequest{
						MSPID: "OtraMSP", ID: farmaciaDosGLN, IDType: IDTypeGLN,
						AgentType: domain.AgentPharmacy, Active: true,
					})
				return err
			},
		},
		{
			name: "timestamp de la transaccion", method: "GetTxTimestamp",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				req := validRegisterUnitRequest()
				req.NumeroSerie = "SN-0010-ABCD"
				_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
				return err
			},
		},
		{
			name: "lectura del campo transient", method: "GetTransient",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				_, err := contract.DispatchTransfer(
					testContext(stub, labMSP, RoleOperator),
					DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			},
		},
		{
			name: "escritura de dato privado", method: "PutPrivateData",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
				_, err := contract.DispatchTransfer(
					testContext(stub, labMSP, RoleOperator),
					DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			},
		},
		{
			name: "politica de endoso por clave", method: "SetStateValidationParameter",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
				_, err := contract.DispatchTransfer(
					testContext(stub, labMSP, RoleOperator),
					DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
				return err
			},
		},
		{
			name: "emision del evento", method: "SetEvent",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				req := validRegisterUnitRequest()
				req.NumeroSerie = "SN-0011-ABCD"
				_, err := contract.RegisterUnit(testContext(stub, labMSP, RoleOperator), req)
				return err
			},
		},
		{
			name: "lectura del historial", method: "GetHistoryForKey",
			run: func(_ *testing.T, stub *mockStub, contract *SNTContract) error {
				_, err := contract.GetUnitHistory(
					testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := transferFixture(t)
			stub.failOn(tc.method, boom)
			requireCode(t, tc.run(t, stub, contract), cerr.InternalError)
		})
	}
}

// TestPrivateDataFailuresAreTypedAsInternal cubre las fallas de plataforma del
// camino de datos privados que solo aparecen en la recepcion.
func TestPrivateDataFailuresAreTypedAsInternal(t *testing.T) {
	boom := errors.New("fallo simulado de datos privados")

	for _, method := range []string{"GetPrivateData", "DelPrivateData"} {
		t.Run(method, func(t *testing.T) {
			stub, contract := transferFixture(t)
			dispatchToDrugstore(t, stub, contract)
			stub.failOn(method, boom)

			_, err := contract.ReceiveTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, cerr.InternalError)
		})
	}
}

// TestCorruptStateIsTypedAsInternal verifica que un valor ilegible del ledger
// no se confunda con una regla de negocio.
func TestCorruptStateIsTypedAsInternal(t *testing.T) {
	t.Run("unidad corrupta", func(t *testing.T) {
		stub, contract := transferFixture(t)
		key, err := medicationUnitKey(stub, validGTIN, validSerial)
		requireNoError(t, err)
		stub.state[key] = []byte("{ esto no es json")

		_, err = contract.ReadUnit(testContext(stub, labMSP, RoleOperator), validGTIN, validSerial)
		requireCode(t, err, cerr.InternalError)
	})

	t.Run("entrada del registro corrupta", func(t *testing.T) {
		stub, contract := transferFixture(t)
		key, err := organizationKey(stub, labMSP)
		requireNoError(t, err)
		stub.state[key] = []byte("{ esto no es json")

		_, err = contract.RegisterUnit(
			testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest())
		requireCode(t, err, cerr.InternalError)
	})

	t.Run("historial corrupto", func(t *testing.T) {
		stub, contract := transferFixture(t)
		key, err := medicationUnitKey(stub, validGTIN, validSerial)
		requireNoError(t, err)
		stub.history[key][0].Value = []byte("{ esto no es json")

		_, err = contract.GetUnitHistory(
			testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
		requireCode(t, err, cerr.InternalError)
	})

	t.Run("autorizacion de intervencion corrupta", func(t *testing.T) {
		stub, contract := transferFixture(t)
		key, err := labInterventionKey(stub, validGTIN, validSerial)
		requireNoError(t, err)
		stub.state[key] = []byte("{ esto no es json")

		_, err = contract.RevokeLabIntervention(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
		requireCode(t, err, cerr.InternalError)
	})

	t.Run("registro de operacion corrupto", func(t *testing.T) {
		stub, contract := transferFixture(t)
		dispatchToDrugstore(t, stub, contract)
		collection := pairCollectionName(labMSP, drogueriaMSP)
		activeKey, err := transferOpActiveKey(stub, validGTIN, validSerial)
		requireNoError(t, err)
		stub.privateData[collection][activeKey] = []byte("{ esto no es json")

		_, err = contract.ReceiveTransfer(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.InternalError)
	})
}

// TestIdentityFailureIsTypedAsInternal cubre la falla al resolver la identidad
// del invocador, que no es una regla de negocio sino una falla de plataforma.
func TestIdentityFailureIsTypedAsInternal(t *testing.T) {
	stub, contract := transferFixture(t)

	ctx := new(contractapi.TransactionContext)
	ctx.SetStub(stub)
	ctx.SetClientIdentity(&mockIdentity{mspID: labMSP, failMSPID: true, attributes: map[string]string{}})

	_, err := contract.RegisterUnit(ctx, validRegisterUnitRequest())
	requireCode(t, err, cerr.InternalError)

	_, err = contract.Init(ctx)
	requireCode(t, err, cerr.InternalError)
}

// TestSetKeyEndorsementRejectsEmptyPolicy deja fijado que nunca se fije una
// politica de endoso por clave vacia: una clave sin organizaciones exigidas no
// protege nada.
func TestSetKeyEndorsementRejectsEmptyPolicy(t *testing.T) {
	stub := newMockStub()
	err := setKeyEndorsement(testContext(stub, labMSP, RoleOperator), "clave")
	requireCode(t, err, cerr.InternalError)
}

// TestCompositeKeysAreDistinctByObjectType verifica que los cinco tipos de
// clave del prototipo no colisionen entre si en el mismo world state ni en la
// misma coleccion privada.
func TestCompositeKeysAreDistinctByObjectType(t *testing.T) {
	stub := newMockStub()

	unit, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	org, err := organizationKey(stub, labMSP)
	requireNoError(t, err)
	lab, err := labInterventionKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	active, err := transferOpActiveKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	historical, err := transferOpKey(stub, validGTIN, validSerial, "tx-1")
	requireNoError(t, err)
	// Clave del registro de devolucion de ADR-009 punto 2, que implementa
	// EXT-4 (#30); su esquema lo fija CC-1.
	returned, err := returnOpKey(stub, validGTIN, validSerial, "tx-2")
	requireNoError(t, err)

	keys := []string{unit, org, lab, active, historical, returned}
	seen := map[string]struct{}{}
	for _, key := range keys {
		if _, dup := seen[key]; dup {
			t.Fatalf("colision de claves: %q", key)
		}
		seen[key] = struct{}{}
	}
}
