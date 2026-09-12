// Package seed valida y adapta el bundle deterministico de CLI-3 al snapshot
// relacional inicial de la baseline.
package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/dataset"
	foundational "github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/manifest"
)

const (
	datasetSchema  = "urn:pfi-snt:synthetic-dataset:schema:1.0.0"
	manifestSchema = "urn:pfi-snt:synthetic-dataset-manifest:schema:1.0.0"
	generatorName  = "cli-3-dataset-generator"
	initialState   = "EN_LABORATORIO"
)

// Bundle contiene solamente la parte que BASE-4 debe persistir. Las recetas de
// transferencia, rechazo y dispensa permanecen en el archivo para EVAL-3.
type Bundle struct {
	Manifest      dataset.Manifest
	Hash          string
	Organizations []core.Organization
	Registrations []core.SeedRegistration
}

// LoadBundle valida el bundle completo con el piso experimental de CLI-3.
func LoadBundle(directory string) (Bundle, error) {
	return loadBundle(directory, dataset.MinimumUnits)
}

func loadBundle(directory string, minimumUnits int) (Bundle, error) {
	manifestPath := filepath.Join(directory, dataset.ManifestFileName)
	var manifest dataset.Manifest
	if err := decodeStrictJSON(manifestPath, &manifest); err != nil {
		return Bundle{}, fmt.Errorf("leer manifest.json: %w", err)
	}

	organizations, workloadOrganizations, err := expectedOrganizations()
	if err != nil {
		return Bundle{}, err
	}
	if err := validateManifest(manifest, workloadOrganizations, minimumUnits); err != nil {
		return Bundle{}, err
	}

	sidecarPath := filepath.Join(directory, dataset.HashFileName)
	sidecar, err := os.ReadFile(sidecarPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("leer dataset.sha256: %w", err)
	}
	fields := strings.Fields(string(sidecar))
	if len(fields) != 2 || fields[0] != manifest.Dataset.SHA256 || fields[1] != dataset.DatasetFileName {
		return Bundle{}, errors.New("dataset.sha256 no coincide con manifest.json y dataset.json")
	}

	registrations, actualHash, counts, err := readDataset(
		filepath.Join(directory, dataset.DatasetFileName),
		workloadOrganizations,
	)
	if err != nil {
		return Bundle{}, err
	}
	if actualHash != manifest.Dataset.SHA256 {
		return Bundle{}, fmt.Errorf("hash SHA-256 de dataset.json invalido: esperado %s, obtenido %s", manifest.Dataset.SHA256, actualHash)
	}
	if len(registrations) != manifest.Parameters.Units || len(registrations) != manifest.Dataset.Units {
		return Bundle{}, fmt.Errorf(
			"cantidad de unidades inconsistente: dataset=%d parameters=%d manifest=%d",
			len(registrations), manifest.Parameters.Units, manifest.Dataset.Units,
		)
	}
	if counts.happy != manifest.Dataset.HappyPathUnits ||
		counts.rejected != manifest.Dataset.RejectionUnits ||
		counts.explicit != manifest.Dataset.ExplicitProhibitionCases ||
		counts.defaultDeny != manifest.Dataset.DefaultDenyCases {
		return Bundle{}, errors.New("los conteos de escenarios de dataset.json no coinciden con manifest.json")
	}

	return Bundle{
		Manifest: manifest, Hash: actualHash,
		Organizations: organizations, Registrations: registrations,
	}, nil
}

func decodeStrictJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("el archivo contiene mas de un valor JSON")
		}
		return err
	}
	return nil
}

func expectedOrganizations() ([]core.Organization, []dataset.Organization, error) {
	entries, err := foundational.Organizations()
	if err != nil {
		return nil, nil, fmt.Errorf("leer manifiesto fundacional embebido: %w", err)
	}
	organizations := make([]core.Organization, 0, len(entries))
	workload := make([]dataset.Organization, 0, len(entries))
	for _, entry := range entries {
		organizations = append(organizations, core.Organization{
			MSPID: entry.MSPID, ID: entry.ID, IDType: entry.IDType,
			AgentType: entry.AgentType, Active: entry.Active,
		})
		custodial, err := domain.IsCustodialAgentType(entry.AgentType)
		if err != nil {
			return nil, nil, fmt.Errorf("clasificar organizacion %s: %w", entry.MSPID, err)
		}
		if entry.Active && custodial {
			workload = append(workload, dataset.Organization{
				MSPID: entry.MSPID, CanonicalID: entry.IDType + ":" + entry.ID,
				AgentType: string(entry.AgentType), ClientRole: entry.ClientRole,
			})
		}
	}
	sort.Slice(workload, func(i, j int) bool { return workload[i].MSPID < workload[j].MSPID })
	return organizations, workload, nil
}

