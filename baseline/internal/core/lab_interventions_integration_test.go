package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func TestLabInterventionLifecycleAndMatching(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "LAB-CYCLE", domain.StateRetiradoMercado, "DistribuidorMSP", "", "2028-12-31")
	_, err := store.Restock(ctx, credentials["lab2-key"], testGTIN, "LAB-CYCLE", UnitEventRequest{Motivo: "reingreso"})
	requireCode(t, err, LabInterventionRequired)
	authorization := AuthorizeLabInterventionRequest{
		Laboratorio: "GLN:7791234500086", Operacion: LabOpRestock,
		Motivo: "control inicial", ExpiraEn: "2026-09-09T12:00:00Z",
	}
	first, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-CYCLE", authorization)
	if err != nil || first.Estado != LabInterventionActive {
		t.Fatalf("authorize: %#v, %v", first, err)
	}
	authorization.Motivo = "autorizacion reemplazada"
	replaced, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-CYCLE", authorization)
	if err != nil || replaced.Motivo != authorization.Motivo || replaced.Estado != LabInterventionActive {
		t.Fatalf("replace: %#v, %v", replaced, err)
	}
	unit, err := store.Restock(ctx, credentials["lab2-key"], testGTIN, "LAB-CYCLE", UnitEventRequest{Motivo: "reingreso"})
	if err != nil || unit.Estado != domain.StateEnCustodia {
		t.Fatalf("consume: %#v, %v", unit, err)
	}
	consumed, found, err := readLabInterventionForTest(ctx, store, testGTIN, "LAB-CYCLE")
	if err != nil || !found || consumed.Estado != LabInterventionConsumed || consumed.ConsumidaEn == "" {
		t.Fatalf("consumed view: %#v, %v", consumed, err)
	}
	history, err := store.GetLabInterventionHistory(ctx, testGTIN, "LAB-CYCLE")
	if err != nil || len(history) != 3 {
		t.Fatalf("historial emision/reemplazo/consumo: %+v, %v", history, err)
	}
	for index, want := range []LabInterventionView{first, replaced, consumed} {
		entry := history[index]
		if entry.TxID == "" || entry.Timestamp == "" || entry.IsDelete ||
			entry.Value == nil || *entry.Value != want {
			t.Fatalf("entrada %d inesperada: %+v", index, entry)
		}
		if index > 0 && entry.TxID == history[index-1].TxID {
			t.Fatalf("identificadores historicos duplicados: %+v", history)
		}
	}
	unitHistory, err := store.GetUnitHistory(ctx, testGTIN, "LAB-CYCLE")
	if err != nil || len(unitHistory) == 0 ||
		history[2].TxID != unitHistory[len(unitHistory)-1].TxID {
		t.Fatalf("consumo y evento de unidad no comparten txId: %+v, %+v, %v",
			history[2], unitHistory, err)
	}
	_, err = store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-CYCLE", RevokeLabInterventionRequest{Motivo: "tardia"})
	requireCode(t, err, LabInterventionNotActive)

	scenarios := []struct {
		name       string
		authorized string
		operation  LabInterventionOperation
		actionKey  string
		expires    string
		revoke     bool
	}{
		{"revoked", "GLN:7791234500086", LabOpRestock, "lab2-key", "2026-09-09T12:00:00Z", true},
		{"expired", "GLN:7791234500086", LabOpRestock, "lab2-key", "2026-09-08T13:00:00Z", false},
		{"wrong-lab", "GLN:7791234500017", LabOpRestock, "lab2-key", "2026-09-09T12:00:00Z", false},
		{"wrong-operation", "GLN:7791234500086", LabOpWithdrawFromMarket, "lab2-key", "2026-09-09T12:00:00Z", false},
	}
	for index, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			serial := fmt.Sprintf("LAB-%02d", index)
			seedUnitAtState(t, pool, serial, domain.StateRetiradoMercado, "DistribuidorMSP", "", "2028-12-31")
			_, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, serial, AuthorizeLabInterventionRequest{
				Laboratorio: scenario.authorized, Operacion: scenario.operation,
				Motivo: "control", ExpiraEn: scenario.expires,
			})
			if err != nil {
				t.Fatal(err)
			}
			if scenario.revoke {
				view, err := store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, serial,
					RevokeLabInterventionRequest{Motivo: "cancelada"})
				if err != nil || view.Estado != LabInterventionRevoked || view.MotivoRevocacion != "cancelada" {
					t.Fatalf("revoke: %#v, %v", view, err)
				}
			}
			if scenario.name == "expired" {
				store.now = func() time.Time { return time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC) }
				defer func() {
					store.now = func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) }
				}()
			}
			_, err = store.Restock(ctx, credentials[scenario.actionKey], testGTIN, serial, UnitEventRequest{Motivo: "reingreso"})
			requireCode(t, err, LabInterventionRequired)
		})
	}
}

