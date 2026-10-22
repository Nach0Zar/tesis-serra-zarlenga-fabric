package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func requireRejection(t *testing.T, err error, code Code, contains string) {
	t.Helper()
	requireCode(t, err, code)
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %q, want %q", err.Error(), contains)
	}
}

// TestValidateRegisterUnitRequestAppliesTheGS1Rules cubre las reglas de
// identificacion del SNT que la baseline aplica igual que el chaincode.
func TestValidateRegisterUnitRequestAppliesTheGS1Rules(t *testing.T) {
	valid := RegisterUnitRequest{GTIN: "07791234567898", NumeroSerie: "SN-1", Lote: "L1", FechaVencimiento: "2030-12-31"}
	if err := ValidateRegisterUnitRequest(valid); err != nil {
		t.Fatalf("request valido rechazado: %v", err)
	}
	cases := []struct {
		name     string
		mutate   func(*RegisterUnitRequest)
		contains string
	}{
		{"GTIN corto", func(r *RegisterUnitRequest) { r.GTIN = "0779123456789" }, "14 digitos"},
		{"GTIN no numerico", func(r *RegisterUnitRequest) { r.GTIN = "0779123456789X" }, "14 digitos"},
		{"verificador", func(r *RegisterUnitRequest) { r.GTIN = "07791234567890" }, "digito verificador"},
		{"serie vacia", func(r *RegisterUnitRequest) { r.NumeroSerie = "" }, "obligatorio"},
		{"serie larga", func(r *RegisterUnitRequest) { r.NumeroSerie = strings.Repeat("A", 21) }, "20 caracteres"},
		{"fuera de GS1 82", func(r *RegisterUnitRequest) { r.NumeroSerie = "SERIE#1" }, "GS1 82"},
		{"20 caracteres con 779", func(r *RegisterUnitRequest) { r.NumeroSerie = "779" + strings.Repeat("1", 17) }, `"779"`},
		{"lote vacio", func(r *RegisterUnitRequest) { r.Lote = "" }, "lote"},
		{"vencimiento", func(r *RegisterUnitRequest) { r.FechaVencimiento = "31/12/2030" }, "ISO 8601"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := valid
			tc.mutate(&request)
			requireRejection(t, ValidateRegisterUnitRequest(request), InvalidRequest, tc.contains)
		})
	}
	if err := ValidateRegisterUnitRequest(RegisterUnitRequest{GTIN: valid.GTIN, NumeroSerie: "779" + strings.Repeat("1", 16),
		Lote: "L1", FechaVencimiento: "2030-12-31"}); err != nil {
		t.Fatalf("una serie de 19 caracteres puede empezar con 779: %v", err)
	}
}

// TestValidateOrganizationKeepsIdentifierAndAgentTypeConsistent fija la
// relacion entre agentType e idType del registro (ADR-003, ADR-010).
func TestValidateOrganizationKeepsIdentifierAndAgentTypeConsistent(t *testing.T) {
	valid := []RegisterOrganizationRequest{
		{MSPID: "Lab2MSP", ID: "7791234500086", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory},
		{MSPID: "HospitalMSP", ID: "7791234500055", IDType: IDTypeCUFE, AgentType: domain.AgentHealthcare},
		{MSPID: "OSDEMSP", ID: "OSDE-1", IDType: IDTypeREG, AgentType: domain.AgentFinancier},
	}
	for _, request := range valid {
		if err := validateOrganization(request); err != nil {
			t.Fatalf("%+v rechazada: %v", request, err)
		}
	}
	invalid := []struct {
		request  RegisterOrganizationRequest
		contains string
	}{
		{RegisterOrganizationRequest{ID: "7791234500086", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory}, "mspId"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "7791234500086", IDType: IDTypeGLN, AgentType: "PANADERIA"}, "fuera del catalogo"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "7791234500086", IDType: IDTypeGLN, AgentType: domain.AgentFinancier}, "custodial"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "7791234500087", IDType: IDTypeGLN, AgentType: domain.AgentLaboratory}, "13 digitos"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "OSDE", IDType: IDTypeREG, AgentType: domain.AgentLaboratory}, "no custodial"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "osde minuscula", IDType: IDTypeREG, AgentType: domain.AgentFinancier}, "slug"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "", IDType: IDTypeREG, AgentType: domain.AgentRegulator}, "slug"},
		{RegisterOrganizationRequest{MSPID: "X", ID: "7791234500086", IDType: "CUIT", AgentType: domain.AgentLaboratory}, "idType"},
	}
	for _, tc := range invalid {
		requireRejection(t, validateOrganization(tc.request), InvalidRequest, tc.contains)
	}
}

// TestParseCanonicalIDAcceptsOnlyGLNOrCUFE: el destino de una transferencia
// se expresa como GLN:<id> o CUFE:<id> con verificador valido.
func TestParseCanonicalIDAcceptsOnlyGLNOrCUFE(t *testing.T) {
	idType, id, err := parseCanonicalID("GLN:7791234500048")
	if err != nil || idType != IDTypeGLN || id != "7791234500048" {
		t.Fatalf("parseCanonicalID(GLN) = %q, %q, %v", idType, id, err)
	}
	for _, value := range []string{"7791234500048", "REG:ANMAT", "GLN:779123450004", "CUFE:7791234500049"} {
		requireRejection(t, func() error { _, _, err := parseCanonicalID(value); return err }(), InvalidRequest, "identificador canonico")
	}
}

// TestCommercialDataRequiresEveryDocumentField: remito, factura y cantidad
// positiva son obligatorios cuando se informan.
func TestCommercialDataRequiresEveryDocumentField(t *testing.T) {
	if err := validateCommercial(CommercialData{NumeroRemito: "R", NumeroFactura: "F", Cantidad: 1}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []CommercialData{
		{NumeroFactura: "F", Cantidad: 1}, {NumeroRemito: "R", Cantidad: 1}, {NumeroRemito: "R", NumeroFactura: "F"},
	} {
		requireRejection(t, validateCommercial(data), InvalidRequest, "datos documentales")
	}
}

// TestContractErrorsKeepTheCauseOutOfTheMessage: el error interno conserva la
// causa para los logs pero no la expone en su mensaje.
func TestContractErrorsKeepTheCauseOutOfTheMessage(t *testing.T) {
	cause := errors.New("pq: conexion rechazada")
	wrapped := internal(cause, "leer unidad")
	if !errors.Is(wrapped, cause) {
		t.Fatal("el error interno no conserva su causa")
	}
	if strings.Contains(wrapped.Error(), "conexion rechazada") {
		t.Fatalf("el mensaje expone la causa: %q", wrapped.Error())
	}
	if code, ok := ErrorCode(errors.Join(errors.New("contexto"), wrapped)); !ok || code != InternalError {
		t.Fatalf("ErrorCode(envuelto) = %q, %v", code, ok)
	}
	if _, ok := ErrorCode(cause); ok {
		t.Fatal("un error comun no tiene codigo del contrato")
	}
	original := NewError(UnitNotFound, "no existe")
	detailed := original.WithDetails(map[string]any{"gtin": "G"})
	if original.Details != nil || detailed.Details["gtin"] != "G" {
		t.Fatal("WithDetails debe devolver una copia")
	}
}
