package core

import "testing"

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
