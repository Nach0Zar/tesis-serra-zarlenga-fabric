package contracterr

import (
	"encoding/json"
	"errors"
	"strings"

	gatewayproto "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/status"
)

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

	details, _ := json.Marshal(map[string]string{"stage": stage})
	if grpcStatus, ok := status.FromError(err); ok {
		details, _ = json.Marshal(map[string]string{
			"stage":    stage,
			"grpcCode": grpcStatus.Code().String(),
		})
	}

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
			candidate.Code != "" &&
			candidate.Message != "" &&
			(len(candidate.Details) == 0 || json.Valid(candidate.Details)) {
			return candidate, true
		}
		offset++
	}
	return Error{}, false
}
