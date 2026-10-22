package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
)

// TestRunRejectsInvalidInvocationsWithUsage fija los codigos de salida y el
// mensaje de cada invocacion mal formada, sin tocar la red.
func TestRunRejectsInvalidInvocationsWithUsage(t *testing.T) {
	cases := []struct {
		name      string
		arguments []string
		exit      int
		stderr    string
	}{
		{"sin comando", nil, exitUsage, "expected query or invoke command"},
		{"comando desconocido", []string{"borrar"}, exitUsage, `unknown command "borrar"`},
		{"ayuda", []string{"query", "--help"}, exitSuccess, "Usage: snt-client query --org <org> --function <name>"},
		{"posicionales", []string{"query", "--org", "lab", "--function", "ReadUnit", "suelto"}, exitUsage, "use --arg for each contract argument"},
		{"sin organizacion", []string{"invoke", "--function", "RegisterUnit"}, exitUsage, "--org is required"},
		{"sin funcion", []string{"query", "--org", "lab"}, exitUsage, "--function is required"},
		{"timeout invalido", []string{"query", "--org", "lab", "--function", "ReadUnit", "--timeout", "0s"}, exitUsage, "--timeout must be greater than zero"},
		{"flag desconocido", []string{"query", "--org", "lab", "--function", "ReadUnit", "--nada", "x"}, exitUsage, "flag provided but not defined"},
		{"ayuda de negocio", []string{"read-unit", "--help"}, exitSuccess, "Usage: snt-client read-unit [options]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exit := run(tc.arguments, &stdout, &stderr, strings.NewReader(""), dependencies{})
			if exit != tc.exit {
				t.Fatalf("exit = %d, want %d; stderr = %s", exit, tc.exit, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.stderr)
			}
		})
	}

	var stderr bytes.Buffer
	run(nil, &bytes.Buffer{}, &stderr, strings.NewReader(""), dependencies{})
	for _, command := range []string{"register-unit", "listen-anmat", "demo-server", "query  --org"} {
		if !strings.Contains(stderr.String(), command) {
			t.Fatalf("la ayuda general no menciona %q", command)
		}
	}
}

