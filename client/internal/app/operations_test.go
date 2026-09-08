package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseBusinessOptionsMapsPublicContract(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		command   string
		function  string
		wantArgs  []string
	}{
		{
			name: "register unit",
			arguments: []string{
				"register-unit", "--org", "lab", "--gtin", "07791234567898",
				"--serial", "SN-1", "--lot", "L-1", "--expiry", "2028-12-31",
			},
			command:  "invoke",
			function: "RegisterUnit",
			wantArgs: []string{
				`{"gtin":"07791234567898","numeroSerie":"SN-1","lote":"L-1","fechaVencimiento":"2028-12-31"}`,
			},
		},
		{
			name: "dispatch transfer",
			arguments: []string{
				"dispatch-transfer", "--org", "lab", "--gtin", "07791234567898",
				"--serial", "SN-1", "--transient-file", "transfer.json",
			},
			command:  "invoke",
			function: "DispatchTransfer",
			wantArgs: []string{
				`{"gtin":"07791234567898","numeroSerie":"SN-1"}`,
			},
		},
		{
			name: "receive transfer",
			arguments: []string{
				"receive-transfer", "--org", "drogueria", "--gtin", "07791234567898",
				"--serial", "SN-1",
			},
			command:  "invoke",
			function: "ReceiveTransfer",
			wantArgs: []string{
				`{"gtin":"07791234567898","numeroSerie":"SN-1"}`,
			},
		},
		{
			name: "dispense",
			arguments: []string{
				"dispense", "--org", "farmacia", "--gtin", "07791234567898", "--serial", "SN-1",
			},
			command:  "invoke",
			function: "Dispense",
			wantArgs: []string{
				`{"gtin":"07791234567898","numeroSerie":"SN-1"}`,
			},
		},
		{
			name: "read unit",
			arguments: []string{
				"read-unit", "--org", "farmacia", "--gtin", "07791234567898", "--serial", "SN-1",
			},
			command:  "query",
			function: "ReadUnit",
			wantArgs: []string{"07791234567898", "SN-1"},
		},
		{
			name: "unit history",
			arguments: []string{
				"unit-history", "--org", "farmacia", "--gtin", "07791234567898", "--serial", "SN-1",
			},
			command:  "query",
			function: "GetUnitHistory",
			wantArgs: []string{"07791234567898", "SN-1"},
		},
		{
			name: "query units by GTIN",
			arguments: []string{
				"query-units-by-gtin", "--org", "farmacia", "--gtin", "07791234567898",
			},
			command:  "query",
			function: "QueryUnitsByGTIN",
			wantArgs: []string{"07791234567898"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, help, err := parseOptions(test.arguments, io.Discard)
			if err != nil {
				t.Fatalf("parseOptions() error = %v", err)
			}
			if help {
				t.Fatal("parseOptions() help = true")
			}
			if got.command != test.command || got.function != test.function {
				t.Fatalf(
					"parseOptions() transaction = %q %q, want %q %q",
					got.command,
					got.function,
					test.command,
					test.function,
				)
			}
			if strings.Join(got.arguments, "|") != strings.Join(test.wantArgs, "|") {
				t.Fatalf("parseOptions() arguments = %q, want %q", got.arguments, test.wantArgs)
			}
		})
	}
}

func TestRunDispatchTransferUsesOnlyTransientDestination(t *testing.T) {
	gatewayClient := &fakeTransactionClient{invokePayload: []byte(`{"estado":"EN_TRANSITO"}`)}
	deps := stubDependencies(gatewayClient)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{
			"dispatch-transfer",
			"--repo-root", "repository",
			"--org", "lab",
			"--gtin", "07791234567898",
			"--serial", "SN-1",
			"--transient-file", "-",
		},
		&stdout,
		&stderr,
		strings.NewReader(
			`{"destinatario":{"destino":"GLN:7791234500024"},`+
				`"commercial":{"numeroRemito":"R-1","numeroFactura":"F-1","cantidad":1}}`,
		),
		deps,
	)

	if exitCode != exitSuccess {
		t.Fatalf("run() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if gatewayClient.calledFunction != "DispatchTransfer" {
		t.Fatalf("function = %q", gatewayClient.calledFunction)
	}
	if strings.Contains(gatewayClient.calledArguments[0], "destino") {
		t.Fatalf("public argument reveals destination: %s", gatewayClient.calledArguments[0])
	}
	if got := string(gatewayClient.calledTransient["destinatario"]); got != `{"destino":"GLN:7791234500024"}` {
		t.Fatalf("transient destination = %s", got)
	}
}

