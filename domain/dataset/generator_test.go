package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestConfigRequiresExperimentalMinimumAndOutput(t *testing.T) {
	t.Parallel()

	if err := (Config{Units: MinimumUnits - 1, OutputDir: "out"}).validate(); err == nil {
		t.Fatalf("units menor que %d deberia rechazarse", MinimumUnits)
	}
	if err := (Config{Units: MinimumUnits, OutputDir: ""}).validate(); err == nil {
		t.Fatal("output-dir vacio deberia rechazarse")
	}
	if err := (Config{Units: MinimumUnits, OutputDir: "out"}).validate(); err != nil {
		t.Fatalf("configuracion minima valida: %v", err)
	}
}

func TestIdentifiersAreValidAndUniqueForMinimumVolume(t *testing.T) {
	t.Parallel()

	serialPattern := regexp.MustCompile("^[A-Z0-9-]+$")
	expirationPattern := regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
	seen := make(map[string]struct{}, MinimumUnits)
	for sequence := 1; sequence <= MinimumUnits; sequence++ {
		gtin, serial, lot, expiration := identifiersFor(sequence)
		if len(gtin) != 14 || !validGS1CheckDigit(gtin) {
			t.Fatalf("GTIN invalido en secuencia %d: %q", sequence, gtin)
		}
		if serial == "" || len(serial) > 20 || !serialPattern.MatchString(serial) {
			t.Fatalf("serie invalida en secuencia %d: %q", sequence, serial)
		}
		if len(serial) == 20 && strings.HasPrefix(serial, "779") {
			t.Fatalf("serie de 20 caracteres con prefijo prohibido: %q", serial)
		}
		if lot == "" {
			t.Fatalf("lote vacio en secuencia %d", sequence)
		}
		if !expirationPattern.MatchString(expiration) {
			t.Fatalf("vencimiento no ISO 8601 en secuencia %d: %q", sequence, expiration)
		}
		if _, err := time.Parse("2006-01-02", expiration); err != nil {
			t.Fatalf("vencimiento invalido en secuencia %d: %q: %v", sequence, expiration, err)
		}
		key := gtin + "|" + serial
		if _, exists := seen[key]; exists {
			t.Fatalf("GTIN+serie duplicado en secuencia %d", sequence)
		}
		seen[key] = struct{}{}
	}
}

func TestPlanDerivesEveryExpectedRejectionFamily(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	if plan.explicitDenials == 0 || plan.defaultDenials == 0 {
		t.Fatalf("cobertura de transferencias incompleta: explicit=%d default=%d", plan.explicitDenials, plan.defaultDenials)
	}
	if plan.duplicateCaseCount < 1 || len(plan.blockingCases) < 1 {
		t.Fatalf("cobertura de categorias incompleta: duplicate=%d blocking=%d", plan.duplicateCaseCount, len(plan.blockingCases))
	}

	want := map[string]int{
		RejectionUnauthorizedTransfer: len(plan.deniedCases),
		RejectionDuplicateIdentity:    plan.duplicateCaseCount,
		RejectionBlockingState:        len(plan.blockingCases),
	}
	got := make(map[string]int, len(want))
	total := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	for index := 0; index < total; index++ {
		scenario, err := buildUnitScenario(index, plan)
		if err != nil {
			t.Fatalf("construir rechazo %d: %v", index, err)
		}
		if scenario.ExpectedSuccess != nil || scenario.ExpectedRejection == nil {
			t.Fatalf("rechazo %d no declara exclusivamente expectedRejection", index)
		}
		got[scenario.ExpectedRejection.Category]++
	}
	for category, count := range want {
		if got[category] != count {
			t.Fatalf("categoria %s: got=%d want=%d", category, got[category], count)
		}
	}
}

func TestManifestReportsExactCountsPerCategory(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	manifest := buildManifest(MinimumUnits, strings.Repeat("a", sha256.Size*2), plan)
	rejections := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	if manifest.Dataset.Units != MinimumUnits ||
		manifest.Dataset.HappyPathUnits != MinimumUnits-rejections ||
		manifest.Dataset.RejectionUnits != rejections ||
		manifest.Dataset.UnauthorizedTransferCases != len(plan.deniedCases) ||
		manifest.Dataset.DuplicateIdentityCases != plan.duplicateCaseCount ||
		manifest.Dataset.BlockingStateCases != len(plan.blockingCases) ||
		manifest.Dataset.ExplicitProhibitionCases != plan.explicitDenials ||
		manifest.Dataset.DefaultDenyCases != plan.defaultDenials {
		t.Fatalf("conteos del manifiesto inconsistentes: %+v", manifest.Dataset)
	}
	if manifest.Seed != FixedSeed || manifest.Sources.StateMachineVersion != domain.StateMachineVersion {
		t.Fatalf("fuentes de reproducibilidad incompletas: %+v", manifest)
	}
}

