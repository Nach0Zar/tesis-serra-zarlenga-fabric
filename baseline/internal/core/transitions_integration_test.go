package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func TestEveryExtendedTransitionAndActorUsesDomain(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	caseNumber := 0
	for _, transition := range domain.Transitions() {
		if !strings.HasPrefix(transition.ID, "T") || transition.ID < "T07" {
			continue
		}
		for _, actor := range transition.Actors {
			transition, actor := transition, actor
			t.Run(fmt.Sprintf("%s/%s", transition.ID, actor), func(t *testing.T) {
				caseNumber++
				from := transition.From[0]
				if actor == domain.ActorDestinationAgent {
					from = domain.StateEnTransito
				}
				custodianMSP := "DistribuidorMSP"
				if from == domain.StateEnLaboratorio {
					custodianMSP = "LabMSP"
				}
				serial := fmt.Sprintf("TX%03d", caseNumber)
				expiry := "2028-12-31"
				if transition.ID == transitionExpiredT13 {
					expiry = "2026-09-07"
				}
				seed := seedUnitAtState(t, pool, serial, from, custodianMSP, "FarmaciaMSP", expiry)

				var credential Credential
				switch actor {
				case domain.ActorANMAT:
					credential = credentials["reg-key"]
				case domain.ActorDestinationAgent:
					credential = credentials["pharmacy-key"]
				case domain.ActorLaboratory:
					credential = credentials["lab-key"]
					if custodianMSP != "LabMSP" {
						credential = credentials["lab2-key"]
						_, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, serial,
							AuthorizeLabInterventionRequest{
								Laboratorio: "GLN:7791234500086",
								Operacion:   labOperationForEvent(t, transition.Event),
								Motivo:      "intervencion de prueba",
								ExpiraEn:    "2026-09-09T12:00:00Z",
							})
						if err != nil {
							t.Fatal(err)
						}
					}
				case domain.ActorCurrentCustodian, domain.ActorRecoveryOrDisposalAgent:
					credential = credentials["distributor-key"]
					if custodianMSP == "LabMSP" {
						credential = credentials["lab-key"]
					}
				default:
					t.Fatalf("unexpected actor %s", actor)
				}

				updated, err := invokeExtendedEvent(ctx, store, credential, testGTIN, serial, transition.Event)
				if err != nil {
					t.Fatalf("%s failed: %v", transition.ID, err)
				}
				if updated.Estado != transition.To {
					t.Fatalf("expected domain destination %s, got %s", transition.To, updated.Estado)
				}
				if updated.CustodioActual != seed.CustodioActual {
					t.Fatalf("custody changed from %s to %s", seed.CustodioActual, updated.CustodioActual)
				}
				if from == domain.StateEnTransito {
					var state, reason string
					if err := pool.QueryRow(ctx, `SELECT estado,motivo_cierre FROM public.transfer_operations
						WHERE gtin=$1 AND numero_serie=$2`, testGTIN, serial).Scan(&state, &reason); err != nil {
						t.Fatal(err)
					}
					if state != "CERRADA" || reason != "EVENTO_EXTRAORDINARIO" {
						t.Fatalf("unexpected transfer closure: %s/%s", state, reason)
					}
				}
			})
		}
	}
}

func TestT13QuarantineAndT19TransitRules(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "T13Q", domain.StateEnCuarentena, "DistribuidorMSP", "", "2026-09-07")
	unit, err := store.ReportExpired(ctx, credentials["distributor-key"], testGTIN, "T13Q", UnitEventRequest{Motivo: "vencida"})
	if err != nil || unit.Estado != domain.StateVencido {
		t.Fatalf("T13 from quarantine: %#v, %v", unit, err)
	}

	seedUnitAtState(t, pool, "T19LAB", domain.StateEnTransito, "DistribuidorMSP", "FarmaciaMSP", "2028-12-31")
	_, err = store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "T19LAB", AuthorizeLabInterventionRequest{
		Laboratorio: "GLN:7791234500086", Operacion: LabOpWithdrawFromMarket,
		Motivo: "retiro", ExpiraEn: "2026-09-09T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.WithdrawFromMarket(ctx, credentials["lab2-key"], testGTIN, "T19LAB", UnitEventRequest{Motivo: "retiro"})
	requireCode(t, err, InvalidStateTransition)
	var activeState string
	if err := pool.QueryRow(ctx, `SELECT estado FROM public.transfer_operations WHERE gtin=$1 AND numero_serie=$2`, testGTIN, "T19LAB").Scan(&activeState); err != nil {
		t.Fatal(err)
	}
	if activeState != "ACTIVA" {
		t.Fatalf("rejected laboratory withdrawal closed transfer: %s", activeState)
	}
	unit, err = store.WithdrawFromMarket(ctx, credentials["reg-key"], testGTIN, "T19LAB", UnitEventRequest{Motivo: "retiro ANMAT"})
	if err != nil || unit.Estado != domain.StateRetiradoMercado {
		t.Fatalf("ANMAT T19: %#v, %v", unit, err)
	}
}

func TestEachExtendedOperationRejectsAnUndeclaredState(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	tests := []struct {
		name       string
		event      domain.Event
		credential Credential
	}{
		{"quarantine", domain.EventPonerEnCuarentena, credentials["pharmacy-key"]},
		{"release", domain.EventLiberarCuarentena, credentials["pharmacy-key"]},
		{"expired", domain.EventInformarVencimiento, credentials["pharmacy-key"]},
		{"stolen", domain.EventInformarRobo, credentials["pharmacy-key"]},
		{"lost", domain.EventInformarExtravio, credentials["pharmacy-key"]},
		{"damaged", domain.EventInformarDeterioro, credentials["pharmacy-key"]},
		{"return", domain.EventDevolverProducto, credentials["pharmacy-key"]},
		{"restock", domain.EventReingresarStock, credentials["pharmacy-key"]},
		{"withdraw", domain.EventRetirarMercado, credentials["lab2-key"]},
		{"prohibit", domain.EventProhibirProducto, credentials["reg-key"]},
		{"final", domain.EventDisponerFinal, credentials["pharmacy-key"]},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serial := fmt.Sprintf("BAD%02d", index)
			seedUnitAtState(t, pool, serial, domain.StateDispensado, "FarmaciaMSP", "", "2028-12-31")
			_, err := invokeExtendedEvent(ctx, store, test.credential, testGTIN, serial, test.event)
			requireCode(t, err, InvalidStateTransition)
		})
	}
	seedUnitAtState(t, pool, "PROLE", domain.StateEnCustodia, "DistribuidorMSP", "", "2028-12-31")
	_, err := store.ProhibitProduct(ctx, credentials["reg-audit-key"], testGTIN, "PROLE", UnitEventRequest{Motivo: "orden"})
	requireCode(t, err, RegulatoryOnly)
}
