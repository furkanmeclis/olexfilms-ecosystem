// Package handler serves POST /v1/mobile/measurements (TEC-233, K28).
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// MaxBodyBytes bounds one upload (a NexPTG report is a few hundred KB).
const MaxBodyBytes = 4 << 20

// Creator is the use case the handler calls.
type Creator interface {
	Create(ctx context.Context, c usecase.Caller, in usecase.Input) (usecase.Result, error)
}

// Handler serves the measurement upload.
type Handler struct{ svc Creator }

// New creates the handler.
func New(svc Creator) *Handler { return &Handler{svc: svc} }

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
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrServiceNotFound):
		response.NotFound(w, r, "Service not found")
	default:
		response.InternalErr(w, r, err, "measurement upload failed")
	}
}
