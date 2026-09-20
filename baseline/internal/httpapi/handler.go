package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	store  Service
	logger *slog.Logger
}

type Service interface {
	Authenticate(string) (core.Credential, error)
	RegisterUnit(context.Context, core.Credential, core.RegisterUnitRequest) (core.MedicationUnit, error)
	QueryUnitsByGTIN(context.Context, string) ([]core.MedicationUnit, error)
	QueryUnitsByState(context.Context, domain.State) ([]core.MedicationUnit, error)
	ReadUnit(context.Context, string, string) (core.MedicationUnit, error)
	GetUnitHistory(context.Context, string, string) ([]core.HistoryEntry, error)
	Dispatch(context.Context, core.Credential, string, string, core.DispatchRequest) (core.MedicationUnit, error)
	Receive(context.Context, core.Credential, string, string, *core.CommercialData) (core.MedicationUnit, error)
	Reject(context.Context, core.Credential, string, string, core.RejectRequest) (core.MedicationUnit, error)
	Dispense(context.Context, core.Credential, string, string) (core.MedicationUnit, error)
	Quarantine(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReleaseQuarantine(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReportExpired(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReportStolen(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReportLost(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReportDamaged(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ReturnProduct(context.Context, core.Credential, string, string, core.ReturnProductRequest) (core.MedicationUnit, error)
	Restock(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	WithdrawFromMarket(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	ProhibitProduct(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	FinalDisposition(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)
	AuthorizeLabIntervention(context.Context, core.Credential, string, string, core.AuthorizeLabInterventionRequest) (core.LabInterventionView, error)
	RevokeLabIntervention(context.Context, core.Credential, string, string, core.RevokeLabInterventionRequest) (core.LabInterventionView, error)
	VerifyUnit(context.Context, core.Credential, string, string) (core.UnitVerdict, error)
	VerifyTrace(context.Context, core.Credential, string, string) (core.TraceVerdict, error)
	RegisterOrganization(context.Context, core.Credential, core.RegisterOrganizationRequest) (core.Organization, error)
	SetOrganizationActive(context.Context, core.Credential, string, bool) (core.Organization, error)
}

type unitEventFunc func(context.Context, core.Credential, string, string, core.UnitEventRequest) (core.MedicationUnit, error)

func New(store Service, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/units", handler.registerUnit)
	mux.HandleFunc("GET /v1/units", handler.queryUnits)
	mux.HandleFunc("GET /v1/units/{gtin}/{numeroSerie}", handler.readUnit)
	mux.HandleFunc("GET /v1/units/{gtin}/{numeroSerie}/history", handler.history)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/dispatch", handler.dispatch)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/receive", handler.receive)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/reject", handler.reject)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/dispense", handler.dispense)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/quarantine", handler.unitEvent(store.Quarantine))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/release-quarantine", handler.unitEvent(store.ReleaseQuarantine))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/report-expired", handler.unitEvent(store.ReportExpired))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/report-stolen", handler.unitEvent(store.ReportStolen))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/report-lost", handler.unitEvent(store.ReportLost))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/report-damaged", handler.unitEvent(store.ReportDamaged))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/return", handler.returnProduct)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/restock", handler.unitEvent(store.Restock))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/withdraw-from-market", handler.unitEvent(store.WithdrawFromMarket))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/prohibit-product", handler.unitEvent(store.ProhibitProduct))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/final-disposition", handler.unitEvent(store.FinalDisposition))
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/authorize-lab-intervention", handler.authorizeLabIntervention)
	mux.HandleFunc("POST /v1/units/{gtin}/{numeroSerie}/revoke-lab-intervention", handler.revokeLabIntervention)
	mux.HandleFunc("GET /v1/units/{gtin}/{numeroSerie}/verify-unit", handler.verifyUnit)
	mux.HandleFunc("GET /v1/units/{gtin}/{numeroSerie}/verify-trace", handler.verifyTrace)
	mux.HandleFunc("POST /v1/organizations", handler.registerOrganization)
	mux.HandleFunc("PATCH /v1/organizations/{mspId}", handler.setOrganizationActive)
	return mux
}

func HTTPStatusForCode(code core.Code) int {
	switch code {
	case core.InvalidRequest, core.InvalidDestination, core.InvalidLabIntervention:
		return http.StatusBadRequest
	case core.UnitNotFound, core.LabInterventionNotFound:
		return http.StatusNotFound
	case core.UnauthorizedCustodian, core.UnauthorizedRole, core.UnauthorizedAgentType,
		core.OrgNotRegistered, core.OrgInactive, core.TransferNotAuthorized,
		core.ReceiverMismatch, core.RegulatoryOnly, core.LabInterventionRequired:
		return http.StatusForbidden
	case core.UnitAlreadyExists, core.InvalidStateTransition, core.NotInTransit,
		core.LastActiveRegulator, core.AlreadyInitialized, core.LabInterventionNotActive:
		return http.StatusConflict
	case core.InternalError:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) credential(request *http.Request) (core.Credential, error) {
	return h.store.Authenticate(request.Header.Get("X-Org-Key"))
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return core.NewError(core.InvalidRequest, "el body debe ser un objeto JSON valido")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return core.NewError(core.InvalidRequest, "el body debe contener un unico objeto JSON")
	}
	return nil
}

func decodeOptionalCommercial(response http.ResponseWriter, request *http.Request) (*core.CommercialData, error) {
	request.Body = http.MaxBytesReader(response, request.Body, maxBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var data core.CommercialData
	if err := decoder.Decode(&data); errors.Is(err, io.EOF) {
		return nil, nil
	} else if err != nil {
		return nil, core.NewError(core.InvalidRequest, "el body debe ser un objeto JSON valido")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, core.NewError(core.InvalidRequest, "el body debe contener un unico objeto JSON")
	}
	if data.NumeroRemito == "" && data.NumeroFactura == "" && data.Cantidad == 0 {
		return nil, nil
	}
	return &data, nil
}

func requireEmptyBody(response http.ResponseWriter, request *http.Request) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxBodyBytes)
	body, err := io.ReadAll(request.Body)
	if err != nil || len(bytes.TrimSpace(body)) != 0 {
		return core.NewError(core.InvalidRequest, "esta operacion no admite body")
	}
	return nil
}

func (h *Handler) writeError(response http.ResponseWriter, err error) {
	var contractErr *core.ContractError
	if !errors.As(err, &contractErr) {
		contractErr = core.NewError(core.InternalError, "error interno de la baseline")
	}
	if contractErr.Code == core.InternalError {
		h.logger.Error("request failed", "error", err)
	}
	writeJSON(response, HTTPStatusForCode(contractErr.Code), contractErr)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func (h *Handler) registerUnit(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.RegisterUnitRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.RegisterUnit(request.Context(), credential, input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, unit)
}

func (h *Handler) queryUnits(response http.ResponseWriter, request *http.Request) {
	gtin := request.URL.Query().Get("gtin")
	state := request.URL.Query().Get("estado")
	if (gtin == "") == (state == "") {
		h.writeError(response, core.NewError(core.InvalidRequest,
			"la consulta debe incluir exactamente uno de los parametros gtin o estado"))
		return
	}
	var units []core.MedicationUnit
	var err error
	if gtin != "" {
		units, err = h.store.QueryUnitsByGTIN(request.Context(), gtin)
	} else {
		units, err = h.store.QueryUnitsByState(request.Context(), domain.State(state))
	}
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, units)
}

func (h *Handler) readUnit(response http.ResponseWriter, request *http.Request) {
	unit, err := h.store.ReadUnit(request.Context(), request.PathValue("gtin"), request.PathValue("numeroSerie"))
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) history(response http.ResponseWriter, request *http.Request) {
	history, err := h.store.GetUnitHistory(request.Context(), request.PathValue("gtin"), request.PathValue("numeroSerie"))
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, history)
}