func validateManifest(manifest dataset.Manifest, expected []dataset.Organization, minimumUnits int) error {
	if manifest.Schema != manifestSchema || manifest.SchemaVersion != dataset.SchemaVersion {
		return errors.New("schema o schemaVersion de manifest.json incompatible")
	}
	if manifest.Generator.Name != generatorName || manifest.Generator.Version != dataset.GeneratorVersion {
		return errors.New("generador de manifest.json incompatible")
	}
	if manifest.Seed != dataset.FixedSeed {
		return fmt.Errorf("seed invalida: esperado %d, obtenido %d", dataset.FixedSeed, manifest.Seed)
	}
	if manifest.Parameters.Units < minimumUnits || manifest.Dataset.Units < minimumUnits {
		return fmt.Errorf("el bundle debe contener al menos %d unidades", minimumUnits)
	}
	if manifest.Parameters.Units != manifest.Dataset.Units {
		return errors.New("parameters.units no coincide con dataset.units")
	}
	if manifest.Dataset.File != dataset.DatasetFileName || manifest.Dataset.HashFile != dataset.HashFileName {
		return errors.New("los nombres de archivo del manifiesto no son los definidos por CLI-3")
	}
	if len(manifest.Dataset.SHA256) != sha256.Size*2 {
		return errors.New("dataset.sha256 del manifiesto no tiene 64 caracteres hexadecimales")
	}
	if _, err := hex.DecodeString(manifest.Dataset.SHA256); err != nil {
		return errors.New("dataset.sha256 del manifiesto no es hexadecimal minusculo valido")
	}
	if manifest.Dataset.SHA256 != strings.ToLower(manifest.Dataset.SHA256) {
		return errors.New("dataset.sha256 del manifiesto debe estar en minusculas")
	}
	if manifest.Dataset.HappyPathUnits+manifest.Dataset.RejectionUnits != manifest.Dataset.Units ||
		manifest.Dataset.ExplicitProhibitionCases+manifest.Dataset.DefaultDenyCases != manifest.Dataset.RejectionUnits {
		return errors.New("los conteos del manifiesto son inconsistentes")
	}

	matrixVersion, err := domain.MatrixSchemaVersion()
	if err != nil {
		return fmt.Errorf("leer schemaVersion de la matriz: %w", err)
	}
	rulesetID, err := domain.MatrixRulesetID()
	if err != nil {
		return fmt.Errorf("leer rulesetId de la matriz: %w", err)
	}
	organizationsVersion, err := foundational.SchemaVersion()
	if err != nil {
		return fmt.Errorf("leer schemaVersion del manifiesto fundacional: %w", err)
	}
	if manifest.Sources.TransferMatrixSchemaVersion != matrixVersion ||
		manifest.Sources.TransferRulesetID != rulesetID ||
		manifest.Sources.OrganizationsManifestSchemaVersion != organizationsVersion {
		return errors.New("las versiones fuente del bundle no coinciden con las embebidas en esta imagen")
	}

	if len(manifest.Organizations) != len(expected) {
		return errors.New("la lista de organizaciones del bundle no coincide con el manifiesto fundacional")
	}
	for index := range expected {
		if manifest.Organizations[index] != expected[index] {
			return fmt.Errorf("organizations[%d] no coincide con el manifiesto fundacional", index)
		}
	}
	return nil
}

type scenarioCounts struct {
	happy       int
	rejected    int
	explicit    int
	defaultDeny int
}

