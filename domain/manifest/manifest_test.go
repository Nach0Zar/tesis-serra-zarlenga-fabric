package manifest

import (
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// TestEmbeddedManifestDeclaresTheFoundationalOrganizations fija lo que Init
// consume del manifiesto: su version y las siete organizaciones de NET-2.
func TestEmbeddedManifestDeclaresTheFoundationalOrganizations(t *testing.T) {
	version, err := SchemaVersion()
	if err != nil || version == "" {
		t.Fatalf("SchemaVersion() = %q, %v", version, err)
	}
	organizations, err := Organizations()
	if err != nil {
		t.Fatal(err)
	}
	if len(organizations) != 7 {
		t.Fatalf("len(Organizations()) = %d, want 7", len(organizations))
	}
	seen := map[string]bool{}
	for _, org := range organizations {
		if org.MSPID == "" || org.ID == "" || org.IDType == "" || org.AgentType == "" {
			t.Fatalf("organizacion incompleta: %+v", org)
		}
		if seen[org.MSPID] {
			t.Fatalf("mspId duplicado: %s", org.MSPID)
		}
		seen[org.MSPID] = true
	}
}

// TestOrganizationsReturnsACopy impide que un consumidor altere el manifiesto
// embebido que despues lee Init.
func TestOrganizationsReturnsACopy(t *testing.T) {
	first, err := Organizations()
	if err != nil {
		t.Fatal(err)
	}
	first[0].MSPID = "AlteradoMSP"
	second, err := Organizations()
	if err != nil {
		t.Fatal(err)
	}
	if second[0].MSPID == "AlteradoMSP" {
		t.Fatal("Organizations() expone el slice interno del manifiesto")
	}
}

// TestRegulatorIsTheSingleAnmatOrganization: el mspId regulatorio sale del
// manifiesto y no del invocador (ADR-010, punto 4).
func TestRegulatorIsTheSingleAnmatOrganization(t *testing.T) {
	regulator, err := Regulator()
	if err != nil {
		t.Fatal(err)
	}
	if regulator.MSPID != "AnmatMSP" || regulator.AgentType != domain.AgentRegulator {
		t.Fatalf("Regulator() = %+v", regulator)
	}
}
