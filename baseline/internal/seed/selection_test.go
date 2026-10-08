package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
)

func writeSelection(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSelectionAndApply(t *testing.T) {
	hash := "de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d"
	path := writeSelection(t, `{
  "schemaVersion": "1.0.0",
  "datasetSHA256": "`+hash+`",
  "excludedSequences": [2, 4]
}`)
	selection, err := LoadSelection(path, hash, 4)
	if err != nil {
		t.Fatal(err)
	}
	registrations := []core.SeedRegistration{{InvokerMSPID: "one"}, {InvokerMSPID: "two"}, {InvokerMSPID: "three"}, {InvokerMSPID: "four"}}
	filtered := ApplySelection(registrations, selection)
	if len(filtered) != 2 || filtered[0].InvokerMSPID != "one" || filtered[1].InvokerMSPID != "three" {
		t.Fatalf("unexpected filtered registrations: %#v", filtered)
	}
	if len(registrations) != 4 {
		t.Fatal("ApplySelection modified its input")
	}
}

func TestLoadSelectionRejectsMismatchDuplicatesAndRange(t *testing.T) {
	hash := "de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d"
	tests := []string{
		`{"schemaVersion":"1.0.0","datasetSHA256":"wrong","excludedSequences":[]}`,
		`{"schemaVersion":"1.0.0","datasetSHA256":"` + hash + `","excludedSequences":[2,2]}`,
		`{"schemaVersion":"1.0.0","datasetSHA256":"` + hash + `","excludedSequences":[0]}`,
		`{"schemaVersion":"1.0.0","datasetSHA256":"` + hash + `","excludedSequences":[5]}`,
		`{"schemaVersion":"1.0.0","datasetSHA256":"` + hash + `","excludedSequences":[3,2]}`,
		`{"schemaVersion":"1.0.0","datasetSHA256":"` + hash + `","excludedSequences":[],"extra":true}`,
	}
	for _, contents := range tests {
		if _, err := LoadSelection(writeSelection(t, contents), hash, 4); err == nil {
			t.Fatalf("expected rejection for %s", contents)
		}
	}
}
