package snt

import (
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// Tests de EXT-5 (#31): Restock (T25, T26 y T27).

func restockRequest() UnitEventRequest {
	return UnitEventRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "aptitud verificada, la unidad reingresa al stock",
	}
}

// authorizeRestock emite la autorizacion de intervencion con operacion RESTOCK,
// que es la que el contrato exige a un laboratorio no custodio para T27.
func authorizeRestock(t *testing.T, stub *mockStub, laboratorio string) {
	t.Helper()
	contract := new(SNTContract)
	_, err := contract.AuthorizeLabIntervention(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin),
		AuthorizeLabInterventionRequest{
			GTIN: validGTIN, NumeroSerie: validSerial,
			Laboratorio: laboratorio,
			Operacion:   LabOpRestock,
			Motivo:      "cierre de retiro con correccion autorizada",
			ExpiraEn:    "2027-01-01T00:00:00Z",
		})
	requireNoError(t, err)
}

// TestRestockFromEveryOrigin recorre las TRES filas de ADR-001 con el actor que
// cada una habilita, y comprueba en las tres lo que ADR-009 punto 5 decide: el
// estado resultante es EN_CUSTODIA y CustodioActual NO cambia.
//
// La cobertura por origen es deliberada y no redundante: la operacion tiene un
// actor distinto por fila, de modo que un solo caso feliz dejaria dos ramas de
// resolucion del actor sin ejercitar -- que es exactamente la clase de ausencia
// que dejo pasar el bug de coleccion de EXT-4.
func TestRestockFromEveryOrigin(t *testing.T) {
	custodio := "GLN:" + drogueriaGLN

	casos := []struct {
		nombre string
		estado domain.State
		msp    string
		rol    string
		setup  func(t *testing.T, stub *mockStub)
	}{
		{
			// T25: RECOVERY_OR_DISPOSAL_AGENT, que ADR-009 punto 3 resuelve como
			// el custodio actual registrado. El exito prueba esa resolucion: la
			// fila NO lista CURRENT_CUSTODIAN, de modo que sin ella el invocador
			// no tendria caracter habilitado.
			nombre: "T25 desde DEVUELTO, agente de recupero",
			estado: domain.StateDevuelto, msp: drogueriaMSP, rol: RoleOperator,
		},
		{
			nombre: "T26 desde EN_CUARENTENA, custodio actual",
			estado: domain.StateEnCuarentena, msp: drogueriaMSP, rol: RoleOperator,
		},
		{
			nombre: "T26 desde EN_CUARENTENA, ANMAT",
			estado: domain.StateEnCuarentena, msp: anmatMSP, rol: RoleRegulatoryAdmin,
		},
		{
			nombre: "T27 desde RETIRADO_MERCADO, ANMAT",
			estado: domain.StateRetiradoMercado, msp: anmatMSP, rol: RoleRegulatoryAdmin,
		},
		{
			nombre: "T27 desde RETIRADO_MERCADO, laboratorio titular no custodio",
			estado: domain.StateRetiradoMercado, msp: labMSP, rol: RoleOperator,
			setup: func(t *testing.T, stub *mockStub) {
				authorizeRestock(t, stub, "GLN:"+labGLN)
			},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			seedUnit(t, stub, caso.estado, custodio)
			if caso.setup != nil {
				caso.setup(t, stub)
			}

			stub.txID = "tx-reingreso"
			view, err := contract.Restock(
				testContext(stub, caso.msp, caso.rol), restockRequest())
			requireNoError(t, err)
			if view.Estado != domain.StateEnCustodia {
				t.Fatalf("estado = %s, se esperaba EN_CUSTODIA", view.Estado)
			}
			// ADR-009 punto 5: la unidad reingresa al stock de quien la tiene
			// registrada. El reingreso no es una entrega y no mueve la custodia,
			// ni cuando lo ejecuta ANMAT ni cuando lo ejecuta el laboratorio.
			if view.CustodioActual != custodio {
				t.Fatalf("custodio = %s, el reingreso no mueve la custodia", view.CustodioActual)
			}
		})
	}
}

