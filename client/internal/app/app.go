// Package app implementa el parsing y la ejecución de la CLI genérica.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/contracterr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/fabric"
)

const (
	exitSuccess = 0
	exitRuntime = 1
	exitUsage   = 2

	maxTransientFileSize = 1 << 20
)

type options struct {
	command               string
	operation             string
	organization          string
	function              string
	arguments             stringList
	transientFile         string
	requiredTransientKeys []string
	allowedTransientKeys  []string
	retryPrivateData      bool
	retryInterval         time.Duration
	repositoryRoot        string
	gatewayEndpoint       string
	tlsServerName         string
	channelName           string
	chaincodeName         string
	timeout               time.Duration
}

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type transactionClient interface {
	Query(context.Context, string, []string, map[string][]byte) ([]byte, error)
	Invoke(context.Context, string, []string, map[string][]byte) ([]byte, error)
	Close() error
}

type dependencies struct {
	currentDirectory   func() (string, error)
	findRepositoryRoot func(string) (string, error)
	resolveProfile     func(string, string, string, string) (config.Profile, error)
	resolveCanonicalID func(string, string) (string, error)
	connect            func(config.Profile, string, string, time.Duration) (transactionClient, error)
	wait               func(context.Context, time.Duration) error
}

func productionDependencies() dependencies {
	return dependencies{
		currentDirectory:   os.Getwd,
		findRepositoryRoot: config.FindRepositoryRoot,
		resolveProfile:     config.Resolve,
		resolveCanonicalID: config.CanonicalID,
		connect: func(
			profile config.Profile,
			channelName string,
			chaincodeName string,
			timeout time.Duration,
		) (transactionClient, error) {
			return fabric.Connect(profile, channelName, chaincodeName, timeout)
		},
		wait: waitForRetry,
	}
}

// Run ejecuta el cliente CLI y devuelve un código apto para os.Exit.
func Run(arguments []string, stdout, stderr io.Writer, stdin io.Reader) int {
	return run(arguments, stdout, stderr, stdin, productionDependencies())
}

