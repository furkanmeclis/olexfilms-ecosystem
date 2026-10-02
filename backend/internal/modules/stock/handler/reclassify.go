package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Reclassification error codes (TEC-157). Wrong routing has one code per
// case; frontend/src/locales/*/errors.json translates them.
const (
	CodeReclassBrandMismatch    = "STOCK_RECLASSIFICATION_BRAND_MISMATCH"
	CodeReclassUnitTypeMismatch = "STOCK_RECLASSIFICATION_UNIT_TYPE_MISMATCH"
	CodeReclassSameProduct      = "STOCK_RECLASSIFICATION_SAME_PRODUCT"
	CodeReclassProductInactive  = "STOCK_RECLASSIFICATION_PRODUCT_INACTIVE"
	CodeReclassFixedBarcode     = "STOCK_RECLASSIFICATION_FIXED_BARCODE"
	CodeReclassUnitConsumed     = "STOCK_RECLASSIFICATION_UNIT_CONSUMED"
	CodeReclassUnitElsewhere    = "STOCK_RECLASSIFICATION_UNIT_ELSEWHERE"
	CodeReclassUnitInUse        = "STOCK_RECLASSIFICATION_UNIT_IN_USE"
	CodeReclassProductChanged   = "STOCK_RECLASSIFICATION_PRODUCT_CHANGED"
	CodeReclassNotPending       = "STOCK_RECLASSIFICATION_NOT_PENDING"
	CodeReclassPendingExists    = "STOCK_RECLASSIFICATION_PENDING_EXISTS"
	CodeReclassSelfApproval     = "STOCK_RECLASSIFICATION_SELF_APPROVAL"
)

type reclassError struct {
	err    error
	status int
	code   string
	msg    string
}

var reclassErrors = []reclassError{
	{ledger.ErrReclassifyBrandMismatch, http.StatusUnprocessableEntity, CodeReclassBrandMismatch, "The target product belongs to another brand"},
	{ledger.ErrReclassifyUnitTypeMismatch, http.StatusUnprocessableEntity, CodeReclassUnitTypeMismatch, "The target product has another unit type"},
	{ledger.ErrReclassifySameProduct, http.StatusUnprocessableEntity, CodeReclassSameProduct, "The unit already has this product"},
	{ledger.ErrReclassifyProductInactive, http.StatusUnprocessableEntity, CodeReclassProductInactive, "The target product is inactive"},
	{ledger.ErrReclassifyFixedBarcode, http.StatusUnprocessableEntity, CodeReclassFixedBarcode, "Fixed barcodes cannot be reclassified"},
	{ledger.ErrReclassifyUnitConsumed, http.StatusConflict, CodeReclassUnitConsumed, "The unit is consumed"},
	{ledger.ErrReclassifyUnitElsewhere, http.StatusConflict, CodeReclassUnitElsewhere, "The unit is not on hand at the organization"},
	{stockusecase.ErrReclassUnitInUse, http.StatusConflict, CodeReclassUnitInUse, "The unit is on an open service or order"},
	{ledger.ErrReclassifyProductChanged, http.StatusConflict, CodeReclassProductChanged, "The unit product changed since the request"},
	{stockusecase.ErrReclassNotPending, http.StatusConflict, CodeReclassNotPending, "The reclassification is already decided"},
	{stockusecase.ErrReclassPendingExists, http.StatusConflict, CodeReclassPendingExists, "The unit already has a pending reclassification"},
	{stockusecase.ErrReclassSelfApproval, http.StatusForbidden, CodeReclassSelfApproval, "The requester cannot approve its own reclassification"},
}

// Reclassify serves the /v1/stock/reclassifications endpoints (TEC-157).
type Reclassify struct {
	svc *stockusecase.Reclassifications
}

// NewReclassify creates the handler.
func NewReclassify(svc *stockusecase.Reclassifications) *Reclassify { return &Reclassify{svc: svc} }

func writeReclassError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *stockusecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
		return
	}
	for _, e := range reclassErrors {
		if errors.Is(err, e.err) {
			response.Error(w, r, e.status, e.code, e.msg)
			return
		}
	}
	switch {
	case errors.Is(err, stockusecase.ErrNotFound), errors.Is(err, ledger.ErrUnitNotFound):
		response.NotFound(w, r, "Reclassification not found")
	case errors.Is(err, ledger.ErrConcurrentUpdate):
		response.Conflict(w, r, response.CodeConflict, "The unit changed concurrently, retry")
	default:
		response.InternalErr(w, r, err, "reclassification request failed")
	}
}

func reclassCaller(r *http.Request) stockusecase.Caller {
	f, _ := scopefilter.From(r.Context())
	c := stockusecase.Caller{
		Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f,
		UserAgent: r.UserAgent(),
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip, err := netip.ParseAddr(host); err == nil {
			c.IP = &ip
		}
	}
	return c
}

// decodeOptional decodes a JSON body; an empty body is allowed.
func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func reclassUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Reclassification not found")
		return uuid.Nil, false
	}
	return id, true
}

type reclassRequestBody struct {
	Barcode       string `json:"barcode"`
	UnitUUID      string `json:"unit_uuid"`
	ToProductUUID string `json:"to_product_uuid"`
	Reason        string `json:"reason"`
}

type reclassDecisionBody struct {
	Note string `json:"note"`
}

// Create serves POST /v1/stock/reclassifications.
func (h *Reclassify) Create(w http.ResponseWriter, r *http.Request) {
	var body reclassRequestBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, err := h.svc.Request(r.Context(), reclassCaller(r), stockusecase.RequestInput{
		Barcode: body.Barcode, UnitUUID: body.UnitUUID, ToProductUUID: body.ToProductUUID, Reason: body.Reason,
	})
	if err != nil {
		writeReclassError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// List serves GET /v1/stock/reclassifications?status&limit&offset.
func (h *Reclassify) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), reclassCaller(r), stockusecase.ListFilter{
		Status: r.URL.Query().Get("status"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeReclassError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get serves GET /v1/stock/reclassifications/{uuid}.
func (h *Reclassify) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := reclassUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), reclassCaller(r), id)
	if err != nil {
		writeReclassError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Approve serves POST /v1/stock/reclassifications/{uuid}/approve.
func (h *Reclassify) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := reclassUUID(w, r)
	if !ok {
		return
	}
	var body reclassDecisionBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, _, err := h.svc.Approve(r.Context(), reclassCaller(r), id, body.Note)
	if err != nil {
		writeReclassError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Reject serves POST /v1/stock/reclassifications/{uuid}/reject.
func (h *Reclassify) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.svc.Reject)
}

// Cancel serves POST /v1/stock/reclassifications/{uuid}/cancel.
func (h *Reclassify) Cancel(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.svc.Cancel)
}

func (h *Reclassify) decide(w http.ResponseWriter, r *http.Request,
	fn func(ctx context.Context, c stockusecase.Caller, id uuid.UUID, note string) (model.Reclassification, error)) {
	id, ok := reclassUUID(w, r)
	if !ok {
		return
	}
	var body reclassDecisionBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, err := fn(r.Context(), reclassCaller(r), id, body.Note)
	if err != nil {
		writeReclassError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