func TestTransferRejectionsComeOnlyFromSharedMatrix(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	byMSP, byCanonical := organizationIndexes(plan.participants)
	for index, rejected := range plan.deniedCases {
		decision, err := domain.DecideTransfer(rejected.Origin.AgentType, rejected.Destination.AgentType)
		if err != nil {
			t.Fatalf("caso %d: %v", index, err)
		}
		if decision.Allowed || decision.RuleID != rejected.Decision.RuleID || decision.Reason != rejected.Decision.Reason {
			t.Fatalf("caso %d diverge de domain.DecideTransfer", index)
		}

		scenario, err := buildUnitScenario(index, plan)
		if err != nil {
			t.Fatalf("construir caso %d: %v", index, err)
		}
		rejection := scenario.ExpectedRejection
		if rejection.Category != RejectionUnauthorizedTransfer || rejection.ExpectedErrorCode != expectedTransferError {
			t.Fatalf("caso %d no es el rechazo de transferencia esperado: %+v", index, rejection)
		}
		origin := byMSP[rejection.InvokerMSPID]
		destination := byCanonical[rejection.PrivateData.Destinatario.Destino]
		actual, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
		if err != nil {
			t.Fatalf("reevaluar caso %d: %v", index, err)
		}
		if actual.Allowed || actual.RuleID != rejection.TransferDecision.RuleID ||
			actual.Reason != rejection.TransferDecision.Reason ||
			actual.SchemaVersion != rejection.TransferDecision.MatrixSchemaVersion {
			t.Fatalf("caso %d no conserva la decision compartida", index)
		}
	}
}

func TestBlockingCasesAreDerivedFromSharedStateMachine(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	blockingStates := make(map[domain.State]struct{})
	for _, state := range domain.States() {
		if domain.IsBlockingState(state) {
			blockingStates[state] = struct{}{}
		}
	}
	if len(plan.blockingCases) != len(blockingStates)*2 {
		t.Fatalf("casos bloqueantes=%d, estados=%d x operaciones=2", len(plan.blockingCases), len(blockingStates))
	}

	seen := make(map[string]bool, len(plan.blockingCases))
	for _, blocked := range plan.blockingCases {
		if _, ok := blockingStates[blocked.State]; !ok {
			t.Fatalf("estado no bloqueante incluido: %s", blocked.State)
		}
		if blocked.PreparationTransition.To != blocked.State ||
			!transitionContainsState(blocked.PreparationTransition, domain.StateEnCustodia) {
			t.Fatalf("preparacion invalida para %s: %+v", blocked.State, blocked.PreparationTransition)
		}
		if _, declared := domain.LookupTransition(blocked.State, blocked.RejectedOperation.Event); declared {
			t.Fatalf("%s admite %s y no debe ser caso de rechazo", blocked.State, blocked.RejectedOperation.Event)
		}
		seen[string(blocked.State)+"|"+blocked.RejectedOperation.Name] = true
	}
	for state := range blockingStates {
		for _, operation := range []string{"DispatchTransfer", "Dispense"} {
			if !seen[string(state)+"|"+operation] {
				t.Fatalf("falta %s desde %s", operation, state)
			}
		}
	}
}

