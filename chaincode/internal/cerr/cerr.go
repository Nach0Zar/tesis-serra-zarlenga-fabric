// Package cerr implementa el formato de errores del contrato publico del
// chaincode `snt` (docs/api-contract.md, seccion "Formato de errores").
//
// Toda operacion que falla devuelve un error cuyo mensaje es un objeto JSON
// con `code`, `message` y `details` opcional. El `code` pertenece al catalogo
// estable del contrato: el cliente y la baseline ramifican sobre `code`, nunca
// sobre `message`, que no es estable entre versiones.
package cerr

import (
	"encoding/json"
	"fmt"
)

// Code es un identificador estable del catalogo de errores del contrato.
type Code string

// Catalogo de codigos de error de docs/api-contract.md (v2.6.1). Esta lista es
// exhaustiva: agregar un codigo es un cambio MINOR del contrato y no puede
// hacerse desde una issue de implementacion.
const (
	InvalidRequest           Code = "INVALID_REQUEST"
	UnitNotFound             Code = "UNIT_NOT_FOUND"
	UnitAlreadyExists        Code = "UNIT_ALREADY_EXISTS"
	InvalidStateTransition   Code = "INVALID_STATE_TRANSITION"
	UnauthorizedCustodian    Code = "UNAUTHORIZED_CUSTODIAN"
	UnauthorizedRole         Code = "UNAUTHORIZED_ROLE"
	UnauthorizedAgentType    Code = "UNAUTHORIZED_AGENT_TYPE"
	OrgNotRegistered         Code = "ORG_NOT_REGISTERED"
	OrgInactive              Code = "ORG_INACTIVE"
	TransferNotAuthorized    Code = "TRANSFER_NOT_AUTHORIZED"
	InvalidDestination       Code = "INVALID_DESTINATION"
	NotInTransit             Code = "NOT_IN_TRANSIT"
	ReceiverMismatch         Code = "RECEIVER_MISMATCH"
	RegulatoryOnly           Code = "REGULATORY_ONLY"
	LastActiveRegulator      Code = "LAST_ACTIVE_REGULATOR"
	AlreadyInitialized       Code = "ALREADY_INITIALIZED"
	InvalidLabIntervention   Code = "INVALID_LAB_INTERVENTION"
	LabInterventionNotFound  Code = "LAB_INTERVENTION_NOT_FOUND"
	LabInterventionNotActive Code = "LAB_INTERVENTION_NOT_ACTIVE"
	LabInterventionRequired  Code = "LAB_INTERVENTION_REQUIRED"
	InternalError            Code = "INTERNAL_ERROR"
)

// ContractError es el error tipificado que el chaincode devuelve al cliente.
// Su Error() serializa el objeto JSON que fija el contrato.
type ContractError struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error devuelve el objeto JSON del contrato como mensaje del error.
func (e *ContractError) Error() string {
	encoded, err := json.Marshal(e)
	if err != nil {
		// No puede ocurrir con los tipos admitidos en Details, pero degradar a
		// un texto plano es preferible a devolver un error vacio.
		return fmt.Sprintf(`{"code":%q,"message":"error no serializable"}`, e.Code)
	}
	return string(encoded)
}

// New construye un error tipificado sin detalles estructurados.
func New(code Code, format string, args ...any) *ContractError {
	return &ContractError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithDetails devuelve una copia del error con el contexto estructurado
// indicado. Details es opcional en el contrato; su ausencia es valida.
func (e *ContractError) WithDetails(details map[string]any) *ContractError {
	clone := *e
	clone.Details = details
	return &clone
}

// Internal envuelve un error no clasificable de la plataforma o del propio
// chaincode. Se usa para fallas de la API del stub, nunca para reglas de
// negocio, que siempre tienen un codigo propio del catalogo.
func Internal(err error, context string) *ContractError {
	return &ContractError{
		Code:    InternalError,
		Message: fmt.Sprintf("%s: %v", context, err),
	}
}

// Parse recupera el error tipificado a partir de su serializacion. Existe para
// los tests y para el cliente; el chaincode no la usa en tiempo de ejecucion.
func Parse(err error) (*ContractError, bool) {
	if err == nil {
		return nil, false
	}
	var parsed ContractError
	if jsonErr := json.Unmarshal([]byte(err.Error()), &parsed); jsonErr != nil {
		return nil, false
	}
	if parsed.Code == "" {
		return nil, false
	}
	return &parsed, true
}
