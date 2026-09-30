package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func TestQueryUnitsByStateCoversCatalogAndOrdersDeterministically(t *testing.T) {
	store, _, pool := integrationStore(t)
	ctx := context.Background()
	empty, err := store.QueryUnitsByState(ctx, domain.StateRobado)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty query: %#v, %v", empty, err)
	}
	for index, state := range domain.States() {
		serial := fmt.Sprintf("QS%02d", index)
		custodian := "DistribuidorMSP"
		if state == domain.StateEnLaboratorio {
			custodian = "LabMSP"
		}
		seedUnitAtState(t, pool, serial, state, custodian, "FarmaciaMSP", "2028-12-31")
	}
	seedUnitAtState(t, pool, "AA-ORDER", domain.StateEnCustodia, "DistribuidorMSP", "", "2028-12-31")
	seedUnitAtState(t, pool, "ZZ-ORDER", domain.StateEnCustodia, "DistribuidorMSP", "", "2028-12-31")
	for _, state := range domain.States() {
		units, err := store.QueryUnitsByState(ctx, state)
		if err != nil || len(units) == 0 {
			t.Fatalf("state %s: %#v, %v", state, units, err)
		}
	}
	ordered, err := store.QueryUnitsByState(ctx, domain.StateEnCustodia)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{"AA-ORDER", "QS02", "ZZ-ORDER"} {
		if ordered[index].NumeroSerie != want {
			t.Fatalf("ordered[%d]: expected %s, got %s", index, want, ordered[index].NumeroSerie)
		}
	}
	_, err = store.QueryUnitsByState(ctx, domain.State("DESCONOCIDO"))
	requireCode(t, err, InvalidRequest)
}
