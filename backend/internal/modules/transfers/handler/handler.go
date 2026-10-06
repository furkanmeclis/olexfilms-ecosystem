// Package handler serves the /v1/stock-transfers endpoints (TEC-197).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	tr "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to transfers.
const (
	CodeNotSibling        = "TRANSFER_NOT_SIBLING"
	CodeNotParent         = "TRANSFER_NOT_PARENT"
	CodeInvalidTransition = "TRANSFER_INVALID_TRANSITION"
	CodeStockUnavailable  = "TRANSFER_STOCK_UNAVAILABLE"
)

// Handler serves transfer endpoints.
type Handler struct {
	svc *tr.Service
}

// New creates the handler.
func New(svc *tr.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) tr.Caller {
	return tr.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context())}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *tr.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
	case errors.Is(err, tr.ErrNotFound):
		response.NotFound(w, r, "Transfer request not found")
	case errors.Is(err, tr.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot perform this transfer action")
	case errors.Is(err, tr.ErrNotSibling):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeNotSibling,
			"Stock can be transferred only to a sibling under the same parent organization")
	case errors.Is(err, tr.ErrNotParent):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeNotParent,
			"Stock can be returned only to the direct parent organization")
	case errors.Is(err, tr.ErrInvalidTransition):
		response.Conflict(w, r, CodeInvalidTransition, "The transfer status does not allow this transition")
	case errors.Is(err, tr.ErrRateNotFound):
		// TEC-200: like the K25 cari transfer (TEC-198), a missing rate
		// answers 400 and the receipt is rolled back.
		response.Error(w, r, http.StatusBadRequest, response.CodeRateNotFound,
			"No exchange rate between the transfer currency and an organization's currency today")
	case errors.Is(err, tr.ErrStockUnavailable):
		response.Conflict(w, r, CodeStockUnavailable, "The stock movement for this transfer was refused")
	default:
		response.InternalErr(w, r, err, "transfer request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Transfer request not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// List (GET /v1/stock-transfers): kind, direction, status (CSV),
// created_from / created_to, organization_uuid (CSV), q, sort (transfer_no,
// status, created_at), limit, offset (TEC-373).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := tr.ParseListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Targets (GET /v1/stock-transfers/targets?kind): siblings of the active
// organization, or its direct parent for kind=return (TEC-223).
func (h *Handler) Targets(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Targets(r.Context(), caller(r), r.URL.Query().Get("kind"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get (GET /v1/stock-transfers/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type itemBody struct {
	Barcode  string `json:"barcode"`
	Quantity *int64 `json:"quantity"`
}

type createBody struct {
	Kind      string     `json:"kind"`
	ToOrgUUID string     `json:"to_org_uuid"`
	Note      *string    `json:"note"`
	Items     []itemBody `json:"items"`
}

// Create (POST /v1/stock-transfers): a request of the active organization.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if !decode(w, r, &body) {
		return
	}
	items := make([]tr.ItemInput, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, tr.ItemInput{Barcode: it.Barcode, Quantity: it.Quantity})
	}
	v, err := h.svc.Create(r.Context(), caller(r), tr.CreateInput{
		Kind: body.Kind, ToOrgUUID: body.ToOrgUUID, Note: body.Note, Items: items,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

type transitionBody struct {
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

// Transition (POST /v1/stock-transfers/{uuid}/transitions).
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body transitionBody
	if !decode(w, r, &body) {
		return
	}
	v, err := h.svc.Transition(r.Context(), caller(r), id, tr.TransitionInput{Status: body.Status, Reason: body.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
