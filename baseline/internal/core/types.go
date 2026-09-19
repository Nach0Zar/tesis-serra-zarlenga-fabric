package core

import "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"

const (
	IDTypeGLN  = "GLN"
	IDTypeCUFE = "CUFE"
	IDTypeREG  = "REG"

	RoleOperator         = "operator"
	RoleAuditor          = "auditor"
	RoleRegulatoryAdmin  = "regulatory-admin"
	RoleFinancierAuditor = "financier-auditor"
)

type Organization struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

func (o Organization) CanonicalID() string { return o.IDType + ":" + o.ID }

type MedicationUnit struct {
	GTIN                string       `json:"gtin"`
	NumeroSerie         string       `json:"numeroSerie"`
	Lote                string       `json:"lote"`
	FechaVencimiento    string       `json:"fechaVencimiento"`
	CustodioActual      string       `json:"custodioActual"`
	Estado              domain.State `json:"estado"`
	UltimaActualizacion string       `json:"ultimaActualizacion"`
}

type HistoryEntry struct {
	TxID      string          `json:"txId"`
	Timestamp string          `json:"timestamp"`
	IsDelete  bool            `json:"isDelete"`
	Value     *MedicationUnit `json:"value"`
}

type RegisterUnitRequest struct {
	GTIN             string `json:"gtin"`
	NumeroSerie      string `json:"numeroSerie"`
	Lote             string `json:"lote"`
	FechaVencimiento string `json:"fechaVencimiento"`
}

type DispatchRequest struct {
	Destino       string `json:"destino"`
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

type CommercialData struct {
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

type RejectRequest struct {
	Motivo string `json:"motivo"`
}

type RegisterOrganizationRequest struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

type SetOrganizationActiveRequest struct {
	Active bool `json:"active"`
}

type Invoker struct {
	MSPID string
	Org   Organization
	Role  string
}

func (i Invoker) CanonicalID() string { return i.Org.CanonicalID() }
