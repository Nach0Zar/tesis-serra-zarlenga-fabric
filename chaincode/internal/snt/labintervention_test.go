package snt

import (
	"encoding/json"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/chaincode/internal/cerr"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

// seedUnit escribe directamente el estado publico de una unidad. Los tests de
// CC-1 lo usan como precondicion; el alta real (T01) es de CC-2 (#15).
func seedUnit(t *testing.T, stub *mockStub, estado domain.State, custodio string) {
	t.Helper()
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
	_, err := putUnit(ctx, MedicationUnit{
		GTIN:                validGTIN,
		NumeroSerie:         validSerial,
		Lote:                "L2026-014",
		FechaVencimiento:    "2027-12-31",
		CustodioActual:      custodio,
		Estado:              estado,
		UltimaActualizacion: "2026-08-27T12:00:00Z",
	})
	requireNoError(t, err)
}

func labInterventionFixture(t *testing.T) (*mockStub, *SNTContract) {
	t.Helper()
	stub := newMockStub()
	seedRegistry(t, stub)
	registerOrg(t, stub, labMSP, labGLN, domain.AgentLaboratory)
	registerOrg(t, stub, drogueriaMSP, drogueriaGLN, domain.AgentDrugstore)
	seedUnit(t, stub, domain.StateEnCustodia, "GLN:"+drogueriaGLN)
	return stub, new(SNTContract)
}

func validAuthorizationRequest() AuthorizeLabInterventionRequest {
	return AuthorizeLabInterventionRequest{
		GTIN:        validGTIN,
		NumeroSerie: validSerial,
		Laboratorio: "GLN:" + labGLN,
		Operacion:   LabOpWithdrawFromMarket,
		Motivo:      "Retiro de lote solicitado por el titular, expediente ANMAT 1234/2026.",
		ExpiraEn:    "2026-09-30T00:00:00Z",
	}
}

func TestAuthorizeLabInterventionHappyPath(t *testing.T) {
	stub, contract := labInterventionFixture(t)

	view, err := contract.AuthorizeLabIntervention(
		testContext(stub, anmatMSP, RoleRegulatoryAdmin), validAuthorizationRequest())
	requireNoError(t, err)

	if view.Estado != LabInterventionActiva {
		t.Fatalf("estado de la autorizacion = %s", view.Estado)
	}
	if view.EmitidaPor != anmatMSP || view.EmitidaEn == "" {
		t.Fatalf("la autorizacion no registra emisor y timestamp: %+v", view)
	}

	// La SBE de la clave de autorizacion es de la organizacion regulatoria
	// SOLAMENTE, no conjunta con el laboratorio (ADR-007, punto 6.f).
	key, err := labInterventionKey(stub, validGTIN, validSerial)
	requireNoError(t, err)
	if len(stub.validation[key]) == 0 {
		t.Fatal("no se fijo la politica de endoso por clave de la autorizacion")
	}
}

func TestAuthorizeLabInterventionRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(req *AuthorizeLabInterventionRequest)
		invoker string
		role    string
		want    cerr.Code
	}{
		{
			name:    "invocador no regulatorio",
			invoker: labMSP, role: RoleOperator,
			want: cerr.RegulatoryOnly,
		},
		{
			name:    "regulador sin rol regulatory-admin",
			invoker: anmatMSP, role: RoleAuditor,
			want: cerr.RegulatoryOnly,
		},
		{
			name:   "GTIN con digito verificador invalido",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.GTIN = "07791234567890" },
			want:   cerr.InvalidRequest,
		},
		{
			name:   "motivo ausente",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.Motivo = "" },
			want:   cerr.InvalidRequest,
		},
		{
			name:   "unidad inexistente",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.NumeroSerie = "SN-9999-ZZZZ" },
			want:   cerr.UnitNotFound,
		},
		{
			name:   "laboratorio no registrado",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.Laboratorio = "GLN:7791234500055" },
			want:   cerr.OrgNotRegistered,
		},
		{
			name:   "identificador de laboratorio mal formado",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.Laboratorio = labGLN },
			want:   cerr.InvalidRequest,
		},
		{
			name:   "el designado no es un laboratorio",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.Laboratorio = "GLN:" + drogueriaGLN },
			want:   cerr.InvalidLabIntervention,
		},
		{
			name:   "operacion fuera del catalogo",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.Operacion = "DISPENSE" },
			want:   cerr.InvalidLabIntervention,
		},
		{
			name:   "expiraEn no es ISO 8601",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.ExpiraEn = "30/09/2026" },
			want:   cerr.InvalidLabIntervention,
		},
		{
			// Es esta validacion la que hace imposible "revocar" reemitiendo la
			// autorizacion vencida, y por eso existe RevokeLabIntervention.
			name:   "expiraEn anterior al timestamp de la transaccion",
			mutate: func(r *AuthorizeLabInterventionRequest) { r.ExpiraEn = "2026-01-01T00:00:00Z" },
			want:   cerr.InvalidLabIntervention,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, contract := labInterventionFixture(t)
			req := validAuthorizationRequest()
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			invoker, role := anmatMSP, RoleRegulatoryAdmin
			if tc.invoker != "" {
				invoker, role = tc.invoker, tc.role
			}
			_, err := contract.AuthorizeLabIntervention(testContext(stub, invoker, role), req)
			requireCode(t, err, tc.want)
		})
	}
}

