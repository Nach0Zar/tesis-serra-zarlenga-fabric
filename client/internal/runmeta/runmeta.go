// Package runmeta valida el metadata.json de una corrida experimental contra el
// contrato versionado que fija DES-20.
//
// La validacion tiene dos capas. La primera es el JSON Schema
// benchmarks/schema/run-metadata.schema.json, que rechaza campos desconocidos y
// exige los identificadores propios de cada SUT. La segunda son las reglas
// aritmeticas y temporales que JSON Schema no puede expresar: la coherencia
// entre la tasa de operaciones conceptuales y la tasa de transacciones
// efectivas (docs/measurement-protocol.md, seccion 3.4) y la consistencia entre
// la ventana declarada y la duracion medida.
package runmeta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// SchemaID identifica la version del contrato de metadatos que este validador aplica.
const SchemaID = "urn:pfi-snt:run-metadata:schema:1.0.0"

// DefaultSchemaPath ubica el contrato desde el directorio del modulo client,
// que es donde corren `make` y `go test` de este modulo.
const DefaultSchemaPath = "../benchmarks/schema/run-metadata.schema.json"

// rateTolerance acota el error relativo admitido al comparar tasas. Las tasas
// derivadas de una mezcla porcentual no son exactas en punto flotante: la carga
// mixta de la seccion 6.4 da 1,55 transacciones por operacion, y 1,55 x 20 no
// es exactamente 31 en binario.
const rateTolerance = 1e-6

// transactionsPerOperation traduce cada operacion conceptual del protocolo a la
// cantidad de transacciones efectivas que el cliente envia al SUT. La
// transferencia vale dos porque ADR-004 la implementa como despacho mas
// recepcion (seccion 3.4).
var transactionsPerOperation = map[string]float64{
	"register":      1,
	"transfer":      2,
	"dispense":      1,
	"query-unit":    1,
	"query-history": 1,
}

// Validator aplica el contrato a documentos de metadatos.
type Validator struct {
	schema *jsonschema.Schema
}

// NewValidator compila el contrato ubicado en schemaPath.
func NewValidator(schemaPath string) (*Validator, error) {
	// #nosec G304 -- la ruta la elige quien corre el validador, que ya tiene
	// acceso al repositorio y a los crudos de la corrida.
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("leer el contrato %s: %w", schemaPath, err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("interpretar el contrato %s: %w", schemaPath, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	// Sin AssertFormat, `format: date-time` seria una anotacion y no una
	// restriccion, y startedAt podria traer cualquier cadena.
	compiler.AssertFormat()

	const resource = "https://pfi.invalid/run-metadata.schema.json"
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, fmt.Errorf("registrar el contrato %s: %w", schemaPath, err)
	}

	schema, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compilar el contrato %s: %w", schemaPath, err)
	}

	return &Validator{schema: schema}, nil
}

// Validate devuelve los incumplimientos del documento. Una corrida valida
// devuelve una lista vacia.
func (v *Validator) Validate(document []byte) []string {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return []string{fmt.Sprintf("el documento no es JSON valido: %v", err)}
	}

	var findings []string
	if err := v.schema.Validate(value); err != nil {
		findings = append(findings, schemaFindings(err)...)
	}

	// Las reglas semanticas se evaluan sobre la estructura tipada. Si el
	// documento ni siquiera tiene esa forma, el schema ya lo reporto y volver a
	// insistir solo agregaria ruido.
	var parsed metadata
	if err := json.Unmarshal(document, &parsed); err != nil {
		return findings
	}

	return append(findings, parsed.semanticFindings()...)
}

// ValidateFile valida el documento ubicado en path.
func (v *Validator) ValidateFile(path string) ([]string, error) {
	// #nosec G304 -- la ruta la elige quien corre el validador sobre sus propios crudos.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", path, err)
	}

	return v.Validate(raw), nil
}

// schemaFindings baja hasta las hojas del arbol de errores del validador. Los
// nodos intermedios solo dicen que un `allOf` o un objeto fallo; la propiedad
// concreta que sobra o falta aparece unicamente en la hoja, que es lo que
// necesita quien tiene que arreglar el metadata.json.
func schemaFindings(err error) []string {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return []string{err.Error()}
	}

	messages := leafMessages(validationErr, nil)
	if len(messages) == 0 {
		return []string{validationErr.Error()}
	}

	sort.Strings(messages)

	return slices.Compact(messages)
}

