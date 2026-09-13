package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const opReportExpired = "ReportExpired"

// ReportExpired implementa T11, T12 y T13 de ADR-001: el asiento de la
// caducidad de una unidad. Estado resultante: VENCIDO.
//
// NO exige que la fecha de vencimiento ya haya pasado, y es deliberado. La
// precondicion de T11/T12 en ADR-001 es una DISYUNCION: "la fecha de
// vencimiento fue alcanzada O SE DOCUMENTA LA CADUCIDAD". Exigir la fecha
// dejaria sin camino a la segunda rama -- un lote caducado antes de termino por
// corte de cadena de frio, una caducidad declarada por el titular o por la
// autoridad -- que es exactamente el caso que el SNT necesita registrar y que
// ninguna otra operacion del contrato cubre.
//
// Lo que el chaincode SI puede exigir es que la causa quede documentada, y por
// eso `motivo` es obligatorio: es la mitad verificable de esa disyuncion. El
// contrato lo acota a texto breve y neutro, sin datos personales, clinicos ni
// comerciales, porque viaja como argumento publico del canal.
//
// La asimetria con Dispense y VerifyUnit no es una contradiccion. Esas dos
// LEEN la fecha para no entregar ni declarar apta una unidad cuya caducidad el
// propio ledger registra; esta ESCRIBE el hecho de la caducidad, que puede
// anticiparse a la fecha impresa. Una rechaza por la fecha, la otra no depende
// de ella.
//
// La comparacion de fechas del resto del contrato sale siempre de
// GetTxTimestamp() y nunca del reloj local: `time.Now()` rompe el determinismo
// del endoso, porque cada peer endosante la evaluaria en un instante distinto y
// sus read-write sets podrian no coincidir.
//
// T13 abarca dos estados de origen -- EN_TRANSITO y EN_CUARENTENA -- y solo el
// primero tiene un transito que cerrar; el motor comun lo resuelve mirando el
// estado observado, de modo que la caducidad informada sobre una unidad
// inmovilizada no intenta cerrar un registro que no existe.
//
// Responsabilidad de la disposicion final (residuos peligrosos): la unidad
// vencida conserva su CustodioActual, porque ADR-004 acopla estado y custodia y
// solo T04 la mueve. Ese custodio es el responsable registrado de la
// disposicion posterior, que ADR-001 encamina por T28 (EXT-8) hacia
// DISPUESTO_FINAL. El prototipo no persiste un campo aparte de responsabilidad:
// seria una segunda fuente de verdad sobre un dato que el estado publico ya
// contiene.
//
// La unidad vencida queda bloqueada para dispensacion sin necesidad de una
// regla propia: ADR-001 no declara T06 desde VENCIDO, y requireTransition la
// rechaza con INVALID_STATE_TRANSITION.
func (c *SNTContract) ReportExpired(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventInformarVencimiento, opReportExpired)
}
