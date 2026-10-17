package handler

// TEC-768 (Depo F4.3): barcode suggestions, pick locations, the pick list,
// the delivery method catalog and the shipping documents.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the TEC-768 endpoints.
const (
	CodeDeliveryMethodSystem = "DELIVERY_METHOD_SYSTEM"
	CodeDeliveryMethodInUse  = "DELIVERY_METHOD_IN_USE"
	CodeNotShipped           = "ORDER_NOT_SHIPPED"
	CodeReceiptNotAllowed    = "ORDER_RECEIPT_NOT_ALLOWED"
	CodeDocumentUnavailable  = "ORDER_DOCUMENT_UNAVAILABLE"
)

// writePickingError maps the TEC-768 errors, then the order errors.
func writePickingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ord.ErrDeliveryMethodNotFound):
		response.NotFound(w, r, "Delivery method not found")
	case errors.Is(err, ord.ErrDeliveryMethodSystem):
		response.Conflict(w, r, CodeDeliveryMethodSystem, "The system delivery method cannot be changed")
	case errors.Is(err, ord.ErrDeliveryMethodInUse):
		response.Conflict(w, r, CodeDeliveryMethodInUse, "Orders use this delivery method; deactivate it instead")
	case errors.Is(err, ord.ErrDocumentNotFound):
		response.NotFound(w, r, "Document not found")
	case errors.Is(err, ord.ErrShippingDocumentNotShipped):
		response.Conflict(w, r, CodeNotShipped, "The order has not shipped")
	case errors.Is(err, ord.ErrReceiptNotAllowed):
		response.Conflict(w, r, CodeReceiptNotAllowed, "A delivery receipt needs a shipped or received order")
	case errors.Is(err, ord.ErrDocumentUnavailable):
		response.ServiceUnavailable(w, r, CodeDocumentUnavailable, "Document storage or PDF renderer is unavailable")
	default:
		writeError(w, r, err)
	}
}

// Suggest (GET /v1/orders/{uuid}/items/{item_uuid}/suggestions):
// include_unlocated=true adds units without a location.
func (h *Handler) Suggest(w http.ResponseWriter, r *http.Request) {
	id, item, ok := pathItem(w, r)
	if !ok {
		return
	}
	inc, err := apiquery.Bool(r.URL.Query(), "include_unlocated")
	if err != nil {
		writeError(w, r, err)
		return
	}
	in := ord.SuggestInput{IncludeUnlocated: inc != nil && *inc}
	out, err := h.svc.Suggest(r.Context(), caller(r), id, item, in)
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type applyUnitBody struct {
	UnitUUID string       `json:"unit_uuid"`
	Quantity *int64       `json:"quantity"`
	Meters   *json.Number `json:"meters"`
}

type applyBody struct {
	Units []applyUnitBody `json:"units"`
}

// ApplySuggestions (POST /v1/orders/{uuid}/items/{item_uuid}/suggestions/apply):
// assigns the confirmed suggestions in one transaction.
func (h *Handler) ApplySuggestions(w http.ResponseWriter, r *http.Request) {
	id, item, ok := pathItem(w, r)
	if !ok {
		return
	}
	var body applyBody
	if !decode(w, r, &body) {
		return
	}
	units := make([]ord.ApplyInput, 0, len(body.Units))
	for _, u := range body.Units {
		in := ord.ApplyInput{UnitUUID: u.UnitUUID, Quantity: u.Quantity}
		if u.Meters != nil {
			m := u.Meters.String()
			in.Meters = &m
		}
		units = append(units, in)
	}
	o, err := h.svc.ApplySuggestions(r.Context(), caller(r), id, item, units)
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// PickLocations (GET /v1/orders/{uuid}/items/{item_uuid}/pick-locations).
func (h *Handler) PickLocations(w http.ResponseWriter, r *http.Request) {
	id, item, ok := pathItem(w, r)
	if !ok {
		return
	}
	out, err := h.svc.PickLocations(r.Context(), caller(r), id, item)
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// PickList (GET /v1/orders/{uuid}/pick-list).
func (h *Handler) PickList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.PickList(r.Context(), caller(r), id)
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RegenerateShippingDocument (POST /v1/orders/{uuid}/shipping-document).
func (h *Handler) RegenerateShippingDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.RegenerateShippingDocument(r.Context(), caller(r), id)
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// Document (GET /v1/orders/{uuid}/documents/{kind}): kind shipping (the
// shipping note PDF) or receipt (the buyer's delivery receipt).
func (h *Handler) Document(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	f, err := h.svc.OpenDocument(r.Context(), caller(r), id, r.PathValue("kind"))
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+f.Filename+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// AttachReceipt (POST /v1/orders/{uuid}/receipt-document, multipart "file").
func (h *Handler) AttachReceipt(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ord.MaxReceiptBytes+(1<<20))
	if err := r.ParseMultipartForm(ord.MaxReceiptBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	o, err := h.svc.AttachReceipt(r.Context(), caller(r), id, ord.ReceiptInput{Body: file, Size: header.Size})
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// --- Delivery methods ------------------------------------------------------------

// ListDeliveryMethods (GET /v1/delivery-methods): q, active, is_system,
// sort (name, code, sort_order, created_at; default sort_order), limit, offset.
func (h *Handler) ListDeliveryMethods(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	sort, err := apiquery.ResolveSort(q.Sort, ord.DeliveryMethodSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	active, err := apiquery.Bool(values, "active")
	if err != nil {
		writeError(w, r, err)
		return
	}
	system, err := apiquery.Bool(values, "is_system")
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListDeliveryMethods(r.Context(), caller(r), ord.DeliveryMethodFilter{
		Q: q.Q, Active: active, IsSystem: system, Sort: sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

type deliveryMethodBody struct {
	Code                string  `json:"code"`
	Name                string  `json:"name"`
	TrackingURLTemplate *string `json:"tracking_url_template"`
	Active              *bool   `json:"active"`
	SortOrder           *int32  `json:"sort_order"`
}

func (b deliveryMethodBody) input() ord.DeliveryMethodInput {
	return ord.DeliveryMethodInput{
		Code: b.Code, Name: b.Name, TrackingURLTemplate: b.TrackingURLTemplate, Active: b.Active, SortOrder: b.SortOrder,
	}
}

// CreateDeliveryMethod (POST /v1/delivery-methods).
func (h *Handler) CreateDeliveryMethod(w http.ResponseWriter, r *http.Request) {
	var body deliveryMethodBody
	if !decode(w, r, &body) {
		return
	}
	m, err := h.svc.CreateDeliveryMethod(r.Context(), caller(r), body.input())
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, m)
}

func pathMethod(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Delivery method not found")
		return uuid.Nil, false
	}
	return id, true
}

// UpdateDeliveryMethod (PUT /v1/delivery-methods/{uuid}).
func (h *Handler) UpdateDeliveryMethod(w http.ResponseWriter, r *http.Request) {
	id, ok := pathMethod(w, r)
	if !ok {
		return
	}
	var body deliveryMethodBody
	if !decode(w, r, &body) {
		return
	}
	m, err := h.svc.UpdateDeliveryMethod(r.Context(), caller(r), id, body.input())
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

// DeleteDeliveryMethod (DELETE /v1/delivery-methods/{uuid}).
func (h *Handler) DeleteDeliveryMethod(w http.ResponseWriter, r *http.Request) {
	id, ok := pathMethod(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteDeliveryMethod(r.Context(), caller(r), id); err != nil {
		writePickingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