func (h *Handler) dispatch(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.DispatchRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.Dispatch(request.Context(), credential, request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) receive(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	commercial, err := decodeOptionalCommercial(response, request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.Receive(request.Context(), credential, request.PathValue("gtin"), request.PathValue("numeroSerie"), commercial)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) reject(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.RejectRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.Reject(request.Context(), credential, request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) dispense(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	if err := requireEmptyBody(response, request); err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.Dispense(request.Context(), credential, request.PathValue("gtin"), request.PathValue("numeroSerie"))
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) unitEvent(operation unitEventFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		credential, err := h.credential(request)
		if err != nil {
			h.writeError(response, err)
			return
		}
		var input core.UnitEventRequest
		if err := decodeJSON(response, request, &input); err != nil {
			h.writeError(response, err)
			return
		}
		unit, err := operation(request.Context(), credential,
			request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
		if err != nil {
			h.writeError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, unit)
	}
}

func (h *Handler) returnProduct(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.ReturnProductRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	unit, err := h.store.ReturnProduct(request.Context(), credential,
		request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, unit)
}

func (h *Handler) authorizeLabIntervention(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.AuthorizeLabInterventionRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	view, err := h.store.AuthorizeLabIntervention(request.Context(), credential,
		request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, view)
}

func (h *Handler) revokeLabIntervention(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.RevokeLabInterventionRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	view, err := h.store.RevokeLabIntervention(request.Context(), credential,
		request.PathValue("gtin"), request.PathValue("numeroSerie"), input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, view)
}

func (h *Handler) verifyUnit(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	verdict, err := h.store.VerifyUnit(request.Context(), credential,
		request.PathValue("gtin"), request.PathValue("numeroSerie"))
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, verdict)
}

func (h *Handler) verifyTrace(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	verdict, err := h.store.VerifyTrace(request.Context(), credential,
		request.PathValue("gtin"), request.PathValue("numeroSerie"))
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, verdict)
}

func (h *Handler) registerOrganization(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.RegisterOrganizationRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	org, err := h.store.RegisterOrganization(request.Context(), credential, input)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, org)
}

func (h *Handler) setOrganizationActive(response http.ResponseWriter, request *http.Request) {
	credential, err := h.credential(request)
	if err != nil {
		h.writeError(response, err)
		return
	}
	var input core.SetOrganizationActiveRequest
	if err := decodeJSON(response, request, &input); err != nil {
		h.writeError(response, err)
		return
	}
	org, err := h.store.SetOrganizationActive(request.Context(), credential, request.PathValue("mspId"), input.Active)
	if err != nil {
		h.writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, org)
}