func run(
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	stdin io.Reader,
	deps dependencies,
) int {
	if len(arguments) > 0 && arguments[0] == "demo-core" {
		return runDemoCore(arguments[1:], stdout, stderr, deps)
	}

	opts, help, err := parseOptions(arguments, stderr)
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
		opts.organization,
		opts.gatewayEndpoint,
		opts.tlsServerName,
	)
	if err != nil {
		writeRuntimeError(stderr, err, "configuration")
		return exitRuntime
	}

	transient, err := readTransient(opts.transientFile, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	if err := validateTransient(opts, transient); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	gatewayClient, err := deps.connect(
		profile,
		opts.channelName,
		opts.chaincodeName,
		opts.timeout,
	)
	if err != nil {
		writeRuntimeError(stderr, err, "connect")
		return exitRuntime
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	var payload []byte
	if opts.command == "query" {
		payload, err = gatewayClient.Query(ctx, opts.function, opts.arguments, transient)
	} else {
		payload, err = invokeWithRetry(ctx, gatewayClient, opts, transient, stderr, deps.wait)
	}
	closeErr := gatewayClient.Close()

	if err != nil {
		writeRuntimeError(stderr, err, opts.command)
		return exitRuntime
	}
	if closeErr != nil {
		writeRuntimeError(stderr, closeErr, "close")
		return exitRuntime
	}
	if err := writePayload(stdout, payload); err != nil {
		writeRuntimeError(stderr, err, "output")
		return exitRuntime
	}
	return exitSuccess
}

func parseOptions(arguments []string, stderr io.Writer) (options, bool, error) {
	if len(arguments) == 0 {
		printUsage(stderr)
		return options{}, false, errors.New("expected query or invoke command")
	}

	command := arguments[0]
	if isBusinessCommand(command) {
		return parseBusinessOptions(command, arguments[1:], stderr)
	}
	if command != "query" && command != "invoke" {
		printUsage(stderr)
		return options{}, false, fmt.Errorf("unknown command %q", command)
	}

	opts := options{
		command:       command,
		operation:     command,
		channelName:   config.DefaultChannelName,
		chaincodeName: config.DefaultChaincodeName,
		timeout:       30 * time.Second,
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.organization, "org", "", "organización: anmat, lab, drogueria o farmacia")
	flags.StringVar(&opts.function, "function", "", "función pública del contrato")
	flags.Var(&opts.arguments, "arg", "argumento posicional (repetible y ordenado)")
	flags.StringVar(
		&opts.transientFile,
		"transient-file",
		"",
		"objeto JSON con transient data; use - para stdin",
	)
	flags.StringVar(&opts.repositoryRoot, "repo-root", "", "raíz del repositorio")
	flags.StringVar(&opts.gatewayEndpoint, "gateway-endpoint", "", "endpoint gRPC alternativo")
	flags.StringVar(&opts.tlsServerName, "tls-server-name", "", "hostname TLS alternativo")
	flags.StringVar(&opts.channelName, "channel", opts.channelName, "canal Fabric")
	flags.StringVar(&opts.chaincodeName, "chaincode", opts.chaincodeName, "chaincode")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "timeout de la operación")
	flags.Usage = func() {
		printCommandUsage(stderr, command, flags)
	}

	if err := flags.Parse(arguments[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if flags.NArg() != 0 {
		return options{}, false, fmt.Errorf(
			"unexpected positional arguments %q; use --arg for each contract argument",
			flags.Args(),
		)
	}
	if opts.organization == "" {
		return options{}, false, errors.New("--org is required")
	}
	if opts.function == "" {
		return options{}, false, errors.New("--function is required")
	}
	if opts.timeout <= 0 {
		return options{}, false, errors.New("--timeout must be greater than zero")
	}

	return opts, false, nil
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintln(writer, "  snt-client register-unit       --org <org> --gtin <gtin> --serial <serie> --lot <lote> --expiry <fecha>")
	_, _ = fmt.Fprintln(writer, "  snt-client dispatch-transfer   --org <org> --gtin <gtin> --serial <serie> --transient-file <archivo|->")
	_, _ = fmt.Fprintln(writer, "  snt-client receive-transfer    --org <org> --gtin <gtin> --serial <serie> [--transient-file <archivo|->]")
	_, _ = fmt.Fprintln(writer, "  snt-client reject-transfer     --org <org> --gtin <gtin> --serial <serie> --reason <motivo>")
	_, _ = fmt.Fprintln(writer, "  snt-client dispense            --org <org> --gtin <gtin> --serial <serie>")
	_, _ = fmt.Fprintln(writer, "  snt-client read-unit           --org <org> --gtin <gtin> --serial <serie>")
	_, _ = fmt.Fprintln(writer, "  snt-client unit-history        --org <org> --gtin <gtin> --serial <serie>")
	_, _ = fmt.Fprintln(writer, "  snt-client verify-unit         --org <org> --gtin <gtin> --serial <serie>")
	_, _ = fmt.Fprintln(writer, "  snt-client query-units-by-gtin --org <org> --gtin <gtin>")
	_, _ = fmt.Fprintln(writer, "  snt-client demo-core [--repo-root <ruta>]")
	_, _ = fmt.Fprintln(writer)
	_, _ = fmt.Fprintln(writer, "Low-level access:")
	_, _ = fmt.Fprintln(writer, "  snt-client query  --org <org> --function <name> [--arg <value> ...]")
	_, _ = fmt.Fprintln(writer, "  snt-client invoke --org <org> --function <name> [--arg <value> ...]")
	_, _ = fmt.Fprintln(writer, "Run any command with --help to list its options.")
}

func printCommandUsage(writer io.Writer, command string, flags *flag.FlagSet) {
	_, _ = fmt.Fprintf(
		writer,
		"Usage: snt-client %s --org <org> --function <name> [options]\n",
		command,
	)
	flags.PrintDefaults()
}

func readTransient(path string, stdin io.Reader) (map[string][]byte, error) {
	if path == "" {
		return nil, nil
	}

	reader := stdin
	var file *os.File
	var err error
	if path != "-" {
		// La ruta es entrada explícita del operador; no se combina con secretos
		// ni se usa para escribir contenido.
		//nolint:gosec // lectura intencional de un archivo indicado por --transient-file
		file, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open transient file: %w", err)
		}
		defer func() {
			_ = file.Close()
		}()
		reader = file
	}

	contents, err := io.ReadAll(io.LimitReader(reader, maxTransientFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read transient JSON object: %w", err)
	}
	if len(contents) > maxTransientFileSize {
		return nil, fmt.Errorf("transient file exceeds %d bytes", maxTransientFileSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	var values map[string]json.RawMessage
	if err := decoder.Decode(&values); err != nil {
		return nil, fmt.Errorf("decode transient JSON object: %w", err)
	}
	if values == nil {
		return nil, errors.New("transient JSON must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("transient file must contain one JSON object")
		}
		return nil, fmt.Errorf("decode trailing transient data: %w", err)
	}

	transient := make(map[string][]byte, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("transient keys must not be empty")
		}
		if !json.Valid(value) {
			return nil, fmt.Errorf("transient value %q is not valid JSON", key)
		}
		transient[key] = append([]byte(nil), value...)
	}
	return transient, nil
}

func writePayload(writer io.Writer, payload []byte) error {
	if len(payload) == 0 {
		_, err := fmt.Fprintln(writer)
		return err
	}

	if json.Valid(payload) {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, payload, "", "  "); err != nil {
			return err
		}
		_, err := fmt.Fprintln(writer, formatted.String())
		return err
	}

	if _, err := writer.Write(payload); err != nil {
		return err
	}
	if payload[len(payload)-1] != '\n' {
		_, err := fmt.Fprintln(writer)
		return err
	}
	return nil
}

func writeRuntimeError(writer io.Writer, err error, stage string) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(contracterr.Normalize(err, stage))
}
