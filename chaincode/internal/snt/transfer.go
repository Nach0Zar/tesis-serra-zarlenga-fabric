package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// La transferencia son DOS transacciones separadas mas el rechazo (ADR-004),
// no una operacion atomica. Duena de la implementacion: CC-3 (#16).

// DispatchTransfer implementa T02/T03. Estado resultante: EN_TRANSITO;
// custodioActual NO cambia durante el transito. El destinatario declarado entra
// por el transient `destinatario` y se persiste en la coleccion privada del par.
func (c *SNTContract) DispatchTransfer(_ contractapi.TransactionContextInterface, _ DispatchTransferRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("DispatchTransfer", "CC-3 (#16)")
}

// ReceiveTransfer implementa T04. Estado resultante: EN_CUSTODIA, con
// custodioActual actualizado al receptor. Invocable solo por el destinatario
// declarado de la operacion ACTIVA, validado contra la coleccion privada.
func (c *SNTContract) ReceiveTransfer(_ contractapi.TransactionContextInterface, _ UnitRefRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReceiveTransfer", "CC-3 (#16)")
}

// RejectTransfer implementa T05. Estado resultante: DEVUELTO, con
// custodioActual sin cambios en el emisor (ADR-004, ADR-009).
func (c *SNTContract) RejectTransfer(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("RejectTransfer", "CC-3 (#16)")
}
