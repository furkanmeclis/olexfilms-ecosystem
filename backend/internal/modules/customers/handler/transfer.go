package handler

// TEC-190 (F1-06f): vehicle transfer endpoints (vehicles.transfer).
// TEC-243 (F2-03d): the same flow from the customer portal.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Transfer error codes.
const (
	CodeTransferUnavailable  = "VEHICLE_TRANSFER_UNAVAILABLE"
	CodeTransferPending      = "VEHICLE_TRANSFER_PENDING"
	CodeTransferSameOwner    = "VEHICLE_TRANSFER_SAME_OWNER"
	CodeTransferOwnerNoPhone = "VEHICLE_TRANSFER_OWNER_NO_PHONE"
	CodeTransferNotPending   = "VEHICLE_TRANSFER_NOT_PENDING"
	CodeTransferExpired      = "VEHICLE_TRANSFER_EXPIRED"
	CodeTransferInvalidCode  = "VEHICLE_TRANSFER_INVALID_CODE"
	CodeTransferLocked       = "VEHICLE_TRANSFER_LOCKED"
	CodeTransferDelivery     = "VEHICLE_TRANSFER_DELIVERY_FAILED"
	CodeTransferOwnerChanged = "VEHICLE_TRANSFER_OWNER_CHANGED"
)

// Rate limits: starting sends two WhatsApp messages; code entry is also
// bounded by the transfer's attempt counter (DB), this only slows a burst.
const (
	transferStartAction  = "vehicle_transfer_start"
	transferStartLimit   = 10
	transferStartWindow  = time.Hour
	transferVerifyAction = "vehicle_transfer_verify"
	transferVerifyLimit  = 10
	transferVerifyWindow = 15 * time.Minute
)

// Limiter is a fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// WithLimiter sets the limiter of the transfer endpoints.
func (h *Handler) WithLimiter(l Limiter) *Handler {
	h.limiter = l
	return h
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if h.limiter == nil {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, window)
	if ok {
		return true
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	response.TooManyRequests(w, r, "")
	return false
}

func actorKey(r *http.Request) string {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return strconv.FormatInt(p.UserInternal, 10)
	}
	return r.RemoteAddr
}

// ListVehicleTransfers (GET /v1/vehicles/{uuid}/transfers).
func (h *Handler) ListVehicleTransfers(w http.ResponseWriter, r *http.Request) {
	h.listTransfers(w, r, caller(r))
}

// StartVehicleTransfer (POST /v1/vehicles/{uuid}/transfers): opens a
// transfer and sends one code to the current and one to the new owner.
func (h *Handler) StartVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.startTransfer(w, r, caller(r), h.svc.StartTransfer)
}

// VerifyVehicleTransfer (POST /v1/vehicle-transfers/{uuid}/verify): checks
// the codes; when both sides are verified the transfer completes.
func (h *Handler) VerifyVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.verifyTransfer(w, r, caller(r))
}

// CancelVehicleTransfer (POST /v1/vehicle-transfers/{uuid}/cancel).
func (h *Handler) CancelVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.cancelTransfer(w, r, caller(r))
}

type startTransferFunc func(ctx context.Context, c cu.Caller, vehicleID uuid.UUID, in cu.StartTransferInput,
	meta activity.Meta) (cu.VehicleTransferView, error)

func (h *Handler) listTransfers(w http.ResponseWriter, r *http.Request, c cu.Caller) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListVehicleTransfers(r.Context(), c, id)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) startTransfer(w http.ResponseWriter, r *http.Request, c cu.Caller, start startTransferFunc) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in cu.StartTransferInput
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, transferStartAction, actorKey(r), transferStartLimit, transferStartWindow) {
		return
	}
	out, err := start(r.Context(), c, id, in, activity.MetaFromRequest(r))
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) verifyTransfer(w http.ResponseWriter, r *http.Request, c cu.Caller) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in cu.VerifyTransferInput
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, transferVerifyAction, id.String()+"|"+actorKey(r), transferVerifyLimit, transferVerifyWindow) {
		return
	}
	out, err := h.svc.VerifyTransfer(r.Context(), c, id, in, activity.MetaFromRequest(r))
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) cancelTransfer(w http.ResponseWriter, r *http.Request, c cu.Caller) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.CancelTransfer(r.Context(), c, id, activity.MetaFromRequest(r))
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// --- Portal (TEC-243): the owner's own vehicles and transfers -------------

