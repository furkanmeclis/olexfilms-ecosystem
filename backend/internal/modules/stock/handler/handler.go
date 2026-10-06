// Package handler serves the TEC-155 stock read endpoints under /v1/stock.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves stock read endpoints.
type Handler struct {
	svc     *stockusecase.Service
	exports Exports
}

// New creates the handler.
func New(svc *stockusecase.Service) *Handler { return &Handler{svc: svc} }

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, stockusecase.ErrNotFound) {
		response.NotFound(w, r, "Stock record not found")
		return
	}
	response.InternalErr(w, r, err, "stock request failed")
}

// writeQueryError answers a list parameter error (400 VALIDATION_ERROR).
func writeQueryError(w http.ResponseWriter, r *http.Request, err error) {
	var qe *apiquery.ValidationError
	if errors.As(err, &qe) {
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	writeError(w, r, err)
}

func filterFrom(w http.ResponseWriter, r *http.Request) (scopefilter.Filter, bool) {
	f, ok := scopefilter.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "Missing permission: stock.read")
	}
	return f, ok
}

// UnitHistory serves GET /v1/stock/units/by-barcode/{barcode}.
func (h *Handler) UnitHistory(w http.ResponseWriter, r *http.Request) {
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.UnitHistory(r.Context(), orgctx.MustScope(r.Context()), f, r.PathValue("barcode"), q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func stockFilter(w http.ResponseWriter, r *http.Request) (model.StockFilter, bool) {
	q := apiquery.Parse(r.URL.Query())
	f := model.StockFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	for name, dst := range map[string]**uuid.UUID{"product_uuid": &f.ProductUUID, "category_uuid": &f.CategoryUUID} {
		raw := strings.TrimSpace(r.URL.Query().Get(name))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
			return f, false
		}
		*dst = &id
	}
	switch status := strings.TrimSpace(r.URL.Query().Get("status")); status {
	case "", model.StatusInStock, model.StatusOutOfStock:
		f.Status = status
	default:
		response.BadRequest(w, r, response.CodeValidationError, "status must be in_stock or out_of_stock")
		return f, false
	}
	// TEC-373: product, sku, category, quantity, meters, updated_at.
	sort, err := apiquery.ResolveSort(q.Sort, stockusecase.ProductStockSort)
	if err != nil {
		writeQueryError(w, r, err)
		return f, false
	}
	f.Sort = sort
	return f, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request,
	fn func(scopefilter.Filter, uuid.UUID, model.StockFilter) ([]model.ProductStock, int64, error)) {
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	in, ok := stockFilter(w, r)
	if !ok {
		return
	}
	items, total, err := fn(f, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, in.Limit, in.Offset))
}

// OrganizationStock serves GET /v1/stock/organizations/{uuid}/products.
func (h *Handler) OrganizationStock(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, func(f scopefilter.Filter, id uuid.UUID, in model.StockFilter) ([]model.ProductStock, int64, error) {
		return h.svc.OrganizationStock(r.Context(), f, id, in)
	})
}

// OrganizationUnits serves GET /v1/stock/organizations/{uuid}/units (TEC-216).
func (h *Handler) OrganizationUnits(w http.ResponseWriter, r *http.Request) {
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	in, err := stockusecase.ParseUnitFilter(r.URL.Query())
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	v := stockusecase.UnitViewer{Principal: p, Org: orgctx.MustScope(r.Context()), Filter: f}
	items, total, err := h.svc.OrganizationUnits(r.Context(), v, id, in)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, in.Limit, in.Offset))
}

// LocationStock serves GET /v1/stock/locations/{uuid}/products.
func (h *Handler) LocationStock(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, func(f scopefilter.Filter, id uuid.UUID, in model.StockFilter) ([]model.ProductStock, int64, error) {
		return h.svc.LocationStock(r.Context(), f, id, in)
	})
}
