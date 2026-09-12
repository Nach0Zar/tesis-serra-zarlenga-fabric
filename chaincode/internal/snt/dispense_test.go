package snt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

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

// TestDispenseRejectsInactiveDispenser cubre la condicion que #17 enuncia
// explicitamente -- la organizacion dispensadora debe tener `active=true` -- y
// que DES-6 impone a toda organizacion custodial.
//
// Tiene test propio, y no queda subsumido en la tabla de rechazos de
// autorizacion, por dos razones. La primera es que es un criterio de aceptacion
// declarado de CC-4, no una rama interna cubierta de rebote. La segunda es que
// el escenario solo es concluyente si la desactivacion ocurre por la VIA REAL:
// una farmacia registrada, habilitada, custodia de la unidad y con rol
// operador, a la que la organizacion regulatoria da de baja con
// SetOrganizationActive. Marcar `active=false` escribiendo el registro a mano
// probaria que readOrganization lee un booleano, no que el circuito de baja
// regulatoria efectivamente inhabilita a dispensar.
//
// Se verifica ademas que el rechazo NO deje escritura sobre la unidad: una
// dispensa rechazada no puede haber mutado el estado ni la ultima
// actualizacion.
func TestDispenseRejectsInactiveDispenser(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)

	before, err := readUnit(testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	if before.Estado != domain.StateEnCustodia {
		t.Fatalf("la premisa del caso exige la unidad en EN_CUSTODIA, esta en %s", before.Estado)
	}

	// Baja por el circuito regulatorio, no escribiendo el registro a mano.
	if _, err := contract.SetOrganizationActive(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		SetOrganizationActiveRequest{MSPID: farmaciaMSP, Active: false}); err != nil {
		t.Fatalf("SetOrganizationActive: %v", err)
	}

	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.OrgInactive)

	after, err := readUnit(testContext(stub, anmatMSP, RoleRegulatoryAdmin), validGTIN, validSerial)
	requireNoError(t, err)
	if after != before {
		t.Fatalf("la dispensa rechazada dejo escritura sobre la unidad:\n  antes:   %+v\n  despues: %+v",
			before, after)
	}

	// Rehabilitada por la misma via, la dispensa procede: lo que bloqueaba era
	// la habilitacion y no un efecto colateral del intento fallido.
	if _, err := contract.SetOrganizationActive(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		SetOrganizationActiveRequest{MSPID: farmaciaMSP, Active: true}); err != nil {
		t.Fatalf("SetOrganizationActive (rehabilitacion): %v", err)
	}
	view, err := contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	if view.Estado != domain.StateDispensado {
		t.Fatalf("estado tras rehabilitar = %s", view.Estado)
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

// TestDispenseRejectsUnitExpiredByDate cubre la mitad de la aptitud que el
// estado no expresa: la FECHA.
//
// El paso del tiempo no ejecuta transacciones -- VENCIDO se alcanza por
// T11/T12/T13, que alguien tiene que invocar --, de modo que una unidad cuya
// fecha ya paso sigue registrada como EN_CUSTODIA hasta que alguien lo informe.
// Sin esta comprobacion se le entrega a un paciente un medicamento cuya
// caducidad el propio ledger registra.
//
// La condicion se comparte con VerifyUnit (ADR-013): el test verifica ademas
// que las dos operaciones coincidan sobre la MISMA unidad, porque una
// divergencia dejaria al prototipo contradiciendose consigo mismo -- la
// verificacion previa a la compra marcandola vencida y la dispensacion
// entregandola igual.
func TestDispenseRejectsUnitExpiredByDate(t *testing.T) {
	stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)
	// validRegisterUnitRequest persiste fechaVencimiento 2027-12-31.
	stub.timestamp = time.Date(2028, 1, 1, 9, 0, 0, 0, time.UTC)

	before, err := readUnit(testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)

	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidStateTransition)

	parsed, ok := cerr.Parse(err)
	if !ok {
		t.Fatalf("error sin el formato del contrato: %v", err)
	}
	if parsed.Details["causa"] != "VENCIDO_POR_FECHA" {
		t.Fatalf("el rechazo debe distinguirse de un estado invalido: %+v", parsed.Details)
	}

	after, err := readUnit(testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	if after != before {
		t.Fatalf("la dispensa rechazada dejo escritura sobre la unidad:\n  antes:   %+v\n  despues: %+v",
			before, after)
	}

	// Y las dos operaciones coinciden sobre la misma unidad.
	verdict, err := contract.VerifyUnit(
		testContext(stub, farmaciaMSP, RoleOperator), validGTIN, validSerial)
	requireNoError(t, err)
	if verdict.Autentica {
		t.Fatal("VerifyUnit y Dispense deben coincidir: la unidad no es apta")
	}
}

// TestDispenseExpiryBoundary fija el mismo limite que TestVerifyUnitExpiryBoundary
// y por la misma razon: `fechaVencimiento` es el ULTIMO DIA OPERABLE. Que las dos
// operaciones lo prueben por separado es deliberado -- comparten
// implementacion, y este par de tests es lo que hace fallar cualquier intento
// futuro de separarlas.
func TestDispenseExpiryBoundary(t *testing.T) {
	cases := []struct {
		name    string
		at      time.Time
		permite bool
	}{
		{"el ultimo dia operable", time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC), true},
		{"el dia siguiente", time.Date(2028, 1, 1, 0, 0, 1, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := dispenseFixture(t, "GLN:"+farmaciaGLN)
			stub.timestamp = tc.at

			_, err := contract.Dispense(
				testContext(stub, farmaciaMSP, RoleOperator),
				UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
			if tc.permite {
				requireNoError(t, err)
				return
			}
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}
