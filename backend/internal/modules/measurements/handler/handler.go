// Package handler serves POST /v1/mobile/measurements (TEC-233, K28).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// MaxBodyBytes bounds one upload (a NexPTG report is a few hundred KB).
const MaxBodyBytes = 4 << 20

// Creator is the use case the handler calls.
type Creator interface {
	Create(ctx context.Context, c usecase.Caller, in usecase.Input) (usecase.Result, error)
	ListDevices(ctx context.Context, c usecase.PanelCaller) ([]usecase.DeviceView, error)
	CreateDevice(ctx context.Context, c usecase.PanelCaller, in usecase.DeviceInput) (usecase.DeviceView, error)
	UpdateDevice(ctx context.Context, c usecase.PanelCaller, id uuid.UUID, in usecase.DeviceInput) (usecase.DeviceView, error)
	ListMeasurements(ctx context.Context, c usecase.PanelCaller, f usecase.MeasurementFilter) ([]usecase.MeasurementSummary, int64, error)
	GetMeasurement(ctx context.Context, c usecase.PanelCaller, id uuid.UUID) (usecase.MeasurementDetail, error)
}

// Handler serves the measurement upload.
type Handler struct{ svc Creator }

// New creates the handler.
func New(svc Creator) *Handler { return &Handler{svc: svc} }

func panelCaller(r *http.Request) usecase.PanelCaller {
	f, _ := scopefilter.From(r.Context())
	return usecase.PanelCaller{Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Measurement not found")
	case errors.Is(err, usecase.ErrSerialExists):
		response.Conflict(w, r, "MEASUREMENT_DEVICE_SERIAL_EXISTS", "A measurement device with this serial already exists")
	case errors.Is(err, usecase.ErrServiceNotFound):
		response.NotFound(w, r, "Service not found")
	default:
		response.InternalErr(w, r, err, "measurement request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request, msg string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.NotFound(w, r, msg)
		return uuid.Nil, false
	}
	return id, true
}

// Create is POST /v1/mobile/measurements: 202 {uuid, status}; a repeated
// Idempotency-Key or client_measurement_id answers the first result.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: "is too large or unreadable"}})
		return
	}
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	out, err := h.svc.Create(r.Context(), usecase.Caller{
		UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID,
	}, usecase.Input{IdempotencyKey: r.Header.Get("Idempotency-Key"), Body: body})
	var ve *usecase.ValidationError
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusAccepted, out)
	case errors.As(err, &ve), errors.Is(err, usecase.ErrServiceNotFound):
		writeError(w, r, err)
	default:
		response.InternalErr(w, r, err, "measurement upload failed")
	}
}

type deviceBody struct {
	Serial   string  `json:"serial"`
	Label    *string `json:"label"`
	Model    *string `json:"model"`
	IsActive *bool   `json:"is_active"`
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListDevices(r.Context(), panelCaller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	var b deviceBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateDevice(r.Context(), panelCaller(r), usecase.DeviceInput{
		Serial: b.Serial, Label: b.Label, Model: b.Model, IsActive: b.IsActive,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Measurement device not found")
	if !ok {
		return
	}
	var b deviceBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateDevice(r.Context(), panelCaller(r), id, usecase.DeviceInput{
		Label: b.Label, Model: b.Model, IsActive: b.IsActive,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ListMeasurements(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	deviceID, err := optionalUUID(v.Get("device_uuid"))
	if err != nil {
		writeError(w, r, &usecase.ValidationError{Field: "device_uuid", Message: "must be a UUID"})
		return
	}
	linked, err := optionalBool(v.Get("linked"))
	if err != nil {
		writeError(w, r, &usecase.ValidationError{Field: "linked", Message: "must be true or false"})
		return
	}
	from, err := optionalTime(v.Get("measured_from"), false)
	if err != nil {
		writeError(w, r, &usecase.ValidationError{Field: "measured_from", Message: "must be a date or RFC3339 date-time"})
		return
	}
	to, err := optionalTime(v.Get("measured_to"), true)
	if err != nil {
		writeError(w, r, &usecase.ValidationError{Field: "measured_to", Message: "must be a date or RFC3339 date-time"})
		return
	}
	items, total, err := h.svc.ListMeasurements(r.Context(), panelCaller(r), usecase.MeasurementFilter{
		VIN: v.Get("vin"), DeviceUUID: deviceID, Status: strings.TrimSpace(v.Get("status")),
		Linked: linked, MeasuredFrom: from, MeasuredTo: to, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetMeasurement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Measurement not found")
	if !ok {
		return
	}
	out, err := h.svc.GetMeasurement(r.Context(), panelCaller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func optionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func optionalBool(raw string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	switch raw {
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, errors.New("invalid bool")
	}
}

func optionalTime(raw string, endOfDay bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if d, err := time.Parse(time.DateOnly, raw); err == nil {
		if endOfDay {
			d = d.AddDate(0, 0, 1)
		}
		return &d, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
