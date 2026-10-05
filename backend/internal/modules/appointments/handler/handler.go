package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct{ svc *usecase.Service }

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrCapacityFull):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeCapacityFull, "Appointment capacity is full")
	case errors.Is(err, usecase.ErrInvalidTransition):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeInvalidTransition, "Appointment status transition is not allowed")
	case errors.Is(err, usecase.ErrDayClosed):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeDayClosed, "The organization is closed on this day")
	case errors.Is(err, usecase.ErrClosureExists):
		response.Conflict(w, r, usecase.CodeClosureExists, "A closure already exists for this day")
	case errors.Is(err, usecase.ErrCancelWindow):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeCancelWindow, "Appointment can no longer be cancelled")
	case errors.Is(err, usecase.ErrIntakeStarted):
		response.Conflict(w, r, usecase.CodeIntakeStarted, "Appointment intake is already started")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Appointment action is not allowed")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Appointment not found")
	default:
		response.InternalErr(w, r, err, "appointment request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Appointment not found")
		return uuid.Nil, false
	}
	return id, true
}

func parseTimeParam(w http.ResponseWriter, r *http.Request, name string) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "is required"}})
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if d, err := time.Parse(time.DateOnly, raw); err == nil {
		return d, true
	}
	response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be RFC3339 or YYYY-MM-DD"}})
	return time.Time{}, false
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetSettings(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var body usecase.Settings
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.PutSettings(r.Context(), caller(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ListClosures(w http.ResponseWriter, r *http.Request) {
	from, ok := parseTimeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, r, "to")
	if !ok {
		return
	}
	out, err := h.svc.ListClosures(r.Context(), caller(r), from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type closureBody struct {
	ClosedOn string `json:"closed_on"`
	Reason   string `json:"reason"`
}

func (h *Handler) CreateClosure(w http.ResponseWriter, r *http.Request) {
	var body closureBody
	if !decode(w, r, &body) {
		return
	}
	date, err := time.Parse(time.DateOnly, body.ClosedOn)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "closed_on", Message: "must be YYYY-MM-DD"}})
		return
	}
	out, err := h.svc.CreateClosure(r.Context(), caller(r), date, body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) DeleteClosure(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Appointment closure not found")
		return
	}
	if err := h.svc.DeleteClosure(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	from, ok := parseTimeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, r, "to")
	if !ok {
		return
	}
	out, err := h.svc.Availability(r.Context(), caller(r), from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func portalCaller(r *http.Request) usecase.PortalCaller {
	c := usecase.PortalCaller{UserID: authctx.MustPrincipal(r.Context()).UserInternal}
	if b, ok := brandctx.From(r.Context()); ok {
		c.BrandID = b.ID
	}
	return c
}

func (h *Handler) PortalAvailability(w http.ResponseWriter, r *http.Request) {
	dealerID, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Dealer not found")
		return
	}
	from, ok := parseTimeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, r, "to")
	if !ok {
		return
	}
	out, err := h.svc.PortalAvailability(r.Context(), portalCaller(r), dealerID, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) PortalList(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.PortalList(r.Context(), portalCaller(r), usecase.PortalListFilter{
		Period: r.URL.Query().Get("period"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) PortalCreate(w http.ResponseWriter, r *http.Request) {
	var body usecase.PortalCreateInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.PortalCreate(r.Context(), portalCaller(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) PortalCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.PortalCancel(r.Context(), portalCaller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	from, ok := parseTimeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := parseTimeParam(w, r, "to")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), caller(r), usecase.ListFilter{
		From: from, To: to, Status: r.URL.Query().Get("status"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body usecase.CreateInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Create(r.Context(), caller(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body usecase.PatchInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Patch(r.Context(), caller(r), id, body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body usecase.StatusInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.SetStatus(r.Context(), caller(r), id, body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) StartIntake(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.StartIntake(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Occupancy(w http.ResponseWriter, r *http.Request) {
	date, ok := parseTimeParam(w, r, "date")
	if !ok {
		return
	}
	out, err := h.svc.Occupancy(r.Context(), caller(r), date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
