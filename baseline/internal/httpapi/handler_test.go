package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
)

type fakeService struct{ called string }

func (f *fakeService) Authenticate(string) (core.Credential, error) {
	return core.Credential{MSPID: "TestMSP", Role: core.RoleOperator}, nil
}
func (f *fakeService) RegisterUnit(context.Context, core.Credential, core.RegisterUnitRequest) (core.MedicationUnit, error) {
	f.called = "register"
	return core.MedicationUnit{}, nil
}
func (f *fakeService) QueryUnitsByGTIN(context.Context, string) ([]core.MedicationUnit, error) {
	f.called = "query"
	return []core.MedicationUnit{}, nil
}
func (f *fakeService) ReadUnit(context.Context, string, string) (core.MedicationUnit, error) {
	f.called = "read"
	return core.MedicationUnit{}, nil
}
func (f *fakeService) GetUnitHistory(context.Context, string, string) ([]core.HistoryEntry, error) {
	f.called = "history"
	return []core.HistoryEntry{}, nil
}
func (f *fakeService) Dispatch(context.Context, core.Credential, string, string, core.DispatchRequest) (core.MedicationUnit, error) {
	f.called = "dispatch"
	return core.MedicationUnit{}, nil
}
func (f *fakeService) Receive(_ context.Context, _ core.Credential, _, _ string, data *core.CommercialData) (core.MedicationUnit, error) {
	if data != nil {
		f.called = "receive-with-commercial"
	} else {
		f.called = "receive"
	}
	return core.MedicationUnit{}, nil
}
func (f *fakeService) Reject(context.Context, core.Credential, string, string, core.RejectRequest) (core.MedicationUnit, error) {
	f.called = "reject"
	return core.MedicationUnit{}, nil
}
func (f *fakeService) Dispense(context.Context, core.Credential, string, string) (core.MedicationUnit, error) {
	f.called = "dispense"
	return core.MedicationUnit{}, nil
}
func (f *fakeService) RegisterOrganization(context.Context, core.Credential, core.RegisterOrganizationRequest) (core.Organization, error) {
	f.called = "register-org"
	return core.Organization{}, nil
}
func (f *fakeService) SetOrganizationActive(context.Context, core.Credential, string, bool) (core.Organization, error) {
	f.called = "set-org"
	return core.Organization{}, nil
}

func TestEveryCoreEndpointIsWired(t *testing.T) {
	tests := []struct {
		method, path, body, called string
		status                     int
	}{
		{http.MethodPost, "/v1/units", `{}`, "register", http.StatusCreated},
		{http.MethodGet, "/v1/units?gtin=07791234567898", ``, "query", http.StatusOK},
		{http.MethodGet, "/v1/units/07791234567898/SERIE", ``, "read", http.StatusOK},
		{http.MethodGet, "/v1/units/07791234567898/SERIE/history", ``, "history", http.StatusOK},
		{http.MethodPost, "/v1/units/07791234567898/SERIE/dispatch", `{}`, "dispatch", http.StatusOK},
		{http.MethodPost, "/v1/units/07791234567898/SERIE/receive", ``, "receive", http.StatusOK},
		{http.MethodPost, "/v1/units/07791234567898/SERIE/receive", `{"numeroRemito":"R","numeroFactura":"F","cantidad":1}`, "receive-with-commercial", http.StatusOK},
		{http.MethodPost, "/v1/units/07791234567898/SERIE/reject", `{}`, "reject", http.StatusOK},
		{http.MethodPost, "/v1/units/07791234567898/SERIE/dispense", ``, "dispense", http.StatusOK},
		{http.MethodPost, "/v1/organizations", `{}`, "register-org", http.StatusCreated},
		{http.MethodPatch, "/v1/organizations/TestMSP", `{}`, "set-org", http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.called+test.body, func(t *testing.T) {
			service := &fakeService{}
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("X-Org-Key", "test-key")
			response := httptest.NewRecorder()
			New(service, nil).ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, response.Code, response.Body.String())
			}
			if service.called != test.called {
				t.Fatalf("expected %s, got %s", test.called, service.called)
			}
		})
	}
}

func TestHTTPStatusMappingIsExhaustive(t *testing.T) {
	want := map[core.Code]int{
		core.InvalidRequest: 400, core.UnitNotFound: 404, core.UnitAlreadyExists: 409,
		core.InvalidStateTransition: 409, core.UnauthorizedCustodian: 403,
		core.UnauthorizedRole: 403, core.UnauthorizedAgentType: 403,
		core.OrgNotRegistered: 403, core.OrgInactive: 403,
		core.TransferNotAuthorized: 403, core.InvalidDestination: 400,
		core.NotInTransit: 409, core.ReceiverMismatch: 403, core.RegulatoryOnly: 403,
		core.LastActiveRegulator: 409, core.AlreadyInitialized: 409,
		core.InvalidLabIntervention: 400, core.LabInterventionNotFound: 404,
		core.LabInterventionNotActive: 409, core.LabInterventionRequired: 403,
		core.InternalError: 500,
	}
	for code, status := range want {
		if actual := HTTPStatusForCode(code); actual != status {
			t.Errorf("%s: expected %d, got %d", code, status, actual)
		}
	}
}

type unauthenticatedReadService struct{ fakeService }

func (f *unauthenticatedReadService) Authenticate(string) (core.Credential, error) {
	panic("read endpoint must not authenticate")
}

func TestReadEndpointsDoNotRequireAPIKey(t *testing.T) {
	service := &unauthenticatedReadService{}
	request := httptest.NewRequest(http.MethodGet, "/v1/units/07791234567898/SERIE", nil)
	response := httptest.NewRecorder()
	New(service, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected response %d: %s", response.Code, body)
	}
}

func TestDispenseRejectsPatientOrOtherBody(t *testing.T) {
	service := &fakeService{}
	request := httptest.NewRequest(http.MethodPost, "/v1/units/07791234567898/SERIE/dispense", strings.NewReader(`{"paciente":"dato-no-admitido"}`))
	request.Header.Set("X-Org-Key", "test-key")
	response := httptest.NewRecorder()
	New(service, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
	if service.called != "" {
		t.Fatalf("dispense service was called: %s", service.called)
	}
}
