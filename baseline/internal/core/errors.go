package core

import (
	"errors"
	"fmt"
)

// Code reproduce el catalogo estable del contrato publico. La baseline no
// ramifica sobre mensajes: el cliente usa exclusivamente estos identificadores.
type Code string

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

type ContractError struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	cause   error
}

func (e *ContractError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *ContractError) Unwrap() error { return e.cause }

func NewError(code Code, format string, args ...any) *ContractError {
	return &ContractError{Code: code, Message: fmt.Sprintf(format, args...)}
}

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

func ErrorCode(err error) (Code, bool) {
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return "", false
	}
	return contractErr.Code, true
}
