package snt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// dispenseFixture deja la unidad bajo custodia de la farmacia, que es el estado
// del que parte T06.
func dispenseFixture(t *testing.T, custodio string) (*mockStub, *SNTContract) {
	t.Helper()
	stub, contract := transferFixture(t)
	seedUnit(t, stub, domain.StateEnCustodia, custodio)
	return stub, contract
}

func TestDispenseHappyPath(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)

	view, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	if view.Estado != domain.StateDispensado {
		t.Fatalf("estado = %s, se esperaba DISPENSADO", view.Estado)
	}
	if !domain.IsTerminalState(view.Estado) {
		t.Fatal("DISPENSADO debe ser un estado terminal de ADR-001")
	}
	if view.CustodioActual != "GLN:"+farmaciaGLN {
		t.Fatalf("la dispensacion no debe cambiar el custodio: %q", view.CustodioActual)
	}
	if view.UltimaActualizacion != "2026-08-27T12:00:00Z" {
		t.Fatalf("ultimaActualizacion = %q; debe salir de GetTxTimestamp()", view.UltimaActualizacion)
	}
}

// TestDispenseByHealthcareFacility cubre el fin de envase hospitalario, que
// reutiliza esta operacion como simplificacion consciente registrada en
// docs/alcance-prototipo.md.
func TestDispenseByHealthcareFacility(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:7791234500055")

	view, err := contract.Dispense(
		testContext(stub, "CentroMedicoMSP", RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	if view.Estado != domain.StateDispensado {
		t.Fatalf("estado = %s", view.Estado)
	}
}

// TestDispensePersistsNoPatientData es la verificacion explicita del criterio de
// la Ley 25.326: ni el request ni el estado persistido admiten dato alguno del
// paciente. La comprobacion es estructural sobre los tipos, no sobre una
// instancia: un campo nuevo de paciente en el request o en el activo haria
// fallar este test.
func TestDispensePersistsNoPatientData(t *testing.T) {
	prohibited := []string{
		"paciente", "patient", "afiliado", "dni", "documento",
		"obrasocial", "cobertura", "diagnostico", "nombre", "domicilio",
	}

	assertNoProhibitedField := func(what string, value any) {
		t.Helper()
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumField(); i++ {
			field := strings.ToLower(typ.Field(i).Name)
			for _, forbidden := range prohibited {
				if strings.Contains(field, forbidden) {
					t.Fatalf("%s declara el campo %q, excluido del alcance del ledger", what, typ.Field(i).Name)
				}
			}
		}
	}

	assertNoProhibitedField("UnitRefRequest", UnitRefRequest{})
	assertNoProhibitedField("MedicationUnit", MedicationUnit{})

	// Y sobre el estado efectivamente escrito: la unidad dispensada conserva
	// exactamente los siete campos del estado publico del canal.
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)
	_, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)

	var persisted map[string]any
	requireNoError(t, json.Unmarshal(stub.state[key], &persisted))
	expected := []string{
		"gtin", "numeroSerie", "lote", "fechaVencimiento",
		"custodioActual", "estado", "ultimaActualizacion",
	}
	if len(persisted) != len(expected) {
		t.Fatalf("el activo persistido tiene %d campos: %v", len(persisted), persisted)
	}
	for _, field := range expected {
		if _, ok := persisted[field]; !ok {
			t.Fatalf("falta el campo %q en el activo persistido", field)
		}
	}
}

func TestDispenseAuthorizationRejections(t *testing.T) {
	cases := []struct {
		name     string
		custodio string
		mspID    string
		role     string
		want     cerr.Code
	}{
		{
			name: "agentType no habilitado para dispensar", custodio: "GLN:" + drogueriaGLN,
			mspID: drogueriaMSP, role: RoleOperator, want: cerr.UnauthorizedAgentType,
		},
		{
			name: "laboratorio custodio intentando dispensar", custodio: "GLN:" + labGLN,
			mspID: labMSP, role: RoleOperator, want: cerr.UnauthorizedAgentType,
		},
		{
			name: "organizacion regulatoria", custodio: "GLN:" + farmaciaGLN,
			mspID: anmatMSP, role: RoleRegulatoryAdmin, want: cerr.UnauthorizedAgentType,
		},
		{
			name: "farmacia que no es la custodia", custodio: "GLN:" + farmaciaGLN,
			mspID: farmaciaDosMSP, role: RoleOperator, want: cerr.UnauthorizedCustodian,
		},
		{
			name: "farmacia custodia sin rol operador", custodio: "GLN:" + farmaciaGLN,
			mspID: farmaciaMSP, role: RoleAuditor, want: cerr.UnauthorizedRole,
		},
		{
			name: "farmacia custodia sin atributo de rol", custodio: "GLN:" + farmaciaGLN,
			mspID: farmaciaMSP, role: "", want: cerr.UnauthorizedRole,
		},
		{
			name: "organizacion no registrada", custodio: "GLN:" + farmaciaGLN,
			mspID: "OrgFantasmaMSP", role: RoleOperator, want: cerr.OrgNotRegistered,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := dispenseFixture(t, tc.custodio)
			_, err := contract.Dispense(
				testContext(stub, tc.mspID, tc.role),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, tc.want)
		})
	}
}

