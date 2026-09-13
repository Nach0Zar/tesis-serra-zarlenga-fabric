package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	opReportStolen  = "ReportStolen"
	opReportLost    = "ReportLost"
	opReportDamaged = "ReportDamaged"
)

// Incidentes que sacan una unidad de circulacion de forma definitiva: robo
// (T14), extravio (T15) y deterioro (T16) de ADR-001.
//
// Las tres comparten estructura y la diferencia es el estado resultante, que es
// tambien el hecho informado. Los tres bloquean la unidad para toda operacion
// ordinaria posterior sin necesidad de una regla propia del chaincode: la
// maquina de estados es la unica fuente de verdad del bloqueo, y duplicarla en
// condicionales seria una segunda verdad que mantener.
//
// Pero NO son los tres iguales, y la distincion importa. ROBADO y EXTRAVIADO
// son TERMINALES: ADR-001 no les declara ninguna transicion de salida. En
// cambio DETERIORADO es BLOQUEANTE y no terminal, porque conserva la salida
// T29 hacia DISPUESTO_FINAL -- un producto deteriorado todavia tiene que ser
// dispuesto como residuo peligroso, y el ledger debe poder registrarlo. Es la
// misma distincion que VerifyUnit reporta como ESTADO_TERMINAL frente a
// ESTADO_BLOQUEANTE (ADR-013): ante lo primero el adquirente rechaza en firme,
// ante lo segundo hay un proceso que todavia puede cerrarse.
//
// A diferencia de T09 y T13, estas tres NO habilitan al destinatario declarado
// aunque la unidad este en transito. ADR-001 las reserva al custodio actual o a
// ANMAT, y el contrato lo explicita: son hechos sobre los que el destinatario
// declarado no tiene conocimiento propio mientras la unidad no este en su
// poder. eventAllowsDeclaredRecipient no los incluye, de modo que el motor
// comun rechaza a ese invocador con UNAUTHORIZED_CUSTODIAN.
//
// Los tres eventos proceden desde cinco estados de origen -- EN_LABORATORIO,
// EN_TRANSITO, EN_CUSTODIA, EN_CUARENTENA y DEVUELTO --, y cuando el origen es
// EN_TRANSITO cierran el transito con las tres piezas de ADR-007, igual que
// T09 y T13.

// ReportStolen implementa T14. Estado resultante: ROBADO, terminal.
func (c *SNTContract) ReportStolen(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventInformarRobo, opReportStolen)
}

// ReportLost implementa T15. Estado resultante: EXTRAVIADO, terminal.
func (c *SNTContract) ReportLost(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventInformarExtravio, opReportLost)
}

// ReportDamaged implementa T16. Estado resultante: DETERIORADO, bloqueante
// pero NO terminal: conserva la salida T29 hacia DISPUESTO_FINAL (EXT-8).
//
// Cubre tambien el criterio "codigo deteriorado: baja de validez del codigo" de
// la issue: el codigo serializado no tiene una vigencia propia que dar de baja
// por separado -- es la clave compuesta GTIN + numero de serie, que identifica a
// la unidad y no se borra nunca (ADR-013 recomputa la unicidad desde el
// historial justamente para detectar una recreacion). Lo que se da de baja es
// la APTITUD de la unidad, y eso es exactamente lo que expresa el estado
// terminal DETERIORADO: toda operacion posterior queda rechazada y VerifyUnit
// devuelve ESTADO_TERMINAL a quien consulte antes de adquirirla.
func (c *SNTContract) ReportDamaged(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventInformarDeterioro, opReportDamaged)
}
