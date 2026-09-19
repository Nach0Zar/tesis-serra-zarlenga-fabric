package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// Este archivo declara las operaciones que docs/api-contract.md congela
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
//	FinalDisposition ....................... EXT-8 (#63)

// FinalDisposition cubre T28-T33. Estado resultante: DISPUESTO_FINAL (terminal).
func (c *SNTContract) FinalDisposition(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("FinalDisposition", "EXT-8 (#63)")
}
