package core

import (
	"context"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

const (
	labGLN      = "GLN:7791234500017"
	pharmacyGLN = "GLN:7791234500048"
)

// TestTransferRulesRejectBeforeWriting cubre las reglas que la baseline aplica
// en el despacho, la recepcion y el rechazo de una transferencia, igual que el
// chaincode: cada rechazo deja la unidad sin cambios.
func TestTransferRulesRejectBeforeWriting(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "CUSTODIA", domain.StateEnLaboratorio, "LabMSP", "", "2030-12-31")
	seedUnitAtState(t, pool, "TRANSITO", domain.StateEnTransito, "LabMSP", "FarmaciaMSP", "2030-12-31")
	dispatch := DispatchRequest{Destino: "FarmaciaMSP", NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1}

	_, err := store.Dispatch(ctx, credentials["lab-key"], "0779", "CUSTODIA", dispatch)
	requireCode(t, err, InvalidRequest)
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "NO-EXISTE", dispatch)
	requireCode(t, err, UnitNotFound)
	_, err = store.Dispatch(ctx, credentials["pharmacy-key"], testGTIN, "CUSTODIA", dispatch)
	requireCode(t, err, UnauthorizedCustodian)
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "CUSTODIA", DispatchRequest{NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1})
	requireCode(t, err, InvalidRequest)
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "CUSTODIA", DispatchRequest{Destino: "LabMSP", NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1})
	requireCode(t, err, InvalidDestination)
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "CUSTODIA", DispatchRequest{Destino: "GhostMSP", NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1})
	requireCode(t, err, OrgNotRegistered)
	_, err = store.Dispatch(ctx, credentials["lab-key"], testGTIN, "CUSTODIA", DispatchRequest{Destino: "FarmaciaMSP", NumeroRemito: "R"})
	requireCode(t, err, InvalidRequest)

	_, err = store.Receive(ctx, credentials["pharmacy-key"], testGTIN, "CUSTODIA", nil)
	requireCode(t, err, NotInTransit)
	_, err = store.Receive(ctx, credentials["lab-audit-key"], testGTIN, "TRANSITO", nil)
	requireCode(t, err, UnauthorizedRole)
	_, err = store.Receive(ctx, credentials["pharmacy-key"], "0779", "TRANSITO", nil)
	requireCode(t, err, InvalidRequest)

	_, err = store.Reject(ctx, credentials["pharmacy-key"], testGTIN, "TRANSITO", RejectRequest{})
	requireCode(t, err, InvalidRequest)
	_, err = store.Reject(ctx, credentials["pharmacy-key"], testGTIN, "CUSTODIA", RejectRequest{Motivo: "averiado"})
	requireCode(t, err, NotInTransit)
	_, err = store.Reject(ctx, credentials["health-key"], testGTIN, "TRANSITO", RejectRequest{Motivo: "averiado"})
	requireCode(t, err, ReceiverMismatch)
	_, err = store.Reject(ctx, credentials["lab-audit-key"], testGTIN, "TRANSITO", RejectRequest{Motivo: "averiado"})
	requireCode(t, err, UnauthorizedRole)

	unit, err := store.ReadUnit(ctx, testGTIN, "TRANSITO")
	if err != nil || unit.Estado != domain.StateEnTransito || unit.CustodioActual != labGLN {
		t.Fatalf("los rechazos modificaron la unidad: %+v, %v", unit, err)
	}
}

// TestDispenseRequiresAPharmacyCustodianAndAValidProduct: solo dispensa la
// farmacia o el centro medico custodio, y nunca un producto vencido.
func TestDispenseRequiresAPharmacyCustodianAndAValidProduct(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "EN-FARMACIA", domain.StateEnCustodia, "FarmaciaMSP", "", "2030-12-31")
	seedUnitAtState(t, pool, "VENCIDA", domain.StateEnCustodia, "FarmaciaMSP", "", "2026-01-31")
	seedUnitAtState(t, pool, "EN-DROGUERIA", domain.StateEnCustodia, "DrogueriaMSP", "", "2030-12-31")

	_, err := store.Dispense(ctx, credentials["drugstore-key"], testGTIN, "EN-DROGUERIA")
	requireCode(t, err, UnauthorizedAgentType)
	_, err = store.Dispense(ctx, credentials["health-key"], testGTIN, "EN-FARMACIA")
	requireCode(t, err, UnauthorizedCustodian)
	_, err = store.Dispense(ctx, credentials["pharmacy-key"], "0779", "EN-FARMACIA")
	requireCode(t, err, InvalidRequest)
	_, err = store.Dispense(ctx, credentials["pharmacy-key"], testGTIN, "NO-EXISTE")
	requireCode(t, err, UnitNotFound)
	_, err = store.Dispense(ctx, credentials["pharmacy-key"], testGTIN, "VENCIDA")
	requireCode(t, err, InvalidStateTransition)
	dispensed, err := store.Dispense(ctx, credentials["pharmacy-key"], testGTIN, "EN-FARMACIA")
	if err != nil || dispensed.Estado != domain.StateDispensado {
		t.Fatalf("dispensa valida: %+v, %v", dispensed, err)
	}
}

