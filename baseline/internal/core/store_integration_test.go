package core

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testGTIN = "07791234567898"

func integrationStore(t *testing.T) (*Store, map[string]Credential, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("SNT_BASELINE_TEST_DSN")
	if dsn == "" {
		t.Skip("SNT_BASELINE_TEST_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cleanup := func() {
		_, err := pool.Exec(context.Background(), `
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
	t.Cleanup(cleanup)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO public.organizations (msp_id,id,id_type,agent_type,active) VALUES
		('AnmatMSP','ANMAT','REG','REGULATOR',true),
		('LabMSP','7791234500017','GLN','LABORATORY',true),
		('DistribuidorMSP','7791234500031','GLN','DISTRIBUTOR',true),
		('FarmaciaMSP','7791234500048','GLN','PHARMACY',true),
		('CentroMedicoMSP','7791234500055','GLN','HEALTHCARE_FACILITY',true),
		('InactiveMSP','7791234500062','GLN','DRUGSTORE',false)`)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := ParseCredentials(`[
		{"key":"reg-key","mspId":"AnmatMSP","role":"regulatory-admin"},
		{"key":"lab-key","mspId":"LabMSP","role":"operator"},
		{"key":"lab-audit-key","mspId":"LabMSP","role":"auditor"},
		{"key":"distributor-key","mspId":"DistribuidorMSP","role":"operator"},
		{"key":"pharmacy-key","mspId":"FarmaciaMSP","role":"operator"},
		{"key":"health-key","mspId":"CentroMedicoMSP","role":"operator"},
		{"key":"inactive-key","mspId":"InactiveMSP","role":"operator"},
		{"key":"ghost-key","mspId":"GhostMSP","role":"operator"}
	]`)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, credentials)
	store.now = func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) }
	var sequence atomic.Int64
	store.newID = func() (string, error) { return fmt.Sprintf("tx-%06d", sequence.Add(1)), nil }
	resolved := map[string]Credential{}
	for _, key := range []string{"reg-key", "lab-key", "lab-audit-key", "distributor-key", "pharmacy-key", "health-key", "inactive-key", "ghost-key"} {
		resolved[key], err = store.Authenticate(key)
		if err != nil {
			t.Fatal(err)
		}
	}
	return store, resolved, pool
}

func requireCode(t *testing.T, err error, expected Code) {
	t.Helper()
	actual, ok := ErrorCode(err)
	if !ok || actual != expected {
		t.Fatalf("expected %s, got %v", expected, err)
	}
}

