package snt

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// contractOperations es la superficie publica congelada por
// docs/api-contract.md (v2.6.1). Es la lista completa: agregar, quitar o
// renombrar una operacion es un cambio del contrato que exige su propio PR con
// aprobacion explicita, nunca un efecto colateral de una issue de
// implementacion.
var contractOperations = []string{
	// Inicializacion (ADR-010, punto 4)
	"Init",
	// Escritura ordinaria
	"RegisterUnit",
	"DispatchTransfer",
	"ReceiveTransfer",
	"RejectTransfer",
	"Dispense",
	// Eventos extraordinarios y resolucion
	"Quarantine",
	"ReleaseQuarantine",
	"ReportExpired",
	"ReportStolen",
	"ReportLost",
	"ReportDamaged",
	"WithdrawFromMarket",
	"ProhibitProduct",
	"ReturnProduct",
	"Restock",
	"FinalDisposition",
	// Intervencion de laboratorio no custodio (ADR-007, puntos 6.e y 6.f)
	"AuthorizeLabIntervention",
	"RevokeLabIntervention",
	// Registro organizacion-establecimiento (ADR-003, ADR-010)
	"RegisterOrganization",
	"SetOrganizationActive",
	// Lectura
	"ReadUnit",
	"GetUnitHistory",
	"QueryUnitsByGTIN",
	"VerifyTrace",
}

// TestContractSurfaceMatchesFrozenContract verifica que el chaincode declare
// exactamente las operaciones del contrato congelado: ni una de menos (una
// operacion que el cliente o la baseline esperan y no existe) ni una de mas
// (superficie publica no documentada).
func TestContractSurfaceMatchesFrozenContract(t *testing.T) {
	contractType := reflect.TypeOf(&SNTContract{})

	// Metodos que SNTContract hereda de contractapi.Contract y que no forman
	// parte de la superficie de negocio.
	inherited := map[string]bool{}
	base := reflect.TypeOf(&contractapi.Contract{})
	for i := 0; i < base.NumMethod(); i++ {
		inherited[base.Method(i).Name] = true
	}

	var declared []string
	for i := 0; i < contractType.NumMethod(); i++ {
		name := contractType.Method(i).Name
		if inherited[name] {
			continue
		}
		declared = append(declared, name)
	}

	expected := append([]string(nil), contractOperations...)
	sort.Strings(expected)
	sort.Strings(declared)

	if !reflect.DeepEqual(expected, declared) {
		t.Fatalf("superficie del chaincode distinta de la del contrato v%s\n  declaradas: %v\n  contrato:   %v",
			ContractVersion, declared, expected)
	}
}

// TestChaincodeBuildsWithContractAPI comprueba que contractapi acepte todas las
// firmas declaradas. Es lo que garantiza el criterio "invocacion dummy
// responde" de CC-1: si una firma no fuera admisible, NewChaincode fallaria y
// el chaincode no arrancaria en el peer.
func TestChaincodeBuildsWithContractAPI(t *testing.T) {
	contract := new(SNTContract)
	contract.Info.Version = ContractVersion

	chaincode, err := contractapi.NewChaincode(contract)
	if err != nil {
		t.Fatalf("contractapi rechazo la superficie del contrato: %v", err)
	}
	if chaincode == nil {
		t.Fatal("NewChaincode devolvio un chaincode nulo")
	}
}

