package core

import (
	"strings"
	"testing"
)

func TestParseCredentialsEnforcesOneKeyPerOrganizationRole(t *testing.T) {
	_, err := ParseCredentials(`[
		{"key":"one","mspId":"LabMSP","role":"operator"},
		{"key":"two","mspId":"LabMSP","role":"operator"}
	]`)
	if err == nil {
		t.Fatal("expected duplicate organization-role pair to fail")
	}
}

func TestCredentialsResolveWithoutKeepingPlaintextIndex(t *testing.T) {
	credentials, err := ParseCredentials(`[{"key":"secret","mspId":"LabMSP","role":"operator"}]`)
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := credentials.Resolve("secret")
	if !ok || resolved.MSPID != "LabMSP" || resolved.Role != RoleOperator {
		t.Fatalf("unexpected credential: %#v, %v", resolved, ok)
	}
	if _, ok := credentials.Resolve("wrong"); ok {
		t.Fatal("unknown key resolved")
	}
}

func TestParseCredentialsRejectsIncompleteDeclarations(t *testing.T) {
	for raw, want := range map[string]string{
		`no-json`: "array JSON valido",
		`[]`:      "al menos una credencial",
		`[{"key":"","mspId":"LabMSP","role":"operator"}]`:                                                  "incompleta",
		`[{"key":"k","mspId":"LabMSP","role":"superuser"}]`:                                                "rol desconocido",
		`[{"key":"k","mspId":"LabMSP","role":"operator"},{"key":"k","mspId":"AnmatMSP","role":"auditor"}]`: "mas de una vez",
	} {
		if _, err := ParseCredentials(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("ParseCredentials(%s) = %v, want %q", raw, err, want)
		}
	}
	credentials, err := ParseCredentials(`[{"key":"k","mspId":"LabMSP","role":"operator"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := credentials.Resolve(""); ok {
		t.Fatal("una clave vacia no debe resolverse")
	}
}
