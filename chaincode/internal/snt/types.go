package snt

import "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"

// Los nombres de campo JSON son espanol lowerCamelCase y los de funcion ingles
// PascalCase, conforme docs/api-contract.md, seccion "Convenciones".

// MedicationUnit es el estado publico del canal de una unidad trazable
// (modelo-datos.md §3). No incluye datos comerciales ni documentales (ADR-002),
// ni el destinatario declarado durante una transferencia en curso, que vive en
// la coleccion privada del par (ADR-004).
type MedicationUnit struct {
	GTIN                string       `json:"gtin"`
	NumeroSerie         string       `json:"numeroSerie"`
	Lote                string       `json:"lote"`
	FechaVencimiento    string       `json:"fechaVencimiento"`
	CustodioActual      string       `json:"custodioActual"`
	Estado              domain.State `json:"estado"`
	UltimaActualizacion string       `json:"ultimaActualizacion"`
}

// MedicationUnitView es el response de las operaciones de escritura sobre una
// unidad y de ReadUnit. Coincide campo a campo con el estado publico
// persistido: el contrato no expone nada que el canal no tenga.
type MedicationUnitView = MedicationUnit

// OrganizationRecord es una entrada del registro organizacion-establecimiento
// (ADR-003, extendido por ADR-010 con los agentType no custodiales). Es la
// unica fuente de verdad que vincula el mspId de una organizacion Fabric con su
// identidad de dominio.
//
// No incluye razon social, domicilio, CUIT ni ningun dato que ADR-003 excluye
// expresamente del registro.
type OrganizationRecord struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

// OrganizationView es el response de las operaciones del registro: los mismos
// campos persistidos.
type OrganizationView = OrganizationRecord

// CanonicalID construye el identificador canonico `<idType>:<id>` que se
// persiste como custodio (ADR-003, punto 4). Nunca se persiste el mspId.
func (o OrganizationRecord) CanonicalID() string {
	return o.IDType + ":" + o.ID
}

// Valores admitidos de idType (ADR-003 para custodiales, ADR-010 para no
// custodiales, donde `id` es un slug estable del organismo).
const (
	IDTypeGLN  = "GLN"
	IDTypeCUFE = "CUFE"
	IDTypeREG  = "REG"
)

// Valores de snt.role definidos por DES-6.
const (
	RoleOperator         = "operator"
	RoleAuditor          = "auditor"
	RoleRegulatoryAdmin  = "regulatory-admin"
	RoleFinancierAuditor = "financier-auditor"
)

// roleAttribute es el unico atributo ABAC definido por DES-6.
const roleAttribute = "snt.role"

// UnitRefRequest referencia una unidad sin datos adicionales.
type UnitRefRequest struct {
	GTIN        string `json:"gtin"`
	NumeroSerie string `json:"numeroSerie"`
}

// UnitEventRequest referencia una unidad y documenta la causa de un evento.
//
// `motivo` es un argumento PUBLICO del canal: no debe incluir datos
// personales, clinicos ni informacion comercial (contrato DES-5, seccion
// "Datos privados"). Para eso existe el transient `commercial`.
type UnitEventRequest struct {
	GTIN        string `json:"gtin"`
	NumeroSerie string `json:"numeroSerie"`
	Motivo      string `json:"motivo"`
}

// RegisterUnitRequest es el alta de una unidad (T01).
type RegisterUnitRequest struct {
	GTIN             string `json:"gtin"`
	NumeroSerie      string `json:"numeroSerie"`
	Lote             string `json:"lote"`
	FechaVencimiento string `json:"fechaVencimiento"`
}

// DispatchTransferRequest es el request PUBLICO del despacho. No incluye el
// destino: el identificador del destinatario declarado revela una relacion
// emisor -> receptor que puede no consumarse y viaja exclusivamente por el
// campo transient (ADR-004).
type DispatchTransferRequest struct {
	GTIN        string `json:"gtin"`
	NumeroSerie string `json:"numeroSerie"`
}

// RegisterOrganizationRequest da de alta una entrada del registro.
type RegisterOrganizationRequest struct {
	MSPID     string           `json:"mspId"`
	ID        string           `json:"id"`
	IDType    string           `json:"idType"`
	AgentType domain.AgentType `json:"agentType"`
	Active    bool             `json:"active"`
}

// SetOrganizationActiveRequest cambia la habilitacion de una organizacion.
type SetOrganizationActiveRequest struct {
	MSPID  string `json:"mspId"`
	Active bool   `json:"active"`
}

// LabInterventionOperation es el catalogo de operaciones que DES-6 habilita a
// un laboratorio no custodio.
type LabInterventionOperation string

// Catalogo de operaciones habilitadas a un laboratorio no custodio: las tres
// que DES-6 enumera y que ADR-007 punto 6.e somete a autorizacion previa.
const (
	LabOpWithdrawFromMarket LabInterventionOperation = "WITHDRAW_FROM_MARKET"
	LabOpRestock            LabInterventionOperation = "RESTOCK"
	LabOpFinalDisposition   LabInterventionOperation = "FINAL_DISPOSITION"
)

// LabInterventionState son los tres estados persistidos de una autorizacion.
// El vencimiento NO es uno de ellos: es una condicion derivada que la logica
// computa contra GetTxTimestamp() y que no borra la clave (ADR-007, punto 6.f).
type LabInterventionState string

// Catalogo de estados persistidos de una autorizacion de intervencion.
const (
	LabInterventionActiva   LabInterventionState = "ACTIVA"
	LabInterventionConsumed LabInterventionState = "CONSUMIDA"
	LabInterventionRevoked  LabInterventionState = "REVOCADA"
)

// AuthorizeLabInterventionRequest emite una autorizacion previa de intervencion.
type AuthorizeLabInterventionRequest struct {
	GTIN        string                   `json:"gtin"`
	NumeroSerie string                   `json:"numeroSerie"`
	Laboratorio string                   `json:"laboratorio"`
	Operacion   LabInterventionOperation `json:"operacion"`
	Motivo      string                   `json:"motivo"`
	ExpiraEn    string                   `json:"expiraEn"`
}

// RevokeLabInterventionRequest deja sin efecto una autorizacion vigente.
type RevokeLabInterventionRequest struct {
	GTIN        string `json:"gtin"`
	NumeroSerie string `json:"numeroSerie"`
	Motivo      string `json:"motivo"`
}

// LabInterventionView es el registro persistido de la autorizacion.
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

// UnitHistoryEntry es una entrada del historial de una unidad
// (GetHistoryForKey). Fabric devuelve el valor entero de la clave en cada
// punto, que es lo que la comprobacion 5 de ADR-011 necesita para recorrer los
// cambios de custodio.
type UnitHistoryEntry struct {
	TxID      string          `json:"txId"`
	Timestamp string          `json:"timestamp"`
	IsDelete  bool            `json:"isDelete"`
	Value     *MedicationUnit `json:"value"`
}

// TraceVerdict es el veredicto estructurado de VerifyTrace (ADR-011).
type TraceVerdict struct {
	Legitima       bool         `json:"legitima"`
	Motivo         string       `json:"motivo"`
	Verificaciones []TraceCheck `json:"verificaciones"`
}

// TraceCheck es una comprobacion individual de la checklist de ADR-011.
type TraceCheck struct {
	Check     string `json:"check"`
	Resultado string `json:"resultado"`
	Detalle   string `json:"detalle"`
}
