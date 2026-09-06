package contracterr

import (
	"errors"
	"fmt"
	"testing"

	gatewayproto "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
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
	if string(got.Details) != "{\"grpcCode\":\"Unavailable\",\"stage\":\"connect\"}" {
		t.Fatalf("Normalize().Details = %s", got.Details)
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
