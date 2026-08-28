package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// Dispense implementa T06 de ADR-001. Estado resultante: DISPENSADO (terminal).
// No recibe ni persiste datos personales del paciente (Ley 25.326; ADR-005).
//
// Duena de la implementacion: CC-4 (#17).
func (c *SNTContract) Dispense(_ contractapi.TransactionContextInterface, _ UnitRefRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("Dispense", "CC-4 (#17)")
}
