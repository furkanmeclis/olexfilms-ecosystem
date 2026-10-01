// Package handler serves the /v1/orders endpoints (TEC-166, F1-04b).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to orders.
const (
	CodeInvalidTransition     = "ORDER_INVALID_TRANSITION"
	CodeTransitionUnavailable = "ORDER_TRANSITION_UNAVAILABLE"
	CodeNotEditable           = "ORDER_NOT_EDITABLE"
	CodeNoSupplier            = "ORDER_NO_SUPPLIER"
	CodePriceNotFound         = ord.CodePriceNotFound
)

// Handler serves order endpoints.
type Handler struct {
	svc *ord.Service
}

// New creates the handler.
func New(svc *ord.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) ord.Caller {
	f, _ := scopefilter.From(r.Context())
	return ord.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *ord.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
	case errors.Is(err, ord.ErrNotFound):
		response.NotFound(w, r, "Order not found")
	case errors.Is(err, ord.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot perform this order action")
	case errors.Is(err, ord.ErrNoSupplier):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeNoSupplier,
			"Only a distributor (from the center) or a dealer (from its distributor) can place product orders")
	case errors.Is(err, ord.ErrInvalidTransition):
		response.Conflict(w, r, CodeInvalidTransition, "The order status does not allow this transition")
	case errors.Is(err, ord.ErrTransitionUnavailable):
		response.Conflict(w, r, CodeTransitionUnavailable, "This transition is not available yet")
	case errors.Is(err, ord.ErrNotEditable):
		response.Conflict(w, r, CodeNotEditable, "Only draft orders can be edited")
	case errors.Is(err, ord.ErrRateNotFound):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeRateNotFound,
			"No exchange rate to freeze for the order currency")
	default:
		response.InternalErr(w, r, err, "order request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Order not found")
		return uuid.Nil, false
	}
	return id, true
}

// decode reads a JSON body. Unknown fields are ignored on purpose: a client
// sending unit_price or line_total must not be able to set them (K8), and
// the price fields are simply not part of the body.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

type itemBody struct {
	ProductUUID string       `json:"product_uuid"`
	Quantity    *int64       `json:"quantity"`
	Meters      *json.Number `json:"meters"`
	Note        *string      `json:"note"`
}

func toItems(in []itemBody) []ord.ItemInput {
	out := make([]ord.ItemInput, 0, len(in))
	for _, it := range in {
		var meters *string
		if it.Meters != nil {
			m := it.Meters.String()
			meters = &m
		}
		out = append(out, ord.ItemInput{ProductUUID: it.ProductUUID, Quantity: it.Quantity, Meters: meters, Note: it.Note})
	}
	return out
}

// List (GET /v1/orders?side&status&limit&offset).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	items, total, err := h.svc.List(r.Context(), caller(r), ord.ListFilter{
		Side: strings.TrimSpace(v.Get("side")), Status: strings.TrimSpace(v.Get("status")),
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get (GET /v1/orders/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

type createBody struct {
	Note  *string    `json:"note"`
	Items []itemBody `json:"items"`
}

// Create (POST /v1/orders): a draft order of the active organization to its
// parent organization.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if !decode(w, r, &body) {
		return
	}
	o, err := h.svc.Create(r.Context(), caller(r), ord.CreateInput{Note: body.Note, Items: toItems(body.Items)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, o)
}

type itemsBody struct {
	Items []itemBody `json:"items"`
}

// ReplaceItems (PUT /v1/orders/{uuid}/items): draft lines, prices fetched again.
func (h *Handler) ReplaceItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body itemsBody
	if !decode(w, r, &body) {
		return
	}
	o, err := h.svc.ReplaceItems(r.Context(), caller(r), id, toItems(body.Items))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

type transitionBody struct {
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

// Transition (POST /v1/orders/{uuid}/transitions).
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body transitionBody
	if !decode(w, r, &body) {
		return
	}
	o, err := h.svc.Transition(r.Context(), caller(r), id, ord.TransitionInput{Status: body.Status, Reason: body.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}
