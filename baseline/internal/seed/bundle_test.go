package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/dataset"
	foundational "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest"
)

type datasetDocument struct {
	Schema        string                 `json:"$schema"`
	SchemaVersion string                 `json:"schemaVersion"`
	Units         []dataset.UnitScenario `json:"units"`
}

func fixtureScenarios(t *testing.T) []dataset.UnitScenario {
	t.Helper()
	_, organizations, err := expectedOrganizations()
	if err != nil {
		t.Fatal(err)
	}
	var laboratory dataset.Organization
	for _, organization := range organizations {
		if organization.AgentType == string(domain.AgentLaboratory) {
			laboratory = organization
		}
	}
	if laboratory.MSPID == "" {
		t.Fatal("fixture has no laboratory")
	}

	scenario := func(sequence int, serial string) dataset.UnitScenario {
		ref := dataset.UnitRef{GTIN: "07791234567898", NumeroSerie: serial}
		return dataset.UnitScenario{
			Sequence: sequence, InitialState: initialState, InitialCustodian: laboratory.CanonicalID,
			Registration: dataset.Registration{
				Operation: "RegisterUnit", InvokerMSPID: laboratory.MSPID,
				Request: dataset.RegisterUnitRequest{
					GTIN: ref.GTIN, NumeroSerie: ref.NumeroSerie,
					Lote: "LOTE-SEED", FechaVencimiento: "2099-12-31",
				},
			},
			ValidTransfers: []dataset.ValidTransfer{},
			Dispense: &dataset.Dispense{
				Operation: "Dispense", InvokerMSPID: "FarmaciaMSP", Request: ref,
			},
		}
	}
	scenarios := []dataset.UnitScenario{
		scenario(1, "SEED-0001"),
		scenario(2, "SEED-0002"),
		scenario(3, "SEED-0003"),
	}
	scenarios[1].Dispense = nil
	scenarios[1].ExpectedRejection = &dataset.ExpectedRejection{DecisionKind: "EXPLICIT_PROHIBITION"}
	scenarios[2].Dispense = nil
	scenarios[2].ExpectedRejection = &dataset.ExpectedRejection{DecisionKind: "DEFAULT_DENY"}
	return scenarios
}

func writeFixture(
	t *testing.T,
	scenarios []dataset.UnitScenario,
	mutateManifest func(*dataset.Manifest),
) string {
	t.Helper()
	directory := t.TempDir()
	document := datasetDocument{
		Schema: datasetSchema, SchemaVersion: dataset.SchemaVersion, Units: scenarios,
	}
	encodedDataset, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encodedDataset = append(encodedDataset, '\n')
	digest := sha256.Sum256(encodedDataset)
	hash := hex.EncodeToString(digest[:])

	matrixVersion, err := domain.MatrixSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	rulesetID, err := domain.MatrixRulesetID()
	if err != nil {
		t.Fatal(err)
	}
	organizationsVersion, err := foundational.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	_, organizations, err := expectedOrganizations()
	if err != nil {
		t.Fatal(err)
	}
	happy, rejected, explicit, defaultDeny := 0, 0, 0, 0
	for _, scenario := range scenarios {
		if scenario.Dispense != nil {
			happy++
		} else {
			rejected++
			if scenario.ExpectedRejection.DecisionKind == "EXPLICIT_PROHIBITION" {
				explicit++
			} else {
				defaultDeny++
			}
		}
	}
	manifest := dataset.Manifest{
		Schema: manifestSchema, SchemaVersion: dataset.SchemaVersion,
		Generator: dataset.GeneratorMetadata{Name: generatorName, Version: dataset.GeneratorVersion},
		Seed:      dataset.FixedSeed, Parameters: dataset.Parameters{Units: len(scenarios)},
		Sources: dataset.SourceMetadata{
			TransferRulesetID: rulesetID, TransferMatrixSchemaVersion: matrixVersion,
			OrganizationsManifestSchemaVersion: organizationsVersion,
		},
		Dataset: dataset.Metadata{
			File: dataset.DatasetFileName, HashFile: dataset.HashFileName,
			SHA256: hash, Units: len(scenarios), HappyPathUnits: happy,
			RejectionUnits: rejected, ExplicitProhibitionCases: explicit,
			DefaultDenyCases: defaultDeny,
		},
		Organizations: organizations,
	}
	if mutateManifest != nil {
		mutateManifest(&manifest)
	}
	encodedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encodedManifest = append(encodedManifest, '\n')

	if err := os.WriteFile(filepath.Join(directory, dataset.DatasetFileName), encodedDataset, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, dataset.ManifestFileName), encodedManifest, 0o600); err != nil {
		t.Fatal(err)
	}
	sidecar := fmt.Sprintf("%s  %s\n", manifest.Dataset.SHA256, dataset.DatasetFileName)
	if err := os.WriteFile(filepath.Join(directory, dataset.HashFileName), []byte(sidecar), 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestLoadBundleAcceptsValidCLI3Bundle(t *testing.T) {
	directory := writeFixture(t, fixtureScenarios(t), nil)
	bundle, err := loadBundle(directory, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Organizations) != 7 || len(bundle.Registrations) != 3 {
		t.Fatalf("unexpected bundle sizes: organizations=%d registrations=%d", len(bundle.Organizations), len(bundle.Registrations))
	}
}

func TestLoadBundleRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*testing.T) string
		minimum    int
		wantSubstr string
	}{
		{
			name: "wrong seed",
			prepare: func(t *testing.T) string {
				return writeFixture(t, fixtureScenarios(t), func(manifest *dataset.Manifest) { manifest.Seed++ })
			},
			minimum: 3, wantSubstr: "seed invalida",
		},
		{
			name: "wrong generator version",
			prepare: func(t *testing.T) string {
				return writeFixture(t, fixtureScenarios(t), func(manifest *dataset.Manifest) {
					manifest.Generator.Version = "9.9.9"
				})
			},
			minimum: 3, wantSubstr: "generador",
		},
		{
			name: "wrong source version",
			prepare: func(t *testing.T) string {
				return writeFixture(t, fixtureScenarios(t), func(manifest *dataset.Manifest) {
					manifest.Sources.TransferMatrixSchemaVersion = "9.9.9"
				})
			},
			minimum: 3, wantSubstr: "versiones fuente",
		},
		{
			name: "below minimum",
			prepare: func(t *testing.T) string {
				return writeFixture(t, fixtureScenarios(t), nil)
			},
			minimum: 4, wantSubstr: "al menos 4",
		},
		{
			name: "out of order",
			prepare: func(t *testing.T) string {
				scenarios := fixtureScenarios(t)
				scenarios[1].Sequence = 9
				return writeFixture(t, scenarios, nil)
			},
			minimum: 3, wantSubstr: "sequence debe ser 2",
		},
		{
			name: "duplicate unit",
			prepare: func(t *testing.T) string {
				scenarios := fixtureScenarios(t)
				scenarios[1].Registration.Request = scenarios[0].Registration.Request
				return writeFixture(t, scenarios, nil)
			},
			minimum: 3, wantSubstr: "unidad duplicada",
		},
		{
			name: "changed dataset hash",
			prepare: func(t *testing.T) string {
				directory := writeFixture(t, fixtureScenarios(t), nil)
				path := filepath.Join(directory, dataset.DatasetFileName)
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString(" "); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				return directory
			},
			minimum: 3, wantSubstr: "hash SHA-256",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadBundle(test.prepare(t), test.minimum)
			if err == nil || !strings.Contains(err.Error(), test.wantSubstr) {
				t.Fatalf("expected error containing %q, got %v", test.wantSubstr, err)
			}
		})
	}
}