func readDataset(path string, organizations []dataset.Organization) ([]core.SeedRegistration, string, scenarioCounts, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", scenarioCounts{}, fmt.Errorf("abrir dataset.json: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	decoder := json.NewDecoder(io.TeeReader(file, hasher))
	decoder.DisallowUnknownFields()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, "", scenarioCounts{}, errors.New("dataset.json debe ser un objeto JSON")
	}

	byMSPID := make(map[string]dataset.Organization, len(organizations))
	for _, organization := range organizations {
		byMSPID[organization.MSPID] = organization
	}
	seenFields := make(map[string]bool, 3)
	var schema, schemaVersion string
	var registrations []core.SeedRegistration
	seenUnits := make(map[string]struct{})
	var counts scenarioCounts
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, "", scenarioCounts{}, fmt.Errorf("leer campo de dataset.json: %w", err)
		}
		field, ok := token.(string)
		if !ok || seenFields[field] {
			return nil, "", scenarioCounts{}, errors.New("dataset.json contiene un campo invalido o duplicado")
		}
		seenFields[field] = true
		switch field {
		case "$schema":
			if err := decoder.Decode(&schema); err != nil {
				return nil, "", scenarioCounts{}, fmt.Errorf("leer $schema de dataset.json: %w", err)
			}
		case "schemaVersion":
			if err := decoder.Decode(&schemaVersion); err != nil {
				return nil, "", scenarioCounts{}, fmt.Errorf("leer schemaVersion de dataset.json: %w", err)
			}
		case "units":
			startUnits, err := decoder.Token()
			if err != nil || startUnits != json.Delim('[') {
				return nil, "", scenarioCounts{}, errors.New("units debe ser un array")
			}
			for decoder.More() {
				var scenario dataset.UnitScenario
				if err := decoder.Decode(&scenario); err != nil {
					return nil, "", scenarioCounts{}, fmt.Errorf("leer units[%d]: %w", len(registrations), err)
				}
				registration, err := validateScenario(scenario, len(registrations)+1, byMSPID, &counts)
				if err != nil {
					return nil, "", scenarioCounts{}, err
				}
				key := registration.Request.GTIN + "\x00" + registration.Request.NumeroSerie
				if _, exists := seenUnits[key]; exists {
					return nil, "", scenarioCounts{}, fmt.Errorf(
						"unidad duplicada en dataset.json: %s/%s",
						registration.Request.GTIN,
						registration.Request.NumeroSerie,
					)
				}
				seenUnits[key] = struct{}{}
				registrations = append(registrations, registration)
			}
			if endUnits, err := decoder.Token(); err != nil || endUnits != json.Delim(']') {
				return nil, "", scenarioCounts{}, errors.New("array units sin cierre valido")
			}
		default:
			return nil, "", scenarioCounts{}, fmt.Errorf("campo no permitido en dataset.json: %s", field)
		}
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return nil, "", scenarioCounts{}, errors.New("dataset.json sin cierre valido")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, "", scenarioCounts{}, errors.New("dataset.json contiene datos despues del objeto")
	}
	if !seenFields["$schema"] || !seenFields["schemaVersion"] || !seenFields["units"] {
		return nil, "", scenarioCounts{}, errors.New("dataset.json no contiene todos los campos requeridos")
	}
	if schema != datasetSchema || schemaVersion != dataset.SchemaVersion {
		return nil, "", scenarioCounts{}, errors.New("schema o schemaVersion de dataset.json incompatible")
	}
	return registrations, hex.EncodeToString(hasher.Sum(nil)), counts, nil
}

func validateScenario(
	scenario dataset.UnitScenario,
	expectedSequence int,
	organizations map[string]dataset.Organization,
	counts *scenarioCounts,
) (core.SeedRegistration, error) {
	if scenario.Sequence != expectedSequence {
		return core.SeedRegistration{}, fmt.Errorf("units[%d].sequence debe ser %d", expectedSequence-1, expectedSequence)
	}
	if scenario.InitialState != initialState {
		return core.SeedRegistration{}, fmt.Errorf("units[%d].initialState debe ser %s", expectedSequence-1, initialState)
	}
	if scenario.Registration.Operation != "RegisterUnit" {
		return core.SeedRegistration{}, fmt.Errorf("units[%d] no comienza con RegisterUnit", expectedSequence-1)
	}
	organization, found := organizations[scenario.Registration.InvokerMSPID]
	if !found || organization.AgentType != string(domain.AgentLaboratory) {
		return core.SeedRegistration{}, fmt.Errorf("units[%d] no declara un laboratorio fundacional como invocador", expectedSequence-1)
	}
	if scenario.InitialCustodian != organization.CanonicalID {
		return core.SeedRegistration{}, fmt.Errorf("units[%d].initialCustodian no coincide con su laboratorio", expectedSequence-1)
	}
	request := core.RegisterUnitRequest{
		GTIN:             scenario.Registration.Request.GTIN,
		NumeroSerie:      scenario.Registration.Request.NumeroSerie,
		Lote:             scenario.Registration.Request.Lote,
		FechaVencimiento: scenario.Registration.Request.FechaVencimiento,
	}
	if err := core.ValidateRegisterUnitRequest(request); err != nil {
		return core.SeedRegistration{}, fmt.Errorf("units[%d].registration invalida: %w", expectedSequence-1, err)
	}

	switch {
	case scenario.Dispense != nil && scenario.ExpectedRejection == nil:
		counts.happy++
	case scenario.Dispense == nil && scenario.ExpectedRejection != nil:
		counts.rejected++
		switch scenario.ExpectedRejection.DecisionKind {
		case "EXPLICIT_PROHIBITION":
			counts.explicit++
		case "DEFAULT_DENY":
			counts.defaultDeny++
		default:
			return core.SeedRegistration{}, fmt.Errorf("units[%d] tiene un decisionKind de rechazo invalido", expectedSequence-1)
		}
	default:
		return core.SeedRegistration{}, fmt.Errorf("units[%d] debe declarar dispensa o rechazo, pero no ambos", expectedSequence-1)
	}

	return core.SeedRegistration{InvokerMSPID: scenario.Registration.InvokerMSPID, Request: request}, nil
}
