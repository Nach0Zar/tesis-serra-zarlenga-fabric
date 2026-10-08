package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/fabric"
)

const anmatMSPID = "AnmatMSP"

type eventClient interface {
	ChaincodeEvents(context.Context, *uint64) (<-chan fabric.ChaincodeEvent, error)
	InvalidTransactions(context.Context, *uint64) (<-chan fabric.InvalidTransaction, error)
	Close() error
}

type listenerOptions struct {
	repositoryRoot  string
	gatewayEndpoint string
	tlsServerName   string
	channelName     string
	chaincodeName   string
	timeout         time.Duration
	startBlock      optionalUint64
}

type optionalUint64 struct {
	value uint64
	set   bool
}

func (value *optionalUint64) String() string {
	if !value.set {
		return ""
	}
	return strconv.FormatUint(value.value, 10)
}

func (value *optionalUint64) Set(raw string) error {
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("must be an unsigned block number: %w", err)
	}
	value.value = parsed
	value.set = true
	return nil
}

func (value optionalUint64) pointer() *uint64 {
	if !value.set {
		return nil
	}
	result := value.value
	return &result
}

type listenerReady struct {
	Type       string  `json:"type"`
	Profile    string  `json:"profile"`
	MSPID      string  `json:"mspId"`
	Channel    string  `json:"channel"`
	Chaincode  string  `json:"chaincode"`
	StartBlock *uint64 `json:"startBlock,omitempty"`
}

type validBusinessEvent struct {
	Type          string          `json:"type"`
	BlockNumber   uint64          `json:"blockNumber"`
	TransactionID string          `json:"transactionId"`
	EventName     string          `json:"eventName"`
	Unit          json.RawMessage `json:"unit"`
}

type invalidCommittedTransaction struct {
	Type                string `json:"type"`
	BlockNumber         uint64 `json:"blockNumber"`
	TransactionID       string `json:"transactionId"`
	TransactionType     string `json:"transactionType"`
	ValidationCode      string `json:"validationCode"`
	ValidationCodeValue int32  `json:"validationCodeValue"`
}

func runANMATListener(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	opts, help, err := parseListenerOptions(arguments, stderr)
	if help {
		return exitSuccess
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	repositoryRoot, err := resolveRepositoryRoot(opts.repositoryRoot, deps)
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}
	profile, err := deps.resolveProfile(
		repositoryRoot,
		"anmat",
		opts.gatewayEndpoint,
		opts.tlsServerName,
	)
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}
	if profile.MSPID != anmatMSPID {
		writeRuntimeError(
			stderr,
			fmt.Errorf("profile anmat resolved MSP %q, want %q", profile.MSPID, anmatMSPID),
			"configuration",
		)
		return exitRuntime
	}

	listener, err := deps.connectEvents(
		profile,
		opts.channelName,
		opts.chaincodeName,
		opts.timeout,
	)
	if err != nil {
		writeRuntimeError(stderr, err, "connect")
		return exitRuntime
	}

	listenErr := listenANMAT(
		ctx,
		listener,
		stdout,
		stderr,
		opts.channelName,
		opts.chaincodeName,
		opts.startBlock.pointer(),
	)
	closeErr := listener.Close()
	if listenErr != nil {
		writeRuntimeError(stderr, listenErr, "listen-anmat")
		return exitRuntime
	}
	if closeErr != nil {
		writeRuntimeError(stderr, closeErr, "close")
		return exitRuntime
	}
	return exitSuccess
}

func parseListenerOptions(
	arguments []string,
	stderr io.Writer,
) (listenerOptions, bool, error) {
	opts := listenerOptions{
		channelName:   config.DefaultChannelName,
		chaincodeName: config.DefaultChaincodeName,
		timeout:       30 * time.Second,
	}
	flags := flag.NewFlagSet("listen-anmat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.gatewayEndpoint, "gateway-endpoint", "", "endpoint gRPC alternativo")
	flags.StringVar(&opts.tlsServerName, "tls-server-name", "", "hostname TLS alternativo")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.Var(
		&opts.startBlock,
		"start-block",
		"bloque inicial inclusivo para replay (admite 0)",
	)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: snt-client listen-anmat [options]")
		flags.PrintDefaults()
	}

	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return listenerOptions{}, true, nil
		}
		return listenerOptions{}, false, err
	}
	if flags.NArg() != 0 {
		return listenerOptions{}, false, fmt.Errorf(
			"unexpected positional arguments %q",
			flags.Args(),
		)
	}
	return opts, false, nil
}

func listenANMAT(
	ctx context.Context,
	client eventClient,
	stdout io.Writer,
	stderr io.Writer,
	channelName string,
	chaincodeName string,
	startBlock *uint64,
) error {
	events, err := client.ChaincodeEvents(ctx, startBlock)
	if err != nil {
		return err
	}
	invalidTransactions, err := client.InvalidTransactions(ctx, startBlock)
	if err != nil {
		return err
	}
	if err := writeJSONLine(stderr, listenerReady{
		Type:       "LISTENER_READY",
		Profile:    "anmat",
		MSPID:      anmatMSPID,
		Channel:    channelName,
		Chaincode:  chaincodeName,
		StartBlock: startBlock,
	}); err != nil {
		return fmt.Errorf("write listener readiness: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("chaincode event stream closed unexpectedly")
			}
			if err := writeBusinessEvent(stdout, event); err != nil {
				return err
			}
		case transaction, ok := <-invalidTransactions:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("filtered block event stream closed unexpectedly")
			}
			if err := writeInvalidTransaction(stdout, transaction); err != nil {
				return err
			}
		}
	}
}

func writeBusinessEvent(writer io.Writer, event fabric.ChaincodeEvent) error {
	if !isANMATAlert(event.EventName) {
		return nil
	}

	var unit map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &unit); err != nil || unit == nil {
		if err == nil {
			err = errors.New("payload is not a JSON object")
		}
		return fmt.Errorf(
			"decode %s payload for transaction %s: %w",
			event.EventName,
			event.TransactionID,
			err,
		)
	}
	if err := writeJSONLine(writer, validBusinessEvent{
		Type:          "VALID_BUSINESS_EVENT",
		BlockNumber:   event.BlockNumber,
		TransactionID: event.TransactionID,
		EventName:     event.EventName,
		Unit:          append(json.RawMessage(nil), event.Payload...),
	}); err != nil {
		return fmt.Errorf("write valid business event: %w", err)
	}
	return nil
}

func writeInvalidTransaction(
	writer io.Writer,
	transaction fabric.InvalidTransaction,
) error {
	if err := writeJSONLine(writer, invalidCommittedTransaction{
		Type:                "INVALID_COMMITTED_TRANSACTION",
		BlockNumber:         transaction.BlockNumber,
		TransactionID:       transaction.TransactionID,
		TransactionType:     transaction.TransactionType,
		ValidationCode:      transaction.ValidationCode,
		ValidationCodeValue: transaction.ValidationCodeValue,
	}); err != nil {
		return fmt.Errorf("write invalid committed transaction: %w", err)
	}
	return nil
}

func isANMATAlert(eventName string) bool {
	switch eventName {
	case "Quarantine", "ReportExpired", "ReportStolen", "ReportLost":
		return true
	default:
		return false
	}
}

func writeJSONLine(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
