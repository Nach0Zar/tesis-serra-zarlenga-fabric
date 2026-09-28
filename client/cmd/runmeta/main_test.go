package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	schemaFile    = "../../../benchmarks/schema/run-metadata.schema.json"
	fabricExample = "../../../benchmarks/examples/run-metadata.fabric.json"
)

func TestRunAcceptsADocumentThatMeetsTheContract(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(schemaFile, []string{fabricExample}, &stdout, &stderr); err != nil {
		t.Fatalf("un ejemplo valido no deberia fallar: %v", err)
	}

	if !strings.HasPrefix(stdout.String(), "ok\t") {
		t.Errorf("se esperaba un ok en la salida y se obtuvo %q", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("no deberia haber hallazgos y se obtuvo %q", stderr.String())
	}
}

func TestRunRejectsADocumentAndDetailsWhy(t *testing.T) {
	t.Parallel()

	degraded := filepath.Join(t.TempDir(), "metadata.json")
	writeDegradedExample(t, degraded)

	var stdout, stderr bytes.Buffer
	err := run(schemaFile, []string{fabricExample, degraded}, &stdout, &stderr)
	if err == nil {
		t.Fatal("un documento que no cumple el contrato deberia hacer fallar la corrida")
	}

	if !strings.Contains(err.Error(), "1 de 2 documentos") {
		t.Errorf("se esperaba el conteo de rechazos y se obtuvo %q", err.Error())
	}

	// El documento valido igual se informa: validar un lote no se detiene en el
	// primer rechazo.
	if !strings.Contains(stdout.String(), fabricExample) {
		t.Errorf("faltó informar el documento valido: %q", stdout.String())
	}

	if !strings.Contains(stderr.String(), "campoInventado") {
		t.Errorf("se esperaba el detalle del incumplimiento y se obtuvo %q", stderr.String())
	}
}

func TestRunRequiresAtLeastOneDocument(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(schemaFile, nil, &stdout, &stderr); err == nil {
		t.Fatal("invocar el validador sin documentos deberia fallar")
	}
}

func TestRunReportsAnUnreadableContract(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := run(filepath.Join(t.TempDir(), "no-existe.json"), []string{fabricExample}, &stdout, &stderr)
	if err == nil {
		t.Fatal("un contrato inexistente deberia fallar")
	}
}

func writeDegradedExample(t *testing.T, path string) {
	t.Helper()

	// #nosec G304 -- la ruta viene de constantes del propio test.
	raw, err := os.ReadFile(fabricExample)
	if err != nil {
		t.Fatalf("leer el ejemplo: %v", err)
	}

	degraded := strings.Replace(string(raw), `"protocol":`, `"campoInventado": 1,
  "protocol":`, 1)
	if degraded == string(raw) {
		t.Fatal("no se pudo degradar el ejemplo")
	}

	// #nosec G703 -- path lo arma el propio test dentro de t.TempDir().
	if err := os.WriteFile(path, []byte(degraded), 0o600); err != nil {
		t.Fatalf("escribir el documento degradado: %v", err)
	}
}
