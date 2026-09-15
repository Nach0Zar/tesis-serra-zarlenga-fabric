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
//	ReportStolen/ReportLost/ReportDamaged .. EXT-3 (#29)
//	Restock ................................ EXT-5 (#31)
//	FinalDisposition ....................... EXT-8 (#63)

// ReportStolen cubre T14. Estado resultante: ROBADO (terminal). ADR-001 reserva
// T14-T16 al custodio actual o a ANMAT aun cuando la unidad este en transito.
func (c *SNTContract) ReportStolen(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReportStolen", "EXT-3 (#29)")
}

// ReportLost cubre T15. Estado resultante: EXTRAVIADO (terminal).
func (c *SNTContract) ReportLost(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReportLost", "EXT-3 (#29)")
}

// ReportDamaged cubre T16. Estado resultante: DETERIORADO.
func (c *SNTContract) ReportDamaged(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReportDamaged", "EXT-3 (#29)")
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