// TestDispenseRejectsBlockingAndNonCustodyStates recorre TODOS los estados de
// ADR-001 distintos de EN_CUSTODIA y exige que ninguno admita T06. Una unidad
// vencida, en cuarentena, retirada, prohibida, robada, extraviada, deteriorada
// o devuelta no es dispensable, y tampoco lo es una que sigue bajo custodia del
// laboratorio o en transito.
func TestDispenseRejectsBlockingAndNonCustodyStates(t *testing.T) {
	states := []domain.State{
		domain.StateEnLaboratorio, domain.StateEnTransito, domain.StateEnCuarentena,
		domain.StateVencido, domain.StateRobado, domain.StateExtraviado,
		domain.StateDeteriorado, domain.StateRetiradoMercado, domain.StateProhibido,
		domain.StateDevuelto, domain.StateDispensado, domain.StateDispuestoFinal,
	}

	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			stub, contract := transferFixture(t)
			seedUnit(t, stub, state, "GLN:"+farmaciaGLN)

			_, err := contract.Dispense(
				testContext(stub, farmaciaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}

// TestDispenseIsTerminal verifica que no haya transiciones de negocio
// posteriores: una segunda dispensacion y un despacho posterior se rechazan.
func TestDispenseIsTerminal(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)
	ctx := testContext(stub, farmaciaMSP, RoleOperator)
	req := UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial}

	_, err := contract.Dispense(ctx, req)
	requireNoError(t, err)

	_, err = contract.Dispense(ctx, req)
	requireCode(t, err, cerr.InvalidStateTransition)

	withTransient(stub, dispatchTransient("GLN:7791234500055"))
	_, err = contract.DispatchTransfer(ctx, DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidStateTransition)
}

func TestDispenseRejectsInvalidUnitRef(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)

	cases := map[string]UnitRefRequest{
		"GTIN invalido":      {GTIN: "07791234567890", NumeroSerie: validSerial},
		"serie vacia":        {GTIN: validGTIN},
		"unidad inexistente": {GTIN: validGTIN, NumeroSerie: "SN-9999-ZZZZ"},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := contract.Dispense(testContext(stub, farmaciaMSP, RoleOperator), req)
			if name == "unidad inexistente" {
				requireCode(t, err, cerr.UnitNotFound)
				return
			}
			requireCode(t, err, cerr.InvalidRequest)
		})
	}
}

// TestDispenseEmitsEvent cubre la notificacion del evento de dispensacion, que
// es la que el listener de ANMAT (NET-8, #64) consume.
func TestDispenseEmitsEvent(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)

	_, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	payload, ok := stub.events[opDispense]
	if !ok {
		t.Fatal("Dispense no emitio evento de chaincode")
	}
	var emitted MedicationUnit
	requireNoError(t, json.Unmarshal(payload, &emitted))
	if emitted.Estado != domain.StateDispensado {
		t.Fatalf("payload del evento: %+v", emitted)
	}
}

// TestDispenseDoesNotHardenEndorsementPolicy fija el punto 6.h de ADR-007: en un
// estado terminal la politica queda en el valor de reposo del ultimo custodio
// registrado y no se endurece; es la logica la que rechaza los intentos.
func TestDispenseDoesNotHardenEndorsementPolicy(t *testing.T) {
	stub, contract := transferFixture(t)
	dispatchToDrugstore(t, stub, contract)
	_, err := contract.ReceiveTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	// La drogueria despacha a la farmacia y esta recibe, de modo que la politica
	// de reposo queda en la farmacia antes de dispensar.
	stub.txID = "tx-0000000000000002"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err = contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}
	_, err = contract.ReceiveTransfer(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	before := endorsingOrganizations(t, stub.validation[key])

	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)

	after := endorsingOrganizations(t, stub.validation[key])
	if len(after) != 1 || after[0] != farmaciaMSP {
		t.Fatalf("politica tras dispensar = %v, se esperaba unicamente %s", after, farmaciaMSP)
	}
	if len(before) != len(after) || before[0] != after[0] {
		t.Fatalf("la dispensacion no debe modificar la politica de la clave: %v -> %v", before, after)
	}
}