// portalCaller is the portal session: the domain brand and the signed-in
// user; the use case limits it to the user's own vehicles.
func portalCaller(r *http.Request) cu.Caller {
	var brandID int64
	if b, ok := brandctx.From(r.Context()); ok {
		brandID = b.ID
	}
	return cu.PortalCaller(brandID, authctx.MustPrincipal(r.Context()).UserInternal, i18n.FromContext(r.Context()).Locale)
}

// PortalListVehicleTransfers (GET /v1/portal/vehicles/{uuid}/transfers).
func (h *Handler) PortalListVehicleTransfers(w http.ResponseWriter, r *http.Request) {
	h.listTransfers(w, r, portalCaller(r))
}

// PortalStartVehicleTransfer (POST /v1/portal/vehicles/{uuid}/transfers).
func (h *Handler) PortalStartVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.startTransfer(w, r, portalCaller(r), h.svc.PortalStartTransfer)
}

// PortalVerifyVehicleTransfer (POST /v1/portal/vehicle-transfers/{uuid}/verify).
func (h *Handler) PortalVerifyVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.verifyTransfer(w, r, portalCaller(r))
}

// PortalCancelVehicleTransfer (POST /v1/portal/vehicle-transfers/{uuid}/cancel).
func (h *Handler) PortalCancelVehicleTransfer(w http.ResponseWriter, r *http.Request) {
	h.cancelTransfer(w, r, portalCaller(r))
}

func writeTransferError(w http.ResponseWriter, r *http.Request, err error) {
	var ic *cu.InvalidTransferCodeError
	switch {
	case errors.As(err, &ic):
		details := make([]response.Detail, 0, len(ic.Fields))
		for _, f := range ic.Fields {
			details = append(details, response.Detail{Field: f, Message: "wrong code", Code: strconv.Itoa(int(ic.Remaining))})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeTransferInvalidCode,
			"The code is wrong. Attempts left: "+strconv.Itoa(int(ic.Remaining)), details)
	case errors.Is(err, cu.ErrTransferUnavailable):
		response.ServiceUnavailable(w, r, CodeTransferUnavailable, "Vehicle transfer is not available")
	case errors.Is(err, cu.ErrTransferNotFound):
		response.NotFound(w, r, "Vehicle transfer not found")
	case errors.Is(err, cu.ErrTransferPending):
		response.Conflict(w, r, CodeTransferPending, "The vehicle already has a pending transfer")
	case errors.Is(err, cu.ErrTransferSameOwner):
		response.Conflict(w, r, CodeTransferSameOwner, "The new owner is the current owner")
	case errors.Is(err, cu.ErrTransferOwnerNoPhone):
		response.Conflict(w, r, CodeTransferOwnerNoPhone, "The current owner has no phone number for the code")
	case errors.Is(err, cu.ErrTransferNotPending):
		response.Conflict(w, r, CodeTransferNotPending, "The transfer is no longer pending")
	case errors.Is(err, cu.ErrTransferExpired):
		response.Conflict(w, r, CodeTransferExpired, "The transfer expired; start a new one")
	case errors.Is(err, cu.ErrTransferLocked):
		response.Conflict(w, r, CodeTransferLocked, "Too many wrong codes; the transfer was cancelled")
	case errors.Is(err, cu.ErrTransferOwnerChanged):
		response.Conflict(w, r, CodeTransferOwnerChanged, "The vehicle owner changed; start a new transfer")
	case errors.Is(err, cu.ErrTransferDelivery):
		response.Error(w, r, http.StatusBadGateway, CodeTransferDelivery, "The codes could not be delivered. Try again later.")
	default:
		writeError(w, r, err)
	}
}