// TestRestockFromReturnedIsNotOpenToRegulator fija el detalle mas filoso de
// ADR-001 en esta operacion: T25 habilita UNICAMENTE a
// RECOVERY_OR_DISPOSAL_AGENT. ANMAT no es actor de esa fila, y no es una
// omision: la aptitud de una unidad devuelta la evalua quien la tiene
// fisicamente, no la autoridad.
//
// El rechazo es de TRANSICION y no de autorizacion: el regulador tiene caracter
// -- ANMAT --, lo que no tiene es habilitacion en este estado de origen.
func TestRestockFromReturnedIsNotOpenToRegulator(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateDevuelto, "GLN:"+drogueriaGLN)

	_, err := contract.Restock(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), restockRequest())
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestRestockFromWithdrawnIsNotOpenToCustodian es la contracara de T17-T19: si
// el retiro del mercado es una decision del titular o de la autoridad, deshacerlo
// tambien. El custodio no puede reincorporar por su cuenta una unidad retirada.
func TestRestockFromWithdrawnIsNotOpenToCustodian(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)

	_, err := contract.Restock(
		testContext(stub, drogueriaMSP, RoleOperator), restockRequest())
	requireCode(t, err, cerr.InvalidStateTransition)
}

// TestRestockRejectsUnitExpiredByDate cubre el unico termino de la aptitud de
// ADR-009 punto 5 que la tabla de ADR-001 no puede expresar, en los tres
// origenes: la unidad cuya fechaVencimiento ya paso sin que el evento
// INFORMAR_VENCIMIENTO se haya registrado.
//
// El caso es real: una unidad puede quedar inmovilizada o devuelta meses y
// alcanzar la fecha mientras espera. Sin esta comprobacion el reingreso la
// devolveria al circuito de dispensacion ya vencida.
func TestRestockRejectsUnitExpiredByDate(t *testing.T) {
	for _, estado := range []domain.State{
		domain.StateDevuelto, domain.StateEnCuarentena, domain.StateRetiradoMercado,
	} {
		t.Run(string(estado), func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			seedUnit(t, stub, estado, "GLN:"+drogueriaGLN)
			// El reloj de la transaccion avanza mas alla de fechaVencimiento
			// (2027-12-31, que siembra seedUnit).
			stub.timestamp = time.Date(2028, time.January, 2, 0, 0, 0, 0, time.UTC)

			msp, rol := drogueriaMSP, RoleOperator
			if estado == domain.StateRetiradoMercado {
				msp, rol = anmatMSP, RoleRegulatoryAdmin
			}
			_, err := contract.Restock(testContext(stub, msp, rol), restockRequest())
			requireCode(t, err, cerr.InvalidStateTransition)
			parsed, ok := cerr.Parse(err)
			if !ok || parsed.Details["causa"] != "VENCIDO_POR_FECHA" {
				t.Fatalf("el rechazo por fecha debe declarar causa VENCIDO_POR_FECHA: %v", err)
			}

			// El rechazo no deja rastro: la unidad sigue en su estado de origen.
			view, err := contract.ReadUnit(
				testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
			requireNoError(t, err)
			if view.Estado != estado {
				t.Fatalf("estado = %s, se esperaba %s intacto", view.Estado, estado)
			}
		})
	}
}

// TestRestockReportsUnfitnessAheadOfAuthorization fija el orden de las dos
// precondiciones por lo que el orden SI decide: cuando fallan las dos, el
// invocador recibe el rechazo de aptitud y no el de autorizacion.
//
// La diferencia es practica. Ninguna autorizacion de la autoridad vuelve
// reingresable una unidad cuya fecha de vencimiento ya paso, de modo que
// LAB_INTERVENTION_REQUIRED mandaria al laboratorio a pedir una autorizacion
// inutil. El orden NO protege el estado: si la segunda precondicion falla, la
// transaccion devuelve error y Fabric descarta el write-set completo, asi que la
// autorizacion no queda consumida en ningun orden -- lo que este test observa es
// el error, y la autorizacion ACTIVA como consecuencia del camino recorrido.
func TestRestockReportsUnfitnessAheadOfAuthorization(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)
	authorizeRestock(t, stub, "GLN:"+labGLN)
	stub.timestamp = time.Date(2028, time.January, 2, 0, 0, 0, 0, time.UTC)

	_, err := contract.Restock(
		testContext(stub, labMSP, RoleOperator), restockRequest())
	requireCode(t, err, cerr.InvalidStateTransition)

	authorization, found, err := readLabIntervention(
		testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
	requireNoError(t, err)
	if !found || authorization.Estado != LabInterventionActiva {
		t.Fatalf("el rechazo de aptitud no debe recorrer el consumo: %+v", authorization)
	}
}

