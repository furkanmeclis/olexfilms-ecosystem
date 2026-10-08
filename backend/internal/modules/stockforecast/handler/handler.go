package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type OrderDraftCreator interface {
	CreateOrAppendDraft(context.Context, ordersuc.Caller, ordersuc.CreateInput) (ordersuc.DraftResult, error)
}

type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

type Handler struct {
	svc     *usecase.Service
	orders  OrderDraftCreator
	exports Exports
}

func New(svc *usecase.Service, orders OrderDraftCreator) *Handler {
	return &Handler{svc: svc, orders: orders}
}

func (h *Handler) WithExports(exports Exports) *Handler {
	h.exports = exports
	return h
}

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var qe *apiquery.ValidationError
	var oe *ordersuc.ValidationError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &oe):
		response.ValidationError(w, r, []response.Detail{{Field: oe.Field, Message: oe.Message, Code: oe.Code}})
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, ordersuc.ErrNotFound):
		response.NotFound(w, r, "Stock forecast not found")
	case errors.Is(err, usecase.ErrForbidden), errors.Is(err, ordersuc.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot access stock forecasts")
	case errors.Is(err, usecase.ErrInsufficientData):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodeInsufficientData, "One or more products do not have enough forecast data")
	case errors.Is(err, ordersuc.ErrNoSupplier):
		response.Error(w, r, http.StatusUnprocessableEntity, "ORDER_NO_SUPPLIER", "This organization cannot place product orders")
	default:
		response.InternalErr(w, r, err, "stock forecast request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func pathProduct(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("product_uuid"))
	if err != nil {
		response.NotFound(w, r, "Stock forecast not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseListFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathProduct(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Detail(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Meta(r.Context(), caller(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type thresholdBody struct {
	WarningDays int32 `json:"warning_days"`
	CoverDays   int32 `json:"cover_days"`
	Products    []struct {
		ProductUUID string `json:"product_uuid"`
		WarningDays int32  `json:"warning_days"`
		CoverDays   int32  `json:"cover_days"`
	} `json:"products"`
}

func (h *Handler) PutThresholds(w http.ResponseWriter, r *http.Request) {
	var body thresholdBody
	if !decode(w, r, &body) {
		return
	}
	in := usecase.ThresholdInput{WarningDays: body.WarningDays, CoverDays: body.CoverDays}
	for i, p := range body.Products {
		id, err := uuid.Parse(p.ProductUUID)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "products[" + strconv.Itoa(i) + "].product_uuid", Message: "must be a uuid", Code: "invalid"}})
			return
		}
		in.Products = append(in.Products, usecase.ProductThresholdInput{ProductUUID: id, WarningDays: p.WarningDays, CoverDays: p.CoverDays})
	}
	item, err := h.svc.UpsertThresholds(r.Context(), caller(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

type orderDraftBody struct {
	Note  *string `json:"note"`
	Items []struct {
		ProductUUID string  `json:"product_uuid"`
		Quantity    *int64  `json:"quantity"`
		Meters      *string `json:"meters"`
	} `json:"items"`
}

func (h *Handler) OrderDraft(w http.ResponseWriter, r *http.Request) {
	var body orderDraftBody
	if !decode(w, r, &body) {
		return
	}
	in := usecase.OrderDraftInput{Note: body.Note}
	for i, p := range body.Items {
		id, err := uuid.Parse(p.ProductUUID)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "items[" + strconv.Itoa(i) + "].product_uuid", Message: "must be a uuid", Code: "invalid"}})
			return
		}
		in.Items = append(in.Items, usecase.OrderDraftItem{ProductUUID: id, Quantity: p.Quantity, Meters: p.Meters})
	}
	item, err := h.svc.CreateOrderDraft(r.Context(), caller(r), h.orders, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) Network(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseNetworkFilter(r.URL.Query(), time.Now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.Network(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

type networkExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

func (h *Handler) RequestNetworkExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var body networkExportBody
	if !decode(w, r, &body) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(body.Format)))
	if format != ioengine.ExportXLSX {
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be xlsx", Code: "invalid"}})
		return
	}
	c := caller(r)
	query, err := usecase.NetworkExportQuery(body.Query, time.Now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(body.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.Principal.UserInternal, &orgID, usecase.ResourceNetworkExport, format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

func (h *Handler) Subtree(w http.ResponseWriter, r *http.Request) {
	f := usecase.ParseSubtreeFilter(r.URL.Query())
	items, total, err := h.svc.Subtree(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}
