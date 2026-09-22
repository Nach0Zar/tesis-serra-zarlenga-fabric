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

// UnitEventRequest documenta la causa de un evento extraordinario.
type UnitEventRequest struct {
	Motivo string `json:"motivo"`
}

// ReturnProductRequest agrega el receptor opcional de una devolucion.
type ReturnProductRequest struct {
	Motivo   string `json:"motivo"`
	Receptor string `json:"receptor,omitempty"`
}

// LabInterventionOperation identifica la operacion autorizada al laboratorio.
type LabInterventionOperation string

// Operaciones admitidas para la intervencion de laboratorio.
const (
	LabOpWithdrawFromMarket LabInterventionOperation = "WITHDRAW_FROM_MARKET"
	LabOpRestock            LabInterventionOperation = "RESTOCK"
	LabOpFinalDisposition   LabInterventionOperation = "FINAL_DISPOSITION"
)

// LabInterventionState es el estado persistido de una autorizacion.
type LabInterventionState string

// Estados persistidos de la intervencion de laboratorio.
const (
	LabInterventionActive   LabInterventionState = "ACTIVA"
	LabInterventionConsumed LabInterventionState = "CONSUMIDA"
	LabInterventionRevoked  LabInterventionState = "REVOCADA"
)

// AuthorizeLabInterventionRequest solicita una autorizacion regulatoria.
type AuthorizeLabInterventionRequest struct {
	Laboratorio string                   `json:"laboratorio"`
	Operacion   LabInterventionOperation `json:"operacion"`
	Motivo      string                   `json:"motivo"`
	ExpiraEn    string                   `json:"expiraEn"`
}

// RevokeLabInterventionRequest documenta la causa de una revocacion.
type RevokeLabInterventionRequest struct {
	Motivo string `json:"motivo"`
}

// LabInterventionView refleja la autorizacion vigente de una unidad.
type LabInterventionView struct {
	GTIN             string                   `json:"gtin"`
	NumeroSerie      string                   `json:"numeroSerie"`
	Laboratorio      string                   `json:"laboratorio"`
	Operacion        LabInterventionOperation `json:"operacion"`
	Motivo           string                   `json:"motivo"`
	ExpiraEn         string                   `json:"expiraEn"`
	Estado           LabInterventionState     `json:"estado"`
	EmitidaPor       string                   `json:"emitidaPor"`
	EmitidaEn        string                   `json:"emitidaEn"`
	ConsumidaEn      string                   `json:"consumidaEn,omitempty"`
	RevocadaEn       string                   `json:"revocadaEn,omitempty"`
	MotivoRevocacion string                   `json:"motivoRevocacion,omitempty"`
}

// TraceCheck es una comprobacion ordenada de un veredicto.
type TraceCheck struct {
	Check     string `json:"check"`
	Resultado string `json:"resultado"`
	Detalle   string `json:"detalle"`
}

// UnitVerdict es el veredicto de autenticidad de una unidad.
type UnitVerdict struct {
	Autentica      bool         `json:"autentica"`
	Motivo         string       `json:"motivo"`
	Estado         domain.State `json:"estado"`
	Verificaciones []TraceCheck `json:"verificaciones"`
}

// TraceVerdict es el veredicto de legitimidad de una traza.
type TraceVerdict struct {
	Legitima       bool         `json:"legitima"`
	Motivo         string       `json:"motivo"`
	Verificaciones []TraceCheck `json:"verificaciones"`
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
