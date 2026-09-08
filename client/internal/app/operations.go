package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/contracterr"
)

const defaultRetryInterval = 500 * time.Millisecond

type registerUnitRequest struct {
	GTIN           string `json:"gtin"`
	SerialNumber   string `json:"numeroSerie"`
	Lot            string `json:"lote"`
	ExpirationDate string `json:"fechaVencimiento"`
}

type unitRefRequest struct {
	GTIN         string `json:"gtin"`
	SerialNumber string `json:"numeroSerie"`
}

type privateDataRetryDetails struct {
	Retryable bool   `json:"reintentable"`
	Cause     string `json:"causa"`
}

type retryEvent struct {
	Event     string `json:"event"`
	Operation string `json:"operation"`
	Attempt   int    `json:"attempt"`
	Cause     string `json:"cause"`
}

func isBusinessCommand(command string) bool {
	switch command {
	case "register-unit",
		"dispatch-transfer",
		"receive-transfer",
		"dispense",
		"read-unit",
		"unit-history",
		"query-units-by-gtin":
		return true
	default:
		return false
	}
}

func parseBusinessOptions(
	command string,
	arguments []string,
	stderr io.Writer,
) (options, bool, error) {
	opts := options{
		operation:     command,
		channelName:   config.DefaultChannelName,
		chaincodeName: config.DefaultChaincodeName,
		timeout:       30 * time.Second,
		retryInterval: defaultRetryInterval,
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)

	var gtin string
	var serialNumber string
	var lot string
	var expirationDate string

	flags.StringVar(&opts.organization, "org", "", "organización: anmat, lab, drogueria o farmacia")
	flags.StringVar(&gtin, "gtin", "", "GTIN-14 de la unidad")
	if command != "query-units-by-gtin" {
		flags.StringVar(&serialNumber, "serial", "", "número de serie de la unidad")
	}
	if command == "register-unit" {
		flags.StringVar(&lot, "lot", "", "lote de elaboración")
		flags.StringVar(&expirationDate, "expiry", "", "fecha de vencimiento YYYY-MM-DD")
	}
	if command == "dispatch-transfer" || command == "receive-transfer" {
		flags.StringVar(
			&opts.transientFile,
			"transient-file",
			"",
			"objeto JSON con transient data; use - para stdin",
		)
	}
	if command == "receive-transfer" {
		flags.DurationVar(
			&opts.retryInterval,
			"retry-interval",
			opts.retryInterval,
			"espera entre reintentos por datos privados aún no diseminados",
		)
	}
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.gatewayEndpoint, "gateway-endpoint", "", "endpoint gRPC alternativo")
	flags.StringVar(&opts.tlsServerName, "tls-server-name", "", "hostname TLS alternativo")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "timeout total de la operación")
	flags.Usage = func() {
		printBusinessUsage(stderr, command, flags)
	}

	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if flags.NArg() != 0 {
		return options{}, false, fmt.Errorf("unexpected positional arguments %q", flags.Args())
	}
	if opts.organization == "" {
		return options{}, false, errors.New("--org is required")
	}
	if strings.TrimSpace(gtin) == "" {
		return options{}, false, errors.New("--gtin is required")
	}
	if command != "query-units-by-gtin" && strings.TrimSpace(serialNumber) == "" {
		return options{}, false, errors.New("--serial is required")
	}
	if opts.timeout <= 0 {
		return options{}, false, errors.New("--timeout must be greater than zero")
	}

	unitReference := unitRefRequest{GTIN: gtin, SerialNumber: serialNumber}
	switch command {
	case "register-unit":
		if strings.TrimSpace(lot) == "" {
			return options{}, false, errors.New("--lot is required")
		}
		if strings.TrimSpace(expirationDate) == "" {
			return options{}, false, errors.New("--expiry is required")
		}
		opts.command = "invoke"
		opts.function = "RegisterUnit"
		opts.arguments = stringList{marshalArgument(registerUnitRequest{
			GTIN:           gtin,
			SerialNumber:   serialNumber,
			Lot:            lot,
			ExpirationDate: expirationDate,
		})}
	case "dispatch-transfer":
		if opts.transientFile == "" {
			return options{}, false, errors.New("--transient-file is required")
		}
		opts.command = "invoke"
		opts.function = "DispatchTransfer"
		opts.arguments = stringList{marshalArgument(unitReference)}
		opts.requiredTransientKeys = []string{"destinatario", "commercial"}
		opts.allowedTransientKeys = []string{"destinatario", "commercial"}
	case "receive-transfer":
		if opts.retryInterval <= 0 {
			return options{}, false, errors.New("--retry-interval must be greater than zero")
		}
		opts.command = "invoke"
		opts.function = "ReceiveTransfer"
		opts.arguments = stringList{marshalArgument(unitReference)}
		opts.allowedTransientKeys = []string{"commercial"}
		opts.retryPrivateData = true
	case "dispense":
		opts.command = "invoke"
		opts.function = "Dispense"
		opts.arguments = stringList{marshalArgument(unitReference)}
	case "read-unit":
		opts.command = "query"
		opts.function = "ReadUnit"
		opts.arguments = stringList{gtin, serialNumber}
	case "unit-history":
		opts.command = "query"
		opts.function = "GetUnitHistory"
		opts.arguments = stringList{gtin, serialNumber}
	case "query-units-by-gtin":
		opts.command = "query"
		opts.function = "QueryUnitsByGTIN"
		opts.arguments = stringList{gtin}
	default:
		return options{}, false, fmt.Errorf("unsupported business command %q", command)
	}

	return opts, false, nil
}

