// Package core implementa la logica de dominio de la linea base centralizada:
// registro de organizaciones y unidades, transferencia de custodia en dos
// pasos, dispensacion, eventos extraordinarios e historial. Reproduce los
// mismos procesos que el chaincode para que la comparacion experimental mida
// lo mismo en ambos SUT, sobre PostgreSQL y sin endoso ni ledger.
package core

import "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"

// Tipos de identificador y roles admitidos, conforme ADR-003 y el manifiesto
// de organizaciones.
const (
	IDTypeGLN  = "GLN"
	IDTypeCUFE = "CUFE"
	IDTypeREG  = "REG"

	RoleOperator         = "operator"
	RoleAuditor          = "auditor"
	RoleRegulatoryAdmin  = "regulatory-admin"
	RoleFinancierAuditor = "financier-auditor"
)

// Organization es una entidad registrada y habilitada para operar.
type Organization struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

// CanonicalID devuelve el identificador en la forma tipo:valor con la que se
// comparan origen y destino de una transferencia.
func (o Organization) CanonicalID() string { return o.IDType + ":" + o.ID }

// MedicationUnit es el estado publico vigente de una unidad trazable.
type MedicationUnit struct {
	GTIN                string       `json:"gtin"`
	NumeroSerie         string       `json:"numeroSerie"`
	Lote                string       `json:"lote"`
	FechaVencimiento    string       `json:"fechaVencimiento"`
	CustodioActual      string       `json:"custodioActual"`
	Estado              domain.State `json:"estado"`
	UltimaActualizacion string       `json:"ultimaActualizacion"`
}

// HistoryEntry es una version confirmada de una unidad. Value queda en nil
// cuando la entrada corresponde a un borrado.
type HistoryEntry struct {
	TxID      string          `json:"txId"`
	Timestamp string          `json:"timestamp"`
	IsDelete  bool            `json:"isDelete"`
	Value     *MedicationUnit `json:"value"`
}

// LabInterventionHistoryEntry conserva un snapshot confirmado de la autorizacion.
type LabInterventionHistoryEntry struct {
	TxID      string               `json:"txId"`
	Timestamp string               `json:"timestamp"`
	IsDelete  bool                 `json:"isDelete"`
	Value     *LabInterventionView `json:"value"`
}

// RegisterUnitRequest es el alta de una unidad por su laboratorio.
type RegisterUnitRequest struct {
	GTIN             string `json:"gtin"`
	NumeroSerie      string `json:"numeroSerie"`
	Lote             string `json:"lote"`
	FechaVencimiento string `json:"fechaVencimiento"`
}

// DispatchRequest es el primer paso de la transferencia de custodia: el
// emisor despacha hacia un destino declarado (ADR-004).
type DispatchRequest struct {
	Destino       string `json:"destino"`
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

// CommercialData son los datos comerciales del despacho, equivalentes a los
// que en Fabric viajan por coleccion privada.
type CommercialData struct {
	NumeroRemito  string `json:"numeroRemito"`
	NumeroFactura string `json:"numeroFactura"`
	Cantidad      int    `json:"cantidad"`
}

// RejectRequest es el rechazo de una transferencia en transito por parte del
// receptor declarado.
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

// RegisterOrganizationRequest es el alta de una organizacion en el registro.
type RegisterOrganizationRequest struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

// SetOrganizationActiveRequest habilita o deshabilita una organizacion sin
// borrarla del registro.
type SetOrganizationActiveRequest struct {
	Active bool `json:"active"`
}

// Invoker es quien ejecuta una operacion: su organizacion ya resuelta contra
// el registro y el rol con el que se presenta.
type Invoker struct {
	MSPID string
	Org   Organization
	Role  string
}

// CanonicalID devuelve el identificador canonico de la organizacion invocante.
func (i Invoker) CanonicalID() string { return i.Org.CanonicalID() }