// TestDeclaredOperationsReportTheirOwner verifica que las operaciones que este
// scaffold declara sin implementar devuelvan un error tipificado que nombre a
// su issue duena, en lugar de fallar de forma opaca.
func TestDeclaredOperationsReportTheirOwner(t *testing.T) {
	stub := newMockStub()
	ctx := testContext(stub, labMSP, RoleOperator)
	contract := new(SNTContract)

	pending := map[string]func() error{
		"Dispense": func() error {
			_, err := contract.Dispense(ctx, UnitRefRequest{})
			return err
		},
		"Quarantine": func() error {
			_, err := contract.Quarantine(ctx, UnitEventRequest{})
			return err
		},
		"ReleaseQuarantine": func() error {
			_, err := contract.ReleaseQuarantine(ctx, UnitEventRequest{})
			return err
		},
		"ReportExpired": func() error {
			_, err := contract.ReportExpired(ctx, UnitEventRequest{})
			return err
		},
		"ReportStolen": func() error {
			_, err := contract.ReportStolen(ctx, UnitEventRequest{})
			return err
		},
		"ReportLost": func() error {
			_, err := contract.ReportLost(ctx, UnitEventRequest{})
			return err
		},
		"ReportDamaged": func() error {
			_, err := contract.ReportDamaged(ctx, UnitEventRequest{})
			return err
		},
		"WithdrawFromMarket": func() error {
			_, err := contract.WithdrawFromMarket(ctx, UnitEventRequest{})
			return err
		},
		"ProhibitProduct": func() error {
			_, err := contract.ProhibitProduct(ctx, UnitEventRequest{})
			return err
		},
		"ReturnProduct": func() error {
			_, err := contract.ReturnProduct(ctx, UnitEventRequest{})
			return err
		},
		"Restock": func() error {
			_, err := contract.Restock(ctx, UnitEventRequest{})
			return err
		},
		"FinalDisposition": func() error {
			_, err := contract.FinalDisposition(ctx, UnitEventRequest{})
			return err
		},
		"ReadUnit": func() error {
			_, err := contract.ReadUnit(ctx, validGTIN, validSerial)
			return err
		},
		"GetUnitHistory": func() error {
			_, err := contract.GetUnitHistory(ctx, validGTIN, validSerial)
			return err
		},
		"QueryUnitsByGTIN": func() error {
			_, err := contract.QueryUnitsByGTIN(ctx, validGTIN)
			return err
		},
		"VerifyTrace": func() error {
			_, err := contract.VerifyTrace(ctx, validGTIN, validSerial)
			return err
		},
	}

	for name, invoke := range pending {
		t.Run(name, func(t *testing.T) {
			err := invoke()
			parsed, ok := cerr.Parse(err)
			if !ok {
				t.Fatalf("%s no devolvio un error con el formato del contrato: %v", name, err)
			}
			if parsed.Code != cerr.InternalError {
				t.Fatalf("%s devolvio %s", name, parsed.Code)
			}
			if parsed.Details["issue"] == nil {
				t.Fatalf("%s no nombra a la issue duena de su implementacion", name)
			}
		})
	}
}

// TestImplementedOperationsAreNotStubs deja constancia de cuales quedan
// efectivamente implementadas por CC-1 (#14).
func TestImplementedOperationsAreNotStubs(t *testing.T) {
	stub := newMockStub()
	seedRegistry(t, stub)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	contract := new(SNTContract)

	// Init ya se ejecuto en seedRegistry y devolvio la entrada sembrada.
	if _, err := contract.RegisterOrganization(ctx, RegisterOrganizationRequest{
		MSPID: labMSP, ID: labGLN, IDType: IDTypeGLN,
		AgentType: "LABORATORY", Active: true,
	}); err != nil {
		t.Fatalf("RegisterOrganization deberia estar implementada: %v", err)
	}
	if _, err := contract.SetOrganizationActive(ctx, SetOrganizationActiveRequest{
		MSPID: labMSP, Active: true,
	}); err != nil {
		t.Fatalf("SetOrganizationActive deberia estar implementada: %v", err)
	}
	// T01, implementada por CC-2 (#15).
	if _, err := contract.RegisterUnit(
		testContext(stub, labMSP, RoleOperator), validRegisterUnitRequest()); err != nil {
		t.Fatalf("RegisterUnit deberia estar implementada: %v", err)
	}

	// T02/T03, T04 y T05, implementadas por CC-3 (#16).
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	if _, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("DispatchTransfer deberia estar implementada: %v", err)
	}
	stub.transient = map[string][]byte{}
	if _, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}); err != nil {
		t.Fatalf("ReceiveTransfer deberia estar implementada: %v", err)
	}
}
