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
