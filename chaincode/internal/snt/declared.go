package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// Este archivo declara las operaciones que docs/api-contract.md (v2.7.1) congela
// y que CC-1 (#14) exige tener declaradas en el scaffold, pero cuya logica
// pertenece a otra issue.
//
// Cada una devuelve un error tipificado que nombra a su issue duena. Ninguna
// firma cambiara al implementarse: el contrato esta congelado y su modificacion
// exige un PR propio con aprobacion explicita (docs/api-contract.md, "Politica
// de versionado y congelamiento").
//
// Mapa de propiedad:
//
//	RegisterUnit ........................... CC-2 (#15)
//	DispatchTransfer/ReceiveTransfer/
//	  RejectTransfer ....................... CC-3 (#16)
//	Dispense ............................... CC-4 (#17)
//	ReadUnit/GetUnitHistory/
//	  QueryUnitsByGTIN ..................... CC-5 (#18)
//	Restock ................................ EXT-5 (#31)
//	WithdrawFromMarket/ProhibitProduct ..... EXT-6 (#32)
//	FinalDisposition ....................... EXT-8 (#63)

// WithdrawFromMarket cubre T17, T18 y T19. Estado resultante:
// RETIRADO_MERCADO. Un laboratorio no custodio exige una AuthorizeLabIntervention
// ACTIVA y vigente (ADR-007, punto 6.e).
func (c *SNTContract) WithdrawFromMarket(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("WithdrawFromMarket", "EXT-6 (#32)")
}

// ProhibitProduct cubre T20. Estado resultante: PROHIBIDO. Solo ANMAT.
func (c *SNTContract) ProhibitProduct(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ProhibitProduct", "EXT-6 (#32)")
}

// Restock cubre T25, T26 y T27. Estado resultante: EN_CUSTODIA, con
// custodioActual sin cambios (ADR-009, punto 5).
func (c *SNTContract) Restock(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("Restock", "EXT-5 (#31)")
}

// FinalDisposition cubre T28-T33. Estado resultante: DISPUESTO_FINAL (terminal).
func (c *SNTContract) FinalDisposition(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("FinalDisposition", "EXT-8 (#63)")
}
