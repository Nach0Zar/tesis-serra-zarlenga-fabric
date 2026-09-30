package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/fabric"
)

type fakeEventClient struct {
	events                 chan fabric.ChaincodeEvent
	invalidTransactions    chan fabric.InvalidTransaction
	eventsErr              error
	invalidTransactionsErr error
	eventsStartBlock       *uint64
	invalidStartBlock      *uint64
	closed                 bool
}

func (client *fakeEventClient) ChaincodeEvents(
	_ context.Context,
	startBlock *uint64,
) (<-chan fabric.ChaincodeEvent, error) {
	client.eventsStartBlock = copyBlockNumber(startBlock)
	return client.events, client.eventsErr
}

func (client *fakeEventClient) InvalidTransactions(
	_ context.Context,
	startBlock *uint64,
) (<-chan fabric.InvalidTransaction, error) {
	client.invalidStartBlock = copyBlockNumber(startBlock)
	return client.invalidTransactions, client.invalidTransactionsErr
}

func (client *fakeEventClient) Close() error {
	client.closed = true
	return nil
}

func copyBlockNumber(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestParseListenerOptionsPreservesStartBlockZero(t *testing.T) {
	opts, help, err := parseListenerOptions(
		[]string{
			"--repo-root", "repository",
			"--gateway-endpoint", "dns:///peer.example:7051",
			"--tls-server-name", "peer.example",
			"--channel", "channel",
			"--chaincode", "contract",
			"--start-block", "0",
		},
		io.Discard,
	)
	if err != nil || help {
		t.Fatalf("parseListenerOptions() = help %t, err %v", help, err)
	}
	if !opts.startBlock.set || opts.startBlock.value != 0 {
		t.Fatalf("start block = %+v, want explicit zero", opts.startBlock)
	}
	if opts.gatewayEndpoint != "dns:///peer.example:7051" ||
		opts.tlsServerName != "peer.example" ||
		opts.channelName != "channel" ||
		opts.chaincodeName != "contract" {
		t.Fatalf("unexpected listener overrides: %+v", opts)
	}
}

func TestParseListenerOptionsRejectsOrganizationAndInvalidBlock(t *testing.T) {
	for _, arguments := range [][]string{
		{"--org", "lab"},
		{"--start-block", "-1"},
		{"--start-block", "not-a-number"},
		{"unexpected"},
	} {
		if _, _, err := parseListenerOptions(arguments, io.Discard); err == nil {
			t.Fatalf("parseListenerOptions(%q) error = nil", arguments)
		}
	}
}

func TestRunANMATListenerUsesFixedIdentityAndStartBlock(t *testing.T) {
	listener := &fakeEventClient{
		events:              make(chan fabric.ChaincodeEvent),
		invalidTransactions: make(chan fabric.InvalidTransaction),
	}
	var resolvedOrganization string
	var connectedProfile config.Profile
	deps := dependencies{
		resolveProfile: func(
			_ string,
			organization string,
			_ string,
			_ string,
		) (config.Profile, error) {
			resolvedOrganization = organization
			return config.Profile{Organization: "anmat", MSPID: anmatMSPID}, nil
		},
		connectEvents: func(
			profile config.Profile,
			_ string,
			_ string,
			_ time.Duration,
		) (eventClient, error) {
			connectedProfile = profile
			return listener, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer

	exitCode := runContext(
		ctx,
		[]string{"listen-anmat", "--repo-root", "repository", "--start-block", "0"},
		&bytes.Buffer{},
		&stderr,
		strings.NewReader(""),
		deps,
	)

	if exitCode != exitSuccess {
		t.Fatalf("runContext() exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if resolvedOrganization != "anmat" ||
		connectedProfile.Organization != "anmat" ||
		connectedProfile.MSPID != anmatMSPID {
		t.Fatalf(
			"resolved organization = %q, connected profile = %+v",
			resolvedOrganization,
			connectedProfile,
		)
	}
	if listener.eventsStartBlock == nil || *listener.eventsStartBlock != 0 ||
		listener.invalidStartBlock == nil || *listener.invalidStartBlock != 0 {
		t.Fatalf(
			"start blocks = %v and %v, want explicit zero",
			listener.eventsStartBlock,
			listener.invalidStartBlock,
		)
	}
	if !listener.closed {
		t.Fatal("event client was not closed")
	}
	if !strings.Contains(stderr.String(), `"type":"LISTENER_READY"`) {
		t.Fatalf("stderr = %q, want readiness record", stderr.String())
	}
}

func TestRunANMATListenerRejectsUnexpectedMSP(t *testing.T) {
	connectCalled := false
	deps := dependencies{
		resolveProfile: func(
			_ string,
			_ string,
			_ string,
			_ string,
		) (config.Profile, error) {
			return config.Profile{Organization: "anmat", MSPID: "LabMSP"}, nil
		},
		connectEvents: func(
			_ config.Profile,
			_ string,
			_ string,
			_ time.Duration,
		) (eventClient, error) {
			connectCalled = true
			return nil, nil
		},
	}
	var stderr bytes.Buffer
	exitCode := runContext(
		context.Background(),
		[]string{"listen-anmat", "--repo-root", "repository"},
		io.Discard,
		&stderr,
		strings.NewReader(""),
		deps,
	)
	if exitCode != exitRuntime || connectCalled {
		t.Fatalf("exit = %d, connectCalled = %t", exitCode, connectCalled)
	}
	if !strings.Contains(stderr.String(), "AnmatMSP") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWriteBusinessEventEmitsFourANMATAlerts(t *testing.T) {
	var output bytes.Buffer
	for index, eventName := range []string{
		"Quarantine",
		"ReportExpired",
		"ReportStolen",
		"ReportLost",
	} {
		err := writeBusinessEvent(&output, fabric.ChaincodeEvent{
			BlockNumber:   uint64(index + 10),
			TransactionID: "tx-" + eventName,
			EventName:     eventName,
			Payload:       []byte(`{"gtin":"07791234567898","numeroSerie":"SN-1"}`),
		})
		if err != nil {
			t.Fatalf("writeBusinessEvent(%s) error = %v", eventName, err)
		}
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4; output = %q", len(lines), output.String())
	}
	for index, eventName := range []string{
		"Quarantine",
		"ReportExpired",
		"ReportStolen",
		"ReportLost",
	} {
		if !strings.Contains(lines[index], `"type":"VALID_BUSINESS_EVENT"`) ||
			!strings.Contains(lines[index], `"eventName":"`+eventName+`"`) ||
			!strings.Contains(lines[index], `"unit":{"gtin"`) {
			t.Fatalf("line %d = %q", index, lines[index])
		}
	}
}

func TestWriteBusinessEventIgnoresOtherConfirmedEvents(t *testing.T) {
	for _, eventName := range []string{
		"RegisterUnit",
		"DispatchTransfer",
		"ReceiveTransfer",
		"RejectTransfer",
		"Dispense",
		"ReleaseQuarantine",
		"ReportDamaged",
		"WithdrawFromMarket",
		"ProhibitProduct",
		"ReturnProduct",
		"Restock",
		"FinalDisposition",
	} {
		var output bytes.Buffer
		err := writeBusinessEvent(&output, fabric.ChaincodeEvent{
			EventName: eventName,
			Payload:   []byte(`{"gtin":"07791234567898"}`),
		})
		if err != nil || output.Len() != 0 {
			t.Fatalf(
				"writeBusinessEvent(%s) = err %v, output %q",
				eventName,
				err,
				output.String(),
			)
		}
	}
}

func TestWriteBusinessEventRejectsMalformedPayload(t *testing.T) {
	for _, payload := range [][]byte{[]byte("{"), []byte("null"), []byte("[]")} {
		err := writeBusinessEvent(io.Discard, fabric.ChaincodeEvent{
			TransactionID: "tx-malformed",
			EventName:     "ReportStolen",
			Payload:       payload,
		})
		if err == nil {
			t.Fatalf("payload %q error = nil", payload)
		}
	}
}

func TestListenerReportsInvalidTransaction(t *testing.T) {
	var output bytes.Buffer
	err := writeInvalidTransaction(&output, fabric.InvalidTransaction{
		BlockNumber:         91,
		TransactionID:       "tx-invalid",
		TransactionType:     "ENDORSER_TRANSACTION",
		ValidationCode:      "MVCC_READ_CONFLICT",
		ValidationCodeValue: 11,
	})
	if err != nil {
		t.Fatalf("writeInvalidTransaction() error = %v", err)
	}
	wantParts := []string{
		`"type":"INVALID_COMMITTED_TRANSACTION"`,
		`"blockNumber":91`,
		`"transactionId":"tx-invalid"`,
		`"validationCode":"MVCC_READ_CONFLICT"`,
		`"validationCodeValue":11`,
	}
	for _, want := range wantParts {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
}

func TestListenerTreatsOutputFailureAsOperationalError(t *testing.T) {
	err := writeBusinessEvent(failingWriter{}, fabric.ChaincodeEvent{
		TransactionID: "tx-output",
		EventName:     "ReportLost",
		Payload:       []byte(`{"gtin":"07791234567898"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("writeBusinessEvent() error = %v", err)
	}
}

func TestListenerRequiresBothSubscriptionsBeforeReady(t *testing.T) {
	listener := &fakeEventClient{
		events:                 make(chan fabric.ChaincodeEvent),
		invalidTransactionsErr: errors.New("filtered stream unavailable"),
	}
	var stderr bytes.Buffer
	err := listenANMAT(
		context.Background(),
		listener,
		io.Discard,
		&stderr,
		"snt-channel",
		"snt",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "filtered stream unavailable") {
		t.Fatalf("listenANMAT() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("readiness written before both subscriptions: %q", stderr.String())
	}
}

func TestListenerReportsUnexpectedStreamClosure(t *testing.T) {
	events := make(chan fabric.ChaincodeEvent)
	close(events)
	listener := &fakeEventClient{
		events:              events,
		invalidTransactions: make(chan fabric.InvalidTransaction),
	}
	var stderr bytes.Buffer
	err := listenANMAT(
		context.Background(),
		listener,
		io.Discard,
		&stderr,
		"snt-channel",
		"snt",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "closed unexpectedly") {
		t.Fatalf("listenANMAT() error = %v", err)
	}
	if !strings.Contains(stderr.String(), `"type":"LISTENER_READY"`) {
		t.Fatalf("stderr = %q, want readiness record", stderr.String())
	}
}

func TestListenerReportsUnexpectedFilteredStreamClosure(t *testing.T) {
	invalidTransactions := make(chan fabric.InvalidTransaction)
	close(invalidTransactions)
	listener := &fakeEventClient{
		events:              make(chan fabric.ChaincodeEvent),
		invalidTransactions: invalidTransactions,
	}
	err := listenANMAT(
		context.Background(),
		listener,
		io.Discard,
		io.Discard,
		"snt-channel",
		"snt",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "filtered block event stream closed unexpectedly") {
		t.Fatalf("listenANMAT() error = %v", err)
	}
}
