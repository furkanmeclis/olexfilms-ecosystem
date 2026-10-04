package handler

// TEC-296 (F3-02d): the before/after measurements of a service.

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the service measurement routes.
const (
	CodeMeasurementNotExpected = "MEASUREMENT_NOT_EXPECTED"
	CodeMeasurementVINPending  = "MEASUREMENT_VIN_PENDING"
	CodeMeasurementVINMismatch = "MEASUREMENT_VIN_MISMATCH"
	CodeMeasurementPhaseTaken  = "MEASUREMENT_PHASE_TAKEN"
	CodeMeasurementLinked      = "MEASUREMENT_ALREADY_LINKED"
	CodeServiceNotEditable     = "SERVICE_NOT_EDITABLE"
)

// Linker is the use case of the service measurement routes.
type Linker interface {
	ServiceMeasurements(ctx context.Context, c usecase.LinkCaller, id uuid.UUID) (usecase.ServiceMeasurementsView, error)
	LinkMeasurement(ctx context.Context, c usecase.LinkCaller, id uuid.UUID, in usecase.LinkInput) (usecase.ServiceMeasurementsView, error)
	UnlinkMeasurement(ctx context.Context, c usecase.LinkCaller, id uuid.UUID, phase string) error
}

// LinkHandler serves /v1/services/{uuid}/measurements.
type LinkHandler struct{ svc Linker }

// NewLink creates the handler.
func NewLink(svc Linker) *LinkHandler { return &LinkHandler{svc: svc} }

func linkCaller(r *http.Request) usecase.LinkCaller {
	f, _ := scopefilter.From(r.Context())
	p := authctx.MustPrincipal(r.Context())
	return usecase.LinkCaller{UserID: p.UserInternal, Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeLinkError(w http.ResponseWriter, r *http.Request, err error) {
	unprocessable := func(code, msg string) { response.Error(w, r, http.StatusUnprocessableEntity, code, msg) }
	switch {
	case errors.Is(err, usecase.ErrServiceNotFound):
		response.NotFound(w, r, "Service not found")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Measurement not found")
	case errors.Is(err, usecase.ErrLinkNotFound):
		response.NotFound(w, r, "Measurement link not found")
	case errors.Is(err, usecase.ErrMeasurementNotExpected):
		unprocessable(CodeMeasurementNotExpected, "The service does not expect a measurement")
	case errors.Is(err, usecase.ErrMeasurementVINPending):
		unprocessable(CodeMeasurementVINPending, "The measurement has no VIN yet")
	case errors.Is(err, usecase.ErrMeasurementVINMismatch):
		unprocessable(CodeMeasurementVINMismatch, "The measurement VIN differs from the service VIN")
	case errors.Is(err, usecase.ErrPhaseTaken):
		response.Conflict(w, r, CodeMeasurementPhaseTaken, "The service already has a confirmed measurement for this phase")
	case errors.Is(err, usecase.ErrMeasurementLinked):
		response.Conflict(w, r, CodeMeasurementLinked, "The measurement is already linked")
	case errors.Is(err, usecase.ErrServiceCancelled):
		response.Conflict(w, r, CodeServiceNotEditable, "The service status does not allow this change")
	case errors.Is(err, usecase.ErrLinkLocked):
		response.Forbidden(w, r, "Only the center can change the measurements of a completed service")
	default:
		writeError(w, r, err)
	}
}

// List is GET /v1/services/{uuid}/measurements.
func (h *LinkHandler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Service not found")
	if !ok {
		return
	}
	out, err := h.svc.ServiceMeasurements(r.Context(), linkCaller(r), id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type linkBody struct {
	MeasurementUUID string `json:"measurement_uuid"`
	Phase           string `json:"phase"`
}

// Link is POST /v1/services/{uuid}/measurements {measurement_uuid, phase}.
func (h *LinkHandler) Link(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Service not found")
	if !ok {
		return
	}
	var b linkBody
	if !decode(w, r, &b) {
		return
	}
	mid, err := uuid.Parse(strings.TrimSpace(b.MeasurementUUID))
	if err != nil {
		writeError(w, r, &usecase.ValidationError{Field: "measurement_uuid", Message: "must be a UUID"})
		return
	}
	out, err := h.svc.LinkMeasurement(r.Context(), linkCaller(r), id, usecase.LinkInput{
		MeasurementUUID: mid, Phase: strings.TrimSpace(b.Phase),
	})
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Unlink is DELETE /v1/services/{uuid}/measurements/{phase}.
func (h *LinkHandler) Unlink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Service not found")
	if !ok {
		return
	}
	if err := h.svc.UnlinkMeasurement(r.Context(), linkCaller(r), id, strings.TrimSpace(r.PathValue("phase"))); err != nil {
		writeLinkError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
