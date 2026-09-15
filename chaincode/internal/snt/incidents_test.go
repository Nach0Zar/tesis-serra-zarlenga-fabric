package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Tests de EXT-3 (#29): ReportStolen (T14), ReportLost (T15) y
// ReportDamaged (T16).

type incidentCase struct {
	name     string
	invoke   func(*SNTContract, contractapi.TransactionContextInterface, UnitEventRequest) (*MedicationUnitView, error)
	estado   domain.State
	terminal bool
}

func incidentRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "incidente documentado por el custodio",
	}
}

func incidentCases() []incidentCase {
	return []incidentCase{
		{"ReportStolen", (*SNTContract).ReportStolen, domain.StateRobado, true},
		{"ReportLost", (*SNTContract).ReportLost, domain.StateExtraviado, true},
		// DETERIORADO es bloqueante y NO terminal: conserva la salida T29 hacia
		// DISPUESTO_FINAL, porque un producto deteriorado todavia debe disponerse
		// como residuo peligroso.
		{"ReportDamaged", (*SNTContract).ReportDamaged, domain.StateDeteriorado, false},
	}
}

// TestIncidentsBlockFurtherOperations cubre las tres operaciones desde
// EN_CUSTODIA y el criterio "unidad bloqueada para operaciones futuras".
//
// El bloqueo no necesita regla propia del chaincode: lo produce ADR-001. Pero
// los tres estados NO son equivalentes, y el test lo afirma por separado:
// ROBADO y EXTRAVIADO son terminales, mientras DETERIORADO es bloqueante y
// conserva la salida T29 hacia DISPUESTO_FINAL.
func TestIncidentsBlockFurtherOperations(t *testing.T) {
	for _, c := range incidentCases() {
		t.Run(c.name, func(t *testing.T) {
			stub, contract := verifyFixture(t)

			view, err := c.invoke(contract, testContext(stub, drogueriaMSP, RoleOperator), incidentRequest())
			requireNoError(t, err)
			if view.Estado != c.estado {
				t.Fatalf("estado = %s, se esperaba %s", view.Estado, c.estado)
			}
			if domain.IsTerminalState(view.Estado) != c.terminal {
				t.Fatalf("%s: terminal=%v, ADR-001 declara %v",
					view.Estado, domain.IsTerminalState(view.Estado), c.terminal)
			}
			if !domain.IsBlockingState(view.Estado) && !c.terminal {
				t.Fatalf("%s deberia ser bloqueante", view.Estado)
			}

			// Bloqueo efectivo: ninguna operacion posterior procede.
			withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
			_, err = contract.DispatchTransfer(
				testContext(stub, drogueriaMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, cerr.InvalidStateTransition)
			stub.transient = map[string][]byte{}

			_, err = c.invoke(contract, testContext(stub, drogueriaMSP, RoleOperator), incidentRequest())
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}

// TestIncidentsAreNotOpenToDeclaredRecipient fija la diferencia con T09 y T13:
// ADR-001 reserva T14-T16 al custodio actual o a ANMAT aunque la unidad este en
// transito, porque son hechos sobre los que el destinatario declarado no tiene
// conocimiento propio mientras la unidad no este en su poder.
func TestIncidentsAreNotOpenToDeclaredRecipient(t *testing.T) {
	for _, c := range incidentCases() {
		t.Run(c.name, func(t *testing.T) {
			stub, contract := transferFixture(t)
			withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
			_, err := contract.DispatchTransfer(
				testContext(stub, labMSP, RoleOperator),
				DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireNoError(t, err)
			stub.transient = map[string][]byte{}

			// La drogueria es el destinatario declarado, no el custodio.
			_, err = c.invoke(contract, testContext(stub, drogueriaMSP, RoleOperator), incidentRequest())
			requireCode(t, err, cerr.UnauthorizedCustodian)

			// El emisor, que sigue siendo el custodio registrado, si puede.
			view, err := c.invoke(contract, testContext(stub, labMSP, RoleOperator), incidentRequest())
			requireNoError(t, err)
			if view.Estado != c.estado {
				t.Fatalf("estado = %s, se esperaba %s", view.Estado, c.estado)
			}
		})
	}
}

// TestIncidentFromTransitClosesTransfer comprueba que informar un incidente
// durante el transito cierre el registro de la operacion y restaure la politica
// de reposo al emisor, igual que T09 y T13.
func TestIncidentFromTransitClosesTransfer(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-robo"
	_, err = contract.ReportStolen(
		testContext(stub, labMSP, RoleOperator), incidentRequest())
	requireNoError(t, err)

	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	collection := pairCollectionName(labMSP, drogueriaMSP)
	_, found, err := readActiveTransferOperation(ctx, collection, validGTIN, validSerial)
	requireNoError(t, err)
	if found {
		t.Fatal("el incidente en transito debe cerrar el registro de la operacion activa")
	}

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo = %v, se esperaba unicamente el emisor %s", orgs, labMSP)
	}
}

// TestIncidentsRejections cubre las condiciones de rechazo comunes.
func TestIncidentsRejections(t *testing.T) {
	t.Run("invocador ajeno", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.ReportStolen(
			testContext(stub, farmaciaMSP, RoleOperator), incidentRequest())
		requireCode(t, err, cerr.UnauthorizedCustodian)
	})

	t.Run("motivo ausente", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.ReportLost(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("ANMAT puede informar", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		stub.txID = "tx-deterioro-anmat"
		_, err := contract.ReportDamaged(
			testContext(stub, anmatMSP, RoleRegulatoryAdmin), incidentRequest())
		requireNoError(t, err)
		requireRegulatoryMarker(t, stub, opReportDamaged)
	})
}
