// Package contracterr normaliza errores de Fabric al contrato público DES-5.
package contracterr

import (
	"encoding/json"
	"errors"
	"strings"

	gatewayclient "github.com/hyperledger/fabric-gateway/pkg/client"
	gatewayproto "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxDiagnosticCauseRunes = 1024

var knownCodes = map[string]struct{}{
	"INVALID_REQUEST":             {},
	"UNIT_NOT_FOUND":              {},
	"UNIT_ALREADY_EXISTS":         {},
	"INVALID_STATE_TRANSITION":    {},
	"UNAUTHORIZED_CUSTODIAN":      {},
	"UNAUTHORIZED_ROLE":           {},
	"UNAUTHORIZED_AGENT_TYPE":     {},
	"ORG_NOT_REGISTERED":          {},
	"ORG_INACTIVE":                {},
	"TRANSFER_NOT_AUTHORIZED":     {},
	"INVALID_DESTINATION":         {},
	"NOT_IN_TRANSIT":              {},
	"RECEIVER_MISMATCH":           {},
	"REGULATORY_ONLY":             {},
	"LAST_ACTIVE_REGULATOR":       {},
	"ALREADY_INITIALIZED":         {},
	"INVALID_LAB_INTERVENTION":    {},
	"LAB_INTERVENTION_NOT_FOUND":  {},
	"LAB_INTERVENTION_NOT_ACTIVE": {},
	"LAB_INTERVENTION_REQUIRED":   {},
	"INTERNAL_ERROR":              {},
}

// Error reproduce el envelope de error público definido por DES-5.
type Error struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// Normalize conserva un error DES-5 emitido por el chaincode. Los fallos de
// transporte o plataforma que no contienen ese envelope se expresan con el
// código contractual INTERNAL_ERROR.
func Normalize(err error, stage string) Error {
	if contractError, ok := Extract(err); ok {
		return contractError
	}

	detailValues := map[string]string{
		"classification": "unexpected",
		"stage":          stage,
	}
	if cause := diagnosticCause(err); cause != "" {
		detailValues["cause"] = cause
	}

	var commitError *gatewayclient.CommitError
	if errors.As(err, &commitError) {
		detailValues["classification"] = "platform_rejection"
		detailValues["validationCode"] = commitError.Code.String()
		if commitError.TransactionID != "" {
			detailValues["transactionId"] = commitError.TransactionID
		}
	}

	if grpcStatus, ok := status.FromError(err); ok {
		detailValues["grpcCode"] = grpcStatus.Code().String()
		if isTransportCode(grpcStatus.Code()) {
			detailValues["classification"] = "transport"
		} else {
			detailValues["classification"] = "gateway"
		}
	}
	details, _ := json.Marshal(detailValues)

	return Error{
		Code:    "INTERNAL_ERROR",
		Message: "Error no clasificable atribuible al chaincode o a la plataforma.",
		Details: details,
	}
}

// Extract busca un envelope DES-5 aun cuando Fabric o gRPC hayan agregado
// prefijos y metadatos alrededor del mensaje original.
func Extract(err error) (Error, bool) {
	if err == nil {
		return Error{}, false
	}

	for current := err; current != nil; current = errors.Unwrap(current) {
		if contractError, ok := extractFromText(current.Error()); ok {
			return contractError, true
		}
	}

	if grpcStatus, ok := status.FromError(err); ok {
		if contractError, found := extractFromText(grpcStatus.Message()); found {
			return contractError, true
		}
		for _, detail := range grpcStatus.Details() {
			gatewayDetail, isGatewayDetail := detail.(*gatewayproto.ErrorDetail)
			if !isGatewayDetail {
				continue
			}
			if contractError, found := extractFromText(gatewayDetail.GetMessage()); found {
				return contractError, true
			}
		}
	}

	return Error{}, false
}

func extractFromText(text string) (Error, bool) {
	for offset := 0; offset < len(text); {
		index := strings.IndexByte(text[offset:], '{')
		if index < 0 {
			return Error{}, false
		}
		offset += index

		decoder := json.NewDecoder(strings.NewReader(text[offset:]))
		var candidate Error
		if err := decoder.Decode(&candidate); err == nil &&
			isKnownCode(candidate.Code) &&
			candidate.Message != "" &&
			(len(candidate.Details) == 0 || json.Valid(candidate.Details)) {
			return candidate, true
		}
		offset++
	}
	return Error{}, false
}

func isKnownCode(code string) bool {
	_, ok := knownCodes[code]
	return ok
}

func diagnosticCause(err error) string {
	if err == nil {
		return ""
	}
	cause := strings.Join(strings.Fields(err.Error()), " ")
	runes := []rune(cause)
	if len(runes) <= maxDiagnosticCauseRunes {
		return cause
	}
	return string(runes[:maxDiagnosticCauseRunes]) + "…"
}

func isTransportCode(code codes.Code) bool {
	switch code {
	case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable:
		return true
	default:
		return false
	}
}