// TestUnitQueriesValidateIdentifiers: las lecturas rechazan identificadores
// mal formados y una unidad sin historial no existe.
func TestUnitQueriesValidateIdentifiers(t *testing.T) {
	store, _, _ := integrationStore(t)
	ctx := context.Background()
	_, err := store.ReadUnit(ctx, "0779", "S")
	requireCode(t, err, InvalidRequest)
	_, err = store.QueryUnitsByGTIN(ctx, "0779")
	requireCode(t, err, InvalidRequest)
	_, err = store.GetUnitHistory(ctx, testGTIN, "")
	requireCode(t, err, InvalidRequest)
	_, err = store.GetUnitHistory(ctx, testGTIN, "NO-EXISTE")
	requireCode(t, err, UnitNotFound)
	units, err := store.QueryUnitsByGTIN(ctx, testGTIN)
	if err != nil || len(units) != 0 {
		t.Fatalf("QueryUnitsByGTIN(sin unidades) = %v, %v", units, err)
	}
}

// TestLabInterventionAuthorizationRules: ANMAT autoriza a un laboratorio
// activo una operacion conocida, con motivo y vencimiento futuro.
func TestLabInterventionAuthorizationRules(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "INTERVENIDA", domain.StateEnCustodia, "FarmaciaMSP", "", "2030-12-31")
	valid := AuthorizeLabInterventionRequest{Laboratorio: labGLN, Operacion: LabOpRestock, Motivo: "control", ExpiraEn: "2026-12-31T00:00:00Z"}
	cases := []struct {
		mutate func(*AuthorizeLabInterventionRequest)
		code   Code
	}{
		{func(r *AuthorizeLabInterventionRequest) { r.Motivo = "" }, InvalidRequest},
		{func(r *AuthorizeLabInterventionRequest) { r.Laboratorio = pharmacyGLN }, InvalidLabIntervention},
		{func(r *AuthorizeLabInterventionRequest) { r.Laboratorio = "GLN:7791234500062" }, OrgInactive},
		{func(r *AuthorizeLabInterventionRequest) { r.Operacion = "BORRAR" }, InvalidLabIntervention},
		{func(r *AuthorizeLabInterventionRequest) { r.ExpiraEn = "mañana" }, InvalidLabIntervention},
		{func(r *AuthorizeLabInterventionRequest) { r.ExpiraEn = "2026-01-01T00:00:00Z" }, InvalidLabIntervention},
	}
	for _, tc := range cases {
		request := valid
		tc.mutate(&request)
		_, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "INTERVENIDA", request)
		requireCode(t, err, tc.code)
	}
	_, err := store.AuthorizeLabIntervention(ctx, credentials["lab-key"], testGTIN, "INTERVENIDA", valid)
	requireCode(t, err, RegulatoryOnly)
	_, err = store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "NO-EXISTE", valid)
	requireCode(t, err, UnitNotFound)

	_, err = store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, "INTERVENIDA", RevokeLabInterventionRequest{Motivo: "sin autorizacion"})
	requireCode(t, err, LabInterventionNotFound)
	_, err = store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, "INTERVENIDA", RevokeLabInterventionRequest{})
	requireCode(t, err, InvalidRequest)
	if _, err := store.AuthorizeLabIntervention(ctx, credentials["reg-key"], testGTIN, "INTERVENIDA", valid); err != nil {
		t.Fatal(err)
	}
	revoked, err := store.RevokeLabIntervention(ctx, credentials["reg-key"], testGTIN, "INTERVENIDA", RevokeLabInterventionRequest{Motivo: "cancelada"})
	if err != nil || revoked.Estado != LabInterventionRevoked {
		t.Fatalf("revocacion valida: %+v, %v", revoked, err)
	}
}

