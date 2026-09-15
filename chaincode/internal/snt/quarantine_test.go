package snt

import (
	"encoding/json"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-1 (#27): Quarantine (T07, T08, T09) y ReleaseQuarantine (T10).

func quarantineRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "anomalia detectada en control de calidad",
	}
}

// TestQuarantineFromLaboratory cubre T07: el laboratorio, que todavia es
// custodio, suspende su propia unidad antes de despacharla.
func TestQuarantineFromLaboratory(t *testing.T) {
	stub, contract := transferFixture(t)

	view, err := contract.Quarantine(
		testContext(stub, labMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateEnCuarentena {
		t.Fatalf("estado = %s, se esperaba EN_CUARENTENA", view.Estado)
	}
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("la cuarentena no debe mover la custodia: %s", view.CustodioActual)
	}
}

// TestQuarantineFromCustody cubre T08, y de paso el bloqueo: ADR-001 no declara
// despacho ni dispensa desde EN_CUARENTENA, de modo que la suspension de la
// circulacion no necesita una regla propia del chaincode.
func TestQuarantineFromCustody(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)

	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err = contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestQuarantineFromTransitByDeclaredRecipient es el caso que motiva la
// precision de ADR-001 para T09 y que el contrato incorporo en la 2.6.0: la
// anomalia detectada AL RECIBIR. El destinatario declarado no es el custodio
// registrado -- durante el transito lo sigue siendo el emisor (ADR-004) -- y aun
// asi puede poner la unidad en cuarentena.
//
// Comprueba ademas las tres piezas del cierre de transito que exige ADR-007:
// registro historico, eliminacion de la clave activa y restauracion de la
// politica de reposo al emisor. Sin la tercera, la unidad queda bajo una
// politica que exige al receptor de un despacho ya resuelto: bloqueo permanente.
func TestQuarantineFromTransitByDeclaredRecipient(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-cuarentena-transito"
	view, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateEnCuarentena {
		t.Fatalf("estado = %s, se esperaba EN_CUARENTENA", view.Estado)
	}
	// La custodia sigue en el emisor: T09 no es una entrega.
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodio = %s, el transito no se consumo y debe seguir en el laboratorio", view.CustodioActual)
	}

	// Cierre del registro: la clave activa ya no existe.
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	collection := pairCollectionName(labMSP, drogueriaMSP)
	_, found, err := readActiveTransferOperation(ctx, collection, validGTIN, validSerial)
	requireNoError(t, err)
	if found {
		t.Fatal("T09 debe cerrar el registro de la operacion activa (ADR-006, punto 4)")
	}

	// Restauracion de la politica de reposo hacia el EMISOR.
	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo tras T09 = %v, se esperaba unicamente el emisor %s", orgs, labMSP)
	}
}

// TestQuarantineFromTransitByRegulatorWritesMarker: cuando el evento lo inicia
// ANMAT, su firma de creador acredita identidad pero no prueba que un peer suyo
// haya ejecutado la logica. El marcador de participacion es lo que convierte esa
// intervencion en coendoso real (ADR-007, punto 6.d).
func TestQuarantineFromTransitByRegulatorWritesMarker(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-cuarentena-anmat"
	_, err = contract.Quarantine(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), quarantineRequest())
	requireNoError(t, err)
	requireRegulatoryMarker(t, stub, opQuarantine)
}

// TestQuarantineByCustodianWritesNoRegulatoryMarker es la contracara: escribir
// el marcador siempre convertiria a AnmatMSP en coendosante obligatoria de
// eventos que no inicio, que es exactamente lo que DES-6 prohibe.
func TestQuarantineByCustodianWritesNoRegulatoryMarker(t *testing.T) {
	stub, contract := verifyFixture(t)

	stub.txID = "tx-cuarentena-custodio"
	_, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)

	// La asercion es por CLAVE de esta transaccion y no sobre la coleccion
	// entera: el alta del registro ya dejo marcadores de ANMAT al sembrar las
	// organizaciones (ADR-007, punto 6.g), y mirar el tamano confundiria esos
	// con el que este evento no debe escribir.
	markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	if stub.privateData[implicitCollection(anmatMSP)][markerKey] != nil {
		t.Fatal("un evento iniciado por el custodio no debe escribir el marcador regulatorio")
	}
}

