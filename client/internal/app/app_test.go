package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
)

type fakeTransactionClient struct {
	queryPayload  []byte
	queryErr      error
	invokePayload []byte
	invokeErr     error

	calledCommand   string
	calledFunction  string
	calledArguments []string
	calledTransient map[string][]byte
	closed          bool
}

func (client *fakeTransactionClient) Query(
	_ context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	client.capture("query", function, arguments, transient)
	return client.queryPayload, client.queryErr
}

func (client *fakeTransactionClient) Invoke(
	_ context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	client.capture("invoke", function, arguments, transient)
	return client.invokePayload, client.invokeErr
}

func (client *fakeTransactionClient) Close() error {
	client.closed = true
	return nil
}

func (client *fakeTransactionClient) capture(
	command string,
	function string,
	arguments []string,
	transient map[string][]byte,
) {
	client.calledCommand = command
	client.calledFunction = function
	client.calledArguments = append([]string(nil), arguments...)
	client.calledTransient = transient
}

func TestRunGenericQuery(t *testing.T) {
	gatewayClient := &fakeTransactionClient{queryPayload: []byte("{\"ok\":true}")}
	var resolvedOrganization string
	deps := dependencies{
		resolveProfile: func(
			_ string,
			organization string,
			_ string,
			_ string,
		) (config.Profile, error) {
			resolvedOrganization = organization
			return config.Profile{}, nil
		},
		connect: func(
			_ config.Profile,
			channelName string,
			chaincodeName string,
			timeout time.Duration,
		) (transactionClient, error) {
			if channelName != "snt-channel" || chaincodeName != "snt" {
				t.Fatalf("connect names = %q, %q", channelName, chaincodeName)
			}
			if timeout != 30*time.Second {
				t.Fatalf("connect timeout = %s", timeout)
			}
			return gatewayClient, nil
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{
			"query",
			"--repo-root", "repository",
			"--org", "farmacia",
			"--function", "ReadUnit",
			"--arg", "07791234567898",
			"--arg", "SN-1",
			"--transient-file", "-",
		},
		&stdout,
		&stderr,
		strings.NewReader("{\"destinatario\":{\"gln\":\"7791234500048\"}}"),
		deps,
	)

	if exitCode != exitSuccess {
		t.Fatalf("run() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if resolvedOrganization != "farmacia" {
		t.Fatalf("resolved organization = %q", resolvedOrganization)
	}
	if gatewayClient.calledCommand != "query" || gatewayClient.calledFunction != "ReadUnit" {
		t.Fatalf(
			"called transaction = %q %q",
			gatewayClient.calledCommand,
			gatewayClient.calledFunction,
		)
	}
	if got := strings.Join(gatewayClient.calledArguments, "|"); got != "07791234567898|SN-1" {
		t.Fatalf("called arguments = %q", got)
	}
	if got := string(gatewayClient.calledTransient["destinatario"]); got != "{\"gln\":\"7791234500048\"}" {
		t.Fatalf("transient destinatario = %s", got)
	}
	if !gatewayClient.closed {
		t.Fatal("Gateway client was not closed")
	}
	if stdout.String() != "{\n  \"ok\": true\n}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunGenericInvokePropagatesDES5Error(t *testing.T) {
	gatewayClient := &fakeTransactionClient{
		invokeErr: errors.New(
			"endorse transaction: rpc error: " +
				"{\"code\":\"INVALID_REQUEST\",\"message\":\"GTIN inválido.\",\"details\":{\"field\":\"gtin\"}}",
		),
	}
	deps := stubDependencies(gatewayClient)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{
			"invoke",
			"--repo-root", "repository",
			"--org", "lab",
			"--function", "RegisterUnit",
			"--arg", "{\"gtin\":\"invalid\"}",
		},
		&stdout,
		&stderr,
		strings.NewReader(""),
		deps,
	)

	if exitCode != exitRuntime {
		t.Fatalf("run() exit = %d, want %d", exitCode, exitRuntime)
	}
	if gatewayClient.calledCommand != "invoke" {
		t.Fatalf("called command = %q", gatewayClient.calledCommand)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	want := "{\"code\":\"INVALID_REQUEST\",\"message\":\"GTIN inválido.\",\"details\":{\"field\":\"gtin\"}}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunNormalizesConnectionError(t *testing.T) {
	deps := stubDependencies(nil)
	deps.connect = func(
		_ config.Profile,
		_ string,
		_ string,
		_ time.Duration,
	) (transactionClient, error) {
		return nil, errors.New("dial failed")
	}
	var stderr bytes.Buffer

	exitCode := run(
		[]string{
			"query",
			"--repo-root", "repository",
			"--org", "anmat",
			"--function", "QueryOrganizations",
		},
		&bytes.Buffer{},
		&stderr,
		strings.NewReader(""),
		deps,
	)

	if exitCode != exitRuntime {
		t.Fatalf("run() exit = %d, want %d", exitCode, exitRuntime)
	}
	want := "{\"code\":\"INTERNAL_ERROR\",\"message\":" +
		"\"Error no clasificable atribuible al chaincode o a la plataforma.\"," +
		"\"details\":{\"cause\":\"dial failed\",\"classification\":\"unexpected\",\"stage\":\"connect\"}}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsUnflaggedArguments(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"query", "--org", "lab", "--function", "ReadUnit", "unexpected"},
		&bytes.Buffer{},
		&stderr,
		strings.NewReader(""),
		dependencies{},
	)

	if exitCode != exitUsage {
		t.Fatalf("run() exit = %d, want %d", exitCode, exitUsage)
	}
	if !strings.Contains(stderr.String(), "use --arg") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestReadTransientRejectsMultipleValues(t *testing.T) {
	_, err := readTransient("-", strings.NewReader("{} {}"))
	if err == nil || !strings.Contains(err.Error(), "one JSON object") {
		t.Fatalf("readTransient() error = %v", err)
	}
}

func TestReadTransientRejectsOversizedInput(t *testing.T) {
	_, err := readTransient(
		"-",
		strings.NewReader("{\"key\":\""+strings.Repeat("x", maxTransientFileSize)+"\"}"),
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readTransient() error = %v", err)
	}
}

func stubDependencies(gatewayClient transactionClient) dependencies {
	return dependencies{
		resolveProfile: func(
			_ string,
			_ string,
			_ string,
			_ string,
		) (config.Profile, error) {
			return config.Profile{}, nil
		},
		connect: func(
			_ config.Profile,
			_ string,
			_ string,
			_ time.Duration,
		) (transactionClient, error) {
			return gatewayClient, nil
		},
	}
}
