package domain

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMatrixMetadata(t *testing.T) {
	version, err := MatrixSchemaVersion()
	if err != nil {
		t.Fatalf("MatrixSchemaVersion: %v", err)
	}
	if version != "1.0.0" {
		t.Fatalf("schemaVersion = %q, se esperaba 1.0.0 (ADR-008, punto 4: matriz unica en v1)", version)
	}

	ruleset, err := MatrixRulesetID()
	if err != nil {
		t.Fatalf("MatrixRulesetID: %v", err)
	}
	if ruleset != "PFI_SNT_AUTHORIZED_TRANSFERS" {
		t.Fatalf("rulesetId = %q", ruleset)
	}
}

// TestEmbeddedMatrixMatchesFile verifica que el JSON embebido sea el archivo
// del repositorio y no una copia divergente.
func TestEmbeddedMatrixMatchesFile(t *testing.T) {
	onDisk, err := os.ReadFile("authorized-transfers.json")
	if err != nil {
		t.Fatalf("no se pudo leer authorized-transfers.json: %v", err)
	}
	if string(onDisk) != string(authorizedTransfersJSON) {
		t.Fatal("el JSON embebido difiere del archivo del repositorio")
	}
}

// TestDecideTransferAuthorizedPairs recorre los 16 pares autorizados del
// archivo y exige que la funcion de decision los autorice devolviendo el mismo
// id de regla. Es la garantia de que la decision sale de la matriz y no de
// condicionales mantenidos a mano (domain/README.md).
func TestDecideTransferAuthorizedPairs(t *testing.T) {
	raw, err := os.ReadFile("authorized-transfers.json")
	if err != nil {
		t.Fatalf("leer matriz: %v", err)
	}

	var parsed struct {
		AuthorizedTransfers []struct {
			ID          string    `json:"id"`
			Origin      AgentType `json:"origin"`
			Destination AgentType `json:"destination"`
		} `json:"authorizedTransfers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsear matriz: %v", err)
	}

	if len(parsed.AuthorizedTransfers) != 16 {
		t.Fatalf("pares autorizados = %d, se esperaban 16 (domain/README.md)", len(parsed.AuthorizedTransfers))
	}

	for _, rule := range parsed.AuthorizedTransfers {
		decision, err := DecideTransfer(rule.Origin, rule.Destination)
		if err != nil {
			t.Fatalf("DecideTransfer(%s, %s): %v", rule.Origin, rule.Destination, err)
		}
		if !decision.Allowed {
			t.Errorf("%s -> %s deberia estar autorizado", rule.Origin, rule.Destination)
			continue
		}
		if decision.RuleID != rule.ID {
			t.Errorf("%s -> %s devolvio ruleId %q, se esperaba %q", rule.Origin, rule.Destination, decision.RuleID, rule.ID)
		}
		if decision.Reason != "" {
			t.Errorf("%s -> %s autorizado pero con razon de rechazo %q", rule.Origin, rule.Destination, decision.Reason)
		}
	}
}

func TestDecideTransferExplicitProhibitions(t *testing.T) {
	cases := []struct {
		origin      AgentType
		destination AgentType
		ruleID      string
	}{
		{AgentPharmacy, AgentLaboratory, "PHARMACY_TO_UPSTREAM_AGENT"},
		{AgentPharmacy, AgentDrugstore, "PHARMACY_TO_UPSTREAM_AGENT"},
		{AgentHealthcare, AgentPharmacy, "HEALTHCARE_FACILITY_TO_UPSTREAM_AGENT"},
		{AgentHealthcare, AgentDistributor, "HEALTHCARE_FACILITY_TO_UPSTREAM_AGENT"},
	}

	for _, tc := range cases {
		decision, err := DecideTransfer(tc.origin, tc.destination)
		if err != nil {
			t.Fatalf("DecideTransfer(%s, %s): %v", tc.origin, tc.destination, err)
		}
		if decision.Allowed {
			t.Errorf("%s -> %s deberia estar prohibido", tc.origin, tc.destination)
		}
		if decision.RuleID != tc.ruleID || decision.Reason != tc.ruleID {
			t.Errorf("%s -> %s devolvio ruleId=%q reason=%q, se esperaba %q en ambos",
				tc.origin, tc.destination, decision.RuleID, decision.Reason, tc.ruleID)
		}
	}
}

// TestDecideTransferDefaultDeny cubre el punto 3 del algoritmo: todo par no
// declarado se rechaza por defaultDecision.
func TestDecideTransferDefaultDeny(t *testing.T) {
	cases := []struct {
		origin      AgentType
		destination AgentType
	}{
		{AgentDistributor, AgentLaboratory},   // eslabon superior sin prohibicion explicita
		{AgentDrugstore, AgentDistributor},    // idem
		{AgentLaboratory, AgentLaboratory},    // par no declarado
		{AgentPharmacy, AgentPharmacy},        // par no declarado
		{"AGENTE_INEXISTENTE", AgentPharmacy}, // agentType fuera del catalogo
		{AgentRegulator, AgentPharmacy},       // no custodial como origen (ADR-010, punto 2)
		{AgentLaboratory, AgentFinancier},     // no custodial como destino
	}

	for _, tc := range cases {
		decision, err := DecideTransfer(tc.origin, tc.destination)
		if err != nil {
			t.Fatalf("DecideTransfer(%s, %s): %v", tc.origin, tc.destination, err)
		}
		if decision.Allowed {
			t.Errorf("%s -> %s no deberia estar autorizado", tc.origin, tc.destination)
		}
		if decision.Reason != DefaultDenyReason {
			t.Errorf("%s -> %s devolvio razon %q, se esperaba %q", tc.origin, tc.destination, decision.Reason, DefaultDenyReason)
		}
		if decision.RuleID != "" {
			t.Errorf("%s -> %s devolvio ruleId %q en una denegacion por defecto", tc.origin, tc.destination, decision.RuleID)
		}
	}
}

// TestIsCustodialAgentType fija la separacion de ADR-010, punto 2: los
// agentType no custodiales nunca son origen ni destino de una transferencia.
func TestIsCustodialAgentType(t *testing.T) {
	custodial := []AgentType{
		AgentLaboratory, AgentDistributor, AgentLogisticsOperator,
		AgentDrugstore, AgentPharmacy, AgentHealthcare,
	}
	for _, agent := range custodial {
		ok, err := IsCustodialAgentType(agent)
		if err != nil {
			t.Fatalf("IsCustodialAgentType(%s): %v", agent, err)
		}
		if !ok {
			t.Errorf("%s deberia ser custodial (catalogo DES-3)", agent)
		}
	}

	nonCustodial := []AgentType{AgentRegulator, AgentFinancier, "OTRO"}
	for _, agent := range nonCustodial {
		ok, err := IsCustodialAgentType(agent)
		if err != nil {
			t.Fatalf("IsCustodialAgentType(%s): %v", agent, err)
		}
		if ok {
			t.Errorf("%s no deberia ser custodial (ADR-010, punto 2)", agent)
		}
	}
}
