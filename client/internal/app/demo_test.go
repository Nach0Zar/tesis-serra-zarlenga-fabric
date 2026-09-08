package app

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
)

type recordedTransaction struct {
	organization string
	command      string
	function     string
	arguments    []string
	transient    map[string][]byte
}

type demoRecorder struct {
	transactions []recordedTransaction
	closeCount   int
}

type recordingTransactionClient struct {
	organization string
	recorder     *demoRecorder
}

func (client *recordingTransactionClient) Query(
	_ context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	client.capture("query", function, arguments, transient)
	return []byte(`{"ok":true}`), nil
}

func (client *recordingTransactionClient) Invoke(
	_ context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	client.capture("invoke", function, arguments, transient)
	return []byte(`{"ok":true}`), nil
}

func (client *recordingTransactionClient) Close() error {
	client.recorder.closeCount++
	return nil
}

func (client *recordingTransactionClient) capture(
	command string,
	function string,
	arguments []string,
	transient map[string][]byte,
) {
	transientCopy := make(map[string][]byte, len(transient))
	for key, value := range transient {
		transientCopy[key] = append([]byte(nil), value...)
	}
	client.recorder.transactions = append(client.recorder.transactions, recordedTransaction{
		organization: client.organization,
		command:      command,
		function:     function,
		arguments:    append([]string(nil), arguments...),
		transient:    transientCopy,
	})
}

func TestRunDemoCoreExecutesCompleteFlow(t *testing.T) {
	recorder := &demoRecorder{}
	deps := dependencies{
		resolveCanonicalID: func(_ string, organization string) (string, error) {
			ids := map[string]string{
				"drogueria": "GLN:7791234500024",
				"farmacia":  "GLN:7791234500048",
			}
			return ids[organization], nil
		},
		resolveProfile: func(
			_ string,
			organization string,
			_ string,
			_ string,
		) (config.Profile, error) {
			return config.Profile{Organization: organization}, nil
		},
		connect: func(
			profile config.Profile,
			channelName string,
			chaincodeName string,
			timeout time.Duration,
		) (transactionClient, error) {
			if channelName != config.DefaultChannelName || chaincodeName != config.DefaultChaincodeName {
				return nil, fmt.Errorf("unexpected contract %s/%s", channelName, chaincodeName)
			}
			if timeout != time.Second {
				return nil, fmt.Errorf("unexpected timeout %s", timeout)
			}
			return &recordingTransactionClient{
				organization: profile.Organization,
				recorder:     recorder,
			}, nil
		},
		wait: func(context.Context, time.Duration) error {
			return nil
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		[]string{
			"demo-core",
			"--repo-root", "repository",
			"--serial", "DEMO-TEST-1",
			"--timeout", "1s",
		},
		&stdout,
		&stderr,
		strings.NewReader(""),
		deps,
	)

	if exitCode != exitSuccess {
		t.Fatalf("run() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	wantFunctions := []string{
		"RegisterUnit",
		"DispatchTransfer",
		"ReceiveTransfer",
		"DispatchTransfer",
		"ReceiveTransfer",
		"Dispense",
		"ReadUnit",
		"GetUnitHistory",
		"QueryUnitsByGTIN",
	}
	wantCommands := []string{"invoke", "invoke", "invoke", "invoke", "invoke", "invoke", "query", "query", "query"}
	wantOrganizations := []string{
		"lab",
		"lab",
		"drogueria",
		"drogueria",
		"farmacia",
		"farmacia",
		"farmacia",
		"farmacia",
		"farmacia",
	}
	if len(recorder.transactions) != len(wantFunctions) {
		t.Fatalf("transactions = %d, want %d", len(recorder.transactions), len(wantFunctions))
	}
	for index, transaction := range recorder.transactions {
		if transaction.function != wantFunctions[index] ||
			transaction.command != wantCommands[index] ||
			transaction.organization != wantOrganizations[index] {
			t.Fatalf(
				"transaction %d = %s/%s/%s, want %s/%s/%s",
				index,
				transaction.organization,
				transaction.command,
				transaction.function,
				wantOrganizations[index],
				wantCommands[index],
				wantFunctions[index],
			)
		}
	}
	if recorder.closeCount != len(wantFunctions) {
		t.Fatalf("closed clients = %d, want %d", recorder.closeCount, len(wantFunctions))
	}
	if got := string(recorder.transactions[1].transient["destinatario"]); got !=
		`{"destino":"GLN:7791234500024"}` {
		t.Fatalf("first transfer destination = %s", got)
	}
	if got := string(recorder.transactions[3].transient["destinatario"]); got !=
		`{"destino":"GLN:7791234500048"}` {
		t.Fatalf("second transfer destination = %s", got)
	}
	if !strings.Contains(stdout.String(), "Demo core completed.") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