// TestOrganizationRegistryKeepsIdentitiesAndTheRegulatorUnique: el registro
// rechaza identidades repetidas, un segundo regulador activo y dejar la red
// sin regulador.
func TestOrganizationRegistryKeepsIdentitiesAndTheRegulatorUnique(t *testing.T) {
	store, credentials, _ := integrationStore(t)
	ctx := context.Background()
	_, err := store.RegisterOrganization(ctx, credentials["reg-key"], RegisterOrganizationRequest{
		MSPID: "LabMSP", ID: "7791234500079", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory, Active: true})
	requireCode(t, err, InvalidRequest)
	_, err = store.RegisterOrganization(ctx, credentials["reg-key"], RegisterOrganizationRequest{
		MSPID: "Anmat2MSP", ID: "ANMAT-2", IDType: IDTypeREG, AgentType: domain.AgentRegulator, Active: true})
	requireCode(t, err, InvalidRequest)
	_, err = store.RegisterOrganization(ctx, credentials["lab-key"], RegisterOrganizationRequest{
		MSPID: "NuevaMSP", ID: "7791234500079", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory, Active: true})
	requireCode(t, err, RegulatoryOnly)
	_, err = store.RegisterOrganization(ctx, credentials["reg-key"], RegisterOrganizationRequest{MSPID: "NuevaMSP", IDType: IDTypeGLN})
	requireCode(t, err, InvalidRequest)

	_, err = store.SetOrganizationActive(ctx, credentials["reg-key"], "", false)
	requireCode(t, err, InvalidRequest)
	_, err = store.SetOrganizationActive(ctx, credentials["reg-key"], "NoExisteMSP", false)
	requireCode(t, err, OrgNotRegistered)
	_, err = store.SetOrganizationActive(ctx, credentials["reg-key"], "AnmatMSP", false)
	requireCode(t, err, LastActiveRegulator)
	_, err = store.SetOrganizationActive(ctx, credentials["lab-key"], "FarmaciaMSP", false)
	requireCode(t, err, RegulatoryOnly)
	disabled, err := store.SetOrganizationActive(ctx, credentials["reg-key"], "FarmaciaMSP", false)
	if err != nil || disabled.Active {
		t.Fatalf("desactivar farmacia: %+v, %v", disabled, err)
	}
}

// TestExtraordinaryEventsAndReturnsValidateTheirInput: un evento o una
// devolucion sin motivo, sobre un identificador invalido o sobre una unidad
// inexistente se rechaza antes de escribir.
func TestExtraordinaryEventsAndReturnsValidateTheirInput(t *testing.T) {
	store, credentials, pool := integrationStore(t)
	ctx := context.Background()
	seedUnitAtState(t, pool, "EVENTO", domain.StateEnCustodia, "FarmaciaMSP", "", "2030-12-31")
	reason := UnitEventRequest{Motivo: "control"}

	_, err := store.Quarantine(ctx, credentials["reg-key"], "0779", "EVENTO", reason)
	requireCode(t, err, InvalidRequest)
	_, err = store.Quarantine(ctx, credentials["reg-key"], testGTIN, "EVENTO", UnitEventRequest{})
	requireCode(t, err, InvalidRequest)
	_, err = store.ReportStolen(ctx, credentials["pharmacy-key"], testGTIN, "NO-EXISTE", reason)
	requireCode(t, err, UnitNotFound)

	returnRequest := ReturnProductRequest{Receptor: "GLN:7791234500093", Motivo: "error de pedido"}
	_, err = store.ReturnProduct(ctx, credentials["financier-key"], testGTIN, "EVENTO", returnRequest)
	requireCode(t, err, UnauthorizedRole)
	_, err = store.ReturnProduct(ctx, credentials["pharmacy-key"], "0779", "EVENTO", returnRequest)
	requireCode(t, err, InvalidRequest)
	_, err = store.ReturnProduct(ctx, credentials["pharmacy-key"], testGTIN, "EVENTO", ReturnProductRequest{Receptor: returnRequest.Receptor})
	requireCode(t, err, InvalidRequest)
	_, err = store.ReturnProduct(ctx, credentials["pharmacy-key"], testGTIN, "NO-EXISTE", returnRequest)
	requireCode(t, err, UnitNotFound)
}