func leafMessages(node *jsonschema.ValidationError, collected []string) []string {
	if len(node.Causes) == 0 {
		// Un subesquema `false` marca una propiedad que este SUT no puede
		// declarar. El mensaje que trae la libreria ("false schema") nombra el
		// campo pero no explica nada, asi que se reemplaza.
		if _, prohibited := node.ErrorKind.(*kind.FalseSchema); prohibited {
			return append(collected, fmt.Sprintf(
				"at '/%s': la propiedad no corresponde a este documento",
				strings.Join(node.InstanceLocation, "/"),
			))
		}

		// Sin causas, Error() rinde exactamente una linea: la ubicacion dentro
		// del documento y el motivo concreto del rechazo.
		return append(collected, node.Error())
	}

	for _, cause := range node.Causes {
		collected = leafMessages(cause, collected)
	}

	return collected
}

type metadata struct {
	Scenario        string    `json:"scenario"`
	DurationSeconds float64   `json:"durationSeconds"`
	StartedAt       time.Time `json:"startedAt"`
	EndedAt         time.Time `json:"endedAt"`
	Rate            rate      `json:"rate"`
}

type rate struct {
	Operation                      string   `json:"operation"`
	TransactionsPerOperation       float64  `json:"transactionsPerOperation"`
	TargetOperationsPerSecond      float64  `json:"targetOperationsPerSecond"`
	TargetTransactionsPerSecond    float64  `json:"targetTransactionsPerSecond"`
	EffectiveTransactionsPerSecond float64  `json:"effectiveTransactionsPerSecond"`
	Mix                            *loadMix `json:"mix"`
}

type loadMix struct {
	Register float64 `json:"register"`
	Transfer float64 `json:"transfer"`
	Dispense float64 `json:"dispense"`
	Query    float64 `json:"query"`
}

func (m metadata) semanticFindings() []string {
	var findings []string
	findings = append(findings, m.Rate.findings()...)
	findings = append(findings, m.windowFindings()...)

	return findings
}

func (r rate) findings() []string {
	var findings []string

	if expected, known := transactionsPerOperation[r.Operation]; known && r.TransactionsPerOperation != expected {
		findings = append(findings, fmt.Sprintf(
			"/rate/transactionsPerOperation: la operacion %q vale %g transacciones efectivas y el documento declara %g",
			r.Operation, expected, r.TransactionsPerOperation,
		))
	}

	if r.Operation == "mixed" && r.Mix != nil {
		findings = append(findings, r.Mix.findings(r.TransactionsPerOperation)...)
	}

	product := r.TargetOperationsPerSecond * r.TransactionsPerOperation
	if !approxEqual(product, r.TargetTransactionsPerSecond) {
		findings = append(findings, fmt.Sprintf(
			"/rate/targetTransactionsPerSecond: %g operaciones conceptuales por segundo a %g transacciones por operacion dan %g, y el documento declara %g",
			r.TargetOperationsPerSecond, r.TransactionsPerOperation, product, r.TargetTransactionsPerSecond,
		))
	}

	return findings
}

func (m loadMix) findings(declared float64) []string {
	var findings []string

	total := m.Register + m.Transfer + m.Dispense + m.Query
	if !approxEqual(total, 100) {
		findings = append(findings, fmt.Sprintf(
			"/rate/mix: los porcentajes de la mezcla suman %g y deben sumar 100",
			total,
		))

		// Sin una mezcla completa el promedio ponderado no significa nada.
		return findings
	}

	// Una transferencia son dos transacciones; el resto de las operaciones de la
	// mezcla, una sola.
	weighted := (m.Register + m.Transfer*2 + m.Dispense + m.Query) / 100
	if !approxEqual(weighted, declared) {
		findings = append(findings, fmt.Sprintf(
			"/rate/transactionsPerOperation: la mezcla declarada implica %g transacciones por operacion y el documento declara %g",
			weighted, declared,
		))
	}

	return findings
}

func (m metadata) windowFindings() []string {
	if m.StartedAt.IsZero() || m.EndedAt.IsZero() {
		return nil
	}

	if !m.EndedAt.After(m.StartedAt) {
		return []string{fmt.Sprintf(
			"/endedAt: la corrida termina en %s, que no es posterior a su inicio en %s",
			m.EndedAt.Format(time.RFC3339), m.StartedAt.Format(time.RFC3339),
		)}
	}

	// La ventana entre inicio y fin no puede ser mas corta que la duracion que
	// la corrida dice haber medido.
	elapsed := m.EndedAt.Sub(m.StartedAt).Seconds()
	if elapsed+rateTolerance < m.DurationSeconds {
		return []string{fmt.Sprintf(
			"/durationSeconds: la corrida declara haber medido %g s dentro de una ventana de %g s",
			m.DurationSeconds, elapsed,
		)}
	}

	return nil
}

func approxEqual(a, b float64) bool {
	difference := math.Abs(a - b)
	if difference <= rateTolerance {
		return true
	}

	scale := math.Max(math.Abs(a), math.Abs(b))

	return difference <= scale*rateTolerance
}
