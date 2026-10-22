package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain/dataset"
)

// TestLoadBundleRejectsInconsistentManifests: la siembra solo acepta un
// manifiesto coherente consigo mismo, con CLI-3 y con las fuentes embebidas.
func TestLoadBundleRejectsInconsistentManifests(t *testing.T) {
	mutations := map[string]struct {
		mutate func(*dataset.Manifest)
		want   string
	}{
		"schema":            {func(m *dataset.Manifest) { m.Schema = "otro" }, "schema o schemaVersion"},
		"schemaVersion":     {func(m *dataset.Manifest) { m.SchemaVersion = "9.9.9" }, "schema o schemaVersion"},
		"generador":         {func(m *dataset.Manifest) { m.Generator.Name = "otro" }, "generador"},
		"unidades":          {func(m *dataset.Manifest) { m.Parameters.Units++ }, "parameters.units"},
		"archivo":           {func(m *dataset.Manifest) { m.Dataset.File = "otro.json" }, "nombres de archivo"},
		"hash corto":        {func(m *dataset.Manifest) { m.Dataset.SHA256 = "abc" }, "64 caracteres"},
		"hash no hex":       {func(m *dataset.Manifest) { m.Dataset.SHA256 = strings.Repeat("z", 64) }, "hexadecimal"},
		"hash mayusculas":   {func(m *dataset.Manifest) { m.Dataset.SHA256 = strings.ToUpper(m.Dataset.SHA256) }, "minusculas"},
		"conteos":           {func(m *dataset.Manifest) { m.Dataset.HappyPathUnits++ }, "inconsistentes"},
		"conteos rechazo":   {func(m *dataset.Manifest) { m.Dataset.DefaultDenyCases++; m.Dataset.ExplicitProhibitionCases-- }, "conteos de escenarios"},
		"maquina estados":   {func(m *dataset.Manifest) { m.Sources.StateMachineVersion = "9.9.9" }, "versiones fuente"},
		"organizaciones":    {func(m *dataset.Manifest) { m.Organizations = m.Organizations[1:] }, "lista de organizaciones"},
		"organizacion rara": {func(m *dataset.Manifest) { m.Organizations[0].ClientRole = "otro" }, "organizations[0]"},
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			directory := writeFixture(t, fixtureScenarios(t), mutation.mutate)
			_, err := loadBundle(directory, 5)
			if err == nil || !strings.Contains(err.Error(), mutation.want) {
				t.Fatalf("loadBundle() = %v, want %q", err, mutation.want)
			}
		})
	}
}

// TestLoadBundleRejectsTamperedFiles: faltantes, valores JSON extra y un
// sidecar SHA-256 que no corresponde detienen la siembra.
func TestLoadBundleRejectsTamperedFiles(t *testing.T) {
	cases := map[string]struct {
		tamper func(t *testing.T, directory string)
		want   string
	}{
		"sin manifiesto": {func(t *testing.T, directory string) {
			removeFile(t, filepath.Join(directory, dataset.ManifestFileName))
		}, "manifest.json"},
		"manifiesto con dos valores": {func(t *testing.T, directory string) {
			appendTo(t, filepath.Join(directory, dataset.ManifestFileName), "{}")
		}, "mas de un valor JSON"},
		"sin sidecar": {func(t *testing.T, directory string) {
			removeFile(t, filepath.Join(directory, dataset.HashFileName))
		}, "dataset.sha256"},
		"sidecar distinto": {func(t *testing.T, directory string) {
			writeFile(t, filepath.Join(directory, dataset.HashFileName), strings.Repeat("0", 64)+"  "+dataset.DatasetFileName+"\n")
		}, "dataset.sha256 no coincide"},
		"sin dataset": {func(t *testing.T, directory string) {
			removeFile(t, filepath.Join(directory, dataset.DatasetFileName))
		}, "dataset.json"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			directory := writeFixture(t, fixtureScenarios(t), nil)
			tc.tamper(t, directory)
			if _, err := loadBundle(directory, 5); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadBundle() = %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := LoadBundle(writeFixture(t, fixtureScenarios(t), nil)); err == nil || !strings.Contains(err.Error(), "al menos") {
		t.Fatalf("LoadBundle exige el minimo experimental: %v", err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, contents string) {
	t.Helper()
	// #nosec G304 -- path apunta al fixture del propio test en t.TempDir().
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(contents); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
