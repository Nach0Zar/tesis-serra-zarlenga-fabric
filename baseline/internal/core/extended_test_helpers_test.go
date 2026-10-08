package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedUnitAtState(t *testing.T, pool *pgxpool.Pool, serial string, state domain.State, custodianMSP, destinationMSP, expiry string) MedicationUnit {
	t.Helper()
	ctx := context.Background()
	var idType, id string
	var custodianType domain.AgentType
	if err := pool.QueryRow(ctx, `SELECT id_type,id,agent_type FROM public.organizations WHERE msp_id=$1`, custodianMSP).
		Scan(&idType, &id, &custodianType); err != nil {
		t.Fatal(err)
	}
	custodian := idType + ":" + id
	unit := MedicationUnit{
		GTIN: testGTIN, NumeroSerie: serial, Lote: "LOT-" + serial, FechaVencimiento: expiry,
		CustodioActual: custodian, Estado: state, UltimaActualizacion: "2026-09-08T11:00:00Z",
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO public.medication_units
		(gtin,numero_serie,lote,fecha_vencimiento,custodio_actual,estado,ultima_actualizacion)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		unit.GTIN, unit.NumeroSerie, unit.Lote, unit.FechaVencimiento, unit.CustodioActual,
		unit.Estado, unit.UltimaActualizacion)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO public.unit_events
		(gtin,numero_serie,tx_id,event_timestamp,event_sequence,operation,invoker_msp_id,
		 lote,fecha_vencimiento,custodio_actual,estado,ultima_actualizacion)
		VALUES ($1,$2,$8,$7,1,'TestSeed',$9,$3,$4,$5,$6,$7)`,
		unit.GTIN, unit.NumeroSerie, unit.Lote, unit.FechaVencimiento, unit.CustodioActual,
		unit.Estado, unit.UltimaActualizacion, "seed-"+serial, custodianMSP)
	if err != nil {
		t.Fatal(err)
	}
	if state != domain.StateEnTransito {
		return unit
	}
	if destinationMSP == "" {
		destinationMSP = "FarmaciaMSP"
	}
	var destinationIDType, destinationID string
	var destinationType domain.AgentType
	if err := pool.QueryRow(ctx, `SELECT id_type,id,agent_type FROM public.organizations WHERE msp_id=$1`, destinationMSP).
		Scan(&destinationIDType, &destinationID, &destinationType); err != nil {
		t.Fatal(err)
	}
	decision, err := domain.DecideTransfer(custodianType, destinationType)
	if err != nil || !decision.Allowed {
		t.Fatalf("test transfer %s -> %s is not allowed: %#v, %v", custodianType, destinationType, decision, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO public.transfer_operations
		(gtin,numero_serie,tx_id_despacho,emisor,destinatario_pendiente,numero_remito,
		 numero_factura,cantidad,rule_id,schema_version,despachada_en,estado)
		VALUES ($1,$2,$3,$4,$5,'R-TEST','F-TEST',1,$6,$7,$8,'ACTIVA')`,
		unit.GTIN, unit.NumeroSerie, "dispatch-"+serial, custodian,
		destinationIDType+":"+destinationID, decision.RuleID, decision.SchemaVersion,
		unit.UltimaActualizacion)
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

func invokeExtendedEvent(ctx context.Context, store *Store, credential Credential, gtin, serial string, event domain.Event) (MedicationUnit, error) {
	req := UnitEventRequest{Motivo: "motivo de prueba"}
	switch event {
	case domain.EventPonerEnCuarentena:
		return store.Quarantine(ctx, credential, gtin, serial, req)
	case domain.EventLiberarCuarentena:
		return store.ReleaseQuarantine(ctx, credential, gtin, serial, req)
	case domain.EventInformarVencimiento:
		return store.ReportExpired(ctx, credential, gtin, serial, req)
	case domain.EventInformarRobo:
		return store.ReportStolen(ctx, credential, gtin, serial, req)
	case domain.EventInformarExtravio:
		return store.ReportLost(ctx, credential, gtin, serial, req)
	case domain.EventInformarDeterioro:
		return store.ReportDamaged(ctx, credential, gtin, serial, req)
	case domain.EventDevolverProducto:
		return store.ReturnProduct(ctx, credential, gtin, serial, ReturnProductRequest{Motivo: req.Motivo})
	case domain.EventReingresarStock:
		return store.Restock(ctx, credential, gtin, serial, req)
	case domain.EventRetirarMercado:
		return store.WithdrawFromMarket(ctx, credential, gtin, serial, req)
	case domain.EventProhibirProducto:
		return store.ProhibitProduct(ctx, credential, gtin, serial, req)
	case domain.EventDisponerFinal:
		return store.FinalDisposition(ctx, credential, gtin, serial, req)
	default:
		return MedicationUnit{}, fmt.Errorf("unsupported test event %s", event)
	}
}

func labOperationForEvent(t *testing.T, event domain.Event) LabInterventionOperation {
	t.Helper()
	switch event {
	case domain.EventRetirarMercado:
		return LabOpWithdrawFromMarket
	case domain.EventReingresarStock:
		return LabOpRestock
	case domain.EventDisponerFinal:
		return LabOpFinalDisposition
	default:
		t.Fatalf("event %s has no lab intervention operation", event)
		return ""
	}
}

func readLabInterventionForTest(ctx context.Context, store *Store, gtin, serial string) (LabInterventionView, bool, error) {
	tx, err := store.begin(ctx)
	if err != nil {
		return LabInterventionView{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return readLabIntervention(ctx, tx, gtin, serial, false)
}
