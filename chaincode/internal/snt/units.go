package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// RegisterUnit implementa T01 de ADR-001. Estado resultante: EN_LABORATORIO.
// Autorizacion: agentType=LABORATORY activo con snt.role=operator, resuelto por
// el registro y nunca contra el literal "LabMSP".
//
// Duena de la implementacion y de sus tests de endoso: CC-2 (#15).
func (c *SNTContract) RegisterUnit(_ contractapi.TransactionContextInterface, _ RegisterUnitRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("RegisterUnit", "CC-2 (#15)")
}
