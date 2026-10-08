package core

import (
	"errors"
	"fmt"
)

// Code reproduce el catalogo estable del contrato publico. La baseline no
// ramifica sobre mensajes: el cliente usa exclusivamente estos identificadores.
type Code string

// Codigos del catalogo estable. Cualquier alta aca debe existir tambien en el
// contrato publico, o la comparacion dejaria de medir los mismos rechazos.
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

// ContractError es un rechazo atribuible al catalogo: lleva el codigo estable,
// un mensaje legible y detalles opcionales. La causa interna se conserva para
// los logs pero no se expone al cliente.
type ContractError struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	cause   error
}

func (e *ContractError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *ContractError) Unwrap() error { return e.cause }

// NewError arma un rechazo con su codigo y un mensaje formateado.
func NewError(code Code, format string, args ...any) *ContractError {
	return &ContractError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithDetails devuelve una copia con los detalles adjuntos, dejando intacto el
// error original.
func (e *ContractError) WithDetails(details map[string]any) *ContractError {
	clone := *e
	clone.Details = details
	return &clone
}

func internal(err error, context string) *ContractError {
	return &ContractError{
		Code: InternalError, Message: "error interno de la baseline",
		cause: fmt.Errorf("%s: %w", context, err),
	}
}

// ErrorCode extrae el codigo del catalogo de una cadena de errores. El segundo
// valor es falso cuando el error no proviene del contrato, caso en el que
// corresponde tratarlo como interno.
func ErrorCode(err error) (Code, bool) {
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return "", false
	}
	return contractErr.Code, true
}
