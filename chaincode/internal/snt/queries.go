package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// Operaciones de lectura del contrato. No mutan estado ni generan endoso de
// escritura. Duena de la implementacion: CC-5 (#18).

// ReadUnit devuelve el estado publico de una unidad.
func (c *SNTContract) ReadUnit(_ contractapi.TransactionContextInterface, _ string, _ string) (*MedicationUnitView, error) {
	return nil, notImplemented("ReadUnit", "CC-5 (#18)")
}

// GetUnitHistory devuelve la traza completa de la unidad con GetHistoryForKey:
// txId, timestamp y el valor entero de la clave en cada punto.
func (c *SNTContract) GetUnitHistory(_ contractapi.TransactionContextInterface, _ string, _ string) ([]UnitHistoryEntry, error) {
	return nil, notImplemented("GetUnitHistory", "CC-5 (#18)")
}

// QueryUnitsByGTIN recupera todas las unidades de un GTIN con
// GetStateByPartialCompositeKey. Sin paginacion (exclusion registrada en
// docs/alcance-prototipo.md).
func (c *SNTContract) QueryUnitsByGTIN(_ contractapi.TransactionContextInterface, _ string) ([]MedicationUnitView, error) {
	return nil, notImplemented("QueryUnitsByGTIN", "CC-5 (#18)")
}
