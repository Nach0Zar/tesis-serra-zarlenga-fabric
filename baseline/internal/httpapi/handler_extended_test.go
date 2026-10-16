package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

func (f *fakeService) QueryUnitsByState(context.Context, domain.State) ([]core.MedicationUnit, error) {
	f.called = "query-state"
	return []core.MedicationUnit{}, nil
}

func (f *fakeService) Quarantine(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "quarantine"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReleaseQuarantine(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "release-quarantine"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReportExpired(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "report-expired"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReportStolen(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "report-stolen"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReportLost(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "report-lost"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReportDamaged(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "report-damaged"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ReturnProduct(context.Context, core.Credential, string, string, core.ReturnProductRequest) (core.MedicationUnit, error) {
	f.called = "return"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) Restock(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "restock"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) WithdrawFromMarket(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "withdraw-from-market"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) ProhibitProduct(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "prohibit-product"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) FinalDisposition(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error) {
	f.called = "final-disposition"
	return core.MedicationUnit{}, nil
}

func (f *fakeService) AuthorizeLabIntervention(context.Context, core.Credential, string, string, core.AuthorizeLabInterventionRequest) (core.LabInterventionView, error) {
	f.called = "authorize-lab-intervention"
	return core.LabInterventionView{}, nil
}

func (f *fakeService) RevokeLabIntervention(context.Context, core.Credential, string, string, core.RevokeLabInterventionRequest) (core.LabInterventionView, error) {
	f.called = "revoke-lab-intervention"
	return core.LabInterventionView{}, nil
}

func (f *fakeService) VerifyUnit(context.Context, core.Credential, string, string) (core.UnitVerdict, error) {
	f.called = "verify-unit"
	return core.UnitVerdict{}, nil
}

func (f *fakeService) VerifyTrace(context.Context, core.Credential, string, string) (core.TraceVerdict, error) {
	f.called = "verify-trace"
	return core.TraceVerdict{}, nil
}

func TestExtendedEndpointsAreWired(t *testing.T) {
	tests := []struct {
		method string
		path   string
		body   string
		called string
	}{
		{http.MethodGet, "/v1/units?estado=EN_CUSTODIA", "", "query-state"},
		{http.MethodPost, "/v1/units/G/S/quarantine", `{}`, "quarantine"},
		{http.MethodPost, "/v1/units/G/S/release-quarantine", `{}`, "release-quarantine"},
		{http.MethodPost, "/v1/units/G/S/report-expired", `{}`, "report-expired"},
		{http.MethodPost, "/v1/units/G/S/report-stolen", `{}`, "report-stolen"},
		{http.MethodPost, "/v1/units/G/S/report-lost", `{}`, "report-lost"},
		{http.MethodPost, "/v1/units/G/S/report-damaged", `{}`, "report-damaged"},
		{http.MethodPost, "/v1/units/G/S/return", `{"receptor":"GLN:AR:123"}`, "return"},
		{http.MethodPost, "/v1/units/G/S/restock", `{}`, "restock"},
		{http.MethodPost, "/v1/units/G/S/withdraw-from-market", `{}`, "withdraw-from-market"},
		{http.MethodPost, "/v1/units/G/S/prohibit-product", `{}`, "prohibit-product"},
		{http.MethodPost, "/v1/units/G/S/final-disposition", `{}`, "final-disposition"},
		{http.MethodPost, "/v1/units/G/S/authorize-lab-intervention", `{"laboratorio":"LabMSP","operacion":"RESTOCK","motivo":"control","expiraEn":"2030-01-01T00:00:00Z"}`, "authorize-lab-intervention"},
		{http.MethodPost, "/v1/units/G/S/revoke-lab-intervention", `{"motivo":"cancelada"}`, "revoke-lab-intervention"},
		{http.MethodGet, "/v1/units/G/S/verify-unit", "", "verify-unit"},
		{http.MethodGet, "/v1/units/G/S/verify-trace", "", "verify-trace"},
	}
	for _, test := range tests {
		t.Run(test.called, func(t *testing.T) {
			service := &fakeService{}
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("X-Org-Key", "test-key")
			response := httptest.NewRecorder()
			New(service, nil).ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
			}
			if service.called != test.called {
				t.Fatalf("expected %s, got %s", test.called, service.called)
			}
		})
	}
}

func TestQueryUnitsRequiresExactlyOneFilter(t *testing.T) {
	for _, path := range []string{
		"/v1/units",
		"/v1/units?gtin=G&estado=EN_CUSTODIA",
	} {
		service := &fakeService{}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		New(service, nil).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", path, response.Code)
		}
		if service.called != "" {
			t.Errorf("%s: service called as %s", path, service.called)
		}
	}
}
