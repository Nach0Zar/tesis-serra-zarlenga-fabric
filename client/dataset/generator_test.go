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

func TestPlanDerivesBothKindsOfMatrixRejection(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	if plan.explicitDenials == 0 || plan.defaultDenials == 0 {
		t.Fatalf("cobertura de rechazos incompleta: explicit=%d default=%d", plan.explicitDenials, plan.defaultDenials)
	}
	if len(plan.deniedCases) != plan.explicitDenials+plan.defaultDenials {
		t.Fatalf("casos=%d, explicit+default=%d", len(plan.deniedCases), plan.explicitDenials+plan.defaultDenials)
	}

	for index, rejected := range plan.deniedCases {
		decision, err := domain.DecideTransfer(rejected.Origin.AgentType, rejected.Destination.AgentType)
		if err != nil {
			t.Fatalf("caso %d: %v", index, err)
		}
		if decision.Allowed {
			t.Fatalf("caso %d no es un rechazo de la matriz", index)
		}
		if decision.RuleID != rejected.Decision.RuleID || decision.Reason != rejected.Decision.Reason {
			t.Fatalf("caso %d diverge de domain.DecideTransfer", index)
		}
		if len(rejected.SetupPath) == 0 || rejected.SetupPath[len(rejected.SetupPath)-1].MSPID != rejected.Origin.MSPID {
			t.Fatalf("caso %d no prepara la custodia del origen rechazado", index)
		}

		unit, err := buildUnitScenario(index, plan)
		if err != nil {
			t.Fatalf("construir caso %d: %v", index, err)
		}
		if unit.ExpectedRejection == nil || unit.Dispense != nil {
			t.Fatalf("caso %d no termina exclusivamente en rechazo", index)
		}
		if unit.ExpectedRejection.ExpectedErrorCode != expectedTransferError {
			t.Fatalf("caso %d espera otro error: %s", index, unit.ExpectedRejection.ExpectedErrorCode)
		}
	}
}

func TestEveryTransferIsAnAuthorizedDispatchReceivePair(t *testing.T) {
	t.Parallel()

	plan := mustPlan(t)
	byMSP := make(map[string]organization, len(plan.organizations))
	for _, org := range plan.organizations {
		byMSP[org.MSPID] = org
	}

	for index := 0; index < 256; index++ {
		unit, err := buildUnitScenario(index+len(plan.deniedCases), plan)
		if err != nil {
			t.Fatalf("unidad %d: %v", index, err)
		}
		if unit.Dispense == nil || unit.ExpectedRejection != nil {
			t.Fatalf("unidad feliz %d no termina exclusivamente en Dispense", index)
		}
		if len(unit.ValidTransfers) == 0 {
			t.Fatalf("unidad feliz %d no contiene transferencias", index)
		}
		for transferIndex, transfer := range unit.ValidTransfers {
			if transfer.Dispatch.Operation != "DispatchTransfer" || transfer.Receive.Operation != "ReceiveTransfer" {
				t.Fatalf("unidad %d transferencia %d no es despacho+recepcion", index, transferIndex)
			}
			if transfer.Dispatch.Request != transfer.Receive.Request {
				t.Fatalf("unidad %d transferencia %d usa referencias distintas", index, transferIndex)
			}
			origin, originOK := byMSP[transfer.Dispatch.InvokerMSPID]
			destination, destinationOK := byMSP[transfer.Receive.InvokerMSPID]
			if !originOK || !destinationOK {
				t.Fatalf("unidad %d transferencia %d referencia organizacion inexistente", index, transferIndex)
			}
			decision, err := domain.DecideTransfer(origin.AgentType, destination.AgentType)
			if err != nil {
				t.Fatalf("unidad %d transferencia %d: %v", index, transferIndex, err)
			}
			if !decision.Allowed || decision.RuleID != transfer.RuleID || decision.SchemaVersion != transfer.MatrixSchemaVersion {
				t.Fatalf("unidad %d transferencia %d diverge de la matriz", index, transferIndex)
			}
			if transfer.Dispatch.PrivateData.Destinatario.Destino != destination.canonicalID() {
				t.Fatalf("unidad %d transferencia %d declara otro destinatario", index, transferIndex)
			}
		}
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
		left, err := os.ReadFile(filepath.Join(firstDir, name))
		if err != nil {
			t.Fatalf("leer primer %s: %v", name, err)
		}
		right, err := os.ReadFile(filepath.Join(secondDir, name))
		if err != nil {
			t.Fatalf("leer segundo %s: %v", name, err)
		}
		if string(left) != string(right) {
			t.Fatalf("%s no es deterministico", name)
		}
	}

	raw, err := os.ReadFile(first.DatasetPath)
	if err != nil {
		t.Fatalf("leer dataset: %v", err)
	}
	sum := sha256.Sum256(raw)
	actual := hex.EncodeToString(sum[:])
	if actual != first.Manifest.Dataset.SHA256 || actual != second.Manifest.Dataset.SHA256 {
		t.Fatalf("hash calculado=%s manifiesto1=%s manifiesto2=%s", actual, first.Manifest.Dataset.SHA256, second.Manifest.Dataset.SHA256)
	}
	hashFile, err := os.ReadFile(first.HashPath)
	if err != nil {
		t.Fatalf("leer sidecar: %v", err)
	}
	expectedHashFile := fmt.Sprintf("%s  %s%c", actual, DatasetFileName, byte(10))
	if string(hashFile) != expectedHashFile {
		t.Fatalf("sidecar inesperado: %q", hashFile)
	}
	if first.Manifest.Seed != FixedSeed || first.Manifest.Generator.Version != GeneratorVersion || first.Manifest.Parameters.Units != 64 {
		t.Fatalf("metadata de reproduccion incompleta: %+v", first.Manifest)
	}
}

func TestSchemasUseDraft202012AndMatchingVersion(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"dataset.schema.json", "manifest.schema.json"} {
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

func validGS1CheckDigit(value string) bool {
	if len(value) < 2 {
		return false
	}
	data := value[:len(value)-1]
	check := value[len(value)-1]
	return check >= '0' && check <= '9' && byte(check-'0') == gs1CheckDigit(data)
}
