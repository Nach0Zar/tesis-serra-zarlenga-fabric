package runmeta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fabricExample   = "../../../benchmarks/examples/run-metadata.fabric.json"
	baselineExample = "../../../benchmarks/examples/run-metadata.baseline.json"
	mixedExample    = "../../../benchmarks/examples/run-metadata.mixed-fabric.json"
	schemaFile      = "../../../benchmarks/schema/run-metadata.schema.json"
)

func TestTheExamplesSatisfyTheContract(t *testing.T) {
	t.Parallel()

	validator := newTestValidator(t)

	for _, example := range []string{fabricExample, baselineExample, mixedExample} {
		t.Run(filepath.Base(example), func(t *testing.T) {
			t.Parallel()

			findings, err := validator.ValidateFile(example)
			if err != nil {
				t.Fatalf("validar %s: %v", example, err)
			}

			if len(findings) != 0 {
				t.Fatalf("el ejemplo %s deberia cumplir el contrato y reporta:\n%s",
					example, strings.Join(findings, "\n"))
			}
		})
	}
}

// TestTheContractRejects recorre los incumplimientos que DES-20 exige detectar.
// Cada caso parte de un ejemplo valido y lo degrada en un solo punto, de modo
// que el rechazo solo pueda atribuirse a esa degradacion. Se verifica ademas el
// mensaje, porque un validador que rechaza todo por el motivo equivocado no
// sirve para diagnosticar una corrida.
func TestTheContractRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		example string
		mutate  func(document map[string]any)
		expect  string
	}{
		{
			name:    "un campo desconocido en la raiz",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["campoInventado"] = "x" },
			expect:  "campoInventado",
		},
		{
			name:    "un campo desconocido anidado",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "host")["gpu"] = "x" },
			expect:  "gpu",
		},
		{
			name:    "Fabric sin packageID",
			example: fabricExample,
			mutate:  func(d map[string]any) { delete(object(d, "environment"), "packageID") },
			expect:  "packageID",
		},
		{
			name:    "Fabric sin version de contrato",
			example: fabricExample,
			mutate:  func(d map[string]any) { delete(object(d, "environment"), "contractVersion") },
			expect:  "contractVersion",
		},
		{
			name:    "Fabric sin version de Caliper",
			example: fabricExample,
			mutate:  func(d map[string]any) { delete(object(d, "environment"), "caliper") },
			expect:  "caliper",
		},
		{
			name:    "un packageID que no es label:sha256",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "environment")["packageID"] = "snt_1.0:no-es-un-hash" },
			expect:  "packageID",
		},
		{
			name:    "la baseline sin su commit",
			example: baselineExample,
			mutate:  func(d map[string]any) { delete(object(d, "environment"), "baselineCommit") },
			expect:  "baselineCommit",
		},
		{
			name:    "la baseline sin su imagen",
			example: baselineExample,
			mutate:  func(d map[string]any) { delete(object(d, "environment"), "baselineImage") },
			expect:  "baselineImage",
		},
		{
			name:    "la baseline declarando identificadores de Fabric",
			example: baselineExample,
			mutate:  func(d map[string]any) { object(d, "environment")["packageID"] = packageID(t) },
			expect:  "'/environment/packageID': la propiedad no corresponde",
		},
		{
			name:    "la baseline en un escenario de Raft",
			example: baselineExample,
			mutate:  func(d map[string]any) { d["scenario"] = "raft-1" },
			expect:  "/scenario",
		},
		{
			name:    "Fabric en un escenario de la baseline",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["scenario"] = "db-1" },
			expect:  "/scenario",
		},
		{
			name:    "un escenario que no corresponde a la operacion medida",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["scenario"] = "read-unit" },
			expect:  "/rate/operation",
		},
		{
			name:    "una transferencia contada como una sola transaccion",
			example: fabricExample,
			mutate: func(d map[string]any) {
				object(d, "rate")["transactionsPerOperation"] = 1.0
				object(d, "rate")["targetTransactionsPerSecond"] = 5.0
			},
			expect: "/rate/transactionsPerOperation",
		},
		{
			name:    "una tasa de transacciones que no deriva de la de operaciones",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "rate")["targetTransactionsPerSecond"] = 5.0 },
			expect:  "/rate/targetTransactionsPerSecond",
		},
		{
			name:    "una mezcla que no suma 100",
			example: mixedExample,
			mutate:  func(d map[string]any) { object(object(d, "rate"), "mix")["query"] = 30.0 },
			expect:  "/rate/mix",
		},
		{
			name:    "una mezcla cuyo promedio ponderado no coincide",
			example: mixedExample,
			mutate: func(d map[string]any) {
				object(d, "rate")["transactionsPerOperation"] = 1.0
				object(d, "rate")["targetTransactionsPerSecond"] = 20.0
			},
			expect: "/rate/transactionsPerOperation",
		},
		{
			name:    "una mezcla declarada fuera de la carga mixta",
			example: fabricExample,
			mutate: func(d map[string]any) {
				object(d, "rate")["mix"] = map[string]any{
					"register": 10.0, "transfer": 55.0, "dispense": 10.0, "query": 25.0,
				}
			},
			expect: "'/rate/mix': la propiedad no corresponde",
		},
		{
			name:    "la carga mixta sin declarar su mezcla",
			example: mixedExample,
			mutate:  func(d map[string]any) { delete(object(d, "rate"), "mix") },
			expect:  "missing property 'mix'",
		},
		{
			name:    "una repeticion numerada sobre un warm-up",
			example: mixedExample,
			mutate:  func(d map[string]any) { d["repetition"] = 1.0 },
			expect:  "/repetition",
		},
		{
			name:    "una medicion sin numero de repeticion",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["repetition"] = 0.0 },
			expect:  "/repetition",
		},
		{
			name:    "una repeticion fuera de la serie extendida",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["repetition"] = 9.0 },
			expect:  "/repetition",
		},
		{
			name:    "una seed distinta de la del protocolo",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "dataset")["seed"] = 1.0 },
			expect:  "/dataset/seed",
		},
		{
			name:    "un dataset por debajo del minimo",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "dataset")["units"] = 100.0 },
			expect:  "/dataset/units",
		},
		{
			name:    "un hash de dataset que no es sha256",
			example: fabricExample,
			mutate:  func(d map[string]any) { object(d, "dataset")["sha256"] = "abc" },
			expect:  "/dataset/sha256",
		},
		{
			name:    "un commit de repositorio abreviado",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["repositoryCommit"] = "ccb58f4" },
			expect:  "/repositoryCommit",
		},
		{
			name:    "una fecha que no es RFC 3339",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["startedAt"] = "ayer a la tarde" },
			expect:  "/startedAt",
		},
		{
			name:    "una corrida que termina antes de empezar",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["endedAt"] = "2026-10-05T14:00:00Z" },
			expect:  "/endedAt",
		},
		{
			name:    "una ventana mas corta que la duracion medida",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["endedAt"] = "2026-10-05T14:03:42Z" },
			expect:  "/durationSeconds",
		},
		{
			name:    "una version de schema que no es la del contrato",
			example: fabricExample,
			mutate:  func(d map[string]any) { d["schemaVersion"] = "2.0.0" },
			expect:  "/schemaVersion",
		},
	}

	validator := newTestValidator(t)

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			document := loadExample(t, testCase.example)
			testCase.mutate(document)

			findings := validator.Validate(marshal(t, document))
			if len(findings) == 0 {
				t.Fatalf("el contrato acepta %s y no deberia", testCase.name)
			}

			if !mentions(findings, testCase.expect) {
				t.Fatalf("se esperaba un hallazgo sobre %q y se obtuvo:\n%s",
					testCase.expect, strings.Join(findings, "\n"))
			}
		})
	}
}

