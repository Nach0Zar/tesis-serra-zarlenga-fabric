package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHTTPToPostgreSQLCoreFlow(t *testing.T) {
	dsn := os.Getenv("SNT_BASELINE_TEST_DSN")
	if dsn == "" {
		t.Skip("SNT_BASELINE_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cleanup := func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `
			ALTER TABLE public.return_operations DISABLE TRIGGER USER;
			DELETE FROM public.return_operations;
			ALTER TABLE public.return_operations ENABLE TRIGGER USER;
			DELETE FROM public.lab_interventions;
			DELETE FROM public.transfer_operations;
			DELETE FROM public.unit_events;
			DELETE FROM public.medication_units;
			DELETE FROM public.organizations;`)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	defer cleanup()
	_, err = pool.Exec(ctx, `
		INSERT INTO public.organizations (msp_id,id,id_type,agent_type,active) VALUES
		('LabMSP','7791234500017','GLN','LABORATORY',true),
		('FarmaciaMSP','7791234500048','GLN','PHARMACY',true),
		('FinanciadorMSP','PAMI','REG','FINANCIER',true)`)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := core.ParseCredentials(`[
		{"key":"lab-key","mspId":"LabMSP","role":"operator"},
		{"key":"pharmacy-key","mspId":"FarmaciaMSP","role":"operator"},
		{"key":"financier-key","mspId":"FinanciadorMSP","role":"financier-auditor"}
	]`)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(core.NewStore(pool, credentials), nil))
	defer server.Close()

	requestJSON(t, server.URL+"/v1/units", "lab-key", `{
		"gtin":"07791234567898","numeroSerie":"SERIE-HTTP",
		"lote":"LOTE-HTTP","fechaVencimiento":"2028-12-31"
	}`, http.StatusCreated)
	requestJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/dispatch", "lab-key", `{
		"destino":"FarmaciaMSP","numeroRemito":"R-HTTP",
		"numeroFactura":"F-HTTP","cantidad":1
	}`, http.StatusOK)
	requestJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/receive", "pharmacy-key", "", http.StatusOK)
	requestJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/quarantine", "pharmacy-key", `{
		"motivo":"control de calidad"
	}`, http.StatusOK)

	var quarantined []core.MedicationUnit
	requestGETJSON(t, server.URL+"/v1/units?estado=EN_CUARENTENA", "", &quarantined)
	if len(quarantined) != 1 || quarantined[0].NumeroSerie != "SERIE-HTTP" {
		t.Fatalf("unexpected state query: %#v", quarantined)
	}
	var unitVerdict core.UnitVerdict
	requestGETJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/verify-unit", "pharmacy-key", &unitVerdict)
	if unitVerdict.Autentica || unitVerdict.Motivo != "ESTADO_BLOQUEANTE" {
		t.Fatalf("unexpected unit verdict: %#v", unitVerdict)
	}
	requestJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/release-quarantine", "pharmacy-key", `{"motivo":"liberada"}`, http.StatusOK)
	requestJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/dispense", "pharmacy-key", "", http.StatusOK)
	var traceVerdict core.TraceVerdict
	requestGETJSON(t, server.URL+"/v1/units/07791234567898/SERIE-HTTP/verify-trace", "financier-key", &traceVerdict)
	if !traceVerdict.Legitima {
		t.Fatalf("unexpected trace verdict: %#v", traceVerdict)
	}

	response, err := http.Get(server.URL + "/v1/units/07791234567898/SERIE-HTTP/history")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("history returned %d: %s", response.StatusCode, body)
	}
	var history []core.HistoryEntry
	if err := json.NewDecoder(response.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 6 {
		t.Fatalf("expected six API-persisted events, got %d entries", len(history))
	}
}

func requestGETJSON(t *testing.T, url, key string, target any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Org-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s returned %d: %s", url, response.StatusCode, payload)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func requestJSON(t *testing.T, url, key, body string, expectedStatus int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Org-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s returned %d: %s", url, response.StatusCode, payload)
	}
}
