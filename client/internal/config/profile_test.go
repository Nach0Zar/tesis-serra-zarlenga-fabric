package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSupportedOrganizations(t *testing.T) {
	repositoryRoot := t.TempDir()
	organizations := []manifestOrganization{
		{MspID: "AnmatMSP", Slug: "anmat", PeerHostname: "peer0.anmat.snt.local", Active: true},
		{MspID: "LabMSP", Slug: "lab", PeerHostname: "peer0.lab.snt.local", Active: true},
		{MspID: "DrogueriaMSP", Slug: "drogueria", PeerHostname: "peer0.drogueria.snt.local", Active: true},
		{MspID: "FarmaciaMSP", Slug: "farmacia", PeerHostname: "peer0.farmacia.snt.local", Active: true},
	}
	writeManifest(t, repositoryRoot, organizations)
	for _, organization := range organizations {
		writeCryptoFixture(t, repositoryRoot, organization)
	}

	expectedEndpoints := map[string]string{
		"anmat":     "dns:///localhost:7051",
		"lab":       "dns:///localhost:8051",
		"drogueria": "dns:///localhost:9051",
		"farmacia":  "dns:///localhost:11051",
	}
	for _, organization := range organizations {
		t.Run(organization.Slug, func(t *testing.T) {
			got, err := Resolve(repositoryRoot, organization.Slug, "", "")
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.MSPID != organization.MspID {
				t.Fatalf("Resolve().MSPID = %q, want %q", got.MSPID, organization.MspID)
			}
			if got.GatewayEndpoint != expectedEndpoints[organization.Slug] {
				t.Fatalf(
					"Resolve().GatewayEndpoint = %q, want %q",
					got.GatewayEndpoint,
					expectedEndpoints[organization.Slug],
				)
			}
			if filepath.Base(got.CertificatePath) != "cert.pem" {
				t.Fatalf("Resolve().CertificatePath = %q", got.CertificatePath)
			}
			if filepath.Base(got.PrivateKeyPath) != "private_sk" {
				t.Fatalf("Resolve().PrivateKeyPath = %q", got.PrivateKeyPath)
			}
		})
	}
}

func TestResolveAppliesConnectionOverrides(t *testing.T) {
	repositoryRoot := t.TempDir()
	organization := manifestOrganization{
		MspID:        "LabMSP",
		Slug:         "lab",
		PeerHostname: "peer0.lab.snt.local",
		Active:       true,
	}
	writeManifest(t, repositoryRoot, []manifestOrganization{organization})
	writeCryptoFixture(t, repositoryRoot, organization)

	got, err := Resolve(repositoryRoot, "LAB", "dns:///gateway.example:443", "gateway.example")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.GatewayEndpoint != "dns:///gateway.example:443" {
		t.Fatalf("Resolve().GatewayEndpoint = %q", got.GatewayEndpoint)
	}
	if got.PeerHostname != "gateway.example" {
		t.Fatalf("Resolve().PeerHostname = %q", got.PeerHostname)
	}
}

func TestResolveRejectsAmbiguousPrivateKey(t *testing.T) {
	repositoryRoot := t.TempDir()
	organization := manifestOrganization{
		MspID:        "LabMSP",
		Slug:         "lab",
		PeerHostname: "peer0.lab.snt.local",
		Active:       true,
	}
	writeManifest(t, repositoryRoot, []manifestOrganization{organization})
	writeCryptoFixture(t, repositoryRoot, organization)
	keyDirectory := filepath.Join(
		repositoryRoot,
		"network",
		"organizations",
		"lab",
		"users",
		"User1@lab.snt.local",
		"msp",
		"keystore",
	)
	if err := os.WriteFile(filepath.Join(keyDirectory, "second_sk"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(repositoryRoot, "lab", "", "")
	if err == nil || !strings.Contains(err.Error(), "expected exactly one regular file") {
		t.Fatalf("Resolve() error = %v, want ambiguous-key error", err)
	}
}

func TestFindRepositoryRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	writeManifest(t, repositoryRoot, nil)
	nested := filepath.Join(repositoryRoot, "client", "cmd")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindRepositoryRoot(nested)
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	if got != repositoryRoot {
		t.Fatalf("FindRepositoryRoot() = %q, want %q", got, repositoryRoot)
	}
}

func writeManifest(t *testing.T, repositoryRoot string, organizations []manifestOrganization) {
	t.Helper()
	manifestDirectory := filepath.Join(repositoryRoot, "network")
	if err := os.MkdirAll(manifestDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	contents, err := json.Marshal(manifest{Organizations: organizations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(manifestDirectory, "organizations-manifest.json"),
		contents,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func writeCryptoFixture(
	t *testing.T,
	repositoryRoot string,
	organization manifestOrganization,
) {
	t.Helper()
	organizationRoot := filepath.Join(
		repositoryRoot,
		"network",
		"organizations",
		organization.Slug,
	)
	userMSP := filepath.Join(
		organizationRoot,
		"users",
		"User1@"+organization.Slug+".snt.local",
		"msp",
	)
	files := map[string][]byte{
		filepath.Join(userMSP, "signcerts", "cert.pem"):                                      []byte("certificate"),
		filepath.Join(userMSP, "keystore", "private_sk"):                                     []byte("key"),
		filepath.Join(organizationRoot, "peers", organization.PeerHostname, "tls", "ca.crt"): []byte("tls"),
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
