package contracterr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	gatewayclient "github.com/hyperledger/fabric-gateway/pkg/client"
	gatewayproto "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExtractWrappedContractError(t *testing.T) {
	source := errors.New("proposal failed: response: status 500, message: " +
		"{\"code\":\"UNIT_NOT_FOUND\",\"message\":\"La unidad no existe.\",\"details\":{\"key\":\"x\"}}")
	err := fmt.Errorf("evaluate transaction: %w", source)

	got, ok := Extract(err)
	if !ok {
		t.Fatal("Extract() did not find the DES-5 error")
	}
	if got.Code != "UNIT_NOT_FOUND" {
		t.Fatalf("Extract().Code = %q, want UNIT_NOT_FOUND", got.Code)
	}
	if string(got.Details) != "{\"key\":\"x\"}" {
		t.Fatalf("Extract().Details = %s", got.Details)
	}
}

func TestNormalizeGatewayError(t *testing.T) {
	got := Normalize(status.Error(codes.Unavailable, "connection refused"), "connect")

	if got.Code != "INTERNAL_ERROR" {
		t.Fatalf("Normalize().Code = %q, want INTERNAL_ERROR", got.Code)
	}
	want := "{\"cause\":\"rpc error: code = Unavailable desc = connection refused\"," +
		"\"classification\":\"transport\",\"grpcCode\":\"Unavailable\",\"stage\":\"connect\"}"
	if string(got.Details) != want {
		t.Fatalf("Normalize().Details = %s", got.Details)
	}
}

func TestNormalizePlatformRejection(t *testing.T) {
	got := Normalize(&gatewayclient.CommitError{
		TransactionID: "tx-123",
		Code:          peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE,
	}, "invoke")

	var details map[string]string
	if err := json.Unmarshal(got.Details, &details); err != nil {
		t.Fatal(err)
	}
	if details["classification"] != "platform_rejection" {
		t.Fatalf("classification = %q", details["classification"])
	}
	if details["validationCode"] != "ENDORSEMENT_POLICY_FAILURE" {
		t.Fatalf("validationCode = %q", details["validationCode"])
	}
	if details["transactionId"] != "tx-123" {
		t.Fatalf("transactionId = %q", details["transactionId"])
	}
}

func TestExtractContractErrorFromGatewayDetail(t *testing.T) {
	grpcStatus, err := status.New(codes.Aborted, "failed to endorse transaction").WithDetails(
		&gatewayproto.ErrorDetail{
			Address: "peer0.lab.snt.local:7051",
			MspId:   "LabMSP",
			Message: "chaincode response 500, " +
				"{\"code\":\"UNIT_ALREADY_EXISTS\",\"message\":\"la unidad ya existe\"}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := Extract(grpcStatus.Err())
	if !ok {
		t.Fatal("Extract() did not find the DES-5 error in gateway details")
	}
	if got.Code != "UNIT_ALREADY_EXISTS" {
		t.Fatalf("Extract().Code = %q, want UNIT_ALREADY_EXISTS", got.Code)
	}
}

func TestExtractRejectsArbitraryJSON(t *testing.T) {
	if _, ok := Extract(errors.New("{\"status\":500}")); ok {
		t.Fatal("Extract() accepted a non DES-5 JSON object")
	}
}

func TestExtractRejectsUnknownErrorCode(t *testing.T) {
	err := errors.New("{\"code\":\"PLATFORM_FAILURE\",\"message\":\"not a DES-5 error\"}")
	if _, ok := Extract(err); ok {
		t.Fatal("Extract() accepted a code outside the DES-5 catalog")
	}
}

func TestNormalizeTruncatesDiagnosticCause(t *testing.T) {
	got := Normalize(errors.New(strings.Repeat("á", maxDiagnosticCauseRunes+10)), "query")

	var details map[string]string
	if err := json.Unmarshal(got.Details, &details); err != nil {
		t.Fatal(err)
	}
	cause := []rune(details["cause"])
	if len(cause) != maxDiagnosticCauseRunes+1 || cause[len(cause)-1] != '…' {
		t.Fatalf("truncated cause has %d runes and suffix %q", len(cause), cause[len(cause)-1])
	}
}