// TestRunReportsEachRuntimeStage: cada falla se informa como error
// normalizado con la etapa en que ocurrio y codigo de salida de runtime.
func TestRunReportsEachRuntimeStage(t *testing.T) {
	query := []string{"query", "--org", "lab", "--function", "ReadUnit", "--arg", "x"}
	failure := errors.New("falla")
	cases := []struct {
		name      string
		arguments []string
		stdin     string
		deps      func(*fakeTransactionClient) dependencies
		exit      int
		stderr    string
	}{
		{
			name: "directorio actual", arguments: query,
			deps: func(*fakeTransactionClient) dependencies {
				return dependencies{currentDirectory: func() (string, error) { return "", failure }}
			},
			exit: exitRuntime, stderr: `"configuration"`,
		},
		{
			name: "raiz del repositorio", arguments: query,
			deps: func(*fakeTransactionClient) dependencies {
				return dependencies{
					currentDirectory:   func() (string, error) { return "/tmp", nil },
					findRepositoryRoot: func(string) (string, error) { return "", failure },
				}
			},
			exit: exitRuntime, stderr: `"configuration"`,
		},
		{
			name: "perfil", arguments: append([]string{}, append(query, "--repo-root", "repo")...),
			deps: func(*fakeTransactionClient) dependencies {
				return dependencies{resolveProfile: func(string, string, string, string) (config.Profile, error) {
					return config.Profile{}, failure
				}}
			},
			exit: exitRuntime, stderr: `"configuration"`,
		},
		{
			name: "transient invalido", arguments: append(append([]string{}, query...), "--repo-root", "repo", "--transient-file", "-"),
			stdin: "[]",
			deps:  func(client *fakeTransactionClient) dependencies { return stubDependencies(client) },
			exit:  exitUsage, stderr: "decode transient JSON object",
		},
		{
			name: "conexion", arguments: append(append([]string{}, query...), "--repo-root", "repo"),
			deps: func(client *fakeTransactionClient) dependencies {
				deps := stubDependencies(client)
				deps.connect = func(config.Profile, string, string, time.Duration) (transactionClient, error) {
					return nil, failure
				}
				return deps
			},
			exit: exitRuntime, stderr: `"connect"`,
		},
		{
			name: "consulta", arguments: append(append([]string{}, query...), "--repo-root", "repo"),
			deps: func(client *fakeTransactionClient) dependencies {
				client.queryErr = failure
				return stubDependencies(client)
			},
			exit: exitRuntime, stderr: `"query"`,
		},
		{
			name: "cierre", arguments: append(append([]string{}, query...), "--repo-root", "repo"),
			deps: func(client *fakeTransactionClient) dependencies {
				client.queryPayload = []byte(`{}`)
				client.closeErr = failure
				return stubDependencies(client)
			},
			exit: exitRuntime, stderr: `"close"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeTransactionClient{}
			var stdout, stderr bytes.Buffer
			exit := run(tc.arguments, &stdout, &stderr, strings.NewReader(tc.stdin), tc.deps(client))
			if exit != tc.exit {
				t.Fatalf("exit = %d, want %d; stderr = %s", exit, tc.exit, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.stderr)
			}
		})
	}
}

// TestReadTransientAcceptsOnlyOneJSONObject cubre las formas de transient
// data que el CLI rechaza antes de llegar al contrato.
func TestReadTransientAcceptsOnlyOneJSONObject(t *testing.T) {
	cases := map[string]string{
		"null":          "transient JSON must be an object",
		"{} {}":         "must contain one JSON object",
		`{} x`:          "decode trailing transient data",
		`{"  ": 1}`:     "transient keys must not be empty",
		`{"a": `:        "decode transient JSON object",
		`"texto plano"`: "decode transient JSON object",
	}
	for input, want := range cases {
		if _, err := readTransient("-", strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("readTransient(%q) error = %v, want %q", input, err, want)
		}
	}

	if _, err := readTransient(filepath.Join(t.TempDir(), "no-existe.json"), nil); err == nil ||
		!strings.Contains(err.Error(), "open transient file") {
		t.Fatalf("archivo inexistente: %v", err)
	}
	path := filepath.Join(t.TempDir(), "transient.json")
	if err := os.WriteFile(path, []byte(`{"destinatario":{"gln":"7791234500048"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	transient, err := readTransient(path, nil)
	if err != nil || string(transient["destinatario"]) != `{"gln":"7791234500048"}` {
		t.Fatalf("readTransient(archivo) = %q, %v", transient, err)
	}
}

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, errors.New("salida cerrada") }

// TestWritePayloadFormatsJSONAndKeepsOtherPayloads: el JSON se indenta, el
// resto se escribe tal cual con un salto final, y un error de salida se
// propaga.
func TestWritePayloadFormatsJSONAndKeepsOtherPayloads(t *testing.T) {
	cases := map[string]string{
		"":            "\n",
		`{"ok":true}`: "{\n  \"ok\": true\n}\n",
		"texto":       "texto\n",
		"con salto\n": "con salto\n",
	}
	for payload, want := range cases {
		var out bytes.Buffer
		if err := writePayload(&out, []byte(payload)); err != nil || out.String() != want {
			t.Fatalf("writePayload(%q) = %q, %v; want %q", payload, out.String(), err, want)
		}
	}
	for _, payload := range []string{"", `{"ok":true}`, "texto"} {
		if err := writePayload(failingOutput{}, []byte(payload)); err == nil {
			t.Fatalf("writePayload(%q) deberia propagar el error de salida", payload)
		}
	}
}

// TestWaitForRetryHonoursCancellation: la espera entre reintentos termina al
// vencer el intervalo o al cancelarse el contexto.
func TestWaitForRetryHonoursCancellation(t *testing.T) {
	if err := waitForRetry(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("waitForRetry() = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRetry(cancelado) = %v", err)
	}
}

// TestMarshalArgumentPanicsOnUnserializableRequests: un request interno que
// no se puede serializar es un error de programacion, no de entrada.
func TestMarshalArgumentPanicsOnUnserializableRequests(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("marshalArgument deberia entrar en panico")
		}
	}()
	marshalArgument(make(chan int))
}

// TestProductionDependenciesAreWired: el CLI real resuelve perfiles y
// conexiones con las implementaciones de config y fabric.
func TestProductionDependenciesAreWired(t *testing.T) {
	deps := productionDependencies()
	if deps.currentDirectory == nil || deps.findRepositoryRoot == nil || deps.resolveProfile == nil ||
		deps.resolveCanonicalID == nil || deps.connect == nil || deps.connectEvents == nil ||
		deps.connectDemo == nil || deps.wait == nil {
		t.Fatalf("dependencias de produccion incompletas: %+v", deps)
	}
	var stderr bytes.Buffer
	if exit := Run([]string{"query", "--help"}, &bytes.Buffer{}, &stderr, strings.NewReader("")); exit != exitSuccess {
		t.Fatalf("Run(--help) = %d", exit)
	}
}

// TestLongRunningCommandsValidateTheirOptions: los comandos de la demo, del
// listener y de lotes rechazan opciones invalidas antes de conectarse.
func TestLongRunningCommandsValidateTheirOptions(t *testing.T) {
	cases := []struct {
		arguments []string
		exit      int
		stderr    string
	}{
		{[]string{"demo-server", "--help"}, exitSuccess, "-addr"},
		{[]string{"demo-server", "suelto"}, exitUsage, "unexpected positional arguments"},
		{[]string{"demo-server", "--addr", "0.0.0.0:8080"}, exitUsage, "must be a loopback address"},
		{[]string{"demo-server", "--addr", "sin-puerto"}, exitUsage, "--addr"},
		{[]string{"demo-server", "--gtin", " "}, exitUsage, "--gtin must not be empty"},
		{[]string{"demo-server", "--timeout", "0s"}, exitUsage, "--timeout must be greater than zero"},
		{[]string{"demo-server", "--retry-interval", "0s"}, exitUsage, "--retry-interval must be greater than zero"},
		{[]string{"demo-core", "--help"}, exitSuccess, "-reject-serial"},
		{[]string{"demo-core", "--nada"}, exitUsage, "flag provided but not defined"},
		{[]string{"listen-anmat", "--help"}, exitSuccess, "-start-block"},
		{[]string{"listen-anmat", "--nada"}, exitUsage, "flag provided but not defined"},
		{[]string{"withdraw-batch", "--help"}, exitSuccess, "-lot"},
		{[]string{"withdraw-batch", "--org", "anmat", "--gtin", "07791234567898"}, exitUsage, "--lot"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.arguments, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exit := runContext(context.Background(), tc.arguments, &stdout, &stderr, strings.NewReader(""), dependencies{})
			if exit != tc.exit {
				t.Fatalf("exit = %d, want %d; stderr = %s", exit, tc.exit, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.stderr)
			}
		})
	}
}