func TestRunDispatchTransferRejectsInvalidTransientKeys(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := run(
		[]string{
			"dispatch-transfer",
			"--repo-root", "repository",
			"--org", "lab",
			"--gtin", "07791234567898",
			"--serial", "SN-1",
			"--transient-file", "-",
		},
		io.Discard,
		&stderr,
		strings.NewReader(`{"destinatario":{},"unexpected":{}}`),
		stubDependencies(nil),
	)

	if exitCode != exitUsage {
		t.Fatalf("run() exit = %d, want %d", exitCode, exitUsage)
	}
	if !strings.Contains(stderr.String(), `transient key "commercial" is required`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

type scriptedTransactionClient struct {
	errors      []error
	invokeCount int
}

func (client *scriptedTransactionClient) Query(
	context.Context,
	string,
	[]string,
	map[string][]byte,
) ([]byte, error) {
	return nil, errors.New("unexpected query")
}

func (client *scriptedTransactionClient) Invoke(
	context.Context,
	string,
	[]string,
	map[string][]byte,
) ([]byte, error) {
	index := client.invokeCount
	client.invokeCount++
	if index < len(client.errors) {
		return nil, client.errors[index]
	}
	return []byte(`{"estado":"EN_CUSTODIA"}`), nil
}

func (client *scriptedTransactionClient) Close() error {
	return nil
}

func TestInvokeWithRetryRetriesOnlyPrivateDataDissemination(t *testing.T) {
	retryable := errors.New(
		`endorse failed: {"code":"INTERNAL_ERROR","message":"retry",` +
			`"details":{"reintentable":true,"causa":"PRIVATE_DATA_NOT_DISSEMINATED"}}`,
	)
	client := &scriptedTransactionClient{errors: []error{retryable}}
	waitCount := 0
	var stderr bytes.Buffer

	payload, err := invokeWithRetry(
		context.Background(),
		client,
		options{
			operation:        "receive-transfer",
			function:         "ReceiveTransfer",
			retryPrivateData: true,
			retryInterval:    time.Millisecond,
		},
		nil,
		&stderr,
		func(context.Context, time.Duration) error {
			waitCount++
			return nil
		},
	)

	if err != nil {
		t.Fatalf("invokeWithRetry() error = %v", err)
	}
	if string(payload) != `{"estado":"EN_CUSTODIA"}` {
		t.Fatalf("payload = %s", payload)
	}
	if client.invokeCount != 2 || waitCount != 1 {
		t.Fatalf("invocations = %d, waits = %d", client.invokeCount, waitCount)
	}
	if !strings.Contains(stderr.String(), `"cause":"PRIVATE_DATA_NOT_DISSEMINATED"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvokeWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	nonRetryable := errors.New(
		`{"code":"RECEIVER_MISMATCH","message":"wrong receiver","details":{"reintentable":false}}`,
	)
	client := &scriptedTransactionClient{errors: []error{nonRetryable}}
	waitCount := 0

	_, err := invokeWithRetry(
		context.Background(),
		client,
		options{function: "ReceiveTransfer", retryPrivateData: true},
		nil,
		io.Discard,
		func(context.Context, time.Duration) error {
			waitCount++
			return nil
		},
	)

	if !errors.Is(err, nonRetryable) {
		t.Fatalf("invokeWithRetry() error = %v, want %v", err, nonRetryable)
	}
	if client.invokeCount != 1 || waitCount != 0 {
		t.Fatalf("invocations = %d, waits = %d", client.invokeCount, waitCount)
	}
}
