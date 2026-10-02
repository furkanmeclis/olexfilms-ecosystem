// Package handler serves /v1/customers and /v1/vehicles (TEC-160, F1-08b).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to customers.
const (
	CodeCustomerAnonymized = "CUSTOMER_ANONYMIZED"
	CodeCustomerInactive   = "CUSTOMER_INACTIVE"
	CodeEmailTaken         = "EMAIL_TAKEN"
	CodeAlreadyMember      = "ALREADY_MEMBER"
	CodePIIUnavailable     = "PII_ENCRYPTION_UNAVAILABLE"
)

// Handler serves the customer and vehicle endpoints.
type Handler struct {
	svc      *cu.Service
	activity *activity.Recorder
	exports  Exports
	limiter  Limiter // TEC-190: transfer endpoints
}

// New creates the handler; rec may be nil.
func New(svc *cu.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func caller(r *http.Request) cu.Caller {
	p := authctx.MustPrincipal(r.Context())
	f, _ := scopefilter.From(r.Context())
	return cu.Caller{
		UserID: p.UserInternal, Org: orgctx.MustScope(r.Context()), Filter: f,
		Locale: i18n.FromContext(r.Context()).Locale,
	}
}

func (h *Handler) record(r *http.Request, action, resource string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		v := p.UserInternal
		actor = &v
	}
	h.activity.Record(r.Context(), actor, action, resource, id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *cu.ValidationError
	var pe *cu.InvalidPlateError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.As(err, &pe):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeInvalidPlate,
			"Plate does not match the country format", []response.Detail{{Field: "plate", Message: "invalid for " + pe.Country}})
	case errors.Is(err, cu.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot do this")
	case errors.Is(err, cu.ErrCustomerNotFound):
		response.NotFound(w, r, "Customer not found")
	case errors.Is(err, cu.ErrVehicleNotFound):
		response.NotFound(w, r, "Vehicle not found")
	case errors.Is(err, cu.ErrOrganizationNotFound):
		response.NotFound(w, r, "Organization not found")
	case errors.Is(err, cu.ErrAnonymized):
		response.Conflict(w, r, CodeCustomerAnonymized, "The customer is anonymized and cannot be changed")
	case errors.Is(err, cu.ErrInactive):
		response.Conflict(w, r, CodeCustomerInactive, "The customer account is not active")
	case errors.Is(err, cu.ErrEmailTaken):
		response.Conflict(w, r, CodeEmailTaken, "The e-mail address is used by another account")
	case errors.Is(err, cu.ErrAlreadyMember):
		response.Conflict(w, r, CodeAlreadyMember, "The customer is already a member of this organization")
	case errors.Is(err, cu.ErrPIIUnavailable):
		response.ServiceUnavailable(w, r, CodePIIUnavailable, "Identity numbers cannot be stored: encryption is not configured")
	default:
		response.InternalErr(w, r, err, "customer request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func queryUUID(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a UUID"}})
		return nil, false
	}
	return &id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// --- Customers ---------------------------------------------------------------

// ListCustomers (GET /v1/customers?q&status&limit&offset).
func (h *Handler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListCustomers(r.Context(), caller(r), cu.ListFilter{
		Q: q.Q, Status: r.URL.Query().Get("status"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateCustomer (POST /v1/customers): 201 for a new user, 200 when the
// phone already belonged to a user who was linked.
func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var in cu.CreateCustomerInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.CreateCustomer(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customers.created", "customers", &res.UUID, map[string]any{
		"existing_user": res.ExistingUser, "ignored_fields": res.IgnoredFields,
	})
	status := http.StatusCreated
	if res.ExistingUser {
		status = http.StatusOK
	}
	response.JSON(w, r, status, res)
}

// GetCustomer (GET /v1/customers/{uuid}).
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.GetCustomer(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// UpdateCustomer (PATCH /v1/customers/{uuid}).
func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in cu.UpdateCustomerInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.UpdateCustomer(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customers.updated", "customers", &res.UUID, map[string]any{"ignored_fields": res.IgnoredFields})
	response.JSON(w, r, http.StatusOK, res)
}

// UpgradeToDealer (POST /v1/customers/{uuid}/upgrade-to-dealer).
func (h *Handler) UpgradeToDealer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in cu.UpgradeInput
	if !decode(w, r, &in) {
		return
	}
	res, err := h.svc.UpgradeToDealer(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customers.upgraded_to_dealer", "customers", &res.CustomerUUID, map[string]any{
		"organization_uuid": res.Organization.UUID.String(), "role": res.Role,
	})
	response.JSON(w, r, http.StatusCreated, res)
}

// --- Vehicles ----------------------------------------------------------------

// ListVehicles (GET /v1/vehicles?customer_uuid&plate&vin&limit&offset).
func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	customer, ok := queryUUID(w, r, "customer_uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListVehicles(r.Context(), caller(r), cu.VehicleFilter{
		CustomerUUID: customer, Plate: r.URL.Query().Get("plate"), VIN: r.URL.Query().Get("vin"),
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateVehicle (POST /v1/vehicles).
func (h *Handler) CreateVehicle(w http.ResponseWriter, r *http.Request) {
	var in cu.CreateVehicleInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateVehicle(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.created", "vehicles", &v.UUID, map[string]any{"customer_uuid": v.CustomerUUID.String()})
	response.JSON(w, r, http.StatusCreated, v)
}

// GetVehicle (GET /v1/vehicles/{uuid}).
func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	v, err := h.svc.GetVehicle(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// UpdateVehicle (PATCH /v1/vehicles/{uuid}).
func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in cu.UpdateVehicleInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateVehicle(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.updated", "vehicles", &v.UUID, nil)
	response.JSON(w, r, http.StatusOK, v)
}

// DeleteVehicle (DELETE /v1/vehicles/{uuid}): soft delete.
func (h *Handler) DeleteVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteVehicle(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.deleted", "vehicles", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}