// TestQuarantineRejectsUnrelatedInvoker cubre el criterio de rechazo de la
// issue: quien no es custodio, destinatario declarado ni regulador recibe
// UNAUTHORIZED_CUSTODIAN, y no un rechazo de transicion — el problema no es que
// ADR-001 no declare T08, es que quien la pide no tiene caracter para pedirla.
func TestQuarantineRejectsUnrelatedInvoker(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.Quarantine(
		testContext(stub, farmaciaMSP, RoleOperator), quarantineRequest())
	requireCode(t, err, cerr.UnauthorizedCustodian)
}

// TestQuarantineRejectsRegulatorWithoutAdminRole: DES-6 asigna los eventos
// extraordinarios al rol regulatory-admin. `auditor` habilita lecturas, no
// escrituras.
func TestQuarantineRejectsRegulatorWithoutAdminRole(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.Quarantine(
		testContext(stub, anmatMSP, RoleAuditor), quarantineRequest())
	requireCode(t, err, cerr.UnauthorizedRole)
}

// TestReleaseQuarantineReturnsToCustody cubre T10.
func TestReleaseQuarantineReturnsToCustody(t *testing.T) {
	stub, contract := verifyFixture(t)
	_, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)

	view, err := contract.ReleaseQuarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateEnCustodia {
		t.Fatalf("estado = %s, se esperaba EN_CUSTODIA", view.Estado)
	}
}

// TestReleaseQuarantineIsNotOpenToDeclaredRecipient fija la asimetria que
// declara la issue: el destinatario declarado puede poner en cuarentena desde
// EN_TRANSITO (T09) pero no liberar (T10). No es una restriccion arbitraria —
// al cerrar el transito, T09 elimino el registro de la operacion y con el la
// figura misma de destinatario declarado.
func TestReleaseQuarantineIsNotOpenToDeclaredRecipient(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-cuarentena"
	_, err = contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)

	// La drogueria era el destinatario declarado; ya no lo es.
	_, err = contract.ReleaseQuarantine(
		testContext(stub, drogueriaMSP, RoleOperator), quarantineRequest())
	requireCode(t, err, cerr.UnauthorizedCustodian)

	// El emisor, que es el custodio registrado, si puede liberar.
	view, err := contract.ReleaseQuarantine(
		testContext(stub, labMSP, RoleOperator), quarantineRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateEnCustodia {
		t.Fatalf("estado = %s, se esperaba EN_CUSTODIA", view.Estado)
	}
}

// requireRegulatoryMarker comprueba el marcador de participacion escrito en la
// coleccion implicita de ANMAT.
func requireRegulatoryMarker(t *testing.T, stub *mockStub, operation string) {
	t.Helper()
	markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
	requireNoError(t, err)
	raw := stub.privateData[implicitCollection(anmatMSP)][markerKey]
	if raw == nil {
		t.Fatal("el evento iniciado por el regulador debe escribir su marcador de participacion")
	}
	var decoded participationMarker
	requireNoError(t, json.Unmarshal(raw, &decoded))
	if decoded.Operacion != operation || decoded.MSPID != anmatMSP {
		t.Fatalf("marcador = %+v", decoded)
	}
}

// TestQuarantineRequiresMotivo: un evento extraordinario deja un asiento
// permanente en la traza y la causa regulatoria es parte del asiento, con el
// mismo criterio que RejectTransfer y AuthorizeLabIntervention.
func TestQuarantineRequiresMotivo(t *testing.T) {
	stub, contract := verifyFixture(t)

	_, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)

	_, err = contract.ReleaseQuarantine(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)
}
