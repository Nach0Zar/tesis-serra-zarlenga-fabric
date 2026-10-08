package snt

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
)

func TestGetLabInterventionHistoryChronologicalAndReadOnly(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	stub.txID = "tx-emision"
	first, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
	requireNoError(t, err)
	stub.txID = "tx-reemplazo"
	replacement := validAuthorizationRequest()
	replacement.Motivo = "Nueva autorizacion documentada"
	second, err := contract.AuthorizeLabIntervention(ctx, replacement)
	requireNoError(t, err)
	stub.txID = "tx-revocacion"
	third, err := contract.RevokeLabIntervention(ctx, RevokeLabInterventionRequest{
		GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Sin efecto",
	})
	requireNoError(t, err)

	key, err := labInterventionKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	beforeState, beforeValidation, beforeEvents := len(stub.state), len(stub.validation), len(stub.events)
	beforeHistory := len(stub.history[key])
	privateBefore, err := json.Marshal(stub.privateData)
	requireNoError(t, err)
	history, err := contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if len(history) != 3 {
		t.Fatalf("historial de %d entradas, esperado 3", len(history))
	}
	for i, want := range []struct {
		txID string
		view LabInterventionView
	}{
		{"tx-emision", *first},
		{"tx-reemplazo", *second},
		{"tx-revocacion", *third},
	} {
		entry := history[i]
		if entry.TxID != want.txID || entry.Timestamp == "" || entry.IsDelete ||
			entry.Value == nil || *entry.Value != want.view {
			t.Fatalf("entrada %d inesperada: %+v", i, entry)
		}
	}
	if len(stub.state) != beforeState || len(stub.validation) != beforeValidation ||
		len(stub.events) != beforeEvents || len(stub.history[key]) != beforeHistory {
		t.Fatal("la consulta modifico el estado o el historial")
	}
	privateAfter, err := json.Marshal(stub.privateData)
	requireNoError(t, err)
	if !bytes.Equal(privateBefore, privateAfter) {
		t.Fatal("la consulta modifico los datos privados o marcadores")
	}
}

func TestGetLabInterventionHistoryExpirationDoesNotCreateEntry(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	_, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
	requireNoError(t, err)
	stub.timestamp = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	history, err := contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if len(history) != 1 || history[0].Value == nil ||
		history[0].Value.Estado != LabInterventionActiva {
		t.Fatalf("vencimiento creo un estado sintetico: %+v", history)
	}
}

func TestGetLabInterventionHistoryErrorsAndDeletion(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleAuditor)
	_, err := contract.GetLabInterventionHistory(ctx, "invalid", validSerial)
	requireCode(t, err, cerr.InvalidRequest)
	_, err = contract.GetLabInterventionHistory(ctx, validGTIN, "SN-9999-ZZZZ")
	requireCode(t, err, cerr.UnitNotFound)
	_, err = contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireCode(t, err, cerr.LabInterventionNotFound)

	_, err = contract.AuthorizeLabIntervention(testContext(stub, anmatMSP, RoleRegulatoryAdmin), validAuthorizationRequest())
	requireNoError(t, err)
	key, err := labInterventionKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	stub.txID = "tx-delete"
	requireNoError(t, stub.DelState(key))
	history, err := contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if len(history) != 2 || !history[1].IsDelete || history[1].Value != nil ||
		history[1].TxID != "tx-delete" {
		t.Fatalf("borrado historico no representado: %+v", history)
	}

	stub.appendHistory(key, []byte("{invalid"), false)
	_, err = contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireCode(t, err, cerr.InternalError)

	stub.failOn("GetHistoryForKey", errors.New("fallo de plataforma"))
	_, err = contract.GetLabInterventionHistory(ctx, validGTIN, validSerial)
	requireCode(t, err, cerr.InternalError)
}
