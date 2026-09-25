package app

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
)

const (
	defaultDemoGTIN       = "07791234567898"
	defaultDemoSerial     = "DEMO-CORE-0001"
	defaultRejectSerial   = "DEMO-CORE-REJECT-1"
	defaultDemoLot        = "LOTE-DEMO-2026"
	defaultDemoExpiration = "2099-12-31"
)

type demoOptions struct {
	gtin           string
	serialNumber   string
	rejectSerial   string
	lot            string
	expirationDate string
	repositoryRoot string
	channelName    string
	chaincodeName  string
	timeout        time.Duration
	retryInterval  time.Duration
}

type demoStep struct {
	name         string
	organization string
	arguments    []string
	stdin        string
}

func runDemoCore(
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	opts, help, err := parseDemoOptions(arguments, stderr)
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
	drugstoreID, err := deps.resolveCanonicalID(repositoryRoot, "drogueria")
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}
	pharmacyID, err := deps.resolveCanonicalID(repositoryRoot, "farmacia")
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}

	steps := buildDemoSteps(opts, repositoryRoot, drugstoreID, pharmacyID)
	for index, step := range steps {
		_, _ = fmt.Fprintf(
			stdout,
			"\n[%d/%d] %s (%s)\n",
			index+1,
			len(steps),
			step.name,
			step.organization,
		)
		var payload bytes.Buffer
		exitCode := run(
			step.arguments,
			&payload,
			stderr,
			strings.NewReader(step.stdin),
			deps,
		)
		if exitCode != exitSuccess {
			return exitCode
		}
		if _, err := io.Copy(stdout, &payload); err != nil {
			writeRuntimeError(stderr, err, "output")
			return exitRuntime
		}
	}
	_, _ = fmt.Fprintln(stdout, "\nDemo core completed.")
	return exitSuccess
}

func parseDemoOptions(arguments []string, stderr io.Writer) (demoOptions, bool, error) {
	opts := demoOptions{
		gtin:           defaultDemoGTIN,
		serialNumber:   defaultDemoSerial,
		rejectSerial:   defaultRejectSerial,
		lot:            defaultDemoLot,
		expirationDate: defaultDemoExpiration,
		channelName:    config.DefaultChannelName,
		chaincodeName:  config.DefaultChaincodeName,
		timeout:        30 * time.Second,
		retryInterval:  defaultRetryInterval,
	}
	flags := flag.NewFlagSet("demo-core", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.gtin, "gtin", opts.gtin, "GTIN-14 usado por la demo")
	flags.StringVar(&opts.serialNumber, "serial", opts.serialNumber, "número de serie usado por el flujo feliz")
	flags.StringVar(&opts.rejectSerial, "reject-serial", opts.rejectSerial, "número de serie usado por el flujo de rechazo")
	flags.StringVar(&opts.lot, "lot", opts.lot, "lote usado por la demo")
	flags.StringVar(&opts.expirationDate, "expiry", opts.expirationDate, "vencimiento YYYY-MM-DD")
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "timeout por operación")
	flags.DurationVar(
		&opts.retryInterval,
		"retry-interval",
		opts.retryInterval,
		"espera ante datos privados aún no diseminados",
	)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: snt-client demo-core [options]")
		flags.PrintDefaults()
	}

	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return demoOptions{}, true, nil
		}
		return demoOptions{}, false, err
	}
	if flags.NArg() != 0 {
		return demoOptions{}, false, fmt.Errorf("unexpected positional arguments %q", flags.Args())
	}
	required := []struct {
		option string
		value  string
	}{
		{option: "--gtin", value: opts.gtin},
		{option: "--serial", value: opts.serialNumber},
		{option: "--reject-serial", value: opts.rejectSerial},
		{option: "--lot", value: opts.lot},
		{option: "--expiry", value: opts.expirationDate},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return demoOptions{}, false, fmt.Errorf("%s must not be empty", field.option)
		}
	}
	if opts.timeout <= 0 {
		return demoOptions{}, false, errors.New("--timeout must be greater than zero")
	}
	if opts.retryInterval <= 0 {
		return demoOptions{}, false, errors.New("--retry-interval must be greater than zero")
	}
	return opts, false, nil
}

func buildDemoSteps(
	opts demoOptions,
	repositoryRoot string,
	drugstoreID string,
	pharmacyID string,
) []demoStep {
	commonFlags := []string{
		"--repo-root", repositoryRoot,
		"--channel", opts.channelName,
		"--chaincode", opts.chaincodeName,
		"--timeout", opts.timeout.String(),
	}
	command := func(name, organization, serialNumber string, extra ...string) []string {
		result := []string{name, "--org", organization, "--gtin", opts.gtin, "--serial", serialNumber}
		result = append(result, extra...)
		return append(result, commonFlags...)
	}
	dispatch := func(organization, destination, serialNumber, documentSuffix string) demoStep {
		return demoStep{
			name:         "dispatch-transfer",
			organization: organization,
			arguments:    command("dispatch-transfer", organization, serialNumber, "--transient-file", "-"),
			stdin: marshalArgument(map[string]any{
				"destinatario": map[string]string{"destino": destination},
				"commercial": map[string]any{
					"numeroRemito":  "R-DEMO-" + documentSuffix,
					"numeroFactura": "F-DEMO-" + documentSuffix,
					"cantidad":      1,
				},
			}),
		}
	}
	receive := func(organization, serialNumber string) demoStep {
		return demoStep{
			name:         "receive-transfer",
			organization: organization,
			arguments: command(
				"receive-transfer",
				organization,
				serialNumber,
				"--retry-interval",
				opts.retryInterval.String(),
			),
		}
	}
	register := func(serialNumber string) demoStep {
		return demoStep{
			name:         "register-unit",
			organization: "lab",
			arguments: command(
				"register-unit",
				"lab",
				serialNumber,
				"--lot",
				opts.lot,
				"--expiry",
				opts.expirationDate,
			),
		}
	}

	return []demoStep{
		register(opts.serialNumber),
		dispatch("lab", drugstoreID, opts.serialNumber, "LAB-DROG-0001"),
		{name: "verify-unit", organization: "drogueria", arguments: command("verify-unit", "drogueria", opts.serialNumber)},
		receive("drogueria", opts.serialNumber),
		dispatch("drogueria", pharmacyID, opts.serialNumber, "DROG-FARM-0001"),
		{name: "verify-unit", organization: "farmacia", arguments: command("verify-unit", "farmacia", opts.serialNumber)},
		receive("farmacia", opts.serialNumber),
		{name: "dispense", organization: "farmacia", arguments: command("dispense", "farmacia", opts.serialNumber)},
		{name: "read-unit", organization: "farmacia", arguments: command("read-unit", "farmacia", opts.serialNumber)},
		{name: "unit-history", organization: "farmacia", arguments: command("unit-history", "farmacia", opts.serialNumber)},
		register(opts.rejectSerial),
		dispatch("lab", drugstoreID, opts.rejectSerial, "LAB-DROG-REJECT"),
		{
			name:         "reject-transfer",
			organization: "drogueria",
			arguments: command(
				"reject-transfer",
				"drogueria",
				opts.rejectSerial,
				"--reason",
				"rechazo de recepción demostrado por CLI-2",
			),
		},
		{name: "read-unit", organization: "drogueria", arguments: command("read-unit", "drogueria", opts.rejectSerial)},
		{
			name:         "query-units-by-gtin",
			organization: "farmacia",
			arguments: append(
				[]string{"query-units-by-gtin", "--org", "farmacia", "--gtin", opts.gtin},
				commonFlags...,
			),
		},
	}
}