// TestTheExamplesStayInSyncWithTheContract evita que el contrato se versione
// sin que los ejemplos lo acompanen, que es como un consumidor terminaria
// generando codigo contra una version que ya no existe.
func TestTheExamplesStayInSyncWithTheContract(t *testing.T) {
	t.Parallel()

	schema := loadExample(t, schemaFile)
	if identifier, _ := schema["$id"].(string); identifier != SchemaID {
		t.Fatalf("el contrato declara $id %q y el validador aplica %q", identifier, SchemaID)
	}

	for _, example := range []string{fabricExample, baselineExample, mixedExample} {
		document := loadExample(t, example)
		if reference, _ := document["$schema"].(string); reference != SchemaID {
			t.Errorf("%s referencia %q en lugar de %q", example, reference, SchemaID)
		}
	}
}

func TestValidateRejectsSomethingThatIsNotJSON(t *testing.T) {
	t.Parallel()

	findings := newTestValidator(t).Validate([]byte("esto no es json"))
	if !mentions(findings, "no es JSON valido") {
		t.Fatalf("se esperaba un hallazgo sobre JSON invalido y se obtuvo: %v", findings)
	}
}

func TestNewValidatorReportsAnUnreadableContract(t *testing.T) {
	t.Parallel()

	if _, err := NewValidator(filepath.Join(t.TempDir(), "no-existe.json")); err == nil {
		t.Fatal("compilar un contrato inexistente deberia fallar")
	}
}

func TestDefaultSchemaPathPointsAtTheContract(t *testing.T) {
	t.Parallel()

	// DefaultSchemaPath se resuelve desde el modulo client; este test corre dos
	// niveles mas abajo.
	if _, err := os.Stat(filepath.Join("../..", DefaultSchemaPath)); err != nil {
		t.Fatalf("la ruta por defecto no encuentra el contrato: %v", err)
	}
}

func newTestValidator(t *testing.T) *Validator {
	t.Helper()

	validator, err := NewValidator(schemaFile)
	if err != nil {
		t.Fatalf("compilar el contrato: %v", err)
	}

	return validator
}

func loadExample(t *testing.T, path string) map[string]any {
	t.Helper()

	// #nosec G304 -- la ruta viene de constantes del propio test.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("interpretar %s: %v", path, err)
	}

	return document
}

func marshal(t *testing.T, document map[string]any) []byte {
	t.Helper()

	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("serializar el documento: %v", err)
	}

	return raw
}

func object(document map[string]any, key string) map[string]any {
	nested, _ := document[key].(map[string]any)

	return nested
}

func packageID(t *testing.T) string {
	t.Helper()

	return object(loadExample(t, fabricExample), "environment")["packageID"].(string)
}

func mentions(findings []string, fragment string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, fragment) {
			return true
		}
	}

	return false
}