func printBusinessUsage(writer io.Writer, command string, flags *flag.FlagSet) {
	_, _ = fmt.Fprintf(writer, "Usage: snt-client %s [options]\n", command)
	flags.PrintDefaults()
}

func marshalArgument(value any) string {
	contents, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal internal request: %v", err))
	}
	return string(contents)
}

func validateTransient(opts options, transient map[string][]byte) error {
	for _, key := range opts.requiredTransientKeys {
		if _, ok := transient[key]; !ok {
			return fmt.Errorf("transient key %q is required for %s", key, opts.operation)
		}
	}
	if len(opts.allowedTransientKeys) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(opts.allowedTransientKeys))
	for _, key := range opts.allowedTransientKeys {
		allowed[key] = struct{}{}
	}
	var unexpected []string
	for key := range transient {
		if _, ok := allowed[key]; !ok {
			unexpected = append(unexpected, key)
		}
	}
	if len(unexpected) == 0 {
		return nil
	}
	sort.Strings(unexpected)
	return fmt.Errorf(
		"transient keys not allowed for %s: %s",
		opts.operation,
		strings.Join(unexpected, ", "),
	)
}

func invokeWithRetry(
	ctx context.Context,
	client transactionClient,
	opts options,
	transient map[string][]byte,
	stderr io.Writer,
	wait func(context.Context, time.Duration) error,
) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		payload, err := client.Invoke(ctx, opts.function, opts.arguments, transient)
		if err == nil || !opts.retryPrivateData || !isPrivateDataNotDisseminated(err) {
			return payload, err
		}
		if wait == nil {
			return nil, err
		}
		_ = json.NewEncoder(stderr).Encode(retryEvent{
			Event:     "retry",
			Operation: opts.operation,
			Attempt:   attempt,
			Cause:     "PRIVATE_DATA_NOT_DISSEMINATED",
		})
		if waitErr := wait(ctx, opts.retryInterval); waitErr != nil {
			return nil, err
		}
	}
}

func isPrivateDataNotDisseminated(err error) bool {
	contractError, ok := contracterr.Extract(err)
	if !ok || contractError.Code != "INTERNAL_ERROR" || len(contractError.Details) == 0 {
		return false
	}
	var details privateDataRetryDetails
	if unmarshalErr := json.Unmarshal(contractError.Details, &details); unmarshalErr != nil {
		return false
	}
	return details.Retryable && details.Cause == "PRIVATE_DATA_NOT_DISSEMINATED"
}

func waitForRetry(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func resolveRepositoryRoot(explicitRoot string, deps dependencies) (string, error) {
	if explicitRoot != "" {
		return explicitRoot, nil
	}
	currentDirectory, err := deps.currentDirectory()
	if err != nil {
		return "", err
	}
	return deps.findRepositoryRoot(currentDirectory)
}
