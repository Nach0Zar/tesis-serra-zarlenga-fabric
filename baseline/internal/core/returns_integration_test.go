package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func TestReturnProductPersistenceAndSixReceiverValidations(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "RET-NONE", domain.StateEnCustodia, "FarmaciaMSP", "", "2028-12-31")
	withoutReceiver, err := store.ReturnProduct(ctx, credentials["pharmacy-key"], testGTIN, "RET-NONE", ReturnProductRequest{Motivo: "devolucion"})
	if err != nil || withoutReceiver.Estado != domain.StateDevuelto || withoutReceiver.CustodioActual != "GLN:7791234500048" {
		t.Fatalf("return without receiver: %#v, %v", withoutReceiver, err)
	}
	var absentReceiver *string
	if err := pool.QueryRow(ctx, `SELECT receptor_declarado FROM public.return_operations
		WHERE gtin=$1 AND numero_serie=$2`, testGTIN, "RET-NONE").Scan(&absentReceiver); err != nil {
		t.Fatal(err)
	}
	if absentReceiver != nil {
		t.Fatalf("return without receiver persisted %#v", absentReceiver)
	}

	seedUnitAtState(t, pool, "RET-WITH", domain.StateEnCustodia, "FarmaciaMSP", "", "2028-12-31")
	withReceiver, err := store.ReturnProduct(ctx, credentials["pharmacy-key"], testGTIN, "RET-WITH", ReturnProductRequest{
		Motivo: "devolucion", Receptor: "GLN:7791234500093",
	})
	if err != nil || withReceiver.CustodioActual != "GLN:7791234500048" {
		t.Fatalf("return with receiver: %#v, %v", withReceiver, err)
	}
	var receiver *string
	if err := pool.QueryRow(ctx, `SELECT receptor_declarado FROM public.return_operations
		WHERE gtin=$1 AND numero_serie=$2`, testGTIN, "RET-WITH").Scan(&receiver); err != nil {
		t.Fatal(err)
	}
	if receiver == nil || *receiver != "GLN:7791234500093" {
		t.Fatalf("unexpected persisted receiver: %#v", receiver)
	}
	if _, err := pool.Exec(ctx, `UPDATE public.return_operations SET motivo='alterado'
		WHERE gtin=$1 AND numero_serie=$2`, testGTIN, "RET-WITH"); err == nil {
		t.Fatal("return_operations accepted an update")
	}

	validations := []struct {
		name          string
		custodianMSP  string
		credentialKey string
		receiver      string
		code          Code
	}{
		{"format", "FarmaciaMSP", "pharmacy-key", "FarmaciaMSP", InvalidRequest},
		{"registered", "FarmaciaMSP", "pharmacy-key", "GLN:0000000000000", OrgNotRegistered},
		{"active", "FarmaciaMSP", "pharmacy-key", "GLN:7791234500062", OrgInactive},
		{"non-custodial-defensive", "FarmaciaMSP", "pharmacy-key", "REG:ANMAT", InvalidRequest},
		{"different", "FarmaciaMSP", "pharmacy-key", "GLN:7791234500048", InvalidDestination},
		{"reverse-matrix", "DistribuidorMSP", "distributor-key", "GLN:7791234500048", TransferNotAuthorized},
	}
	for index, test := range validations {
		t.Run(test.name, func(t *testing.T) {
			serial := fmt.Sprintf("RETV%02d", index)
			seedUnitAtState(t, pool, serial, domain.StateEnCustodia, test.custodianMSP, "", "2028-12-31")
			_, err := store.ReturnProduct(ctx, credentials[test.credentialKey], testGTIN, serial, ReturnProductRequest{
				Motivo: "devolucion", Receptor: test.receiver,
			})
			requireCode(t, err, test.code)
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.return_operations
				WHERE gtin=$1 AND numero_serie=$2`, testGTIN, serial).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("failed return persisted %d records", count)
			}
		})
	}
}
