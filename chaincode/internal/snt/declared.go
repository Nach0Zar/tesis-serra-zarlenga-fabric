package snt

import "github.com/hyperledger/fabric-contract-api-go/v2/contractapi"

// Este archivo declara las operaciones que docs/api-contract.md (v2.7.0) congela
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
//	Quarantine/ReleaseQuarantine ........... EXT-1 (#27)
//	ReportExpired .......................... EXT-2 (#28)
//	ReportStolen/ReportLost/ReportDamaged .. EXT-3 (#29)
//	ReturnProduct .......................... EXT-4 (#30)
//	Restock ................................ EXT-5 (#31)
//	WithdrawFromMarket/ProhibitProduct ..... EXT-6 (#32)
//	FinalDisposition ....................... EXT-8 (#63)
//	VerifyTrace ............................ CC-8 (#62)

// Quarantine cubre T07, T08 y T09 de ADR-001. Estado resultante:
// EN_CUARENTENA. Actor habilitado: custodio actual, destinatario declarado
// (solo T09, unidad en EN_TRANSITO) o ANMAT.
func (c *SNTContract) Quarantine(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("Quarantine", "EXT-1 (#27)")
}

// ReleaseQuarantine cubre T10. Estado resultante: EN_CUSTODIA.
func (c *SNTContract) ReleaseQuarantine(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReleaseQuarantine", "EXT-1 (#27)")
}

// ReportExpired cubre T11, T12 y T13. Estado resultante: VENCIDO. Actor
// habilitado: custodio actual, destinatario declarado (solo T13 desde
// EN_TRANSITO) o ANMAT.
func (c *SNTContract) ReportExpired(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReportExpired", "EXT-2 (#28)")
}

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

// ReturnProduct cubre T21-T24. Estado resultante: DEVUELTO. Es un evento unico
// que NO modifica custodioActual (ADR-009, punto 1); admite un transient
// opcional `devolucion` con el receptor declarado.
func (c *SNTContract) ReturnProduct(_ contractapi.TransactionContextInterface, _ UnitEventRequest) (*MedicationUnitView, error) {
	return nil, notImplemented("ReturnProduct", "EXT-4 (#30)")
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

// VerifyTrace es la verificacion de trazabilidad del organismo financiador:
// checklist determinística de cinco comprobaciones y veredicto estructurado
// (ADR-011). Es de solo lectura y no muta estado.
func (c *SNTContract) VerifyTrace(_ contractapi.TransactionContextInterface, _ string, _ string) (*TraceVerdict, error) {
	return nil, notImplemented("VerifyTrace", "CC-8 (#62)")
}
