package snt

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Tests de la resolucion de caracteres compartida por los eventos
// extraordinarios (internal/snt/events.go).

const lab2MSP = "Lab2MSP"

const lab2GLN = "7791234500079"

// foreignLabFixture deja la unidad EN_CUSTODIA de la drogueria y registra un
// SEGUNDO laboratorio que no la custodia y no es parte de ninguna operacion
// sobre ella.
func foreignLabFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub, contract := labInterventionFixture(t)
	registerOrg(t, stub, lab2MSP, lab2GLN, domain.AgentLaboratory)
	return stub, contract
}

// TestForeignLaboratoryIsUnauthorizedNotMisrouted protege la semantica de los
// dos codigos de rechazo, que el catalogo del contrato define distinto:
// INVALID_STATE_TRANSITION dice que el estado de origen no admite la transicion,
// UNAUTHORIZED_CUSTODIAN que el invocador no tiene relacion con la unidad.
//
// El caso es un laboratorio AJENO a la unidad: no la custodia, no es el
// destinatario declarado de ninguna operacion y el evento que pide es uno que
// ADR-001 no le abre en ninguna fila. La transicion existe y procede -- una
// drogueria custodia la unidad y podria pedirla --; lo que no procede es que la
// pida este invocador, y el codigo correcto es UNAUTHORIZED_CUSTODIAN.
//
// Sin la comprobacion por evento de invokerCharacters, cualquier organizacion
// LABORATORY reuniria el caracter LABORATORY para CUALQUIER evento y estos casos
// devolverian INVALID_STATE_TRANSITION: una regresion sobre operaciones de
// EXT-1 a EXT-4 que rompe la paridad con el cliente y la baseline, que ramifican
// por `code`.
func TestForeignLaboratoryIsUnauthorizedNotMisrouted(t *testing.T) {
	operaciones := []struct {
		nombre  string
		invocar func(*SNTContract, contractapi.TransactionContextInterface) error
	}{
		{"Quarantine", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.Quarantine(ctx, incidentRequest())
			return err
		}},
		{"ReleaseQuarantine", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReleaseQuarantine(ctx, incidentRequest())
			return err
		}},
		{"ReportExpired", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReportExpired(ctx, incidentRequest())
			return err
		}},
		{"ReportStolen", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReportStolen(ctx, incidentRequest())
			return err
		}},
		{"ReportLost", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReportLost(ctx, incidentRequest())
			return err
		}},
		{"ReportDamaged", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReportDamaged(ctx, incidentRequest())
			return err
		}},
		{"ReturnProduct", func(c *SNTContract, ctx contractapi.TransactionContextInterface) error {
			_, err := c.ReturnProduct(ctx, incidentRequest())
			return err
		}},
	}

	for _, caso := range operaciones {
		t.Run(caso.nombre, func(t *testing.T) {
			stub, contract := foreignLabFixture(t)
			err := caso.invocar(contract, testContext(stub, lab2MSP, RoleOperator))
			requireCode(t, err, cerr.UnauthorizedCustodian)
		})
	}
}

// TestForeignLaboratoryStillReachesWithdrawal es el contraste que demuestra que
// la comprobacion por evento NO es un bloqueo general al laboratorio: para
// RETIRAR_MERCADO, que ADR-001 SI le abre (T17-T19), el mismo invocador ajeno
// reune el caracter, la transicion lo admite y el rechazo pasa a ser el de la
// autorizacion de intervencion que ADR-007 punto 6.e exige.
//
// Sin este caso, la correccion podria haberse implementado exigiendo relacion
// con la unidad y nadie lo habria notado: el retiro voluntario de un laboratorio
// no custodio es justamente el caso de uso principal de EXT-6.
func TestForeignLaboratoryStillReachesWithdrawal(t *testing.T) {
	stub, contract := foreignLabFixture(t)

	_, err := contract.WithdrawFromMarket(
		testContext(stub, lab2MSP, RoleOperator), incidentRequest())
	requireCode(t, err, cerr.LabInterventionRequired)
}