func TestLabInterventionHistoryAbsentRevokeAndDerivedExpiration(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	_, err := store.GetLabInterventionHistory(ctx, testGTIN, "NO-UNIT")
	requireCode(t, err, UnitNotFound)
	seedUnitAtState(t, pool, "LAB-HISTORY", domain.StateEnCustodia, "DistribuidorMSP", "", "2028-12-31")
	_, err = store.GetLabInterventionHistory(ctx, testGTIN, "LAB-HISTORY")
	requireCode(t, err, LabInterventionNotFound)

	first, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-HISTORY",
		AuthorizeLabInterventionRequest{
			Laboratorio: "GLN:7791234500086", Operacion: LabOpRestock,
			Motivo: "control", ExpiraEn: "2026-09-09T12:00:00Z",
		})
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	expiredHistory, err := store.GetLabInterventionHistory(ctx, testGTIN, "LAB-HISTORY")
	if err != nil || len(expiredHistory) != 1 || expiredHistory[0].Value == nil ||
		*expiredHistory[0].Value != first {
		t.Fatalf("vencimiento genero historial sintetico: %+v, %v", expiredHistory, err)
	}
	revoked, err := store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-HISTORY",
		RevokeLabInterventionRequest{Motivo: "sin efecto"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.GetLabInterventionHistory(ctx, testGTIN, "LAB-HISTORY")
	if err != nil || len(history) != 2 || history[1].Value == nil ||
		*history[1].Value != revoked || history[1].IsDelete {
		t.Fatalf("revocacion no reflejada: %+v, %v", history, err)
	}
}

func TestLabInterventionHistoryDoesNotBackfillPreviousAuthorization(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "LAB-PRIOR", domain.StateEnCustodia, "DistribuidorMSP", "", "2028-12-31")
	_, err := pool.Exec(ctx, `
		INSERT INTO public.lab_interventions
		(gtin, numero_serie, laboratorio, operacion, motivo, expira_en,
		 estado, emitida_por, emitida_en)
		VALUES ($1, $2, 'GLN:7791234500086', 'RESTOCK', 'anterior a 000004',
		        '2026-09-09T12:00:00Z', 'ACTIVA', 'AnmatMSP', '2026-09-08T11:00:00Z')`,
		testGTIN, "LAB-PRIOR")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.GetLabInterventionHistory(ctx, testGTIN, "LAB-PRIOR")
	requireCode(t, err, LabInterventionNotFound)

	replaced, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-PRIOR",
		AuthorizeLabInterventionRequest{
			Laboratorio: "GLN:7791234500086", Operacion: LabOpRestock,
			Motivo: "reemplazo posterior", ExpiraEn: "2026-09-10T12:00:00Z",
		})
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.GetLabInterventionHistory(ctx, testGTIN, "LAB-PRIOR")
	if err != nil || len(history) != 1 || history[0].Value == nil ||
		*history[0].Value != replaced {
		t.Fatalf("se fabricaron entradas anteriores: %+v, %v", history, err)
	}
}

func TestRestockRejectsExpiredUnitEvenWithAuthorization(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "LAB-EXPIRED", domain.StateRetiradoMercado, "DistribuidorMSP", "", "2026-09-07")
	_, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "LAB-EXPIRED", AuthorizeLabInterventionRequest{
		Laboratorio: "GLN:7791234500086", Operacion: LabOpRestock,
		Motivo: "control", ExpiraEn: "2026-09-09T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Restock(ctx, credentials["lab2-key"], testGTIN, "LAB-EXPIRED", UnitEventRequest{Motivo: "reingreso"})
	requireCode(t, err, InvalidStateTransition)
}
