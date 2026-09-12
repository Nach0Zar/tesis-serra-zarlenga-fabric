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
		_, err := pool.Exec(ctx, `
			DELETE FROM public.lab_interventions;
			DELETE FROM public.transfer_operations;
			DELETE FROM public.unit_events;
			DELETE FROM public.medication_units;
			DELETE FROM public.organizations;`)
		if err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	defer cleanup()
	_, err = pool.Exec(ctx, `
		INSERT INTO public.organizations (msp_id,id,id_type,agent_type,active) VALUES
		('LabMSP','7791234500017','GLN','LABORATORY',true),
		('FarmaciaMSP','7791234500048','GLN','PHARMACY',true)`)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := core.ParseCredentials(`[
		{"key":"lab-key","mspId":"LabMSP","role":"operator"},
		{"key":"pharmacy-key","mspId":"FarmaciaMSP","role":"operator"}
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
	if len(history) != 3 {
		t.Fatalf("expected register, dispatch and receive, got %d entries", len(history))
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