func TestGeneratedScenariosUseValidReferencesAndMinimalPreparation(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	byMSP, byCanonical := organizationIndexes(plan.participants)
	rejections := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	for index := 0; index < rejections+256; index++ {
		scenario, err := buildUnitScenario(index, plan)
		if err != nil {
			t.Fatalf("unidad %d: %v", index, err)
		}
		if len(scenario.Preparation) == 0 || scenario.Preparation[0].Operation != "RegisterUnit" {
			t.Fatalf("unidad %d no comienza su preparacion con RegisterUnit", index)
		}
		registered := scenario.Preparation[0].Request
		for stepIndex, step := range scenario.Preparation {
			if _, ok := byMSP[step.InvokerMSPID]; !ok {
				t.Fatalf("unidad %d paso %d referencia MSP inexistente: %s", index, stepIndex, step.InvokerMSPID)
			}
			if step.Request.GTIN != registered.GTIN || step.Request.NumeroSerie != registered.NumeroSerie {
				t.Fatalf("unidad %d paso %d usa otra identidad", index, stepIndex)
			}
			if step.Operation == "DispatchTransfer" {
				if step.PrivateData == nil {
					t.Fatalf("unidad %d paso %d omite privateData", index, stepIndex)
				}
				destination, ok := byCanonical[step.PrivateData.Destinatario.Destino]
				if !ok {
					t.Fatalf("unidad %d paso %d referencia destino inexistente", index, stepIndex)
				}
				origin := byMSP[step.InvokerMSPID]
				decision, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
				if err != nil || !decision.Allowed || decision.RuleID != step.RuleID ||
					decision.SchemaVersion != step.MatrixSchemaVersion {
					t.Fatalf("unidad %d paso %d no deriva de un par autorizado", index, stepIndex)
				}
			}
		}

		switch {
		case scenario.ExpectedSuccess != nil && scenario.ExpectedRejection == nil:
			if scenario.ExpectedSuccess.Operation != "Dispense" {
				t.Fatalf("unidad %d tiene exito no soportado: %s", index, scenario.ExpectedSuccess.Operation)
			}
		case scenario.ExpectedSuccess == nil && scenario.ExpectedRejection != nil:
			validateRejectionReferences(t, index, scenario, byMSP, byCanonical)
		default:
			t.Fatalf("unidad %d debe declarar un unico resultado", index)
		}
	}
}

func validateRejectionReferences(
	t *testing.T,
	index int,
	scenario UnitScenario,
	byMSP map[string]organization,
	byCanonical map[string]organization,
) {
	t.Helper()
	rejection := scenario.ExpectedRejection
	if _, ok := byMSP[rejection.InvokerMSPID]; !ok {
		t.Fatalf("unidad %d rechazo referencia MSP inexistente", index)
	}
	registered := scenario.Preparation[0].Request
	if rejection.Request.GTIN != registered.GTIN || rejection.Request.NumeroSerie != registered.NumeroSerie {
		t.Fatalf("unidad %d rechazo usa otra identidad", index)
	}

	switch rejection.Category {
	case RejectionUnauthorizedTransfer:
		if rejection.Operation != "DispatchTransfer" || rejection.PrivateData == nil || rejection.TransferDecision == nil {
			t.Fatalf("unidad %d tiene rechazo de transferencia incompleto", index)
		}
		if _, ok := byCanonical[rejection.PrivateData.Destinatario.Destino]; !ok {
			t.Fatalf("unidad %d tiene destino rechazado inexistente", index)
		}
	case RejectionDuplicateIdentity:
		if len(scenario.Preparation) != 1 || rejection.Operation != "RegisterUnit" ||
			rejection.Request != registered || rejection.ExpectedErrorCode != expectedDuplicateError {
			t.Fatalf("unidad %d no tiene la preparacion minima del duplicado", index)
		}
	case RejectionBlockingState:
		state := domain.State(rejection.BlockingState)
		if !domain.IsBlockingState(state) || rejection.StateMachineVersion != domain.StateMachineVersion {
			t.Fatalf("unidad %d referencia estado bloqueante invalido", index)
		}
		event, ok := ordinaryEvent(rejection.Operation)
		if !ok {
			t.Fatalf("unidad %d usa operacion ordinaria desconocida", index)
		}
		if _, declared := domain.LookupTransition(state, event); declared {
			t.Fatalf("unidad %d combina un estado y una operacion compatibles", index)
		}
		last := scenario.Preparation[len(scenario.Preparation)-1]
		transition, found := transitionByID(last.TransitionID)
		if !found || transition.To != state || last.StateMachineVersion != domain.StateMachineVersion {
			t.Fatalf("unidad %d no prepara el estado bloqueante declarado", index)
		}
	default:
		t.Fatalf("unidad %d tiene categoria desconocida: %s", index, rejection.Category)
	}
}

