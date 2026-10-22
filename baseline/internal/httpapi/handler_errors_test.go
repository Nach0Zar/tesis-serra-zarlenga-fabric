package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
)

type endpoint struct {
	method, path, body string
	authenticated      bool
	decodesJSON        bool
}

// endpoints recorre la API completa con un body valido para cada operacion.
var endpoints = []endpoint{
	{http.MethodPost, "/v1/units", `{}`, true, true},
	{http.MethodGet, "/v1/units?gtin=G", ``, false, false},
	{http.MethodGet, "/v1/units?estado=EN_CUSTODIA", ``, false, false},
	{http.MethodGet, "/v1/units/G/S", ``, false, false},
	{http.MethodGet, "/v1/units/G/S/history", ``, false, false},
	{http.MethodGet, "/v1/units/G/S/lab-intervention-history", ``, false, false},
	{http.MethodPost, "/v1/units/G/S/dispatch", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/receive", ``, true, false},
	{http.MethodPost, "/v1/units/G/S/reject", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/dispense", ``, true, false},
	{http.MethodPost, "/v1/units/G/S/quarantine", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/release-quarantine", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/report-expired", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/report-stolen", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/report-lost", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/report-damaged", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/return", `{"receptor":"GLN:AR:123"}`, true, true},
	{http.MethodPost, "/v1/units/G/S/restock", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/withdraw-from-market", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/prohibit-product", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/final-disposition", `{}`, true, true},
	{http.MethodPost, "/v1/units/G/S/authorize-lab-intervention", `{"laboratorio":"LabMSP"}`, true, true},
	{http.MethodPost, "/v1/units/G/S/revoke-lab-intervention", `{"motivo":"cancelada"}`, true, true},
	{http.MethodGet, "/v1/units/G/S/verify-unit", ``, true, false},
	{http.MethodGet, "/v1/units/G/S/verify-trace", ``, true, false},
	{http.MethodPost, "/v1/organizations", `{}`, true, true},
	{http.MethodPatch, "/v1/organizations/TestMSP", `{"active":false}`, true, true},
}

func serve(service *fakeService, logger *slog.Logger, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("X-Org-Key", "test-key")
	response := httptest.NewRecorder()
	New(service, logger).ServeHTTP(response, request)
	return response
}

// TestEveryEndpointMapsServiceErrorsToTheContract: un error tipificado del
// core llega con su codigo y su estado HTTP; uno no tipificado se informa
// como INTERNAL_ERROR, sin filtrar su texto, y queda en el log.
func TestEveryEndpointMapsServiceErrorsToTheContract(t *testing.T) {
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			typed := serve(&fakeService{err: core.NewError(core.UnitNotFound, "no existe")}, nil, ep.method, ep.path, ep.body)
			if typed.Code != http.StatusNotFound || !strings.Contains(typed.Body.String(), `"UNIT_NOT_FOUND"`) {
				t.Fatalf("error tipificado: %d %s", typed.Code, typed.Body.String())
			}

			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			internal := serve(&fakeService{err: errors.New("conexion perdida con postgres")}, logger, ep.method, ep.path, ep.body)
			if internal.Code != http.StatusInternalServerError || !strings.Contains(internal.Body.String(), `"INTERNAL_ERROR"`) {
				t.Fatalf("error interno: %d %s", internal.Code, internal.Body.String())
			}
			if strings.Contains(internal.Body.String(), "postgres") {
				t.Fatalf("el error interno filtra su causa: %s", internal.Body.String())
			}
			if !strings.Contains(logs.String(), "conexion perdida con postgres") {
				t.Fatalf("el error interno no quedo en el log: %q", logs.String())
			}
		})
	}
}

// TestAuthenticatedEndpointsStopOnAnInvalidKey: sin credencial valida el
// servicio no se invoca.
func TestAuthenticatedEndpointsStopOnAnInvalidKey(t *testing.T) {
	for _, ep := range endpoints {
		if !ep.authenticated {
			continue
		}
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			service := &fakeService{authErr: core.NewError(core.OrgNotRegistered, "clave desconocida")}
			response := serve(service, nil, ep.method, ep.path, ep.body)
			if response.Code != http.StatusForbidden || service.called != "" {
				t.Fatalf("status %d, servicio %q: %s", response.Code, service.called, response.Body.String())
			}
		})
	}
}

// TestJSONEndpointsRejectMalformedBodies: un body que no es un unico objeto
// JSON con campos conocidos no llega al servicio.
func TestJSONEndpointsRejectMalformedBodies(t *testing.T) {
	for _, ep := range endpoints {
		if !ep.decodesJSON {
			continue
		}
		for _, body := range []string{`no-json`, `{} {}`, `{"campoDesconocido":1}`} {
			t.Run(ep.path+" "+body, func(t *testing.T) {
				service := &fakeService{}
				response := serve(service, nil, ep.method, ep.path, body)
				if response.Code != http.StatusBadRequest || service.called != "" {
					t.Fatalf("status %d, servicio %q: %s", response.Code, service.called, response.Body.String())
				}
			})
		}
	}
}

// TestReceiveAcceptsOnlyOptionalCommercialData: la recepcion admite un body
// vacio o los datos comerciales, nunca otra cosa.
func TestReceiveAcceptsOnlyOptionalCommercialData(t *testing.T) {
	path := "/v1/units/G/S/receive"
	for body, called := range map[string]string{
		``:   "receive",
		`{}`: "receive",
		`{"numeroRemito":"R","numeroFactura":"F","cantidad":1}`: "receive-with-commercial",
	} {
		service := &fakeService{}
		if response := serve(service, nil, http.MethodPost, path, body); response.Code != http.StatusOK || service.called != called {
			t.Fatalf("body %q: status %d, servicio %q", body, response.Code, service.called)
		}
	}
	for _, body := range []string{`no-json`, `{} {}`, `{"paciente":"x"}`} {
		service := &fakeService{}
		if response := serve(service, nil, http.MethodPost, path, body); response.Code != http.StatusBadRequest || service.called != "" {
			t.Fatalf("body %q: status %d, servicio %q", body, response.Code, service.called)
		}
	}
}
