package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const opRestock = "Restock"

// Restock implementa T25, T26 y T27 de ADR-001: el reingreso de una unidad al
// stock del custodio que la tiene registrada. Estado resultante: EN_CUSTODIA,
// con CustodioActual SIN CAMBIOS (ADR-009, punto 5) -- la unidad vuelve al
// stock de quien ya la tenia, y el reingreso no es una entrega.
//
// Es la operacion inversa de los tres estados bloqueantes recuperables, y
// ADR-001 le da a cada origen un actor distinto:
//
//   - T25 desde DEVUELTO: RECOVERY_OR_DISPOSAL_AGENT, que ADR-009 punto 3
//     resuelve como el custodio actual registrado con rol operator. ANMAT NO es
//     actor de esta fila: la aptitud de una unidad devuelta la evalua quien la
//     tiene fisicamente, no la autoridad.
//   - T26 desde EN_CUARENTENA: el custodio actual o ANMAT. Equivale a liberar
//     la cuarentena con verificacion de aptitud, y ADR-001 la conserva como
//     evento propio porque la Disposicion 3683/2011 enumera el reingreso a
//     stock entre los movimientos a informar. La diferencia observable con
//     ReleaseQuarantine (T10) no esta en el estado resultante -- es el mismo --
//     sino en el asiento que queda en la traza, que es el punto: el SNT
//     distingue "se descarto la anomalia" de "se verifico la aptitud y la
//     unidad vuelve al stock".
//   - T27 desde RETIRADO_MERCADO: ANMAT o el LABORATORIO titular, y solo
//     cuando existe cierre, correccion o recupero autorizado. El custodio NO
//     puede reincorporar por su cuenta una unidad retirada del mercado: el
//     retiro es una decision del titular o de la autoridad (T17-T19), y
//     deshacerlo tambien.
//
// La precondicion de aptitud que ADR-009 punto 5 enumera -- no vencida, no
// destruida ni deteriorada, no robada ni extraviada, sin retiro ni prohibicion
// vigentes -- la cubre la tabla de ADR-001 para todos sus terminos salvo uno:
// ninguna fila declara REINGRESAR_STOCK desde VENCIDO, DETERIORADO, ROBADO,
// EXTRAVIADO, PROHIBIDO ni DISPUESTO_FINAL, de modo que requireTransition los
// rechaza sin regla propia. El termino que la tabla no puede cubrir es la
// unidad cuya fechaVencimiento ya paso sin que el evento INFORMAR_VENCIMIENTO
// se haya registrado, y esa la comprueba requireFitForRestock.
//
// Un laboratorio titular que no es el custodio exige, como en
// WithdrawFromMarket, autorizacion previa de intervencion con operacion
// RESTOCK: una emitida para retirar del mercado no habilita el reingreso.
func (c *SNTContract) Restock(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(
		ctx, req, domain.EventReingresarStock, opRestock, restockPrecondition)
}

// restockPrecondition compone las dos precondiciones de la operacion.
//
// La aptitud se evalua ANTES de la autorizacion de intervencion. El orden no
// protege el estado: si la segunda precondicion falla, la transaccion devuelve
// error y Fabric descarta el write-set COMPLETO, de modo que una autorizacion
// consumida y despues rechazada nunca se confirma. Lo que decide el orden es
// que error ve el invocador cuando fallan las dos, y ahi la aptitud es la
// respuesta util: ninguna autorizacion de la autoridad vuelve reingresable una
// unidad cuya fecha de vencimiento ya paso, mientras que
// LAB_INTERVENTION_REQUIRED lo mandaria a pedir una autorizacion que no le va a
// servir.
func restockPrecondition(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
	_ domain.Transition,
) error {
	if err := requireFitForRestock(ctx, unit); err != nil {
		return err
	}
	return consumeLabIntervention(ctx, unit, opRestock, LabOpRestock)
}

// requireFitForRestock materializa el unico termino de la aptitud de ADR-009
// punto 5 que la tabla de ADR-001 no puede expresar: la unidad cuya
// fechaVencimiento ya paso aunque el evento INFORMAR_VENCIMIENTO no se haya
// registrado.
//
// El caso es real y no teorico: una unidad puede entrar en cuarentena o ser
// devuelta con vencimiento futuro y quedar inmovilizada meses. Sin esta
// comprobacion, el reingreso la devolveria al stock -- y por lo tanto al
// circuito de dispensacion -- con la fecha ya pasada, que es exactamente lo que
// el articulo 9 de la Disposicion 3683/2011 manda impedir.
//
// Aplica a las TRES transiciones, a diferencia de la precondicion temporal de
// T13: aca no hay disyuncion que preservar. Ninguna fila de REINGRESAR_STOCK
// admite una unidad vencida, y el camino correcto para una unidad con la fecha
// alcanzada es ReportExpired y despues la disposicion final (T28).
//
// El instante sale de GetTxTimestamp(), nunca del reloj local, por el
// determinismo del endoso; unitExpiredByDate lo resuelve y es el mismo helper
// que usan Dispense y VerifyUnit, de modo que las tres operaciones no pueden
// discrepar sobre si una unidad esta vencida. El rechazo viaja con el mismo
// codigo y la misma `causa` que el de Dispense: INVALID_STATE_TRANSITION con
// details.causa = VENCIDO_POR_FECHA. No se agrega ningun `code` al catalogo.
func requireFitForRestock(
	ctx contractapi.TransactionContextInterface,
	unit MedicationUnit,
) error {
	expired, err := unitExpiredByDate(ctx, unit)
	if err != nil {
		return err
	}
	if !expired {
		return nil
	}
	return cerr.New(cerr.InvalidStateTransition,
		"el reingreso a stock exige una unidad apta y la fecha de vencimiento %s ya fue alcanzada; "+
			"corresponde informar el vencimiento (T11-T13) y encaminarla a disposicion final",
		unit.FechaVencimiento).
		WithDetails(map[string]any{
			"estado":           string(unit.Estado),
			"fechaVencimiento": unit.FechaVencimiento,
			// Misma `causa` que Dispense y mismo veredicto que VerifyUnit y
			// VerifyTrace: las cuatro operaciones que tienen opinion sobre la
			// fecha la reportan con una sola etiqueta, de modo que un cliente la
			// discrimina igual en todas.
			"causa": "VENCIDO_POR_FECHA",
		})
}
