package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CalibrationService is the calibration use case (usecase.Calibrations).
type CalibrationService interface {
	Record(ctx context.Context, c usecase.Caller, in usecase.CalibrationInput) (usecase.CalibrationView, bool, error)
	Latest(ctx context.Context, organizationID int64, serial int) (usecase.CalibrationView, error)
}

// maxCalibrationBody bounds a calibration body (a few hundred bytes).
const maxCalibrationBody = 16 << 10

// Calibrations serves the SmartPTG zero calibrations (TEC-939):
// POST /v1/measurements/devices/smartptg/calibrations and
// GET /v1/measurements/devices/smartptg/calibrations/{serial}.
type Calibrations struct{ svc CalibrationService }

// NewCalibrations builds the handler.
func NewCalibrations(svc CalibrationService) *Calibrations { return &Calibrations{svc: svc} }

func writeCalibrationError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrCalibrationNotFound):
		response.NotFound(w, r, "Gauge calibration not found")
	default:
		response.InternalErr(w, r, err, "gauge calibration failed")
	}
}

// Create records a calibration of the active organization: 201, or 200
// with the first row when the client_id was already recorded.
func (h *Calibrations) Create(w http.ResponseWriter, r *http.Request) {
	var in usecase.CalibrationInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCalibrationBody))
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	out, replayed, err := h.svc.Record(r.Context(), usecase.Caller{
		UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID,
	}, in)
	if err != nil {
		writeCalibrationError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, out)
}

// Latest answers the current calibration of a serial (404 when none).
func (h *Calibrations) Latest(w http.ResponseWriter, r *http.Request) {
	serial, ok := usecase.ParseGaugeSerial(r.PathValue("serial"))
	if !ok {
		response.BadRequest(w, r, "INVALID_SERIAL", "Gauge serial must be 1-9 digits")
		return
	}
	out, err := h.svc.Latest(r.Context(), orgctx.MustScope(r.Context()).InternalID, serial)
	if err != nil {
		writeCalibrationError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}