func TestSchemasValidateGeneratedRecordsAndRejectMalformedCombinations(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	unitSchema := compileSchema(t, "dataset.schema.json", "#/$defs/unitScenario")
	total := len(plan.deniedCases) + plan.duplicateCaseCount + len(plan.blockingCases)
	representatives := map[string]UnitScenario{}
	for index := 0; index < total; index++ {
		scenario, err := buildUnitScenario(index, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := unitSchema.Validate(asJSONValue(t, scenario)); err != nil {
			t.Fatalf("unidad %d no cumple el schema: %v", index, err)
		}
		representatives[scenario.ExpectedRejection.Category] = scenario
	}
	happy, err := buildUnitScenario(total, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := unitSchema.Validate(asJSONValue(t, happy)); err != nil {
		t.Fatalf("camino feliz no cumple el schema: %v", err)
	}

	tests := []struct {
		name     string
		category string
		mutate   func(map[string]any)
	}{
		{
			name: "transfer category with register operation", category: RejectionUnauthorizedTransfer,
			mutate: func(rejection map[string]any) { rejection["operation"] = "RegisterUnit" },
		},
		{
			name: "duplicate with transfer private data", category: RejectionDuplicateIdentity,
			mutate: func(rejection map[string]any) {
				rejection["privateData"] = map[string]any{}
			},
		},
		{
			name: "blocking state with register operation", category: RejectionBlockingState,
			mutate: func(rejection map[string]any) { rejection["operation"] = "RegisterUnit" },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := asJSONValue(t, representatives[test.category]).(map[string]any)
			rejection := value["expectedRejection"].(map[string]any)
			test.mutate(rejection)
			if err := unitSchema.Validate(value); err == nil {
				t.Fatal("el schema acepto una combinacion category/operation mal formada")
			}
		})
	}

	manifestSchema := compileSchema(t, "manifest.schema.json", "")
	manifest := buildManifest(MinimumUnits, strings.Repeat("a", sha256.Size*2), plan)
	if err := manifestSchema.Validate(asJSONValue(t, manifest)); err != nil {
		t.Fatalf("manifiesto no cumple el schema: %v", err)
	}
}

func TestBundleIsDeterministicAndHashMatches(t *testing.T) {
	t.Parallel()

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first, err := generateBundle(firstDir, 64)
	if err != nil {
		t.Fatalf("primera generacion: %v", err)
	}
	second, err := generateBundle(secondDir, 64)
	if err != nil {
		t.Fatalf("segunda generacion: %v", err)
	}

	for _, name := range []string{DatasetFileName, ManifestFileName, HashFileName} {
		// #nosec G304 -- firstDir is an isolated directory created by t.TempDir.
		left, err := os.ReadFile(filepath.Join(firstDir, name))
		if err != nil {
			t.Fatalf("leer primer %s: %v", name, err)
		}
		// #nosec G304 -- secondDir is an isolated directory created by t.TempDir.
		right, err := os.ReadFile(filepath.Join(secondDir, name))
		if err != nil {
			t.Fatalf("leer segundo %s: %v", name, err)
		}
		if string(left) != string(right) {
			t.Fatalf("%s no es deterministico", name)
		}
	}

	// #nosec G304 -- DatasetPath was generated beneath the preceding t.TempDir.
	raw, err := os.ReadFile(first.DatasetPath)
	if err != nil {
		t.Fatalf("leer dataset: %v", err)
	}
	sum := sha256.Sum256(raw)
	actual := hex.EncodeToString(sum[:])
	if actual != first.Manifest.Dataset.SHA256 || actual != second.Manifest.Dataset.SHA256 {
		t.Fatalf("hash calculado=%s manifiesto1=%s manifiesto2=%s", actual, first.Manifest.Dataset.SHA256, second.Manifest.Dataset.SHA256)
	}
	// #nosec G304 -- HashPath was generated beneath the preceding t.TempDir.
	hashFile, err := os.ReadFile(first.HashPath)
	if err != nil {
		t.Fatalf("leer sidecar: %v", err)
	}
	expectedHashFile := fmt.Sprintf("%s  %s%c", actual, DatasetFileName, byte(10))
	if string(hashFile) != expectedHashFile {
		t.Fatalf("sidecar inesperado: %q", hashFile)
	}
	if first.Manifest.Seed != FixedSeed || first.Manifest.Generator.Version != GeneratorVersion ||
		first.Manifest.Parameters.Units != 64 {
		t.Fatalf("metadata de reproduccion incompleta: %+v", first.Manifest)
	}
	if first.Manifest.Dataset.UnauthorizedTransferCases == 0 ||
		first.Manifest.Dataset.DuplicateIdentityCases == 0 ||
		first.Manifest.Dataset.BlockingStateCases == 0 {
		t.Fatalf("conteos por categoria incompletos: %+v", first.Manifest.Dataset)
	}
}

func TestBundleIsDeterministicAcrossIdentifierRotations(t *testing.T) {
	if testing.Short() {
		t.Skip("omite la generacion ampliada en modo short")
	}
	t.Parallel()

	first, err := generateBundle(t.TempDir(), 5000)
	if err != nil {
		t.Fatalf("primera generacion ampliada: %v", err)
	}
	second, err := generateBundle(t.TempDir(), 5000)
	if err != nil {
		t.Fatalf("segunda generacion ampliada: %v", err)
	}
	if first.Manifest.Dataset.SHA256 != second.Manifest.Dataset.SHA256 {
		t.Fatalf(
			"hash no deterministico tras rotaciones de GTIN y lote: first=%s second=%s",
			first.Manifest.Dataset.SHA256,
			second.Manifest.Dataset.SHA256,
		)
	}

	firstGTIN, _, firstLot, _ := identifiersFor(1)
	rotatedGTIN, _, _, _ := identifiersFor(101)
	_, _, rotatedLot, _ := identifiersFor(1001)
	if firstGTIN == rotatedGTIN {
		t.Fatal("la prueba ampliada no alcanzo la primera rotacion de GTIN")
	}
	if firstLot == rotatedLot {
		t.Fatal("la prueba ampliada no alcanzo la primera rotacion de lote")
	}
}

func TestSchemasUseDraft202012AndMatchingVersion(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"dataset.schema.json", "manifest.schema.json"} {
		// #nosec G304 -- name comes from the fixed schema filename list above.
		raw, err := os.ReadFile(filepath.Join("schema", name))
		if err != nil {
			t.Fatalf("leer %s: %v", name, err)
		}
		var schema struct {
			Dialect string `json:"$schema"`
			ID      string `json:"$id"`
			Props   struct {
				SchemaVersion struct {
					Const string `json:"const"`
				} `json:"schemaVersion"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("parsear %s: %v", name, err)
		}
		if schema.Dialect != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s no declara Draft 2020-12", name)
		}
		if schema.Props.SchemaVersion.Const != SchemaVersion || !strings.HasSuffix(schema.ID, ":"+SchemaVersion) {
			t.Fatalf("version incoherente en %s: id=%q const=%q", name, schema.ID, schema.Props.SchemaVersion.Const)
		}
	}
}

func mustPlan(t *testing.T) generationPlan {
	t.Helper()
	plan, err := newGenerationPlan()
	if err != nil {
		t.Fatalf("crear plan: %v", err)
	}
	return plan
}

func organizationIndexes(all []organization) (map[string]organization, map[string]organization) {
	byMSP := make(map[string]organization, len(all))
	byCanonical := make(map[string]organization, len(all))
	for _, org := range all {
		byMSP[org.MSPID] = org
		byCanonical[org.canonicalID()] = org
	}
	return byMSP, byCanonical
}

func transitionContainsState(transition domain.Transition, state domain.State) bool {
	for _, from := range transition.From {
		if from == state {
			return true
		}
	}
	return false
}

func transitionByID(id string) (domain.Transition, bool) {
	for _, transition := range domain.Transitions() {
		if transition.ID == id {
			return transition, true
		}
	}
	return domain.Transition{}, false
}

func ordinaryEvent(operation string) (domain.Event, bool) {
	switch operation {
	case "DispatchTransfer":
		return domain.EventDistribuirEslabonPosterior, true
	case "Dispense":
		return domain.EventDispensarPaciente, true
	default:
		return "", false
	}
}

func compileSchema(t *testing.T, name, fragment string) *jsonschema.Schema {
	t.Helper()
	// #nosec G304 -- name comes from fixed test literals.
	raw, err := os.ReadFile(filepath.Join("schema", name))
	if err != nil {
		t.Fatalf("leer %s: %v", name, err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parsear %s: %v", name, err)
	}
	resource := "https://pfi.invalid/" + name
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(resource, document); err != nil {
		t.Fatalf("agregar %s al compilador: %v", name, err)
	}
	schema, err := compiler.Compile(resource + fragment)
	if err != nil {
		t.Fatalf("compilar %s%s: %v", name, fragment, err)
	}
	return schema
}

func asJSONValue(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func validGS1CheckDigit(value string) bool {
	if len(value) < 2 {
		return false
	}
	data := value[:len(value)-1]
	check := value[len(value)-1]
	return check >= '0' && check <= '9' && byte(check-'0') == gs1CheckDigit(data)
}
