package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func emptySeedStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("SNT_BASELINE_TEST_DSN")
	if dsn == "" {
		t.Skip("SNT_BASELINE_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cleanup := func() {
		if _, err := pool.Exec(ctx, `
			DELETE FROM public.lab_interventions;
			DELETE FROM public.transfer_operations;
			DELETE FROM public.unit_events;
			DELETE FROM public.medication_units;
			DELETE FROM public.organizations;`); err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return NewStore(pool, Credentials{}), pool
}

func seedFixture() ([]Organization, []SeedRegistration) {
	organizations := []Organization{
		{MSPID: "AnmatMSP", ID: "ANMAT", IDType: IDTypeREG, AgentType: domain.AgentRegulator, Active: true},
		{MSPID: "LabMSP", ID: "7791234500017", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory, Active: true},
	}
	registrations := []SeedRegistration{
		{InvokerMSPID: "LabMSP", Request: RegisterUnitRequest{
			GTIN: testGTIN, NumeroSerie: "SEED-0001", Lote: "SEED-LOT", FechaVencimiento: "2099-01-01",
		}},
		{InvokerMSPID: "LabMSP", Request: RegisterUnitRequest{
			GTIN: testGTIN, NumeroSerie: "SEED-0002", Lote: "SEED-LOT", FechaVencimiento: "2099-01-01",
		}},
	}
	return organizations, registrations
}

func TestSeedSnapshotIsAtomicAndRejectsReseeding(t *testing.T) {
	store, pool := emptySeedStore(t)
	organizations, registrations := seedFixture()
	sequence := 0
	store.newID = func() (string, error) {
		sequence++
		return fmt.Sprintf("seed-tx-%d", sequence), nil
	}

	result, err := store.SeedSnapshot(context.Background(), organizations, registrations)
	if err != nil {
		t.Fatal(err)
	}
	if result.Organizations != 2 || result.Units != 2 {
		t.Fatalf("unexpected seed result: %#v", result)
	}
	var orgCount, unitCount, eventCount, invalidEvents int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM public.organizations),
			(SELECT count(*) FROM public.medication_units),
			(SELECT count(*) FROM public.unit_events),
			(SELECT count(*) FROM public.unit_events
			 WHERE operation <> 'RegisterUnit' OR event_sequence <> 1)`).
		Scan(&orgCount, &unitCount, &eventCount, &invalidEvents); err != nil {
		t.Fatal(err)
	}
	if orgCount != 2 || unitCount != 2 || eventCount != 2 || invalidEvents != 0 {
		t.Fatalf("unexpected persisted counts: orgs=%d units=%d events=%d invalid=%d", orgCount, unitCount, eventCount, invalidEvents)
	}

	if _, err := store.SeedSnapshot(context.Background(), organizations, registrations); err == nil {
		t.Fatal("expected reseeding to fail")
	} else {
		requireCode(t, err, AlreadyInitialized)
	}
}

func TestSeedSnapshotRollsBackAfterPartialFailure(t *testing.T) {
	store, pool := emptySeedStore(t)
	organizations, registrations := seedFixture()
	calls := 0
	store.newID = func() (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("forced id failure")
		}
		return "seed-tx-1", nil
	}

	if _, err := store.SeedSnapshot(context.Background(), organizations, registrations); err == nil {
		t.Fatal("expected seed to fail")
	}
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM public.organizations) +
			(SELECT count(*) FROM public.medication_units) +
			(SELECT count(*) FROM public.unit_events)`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("seed rollback left %d rows", count)
	}
}