func TestCoreM2FlowAndHistoryParity(t *testing.T) {
	store, credentials, _ := integrationStore(t)
	ctx := context.Background()
	registered, err := store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-M2", Lote: "LOTE-M2", FechaVencimiento: "2028-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Estado != domain.StateEnLaboratorio {
		t.Fatalf("unexpected registration state: %s", registered.Estado)
	}
	_, err = store.RegisterUnit(ctx, credentials["ghost-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-GHOST", Lote: "LOTE-G", FechaVencimiento: "2028-12-31",
	})
	requireCode(t, err, OrgNotRegistered)
	_, err = store.RegisterUnit(ctx, credentials["inactive-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-INACTIVE", Lote: "LOTE-I", FechaVencimiento: "2028-12-31",
	})
	requireCode(t, err, OrgInactive)
	_, err = store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-M2", Lote: "LOTE-M2", FechaVencimiento: "2028-12-31",
	})
	requireCode(t, err, UnitAlreadyExists)
	_, err = store.Dispatch(ctx, credentials["lab-audit-key"], testGTIN, "SERIE-M2", DispatchRequest{
		Destino: "FarmaciaMSP", NumeroRemito: "R-1", NumeroFactura: "F-1", Cantidad: 1,
	})
	requireCode(t, err, UnauthorizedRole)
	dispatched, err := store.Dispatch(ctx, credentials["lab-key"], testGTIN, "SERIE-M2", DispatchRequest{
		Destino: "FarmaciaMSP", NumeroRemito: "R-1", NumeroFactura: "F-1", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Estado != domain.StateEnTransito || dispatched.CustodioActual != "GLN:7791234500017" {
		t.Fatalf("unexpected dispatch: %#v", dispatched)
	}
	_, err = store.Receive(ctx, credentials["health-key"], testGTIN, "SERIE-M2", nil)
	requireCode(t, err, ReceiverMismatch)
	_, err = store.Receive(ctx, credentials["pharmacy-key"], testGTIN, "SERIE-M2", &CommercialData{NumeroRemito: "R-INCOMPLETO"})
	requireCode(t, err, InvalidRequest)
	received, err := store.Receive(ctx, credentials["pharmacy-key"], testGTIN, "SERIE-M2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if received.Estado != domain.StateEnCustodia || received.CustodioActual != "GLN:7791234500048" {
		t.Fatalf("unexpected reception: %#v", received)
	}
	_, err = store.Dispatch(ctx, credentials["pharmacy-key"], testGTIN, "SERIE-M2", DispatchRequest{
		Destino: "LabMSP", NumeroRemito: "R-X", NumeroFactura: "F-X", Cantidad: 1,
	})
	requireCode(t, err, TransferNotAuthorized)
	_, err = store.Dispatch(ctx, credentials["pharmacy-key"], testGTIN, "SERIE-M2", DispatchRequest{
		Destino: "CentroMedicoMSP", NumeroRemito: "R-2", NumeroFactura: "F-2", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Receive(ctx, credentials["health-key"], testGTIN, "SERIE-M2", &CommercialData{
		NumeroRemito: "R-2", NumeroFactura: "F-2", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispensed, err := store.Dispense(ctx, credentials["health-key"], testGTIN, "SERIE-M2")
	if err != nil {
		t.Fatal(err)
	}
	if dispensed.Estado != domain.StateDispensado {
		t.Fatalf("unexpected final state: %s", dispensed.Estado)
	}
	read, err := store.ReadUnit(ctx, testGTIN, "SERIE-M2")
	if err != nil || read.Estado != domain.StateDispensado {
		t.Fatalf("unexpected ReadUnit result: %#v, %v", read, err)
	}
	_, err = store.Dispense(ctx, credentials["health-key"], testGTIN, "SERIE-M2")
	requireCode(t, err, InvalidStateTransition)
	history, err := store.GetUnitHistory(ctx, testGTIN, "SERIE-M2")
	if err != nil {
		t.Fatal(err)
	}
	wantStates := []domain.State{
		domain.StateEnLaboratorio, domain.StateEnTransito, domain.StateEnCustodia,
		domain.StateEnTransito, domain.StateEnCustodia, domain.StateDispensado,
	}
	if len(history) != len(wantStates) {
		t.Fatalf("expected %d history entries, got %d", len(wantStates), len(history))
	}
	for index, entry := range history {
		if entry.Value.Estado != wantStates[index] {
			t.Fatalf("history[%d]: expected %s, got %s", index, wantStates[index], entry.Value.Estado)
		}
		if entry.Timestamp != "2026-09-08T12:00:00Z" {
			t.Fatalf("history timestamp is not stable: %s", entry.Timestamp)
		}
	}
	units, err := store.QueryUnitsByGTIN(ctx, testGTIN)
	if err != nil || len(units) != 1 || units[0].Estado != domain.StateDispensado {
		t.Fatalf("unexpected GTIN query: %#v, %v", units, err)
	}
}

func TestRejectAndRegulatoryAdministration(t *testing.T) {
	store, credentials, _ := integrationStore(t)
	ctx := context.Background()
	_, err := store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-REJECT", Lote: "LOTE-R", FechaVencimiento: "2028-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "SERIE-REJECT", DispatchRequest{
		Destino: "FarmaciaMSP", NumeroRemito: "R-3", NumeroFactura: "F-3", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := store.Reject(ctx, credentials["pharmacy-key"], testGTIN, "SERIE-REJECT", RejectRequest{Motivo: "envase danado"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Estado != domain.StateDevuelto || rejected.CustodioActual != "GLN:7791234500017" {
		t.Fatalf("unexpected rejection: %#v", rejected)
	}
	_, err = store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-REJECT-EMITTER", Lote: "LOTE-E", FechaVencimiento: "2028-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "SERIE-REJECT-EMITTER", DispatchRequest{
		Destino: "FarmaciaMSP", NumeroRemito: "R-4", NumeroFactura: "F-4", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	emitterRejected, err := store.Reject(ctx, credentials["lab-key"], testGTIN, "SERIE-REJECT-EMITTER", RejectRequest{Motivo: "cancelacion del emisor"})
	if err != nil || emitterRejected.Estado != domain.StateDevuelto {
		t.Fatalf("unexpected emitter rejection: %#v, %v", emitterRejected, err)
	}
	created, err := store.RegisterOrganization(ctx, credentials["reg-key"], RegisterOrganizationRequest{
		MSPID: "DrogueriaNuevaMSP", ID: "7791234500079", IDType: IDTypeGLN, AgentType: domain.AgentDrugstore, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.CanonicalID() != "GLN:7791234500079" {
		t.Fatalf("unexpected organization: %#v", created)
	}
	updated, err := store.SetOrganizationActive(ctx, credentials["reg-key"], "DrogueriaNuevaMSP", false)
	if err != nil || updated.Active {
		t.Fatalf("organization was not deactivated: %#v, %v", updated, err)
	}
	_, err = store.SetOrganizationActive(ctx, credentials["reg-key"], "AnmatMSP", false)
	requireCode(t, err, LastActiveRegulator)
}

func TestEventSequenceIsTotalUnderTimestampTiesAndConcurrency(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	_, err := store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "SERIE-CONCURRENT", Lote: "LOTE-C", FechaVencimiento: "2028-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	const writers = 12
	errCh := make(chan error, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			tx, err := store.begin(ctx)
			if err != nil {
				errCh <- err
				return
			}
			defer func() { _ = tx.Rollback(ctx) }()
			unit, err := readLockedUnit(ctx, tx, testGTIN, "SERIE-CONCURRENT")
			if err != nil {
				errCh <- err
				return
			}
			if err := appendUnitEvent(ctx, tx, fmt.Sprintf("concurrent-%02d", index), "ConcurrentTest", "LabMSP", unit); err != nil {
				errCh <- err
				return
			}
			errCh <- commit(ctx, tx)
		}(index)
	}
	group.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := pool.Query(ctx, `SELECT event_sequence, event_timestamp FROM public.unit_events WHERE gtin=$1 AND numero_serie=$2 ORDER BY event_sequence`, testGTIN, "SERIE-CONCURRENT")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	sequence := int64(1)
	for rows.Next() {
		var actual int64
		var timestamp time.Time
		if err := rows.Scan(&actual, &timestamp); err != nil {
			t.Fatal(err)
		}
		if actual != sequence {
			t.Fatalf("expected sequence %d, got %d", sequence, actual)
		}
		if !timestamp.Equal(store.now()) {
			t.Fatalf("expected timestamp tie, got %s", timestamp)
		}
		sequence++
	}
	if sequence != writers+2 {
		t.Fatalf("expected %d events, got %d", writers+1, sequence-1)
	}
}
