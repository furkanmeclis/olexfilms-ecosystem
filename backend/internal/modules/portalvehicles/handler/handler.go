// Package handler serves the TEC-238 portal reads:
// GET /v1/portal/vehicles, GET /v1/portal/vehicles/{uuid},
// GET /v1/portal/services; and TEC-245 GET /v1/portal/contracts.
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the portal vehicle and service reads.
type Handler struct {
	svc *usecase.Service
}

// New creates the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// caller is the portal session: the domain brand and the signed-in user.
func caller(r *http.Request) usecase.Caller {
	c := usecase.Caller{UserID: authctx.MustPrincipal(r.Context()).UserInternal}
	if b, ok := brandctx.From(r.Context()); ok {
		c.BrandID = b.ID
	}
	return c
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var qe *apiquery.ValidationError
	if errors.As(err, &qe) {
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if errors.Is(err, usecase.ErrVehicleNotFound) {
		response.NotFound(w, r, "Vehicle not found")
		return
	}
	response.InternalErr(w, r, err, "portal request failed")
}

// ListVehicles (GET /v1/portal/vehicles).
func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	pq := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListVehicles(r.Context(), caller(r), usecase.Page{Limit: pq.Limit, Offset: pq.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, pq.Limit, pq.Offset))
}

// GetVehicle (GET /v1/portal/vehicles/{uuid}).
func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		writeError(w, r, usecase.ErrVehicleNotFound)
		return
	}
	v, err := h.svc.GetVehicle(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// ListServices (GET /v1/portal/services; TEC-377: q, status, organization_uuid,
// created_from / created_to and sort, docs/list-contract.md).
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseServiceListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListServices(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// ListContracts (GET /v1/portal/contracts, TEC-245): the user's signed
// vehicle intake contracts; an empty page until F3 records contracts.
func (h *Handler) ListContracts(w http.ResponseWriter, r *http.Request) {
	pq := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListContracts(r.Context(), caller(r), usecase.Page{Limit: pq.Limit, Offset: pq.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, pq.Limit, pq.Offset))
}
