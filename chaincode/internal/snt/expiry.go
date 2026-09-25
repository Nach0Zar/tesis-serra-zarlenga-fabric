package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	opReportExpired = "ReportExpired"

	// transitionReportExpiredFromTransitOrQuarantine es el ID de T13 en ADR-001.
	transitionReportExpiredFromTransitOrQuarantine = "T13_MARK_EXPIRED_FROM_TRANSIT_OR_QUARANTINE"
)

// ReportExpired implementa T11, T12 y T13 de ADR-001: el asiento de la
// caducidad de una unidad. Estado resultante: VENCIDO.
//
// La precondicion temporal NO es la misma para las tres transiciones, y ADR-001
// las distingue con cuidado:
//
//   - T11 (EN_LABORATORIO) y T12 (EN_CUSTODIA): "la fecha de vencimiento fue
//     alcanzada O SE DOCUMENTA LA CADUCIDAD". Es una DISYUNCION, y por eso estas
//     dos no exigen que la fecha haya pasado: dejarian sin camino a la segunda
//     rama -- un lote caducado antes de termino por corte de cadena de frio, una
//     caducidad declarada por el titular o por la autoridad --, que es
//     exactamente el caso que el SNT necesita registrar y que ninguna otra
//     operacion del contrato cubre.
//   - T13 (EN_TRANSITO o EN_CUARENTENA): "la fecha de vencimiento FUE ALCANZADA
//     durante traslado o inmovilizacion". Sin disyuncion. Aca la fecha SI se
//     exige, y requireExpiredByDateInTransit la comprueba.
//
// La diferencia no es un descuido de redaccion del ADR. Durante el transito, T13
// habilita al DESTINATARIO DECLARADO -- que no es el custodio -- y ademas cierra
// la transferencia activa. Sin la precondicion temporal, ese invocador podria
// declarar vencida cualquier unidad con fecha futura y cerrar unilateralmente el
// transito: exactamente el poder que la ventana de EN_TRANSITO no debe dar a una
// sola parte.
//
// En T11/T12 lo que el chaincode si puede exigir es que la causa quede
// documentada, y por eso `motivo` es obligatorio: es la mitad verificable de esa
// disyuncion. La notificacion de PROXIMIDAD del vencimiento, que la issue
// menciona, se satisface por esa via en los estados donde el custodio es quien
// informa; no equivale a declarar vencida una unidad con fecha futura durante el
// transito. El
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
	return applyExtraordinaryEvent(
		ctx, req, domain.EventInformarVencimiento, opReportExpired, requireExpiredByDateInTransit)
}

// requireExpiredByDateInTransit materializa la precondicion propia de T13: la
// fecha de vencimiento debe haber sido alcanzada. T11 y T12 no la tienen,
// porque ADR-001 les admite la caducidad documentada como alternativa.
//
// La comparacion sale de GetTxTimestamp() y nunca del reloj local: time.Now()
// rompe el determinismo del endoso, porque cada peer endosante la evaluaria en
// un instante distinto y sus read-write sets podrian no coincidir.
func requireExpiredByDateInTransit(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	transition domain.Transition,
) error {
	if transition.ID != transitionReportExpiredFromTransitOrQuarantine {
		return nil
	}
	expired, err := unitExpiredByDate(ctx, unit)
	if err != nil {
		return err
	}
	if expired {
		return nil
	}
	return cerr.New(cerr.InvalidStateTransition,
		"T13 exige que la fecha de vencimiento %s ya haya sido alcanzada; ADR-001 no admite "+
			"la caducidad documentada como alternativa durante el traslado o la inmovilizacion",
		unit.FechaVencimiento).
		WithDetails(map[string]any{
			"estado":           string(unit.Estado),
			"fechaVencimiento": unit.FechaVencimiento,
			"transicion":       transition.ID,
		})
}
