package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type historySnapshot struct {
	State     domain.State
	Custodian string
	Invoker   string
}

func seedHistory(t *testing.T, pool *pgxpool.Pool, serial, expiry string, snapshots []historySnapshot) {
	t.Helper()
	ctx := context.Background()
	last := snapshots[len(snapshots)-1]
	lastTimestamp := fmt.Sprintf("2026-09-08T10:%02d:00Z", len(snapshots))
	_, err := pool.Exec(ctx, `INSERT INTO public.medication_units
		(gtin,numero_serie,lote,fecha_vencimiento,custodio_actual,estado,ultima_actualizacion)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, testGTIN, serial, "LOT-"+serial, expiry,
		last.Custodian, last.State, lastTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	for index, snapshot := range snapshots {
		timestamp := fmt.Sprintf("2026-09-08T10:%02d:00Z", index+1)
		_, err := pool.Exec(ctx, `INSERT INTO public.unit_events
			(gtin,numero_serie,tx_id,event_timestamp,event_sequence,operation,invoker_msp_id,
			 lote,fecha_vencimiento,custodio_actual,estado,ultima_actualizacion)
			VALUES ($1,$2,$3,$4,$5,'TestHistory',$6,$7,$8,$9,$10,$4)`,
			testGTIN, serial, fmt.Sprintf("history-%s-%02d", serial, index+1), timestamp,
			index+1, snapshot.Invoker, "LOT-"+serial, expiry, snapshot.Custodian, snapshot.State)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func assertCheckResults(t *testing.T, checks []TraceCheck, want ...string) {
	t.Helper()
	if len(checks) != len(want) {
		t.Fatalf("expected %d checks, got %d", len(want), len(checks))
	}
	for index := range want {
		if checks[index].Resultado != want[index] {
			t.Fatalf("check %d (%s): expected %s, got %s", index, checks[index].Check, want[index], checks[index].Resultado)
		}
	}
}

func TestVerificationHappyPathsNotFoundAndAuthorization(t *testing.T) {
	store, credentials, _ := integrationStore(t)
	ctx := context.Background()
	_, err := store.RegisterUnit(ctx, credentials["lab-key"], RegisterUnitRequest{
		GTIN: testGTIN, NumeroSerie: "VERIFY-HAPPY", Lote: "LOT-V", FechaVencimiento: "2028-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "VERIFY-HAPPY", DispatchRequest{
		Destino: "FarmaciaMSP", NumeroRemito: "R-V", NumeroFactura: "F-V", Cantidad: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	inTransit, err := store.VerifyUnit(ctx, credentials["financier-key"], testGTIN, "VERIFY-HAPPY")
	if err != nil || !inTransit.Autentica || inTransit.Estado != domain.StateEnTransito {
		t.Fatalf("VerifyUnit in transit: %#v, %v", inTransit, err)
	}
	assertCheckResults(t, inTransit.Verificaciones, checkOK, checkOK, checkOK, checkOK)
	_, err = store.Receive(ctx, credentials["pharmacy-key"], testGTIN, "VERIFY-HAPPY", nil)
	if err != nil {
		t.Fatal(err)
	}
	inCustody, err := store.VerifyUnit(ctx, credentials["pharmacy-key"], testGTIN, "VERIFY-HAPPY")
	if err != nil || !inCustody.Autentica || inCustody.Estado != domain.StateEnCustodia {
		t.Fatalf("VerifyUnit in custody: %#v, %v", inCustody, err)
	}
	_, err = store.Dispense(ctx, credentials["pharmacy-key"], testGTIN, "VERIFY-HAPPY")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"financier-key", "reg-audit-key", "reg-key"} {
		trace, err := store.VerifyTrace(ctx, credentials[key], testGTIN, "VERIFY-HAPPY")
		if err != nil || !trace.Legitima || trace.Motivo != "" {
			t.Fatalf("VerifyTrace as %s: %#v, %v", key, trace, err)
		}
		assertCheckResults(t, trace.Verificaciones, checkOK, checkOK, checkOK, checkOK, checkOK)
	}
	terminal, err := store.VerifyUnit(ctx, credentials["drugstore-key"], testGTIN, "VERIFY-HAPPY")
	if err != nil || terminal.Autentica || terminal.Motivo != verdictTerminalState {
		t.Fatalf("terminal verdict: %#v, %v", terminal, err)
	}

	notFound, err := store.VerifyUnit(ctx, credentials["lab-key"], testGTIN, "NO-UNIT")
	if err != nil || notFound.Motivo != verdictNotFound {
		t.Fatalf("VerifyUnit not found: %#v, %v", notFound, err)
	}
	assertCheckResults(t, notFound.Verificaciones, checkFailed, checkNotEvaluated, checkNotEvaluated, checkNotEvaluated)
	traceNotFound, err := store.VerifyTrace(ctx, credentials["financier-key"], testGTIN, "NO-UNIT")
	if err != nil || traceNotFound.Motivo != verdictNotFound {
		t.Fatalf("VerifyTrace not found: %#v, %v", traceNotFound, err)
	}
	assertCheckResults(t, traceNotFound.Verificaciones, checkFailed, checkNotEvaluated, checkNotEvaluated, checkNotEvaluated, checkNotEvaluated)

	_, err = store.VerifyUnit(ctx, credentials["inactive-key"], testGTIN, "VERIFY-HAPPY")
	requireCode(t, err, OrgInactive)
	_, err = store.VerifyUnit(ctx, credentials["ghost-key"], testGTIN, "VERIFY-HAPPY")
	requireCode(t, err, OrgNotRegistered)
	_, err = store.VerifyTrace(ctx, credentials["lab-audit-key"], testGTIN, "VERIFY-HAPPY")
	requireCode(t, err, UnauthorizedAgentType)
}

func TestVerifyUnitVerdictsAndEvaluationOrder(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	lab := "GLN:7791234500017"
	lab2 := "GLN:7791234500086"

	seedHistory(t, pool, "V-BLOCK", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnCuarentena, lab, "LabMSP"},
	})
	blocking, err := store.VerifyUnit(ctx, credentials["drugstore-key"], testGTIN, "V-BLOCK")
	if err != nil || blocking.Motivo != verdictBlockingState {
		t.Fatalf("blocking: %#v, %v", blocking, err)
	}
	assertCheckResults(t, blocking.Verificaciones, checkOK, checkOK, checkOK, checkFailed)

	seedHistory(t, pool, "V-EXPIRY", "2026-09-07", []historySnapshot{{domain.StateEnLaboratorio, lab, "LabMSP"}})
	expired, err := store.VerifyUnit(ctx, credentials["drugstore-key"], testGTIN, "V-EXPIRY")
	if err != nil || expired.Motivo != verdictExpiredByDate {
		t.Fatalf("expired: %#v, %v", expired, err)
	}

	seedHistory(t, pool, "V-SEQUENCE", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnCustodia, lab, "LabMSP"},
	})
	invalidSequence, err := store.VerifyUnit(ctx, credentials["drugstore-key"], testGTIN, "V-SEQUENCE")
	if err != nil || invalidSequence.Motivo != verdictInvalidSequence {
		t.Fatalf("sequence: %#v, %v", invalidSequence, err)
	}
	assertCheckResults(t, invalidSequence.Verificaciones, checkOK, checkOK, checkFailed, checkNotEvaluated)

	seedHistory(t, pool, "V-PAIR", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnTransito, lab, "LabMSP"},
		{domain.StateEnCustodia, lab2, "Lab2MSP"},
	})
	invalidPair, err := store.VerifyUnit(ctx, credentials["drugstore-key"], testGTIN, "V-PAIR")
	if err != nil || invalidPair.Motivo != verdictTransferNotAuthorized {
		t.Fatalf("pair: %#v, %v", invalidPair, err)
	}
	assertCheckResults(t, invalidPair.Verificaciones, checkOK, checkOK, checkFailed, checkNotEvaluated)

	if _, err := pool.Exec(ctx, `INSERT INTO public.medication_units
		(gtin,numero_serie,lote,fecha_vencimiento,custodio_actual,estado,ultima_actualizacion)
		VALUES ($1,$2,'DUP','2028-12-31',$3,$4,'2026-09-08T12:00:00Z')`,
		testGTIN, "V-PAIR", lab2, domain.StateEnCustodia); err == nil {
		t.Fatal("database accepted a duplicate unit identity")
	}
}

func TestVerifyTraceVerdictsAndEvaluationOrder(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	lab := "GLN:7791234500017"
	distributor := "GLN:7791234500031"
	lab2 := "GLN:7791234500086"
	pharmacy := "GLN:7791234500048"

	seedHistory(t, pool, "T-NOT-DISP", "2028-12-31", []historySnapshot{{domain.StateEnLaboratorio, lab, "LabMSP"}})
	notDispensed, err := store.VerifyTrace(ctx, credentials["financier-key"], testGTIN, "T-NOT-DISP")
	if err != nil || notDispensed.Motivo != verdictNotDispensed {
		t.Fatalf("not dispensed: %#v, %v", notDispensed, err)
	}
	assertCheckResults(t, notDispensed.Verificaciones, checkOK, checkFailed, checkNotEvaluated, checkNotEvaluated, checkNotEvaluated)

	seedHistory(t, pool, "T-DISPENSER", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnTransito, lab, "LabMSP"},
		{domain.StateEnCustodia, distributor, "DistribuidorMSP"},
		{domain.StateDispensado, distributor, "DistribuidorMSP"},
	})
	badDispenser, err := store.VerifyTrace(ctx, credentials["financier-key"], testGTIN, "T-DISPENSER")
	if err != nil || badDispenser.Motivo != verdictInvalidDispenser {
		t.Fatalf("bad dispenser: %#v, %v", badDispenser, err)
	}
	assertCheckResults(t, badDispenser.Verificaciones, checkOK, checkOK, checkFailed, checkNotEvaluated, checkNotEvaluated)

	seedHistory(t, pool, "T-ORDER", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnTransito, lab, "LabMSP"},
		{domain.StateEnCustodia, lab2, "Lab2MSP"},
		{domain.StateEnCuarentena, lab2, "Lab2MSP"},
		{domain.StateEnCustodia, pharmacy, "FarmaciaMSP"},
		{domain.StateDispensado, pharmacy, "FarmaciaMSP"},
	})
	ordered, err := store.VerifyTrace(ctx, credentials["financier-key"], testGTIN, "T-ORDER")
	if err != nil || ordered.Motivo != verdictInvalidSequence {
		t.Fatalf("evaluation order: %#v, %v", ordered, err)
	}
	assertCheckResults(t, ordered.Verificaciones, checkOK, checkOK, checkOK, checkFailed, checkNotEvaluated)

	seedHistory(t, pool, "T-PAIR", "2028-12-31", []historySnapshot{
		{domain.StateEnLaboratorio, lab, "LabMSP"},
		{domain.StateEnTransito, lab, "LabMSP"},
		{domain.StateEnCustodia, lab2, "Lab2MSP"},
		{domain.StateDispensado, lab2, "Lab2MSP"},
	})
	pair, err := store.VerifyTrace(ctx, credentials["financier-key"], testGTIN, "T-PAIR")
	if err != nil || pair.Motivo != verdictInvalidDispenser {
		t.Fatalf("dispenser must fail before pair: %#v, %v", pair, err)
	}
}