// TestRestockRequiresRestockAuthorization comprueba que la autorizacion de
// intervencion es POR OPERACION. Una emitida para retirar del mercado no
// habilita el reingreso: si lo hiciera, una sola intervencion de la autoridad
// alcanzaria para retirar la unidad y despues reincorporarla.
func TestRestockRequiresRestockAuthorization(t *testing.T) {
	t.Run("sin autorizacion", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)

		_, err := contract.Restock(
			testContext(stub, labMSP, RoleOperator), restockRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion de otra operacion", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)
		authorizeWithdrawal(t, stub, "GLN:"+labGLN)

		_, err := contract.Restock(
			testContext(stub, labMSP, RoleOperator), restockRequest())
		requireCode(t, err, cerr.LabInterventionRequired)
	})

	t.Run("autorizacion consumida por el reingreso", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		seedUnit(t, stub, domain.StateRetiradoMercado, "GLN:"+drogueriaGLN)
		authorizeRestock(t, stub, "GLN:"+labGLN)

		stub.txID = "tx-reingreso-laboratorio"
		_, err := contract.Restock(
			testContext(stub, labMSP, RoleOperator), restockRequest())
		requireNoError(t, err)

		authorization, found, err := readLabIntervention(
			testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
		requireNoError(t, err)
		if !found || authorization.Estado != LabInterventionConsumed {
			t.Fatalf("la autorizacion deberia quedar CONSUMIDA: %+v", authorization)
		}
		history, err := contract.GetLabInterventionHistory(
			testContext(stub, anmatMSP, RoleAuditor), validGTIN, validSerial)
		requireNoError(t, err)
		if len(history) != 2 || history[0].Value == nil ||
			history[0].Value.Estado != LabInterventionActiva ||
			history[1].Value == nil || history[1].Value.Estado != LabInterventionConsumed ||
			history[1].TxID != "tx-reingreso-laboratorio" {
			t.Fatalf("historial de consumo inesperado: %+v", history)
		}

		// Los dos marcadores que convierten la intervencion en coendoso real,
		// esta vez con la operacion Restock y no WithdrawFromMarket.
		markerKey, err := unitParticipationKey(stub, validGTIN, validSerial, stub.GetTxID())
		requireNoError(t, err)
		if stub.privateData[implicitCollection(labMSP)][markerKey] == nil {
			t.Fatal("falta el marcador del laboratorio designado")
		}
		if stub.privateData[implicitCollection(anmatMSP)][markerKey] == nil {
			t.Fatal("falta el marcador de la organizacion regulatoria")
		}
	})
}

// TestRestockBlockedFromNonRecoverableStates deja constancia de que la aptitud
// de ADR-009 punto 5 -- no destruida, no robada, no extraviada, sin prohibicion
// vigente -- la resuelve la TABLA y no una regla propia: ninguna fila declara
// REINGRESAR_STOCK desde estos estados.
//
// Es el test que justifica no haber escrito esas cuatro comprobaciones: si
// alguna fila futura las habilitara, este test falla y obliga a revisarlas.
func TestRestockBlockedFromNonRecoverableStates(t *testing.T) {
	for _, estado := range []domain.State{
		domain.StateVencido, domain.StateRobado, domain.StateExtraviado,
		domain.StateDeteriorado, domain.StateProhibido, domain.StateDispuestoFinal,
		domain.StateEnCustodia, domain.StateEnTransito, domain.StateDispensado,
	} {
		t.Run(string(estado), func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			seedUnit(t, stub, estado, "GLN:"+drogueriaGLN)

			_, err := contract.Restock(
				testContext(stub, drogueriaMSP, RoleOperator), restockRequest())
			requireCode(t, err, cerr.InvalidStateTransition)
		})
	}
}

// TestRestockRequiresMotivo: el reingreso deja un asiento permanente en la traza
// y la causa regulatoria es parte del asiento, con el mismo criterio que el
// resto de los eventos extraordinarios.
func TestRestockRequiresMotivo(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateEnCuarentena, "GLN:"+drogueriaGLN)

	_, err := contract.Restock(
		testContext(stub, drogueriaMSP, RoleOperator),
		UnitEventRequest{GTIN: validGTIN, NumeroSerie: validSerial})
	requireCode(t, err, cerr.InvalidRequest)
}

// TestRestockRequiresOperatorRole aplica la tabla de roles de DES-6: `auditor`
// no habilita escrituras, tampoco las del regulador.
func TestRestockRequiresOperatorRole(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	seedUnit(t, stub, domain.StateEnCuarentena, "GLN:"+drogueriaGLN)

	_, err := contract.Restock(
		testContext(stub, drogueriaMSP, RoleAuditor), restockRequest())
	requireCode(t, err, cerr.UnauthorizedRole)

	_, err = contract.Restock(
		testContext(stub, anmatMSP, RoleAuditor), restockRequest())
	requireCode(t, err, cerr.UnauthorizedRole)
}
