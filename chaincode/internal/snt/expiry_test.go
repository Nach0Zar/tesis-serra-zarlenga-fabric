package snt

import (
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-2 (#28): ReportExpired (T11, T12, T13).

func expiredRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "caducidad documentada por corte de cadena de frio",
	}
}

// TestReportExpiredFromLaboratory cubre T11.
func TestReportExpiredFromLaboratory(t *testing.T) {
	stub, contract := transferFixture(t)

	view, err := contract.ReportExpired(
		testContext(stub, labMSP, RoleOperator), expiredRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateVencido {
		t.Fatalf("estado = %s, se esperaba VENCIDO", view.Estado)
	}
}

// TestReportExpiredFromCustodyBlocksDispensing cubre T12 y el criterio de la
// issue "unidad vencida bloqueada para dispensacion".
//
// El test llega hasta requireTransition A PROPOSITO: Dispense valida primero
// agentType, despues custodia y recien entonces la transicion, de modo que una
// farmacia ajena fallaria con UNAUTHORIZED_CUSTODIAN y el custodio drogueria con
// UNAUTHORIZED_AGENT_TYPE, sin tocar nunca la maquina de estados. Se usa una
// unidad cuyo custodio actual ES la farmacia para que el unico control que puede
// rechazarla sea el ESTADO, y asi el test falle si alguna vez se reabriera T06
// desde VENCIDO.
func TestReportExpiredFromCustodyBlocksDispensing(t *testing.T) {
	stub, contract := dispensableFixture(t)

	_, err := contract.ReportExpired(
		testContext(stub, farmaciaMSP, RoleOperator), expiredRequest())
	requireNoError(t, err)

	_, err = contract.Dispense(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestReportExpiredBeforeExpirationDate fija la lectura de ADR-001 que gobierna
// esta operacion: la precondicion de T11/T12 es una DISYUNCION -- "la fecha fue
// alcanzada O se documenta la caducidad" --, de modo que informar la caducidad
// de una unidad cuya fecha impresa todavia no paso es un camino legitimo y no
// un error.
//
// Es la asimetria deliberada con Dispense y VerifyUnit, que si leen la fecha:
// aquellas no entregan ni declaran apta una unidad ya caduca; esta ESCRIBE el
// hecho de la caducidad, que puede anticiparse a la fecha.
//
// Vale UNICAMENTE para T11/T12. T13 tiene su propia precondicion temporal y la
// cubre TestReportExpiredInTransitRequiresReachedDate.
func TestReportExpiredBeforeExpirationDate(t *testing.T) {
	stub, contract := verifyFixture(t)

	// La fecha del fixture es 2027-12-31 y el timestamp de la transaccion es
	// muy anterior: la unidad NO esta vencida por fecha.
	unit, err := readUnit(testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)
	expired, err := unitExpiredByDate(testContext(stub, anmatMSP, RoleAuditor), unit)
	requireNoError(t, err)
	if expired {
		t.Fatal("el fixture deberia tener una unidad no vencida por fecha")
	}

	view, err := contract.ReportExpired(
		testContext(stub, drogueriaMSP, RoleOperator), expiredRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateVencido {
		t.Fatalf("estado = %s, se esperaba VENCIDO", view.Estado)
	}
}

// TestReportExpiredFromTransitByDeclaredRecipient cubre T13 desde EN_TRANSITO,
// la segunda de las dos transiciones que ADR-001 abre al destinatario
// declarado, y verifica que cierre el transito con sus tres piezas.
func TestReportExpiredFromTransitByDeclaredRecipient(t *testing.T) {
	stub, contract := transferFixture(t)
	stub.txID = "tx-despacho"
	withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, labMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	// T13 exige la fecha alcanzada (ADR-001, sin la disyuncion de T11/T12).
	stub.timestamp = time.Date(2028, time.January, 2, 0, 0, 0, 0, time.UTC)
	stub.txID = "tx-vencimiento-transito"
	view, err := contract.ReportExpired(
		testContext(stub, drogueriaMSP, RoleOperator), expiredRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateVencido {
		t.Fatalf("estado = %s, se esperaba VENCIDO", view.Estado)
	}
	// La custodia sigue en el emisor: T13 no es una entrega, y ese custodio es
	// el responsable registrado de la disposicion final posterior.
	if view.CustodioActual != "GLN:"+labGLN {
		t.Fatalf("custodio = %s, el transito no se consumo", view.CustodioActual)
	}

	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	collection := pairCollectionName(labMSP, drogueriaMSP)
	_, found, err := readActiveTransferOperation(ctx, collection, validGTIN, validSerial)
	requireNoError(t, err)
	if found {
		t.Fatal("T13 desde EN_TRANSITO debe cerrar el registro de la operacion activa")
	}

	key, err := medicationUnitKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	orgs := endorsingOrganizations(t, stub.validation[key])
	if len(orgs) != 1 || orgs[0] != labMSP {
		t.Fatalf("politica de reposo tras T13 = %v, se esperaba unicamente el emisor %s", orgs, labMSP)
	}
}

// TestReportExpiredFromQuarantineClosesNoTransfer cubre la otra mitad de T13:
// desde EN_CUARENTENA no hay transito que cerrar. El motor comun lo decide
// mirando el estado observado, de modo que no intenta cerrar un registro que no
// existe -- que habria sido un INTERNAL_ERROR.
func TestReportExpiredFromQuarantineClosesNoTransfer(t *testing.T) {
	stub, contract := verifyFixture(t)
	_, err := contract.Quarantine(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "anomalia en control"})
	requireNoError(t, err)

	// EN_CUARENTENA tambien es T13, de modo que tambien exige la fecha.
	stub.timestamp = time.Date(2028, time.January, 2, 0, 0, 0, 0, time.UTC)
	view, err := contract.ReportExpired(
		testContext(stub, drogueriaMSP, RoleOperator), expiredRequest())
	requireNoError(t, err)
	if view.Estado != domain.StateVencido {
		t.Fatalf("estado = %s, se esperaba VENCIDO", view.Estado)
	}
}

// TestReportExpiredByRegulatorWritesMarker: cuando lo inicia ANMAT, el marcador
// de participacion convierte su intervencion en coendoso real de peer.
func TestReportExpiredByRegulatorWritesMarker(t *testing.T) {
	stub, contract := verifyFixture(t)

	stub.txID = "tx-vencimiento-anmat"
	_, err := contract.ReportExpired(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), expiredRequest())
	requireNoError(t, err)
	requireRegulatoryMarker(t, stub, opReportExpired)
}

// TestReportExpiredRejections cubre las condiciones de rechazo.
func TestReportExpiredRejections(t *testing.T) {
	t.Run("invocador ajeno", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.ReportExpired(
			testContext(stub, farmaciaMSP, RoleOperator), expiredRequest())
		requireCode(t, err, cerr.UnauthorizedCustodian)
	})

	t.Run("motivo ausente", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.ReportExpired(
			testContext(stub, drogueriaMSP, RoleOperator),
			UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("regulador sin regulatory-admin", func(t *testing.T) {
		stub, contract := verifyFixture(t)
		_, err := contract.ReportExpired(
			testContext(stub, anmatMSP, RoleAuditor), expiredRequest())
		requireCode(t, err, cerr.UnauthorizedRole)
	})

	// T14-T16 quedan reservadas al custodio o a ANMAT aunque la unidad este en
	// transito: la habilitacion del destinatario declarado es de T09 y T13, no
	// una regla general de los eventos extraordinarios.
	t.Run("el destinatario declarado no alcanza estados terminales", func(t *testing.T) {
		stub, contract := transferFixture(t)
		withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
		_, err := contract.DispatchTransfer(
			testContext(stub, labMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
		stub.transient = map[string][]byte{}

		if eventAllowsDeclaredRecipient(domain.EventInformarRobo) {
			t.Fatal("T14 no habilita al destinatario declarado")
		}
	})
}

// dispensableFixture deja la unidad EN_CUSTODIA de la farmacia, que es el unico
// escenario en el que Dispense llega a evaluar la transicion: agentType
// habilitado y custodia propia.
func dispensableFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub, contract := verifyFixture(t)
	stub.txID = "tx-despacho-farmacia"
	withTransient(stub, dispatchTransient("GLN:"+farmaciaGLN))
	_, err := contract.DispatchTransfer(
		testContext(stub, drogueriaMSP, RoleOperator),
		DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	stub.transient = map[string][]byte{}

	stub.txID = "tx-recepcion-farmacia"
	_, err = contract.ReceiveTransfer(
		testContext(stub, farmaciaMSP, RoleOperator),
		UnitRefRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireNoError(t, err)
	return stub, contract
}

// TestReportExpiredInTransitRequiresReachedDate cubre la precondicion propia de
// T13, que ADR-001 enuncia SIN la disyuncion de T11/T12: "la fecha de
// vencimiento fue alcanzada durante traslado o inmovilizacion".
//
// Es la que impide que el destinatario declarado -- que durante el transito no
// es el custodio -- declare vencida una unidad con fecha futura y cierre
// unilateralmente la transferencia activa.
func TestReportExpiredInTransitRequiresReachedDate(t *testing.T) {
	dispatch := func(t *testing.T) (*mockStub, *SNTContract) {
		t.Helper()
		stub, contract := transferFixture(t)
		stub.txID = "tx-despacho"
		withTransient(stub, dispatchTransient("GLN:"+drogueriaGLN))
		_, err := contract.DispatchTransfer(
			testContext(stub, labMSP, RoleOperator),
			DispatchTransferRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireNoError(t, err)
		stub.transient = map[string][]byte{}
		return stub, contract
	}

	t.Run("fecha futura: rechazada", func(t *testing.T) {
		stub, contract := dispatch(t)
		_, err := contract.ReportExpired(
			testContext(stub, drogueriaMSP, RoleOperator), expiredRequest())
		requireCode(t, err, cerr.InvalidStateTransition)

		// Y el transito NO se cerro: el rechazo ocurre antes de toda escritura.
		ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
		_, found, err := readActiveTransferOperation(
			ctx, pairCollectionName(labMSP, drogueriaMSP), validGTIN, validSerial)
		requireNoError(t, err)
		if !found {
			t.Fatal("un T13 rechazado no debe cerrar la transferencia activa")
		}
	})

	t.Run("fecha alcanzada: aceptada", func(t *testing.T) {
		stub, contract := dispatch(t)
		// La unidad del fixture vence el 2027-12-31; se adelanta el reloj de la
		// transaccion mas alla de esa fecha.
		stub.timestamp = time.Date(2028, time.January, 2, 0, 0, 0, 0, time.UTC)

		view, err := contract.ReportExpired(
			testContext(stub, drogueriaMSP, RoleOperator), expiredRequest())
		requireNoError(t, err)
		if view.Estado != domain.StateVencido {
			t.Fatalf("estado = %s, se esperaba VENCIDO", view.Estado)
		}
	})
}