// TestAuthorizeLabInterventionRejectsInactiveLab cubre la validacion de
// habilitacion del laboratorio designado.
func TestAuthorizeLabInterventionRejectsInactiveLab(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	regulatorCtx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.SetOrganizationActive(regulatorCtx,
		SetOrganizationActiveRequest{MSPID: labMSP, Active: false})
	requireNoError(t, err)

	_, err = contract.AuthorizeLabIntervention(regulatorCtx, validAuthorizationRequest())
	requireCode(t, err, cerr.OrgInactive)
}

// TestAuthorizeLabInterventionReplacesPrevious verifica que una autorizacion
// nueva sobre la misma unidad reemplace a la anterior: la clave es UNA por
// unidad (contrato DES-5).
func TestAuthorizeLabInterventionReplacesPrevious(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
	requireNoError(t, err)

	second := validAuthorizationRequest()
	second.Operacion = LabOpFinalDisposition
	view, err := contract.AuthorizeLabIntervention(ctx, second)
	requireNoError(t, err)
	if view.Operacion != LabOpFinalDisposition {
		t.Fatalf("la autorizacion no fue reemplazada: %+v", view)
	}

	stored, found, err := readLabIntervention(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if !found || stored.Operacion != LabOpFinalDisposition {
		t.Fatalf("la clave conserva la autorizacion anterior: %+v", stored)
	}
}

// --- RevokeLabIntervention (ADR-007, punto 6.f) -----------------------------

func TestRevokeLabInterventionHappyPath(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
	requireNoError(t, err)

	view, err := contract.RevokeLabIntervention(ctx, RevokeLabInterventionRequest{
		GTIN: validGTIN, NumeroSerie: validSerial,
		Motivo: "Autorizacion emitida sobre la unidad equivocada.",
	})
	requireNoError(t, err)

	if view.Estado != LabInterventionRevoked || view.RevocadaEn == "" || view.MotivoRevocacion == "" {
		t.Fatalf("la revocacion no quedo registrada: %+v", view)
	}

	// Se CONSERVA la clave: la traza de emision y revocacion es evidencia
	// auditable, y borrarla devolveria la proxima autorizacion a la ventana de
	// creacion de una clave nueva.
	stored, found, err := readLabIntervention(ctx, validGTIN, validSerial)
	requireNoError(t, err)
	if !found || stored.Estado != LabInterventionRevoked {
		t.Fatalf("la clave de autorizacion no se conservo revocada: %+v", stored)
	}

	// Marcador regulatorio de la revocacion.
	markerFound := false
	for _, raw := range stub.privateData[implicitCollection(anmatMSP)] {
		var marker participationMarker
		if err := json.Unmarshal(raw, &marker); err == nil && marker.Operacion == opRevokeLabIntervention {
			markerFound = true
		}
	}
	if !markerFound {
		t.Fatal("RevokeLabIntervention no escribio el marcador regulatorio")
	}
}

func TestRevokeLabInterventionRejections(t *testing.T) {
	t.Run("autorizacion inexistente", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		_, err := contract.RevokeLabIntervention(testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
		requireCode(t, err, cerr.LabInterventionNotFound)
	})

	t.Run("unidad inexistente", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		_, err := contract.RevokeLabIntervention(testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: "SN-9999-ZZZZ", Motivo: "x"})
		requireCode(t, err, cerr.UnitNotFound)
	})

	t.Run("motivo ausente", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		_, err := contract.RevokeLabIntervention(testContext(stub, anmatMSP, RoleRegulatoryAdmin),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial})
		requireCode(t, err, cerr.InvalidRequest)
	})

	t.Run("invocador no regulatorio", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		_, err := contract.RevokeLabIntervention(testContext(stub, labMSP, RoleOperator),
			RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"})
		requireCode(t, err, cerr.RegulatoryOnly)
	})

	// La revocacion no es idempotente y no reabre una autorizacion cerrada.
	t.Run("autorizacion ya revocada", func(t *testing.T) {
		stub, contract := labInterventionFixture(t)
		ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)
		_, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
		requireNoError(t, err)

		req := RevokeLabInterventionRequest{GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "x"}
		_, err = contract.RevokeLabIntervention(ctx, req)
		requireNoError(t, err)

		_, err = contract.RevokeLabIntervention(ctx, req)
		requireCode(t, err, cerr.LabInterventionNotActive)
	})
}

// TestRevokeExpiredButActiveAuthorization deja fijado que una autorizacion
// VENCIDA pero ACTIVA si puede revocarse: cierra el registro de forma explicita
// en lugar de dejarlo en un estado que solo el timestamp desambigua.
func TestRevokeExpiredButActiveAuthorization(t *testing.T) {
	stub, contract := labInterventionFixture(t)
	ctx := testContext(stub, anmatMSP, RoleRegulatoryAdmin)

	_, err := contract.AuthorizeLabIntervention(ctx, validAuthorizationRequest())
	requireNoError(t, err)

	// El vencimiento es una condicion DERIVADA: no se persiste y no borra la
	// clave. Avanzar el reloj de la transaccion la deja vencida pero ACTIVA.
	stub.timestamp = stub.timestamp.AddDate(1, 0, 0)

	view, err := contract.RevokeLabIntervention(ctx, RevokeLabInterventionRequest{
		GTIN: validGTIN, NumeroSerie: validSerial, Motivo: "Cierre administrativo.",
	})
	requireNoError(t, err)
	if view.Estado != LabInterventionRevoked {
		t.Fatalf("una autorizacion vencida pero ACTIVA debe poder revocarse: %+v", view)
	}
}
