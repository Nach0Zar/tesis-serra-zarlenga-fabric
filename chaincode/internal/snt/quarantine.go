package snt

import (
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	opQuarantine        = "Quarantine"
	opReleaseQuarantine = "ReleaseQuarantine"
)

// Quarantine implementa T07, T08 y T09 de ADR-001: la suspension cautelar de
// una unidad ante una anomalia. Estado resultante: EN_CUARENTENA, que ADR-001
// declara bloqueante y no terminal -- conserva siete transiciones de salida, y
// por eso la unidad no queda inmovilizada para siempre.
//
// Suspende circulacion y dispensacion sin necesidad de una regla propia: el
// bloqueo lo produce ADR-001, porque ninguna transicion de despacho ni T06
// declara EN_CUARENTENA como estado de origen, y requireTransition las rechaza
// con INVALID_STATE_TRANSITION.
//
// Autorizacion (DES-6, "Evento extraordinario informado por custodio"): el
// custodio actual o la organizacion regulatoria. Durante EN_TRANSITO se suma el
// DESTINATARIO DECLARADO, que es la precision que ADR-001 enuncia para T09
// -- "se detecta anomalia durante el traslado o recepcion" -- y que el contrato
// incorporo en la version 2.6.0. Sin ella, un receptor que recibe mercaderia
// anomala solo podria aceptar la custodia y recien despues ponerla en
// cuarentena, o rechazar la transferencia entera: dos caminos que registran en
// el ledger algo distinto de lo que ocurrio.
//
// El destinatario declarado NO viaja en el request: se resuelve leyendo el
// registro de la operacion ACTIVA en la PDC del par, con el mismo criterio que
// ReceiveTransfer. Aceptarlo como argumento publico revelaria una relacion
// comercial no consumada (ADR-004) y dejaria la autorizacion a criterio de quien
// envia la propuesta.
func (c *SNTContract) Quarantine(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventPonerEnCuarentena, opQuarantine)
}

// ReleaseQuarantine implementa T10: el retorno al flujo normal cuando la
// anomalia se descarta. Estado resultante: EN_CUSTODIA.
//
// El destinatario declarado NO aparece entre los actores habilitados, y no es
// una omision: si la unidad entro en cuarentena desde EN_TRANSITO por T09, esa
// misma operacion cerro el registro de la transferencia (ADR-006, punto 4), de
// modo que al momento de liberar ya no existe destinatario declarado alguno. La
// unidad sale hacia EN_CUSTODIA del emisor, que es el custodio registrado.
func (c *SNTContract) ReleaseQuarantine(
	ctx contractapi.TransactionContextInterface,
	req UnitEventRequest,
) (*MedicationUnitView, error) {
	return applyExtraordinaryEvent(ctx, req, domain.EventLiberarCuarentena, opReleaseQuarantine)
}
