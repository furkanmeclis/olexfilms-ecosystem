package handler

// TEC-1227: partial shipment and receipt discrepancies.

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the partial shipment and discrepancy endpoints.
const (
	CodeNotPartiallyShippable = "ORDER_NOT_PARTIALLY_SHIPPABLE"
	CodeNothingPicked         = "ORDER_NOTHING_PICKED"
	CodeFullyPicked           = "ORDER_FULLY_PICKED"
	CodeDiscrepancyClosed     = "ORDER_DISCREPANCY_CLOSED"
	CodeDiscrepancyPhotoLimit = "ORDER_DISCREPANCY_PHOTO_LIMIT"
)

// writePartialError maps the TEC-1227 errors, then the picking and order errors.
func writePartialError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ord.ErrNotPartiallyShippable):
		response.Conflict(w, r, CodeNotPartiallyShippable, "Only a preparing order can be shipped partially")
	case errors.Is(err, ord.ErrNothingPicked):
		response.Conflict(w, r, CodeNothingPicked, "Nothing is picked yet")
	case errors.Is(err, ord.ErrFullyPicked):
		response.Conflict(w, r, CodeFullyPicked, "Every line is fully picked; ship the whole order")
	case errors.Is(err, ord.ErrDiscrepancyNotFound):
		response.NotFound(w, r, "Discrepancy not found")
	case errors.Is(err, ord.ErrDiscrepancyClosed):
		response.Conflict(w, r, CodeDiscrepancyClosed, "The discrepancy is already resolved")
	case errors.Is(err, ord.ErrDiscrepancyPhotoLimit):
		response.Conflict(w, r, CodeDiscrepancyPhotoLimit, "A discrepancy has at most 5 photos")
	default:
		writePickingError(w, r, err)
	}
}

type partialShipBody struct {
	Reason *string `json:"reason"`
}

// PartialShip (POST /v1/orders/{uuid}/partial-ship): the seller's warehouse
// staff ship the picked part; the rest becomes a backorder.
func (h *Handler) PartialShip(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body partialShipBody
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	o, err := h.svc.PartialShip(r.Context(), caller(r), id, ord.PartialShipInput{Reason: body.Reason})
	if err != nil {
		writePartialError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

func pathDiscrepancy(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	id, ok := pathUUID(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	d, err := uuid.Parse(r.PathValue("discrepancy_uuid"))
	if err != nil {
		response.NotFound(w, r, "Discrepancy not found")
		return uuid.Nil, uuid.Nil, false
	}
	return id, d, true
}

type resolveDiscrepancyBody struct {
	Resolution string  `json:"resolution"`
	Note       *string `json:"note"`
}

// ResolveDiscrepancy (POST /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/resolve).
func (h *Handler) ResolveDiscrepancy(w http.ResponseWriter, r *http.Request) {
	id, d, ok := pathDiscrepancy(w, r)
	if !ok {
		return
	}
	var body resolveDiscrepancyBody
	if !decode(w, r, &body) {
		return
	}
	o, err := h.svc.ResolveDiscrepancy(r.Context(), caller(r), id, d, ord.ResolveDiscrepancyInput{
		Resolution: body.Resolution, Note: body.Note,
	})
	if err != nil {
		writePartialError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// AddDiscrepancyPhoto (POST /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/photos,
// multipart "file").
func (h *Handler) AddDiscrepancyPhoto(w http.ResponseWriter, r *http.Request) {
	id, d, ok := pathDiscrepancy(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ord.MaxDiscrepancyPhotoLen+(1<<20))
	if err := r.ParseMultipartForm(ord.MaxDiscrepancyPhotoLen + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	o, err := h.svc.AddDiscrepancyPhoto(r.Context(), caller(r), id, d, ord.ReceiptInput{Body: file, Size: header.Size})
	if err != nil {
		writePartialError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, o)
}

// DiscrepancyPhoto (GET /v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/photos/{index}).
func (h *Handler) DiscrepancyPhoto(w http.ResponseWriter, r *http.Request) {
	id, d, ok := pathDiscrepancy(w, r)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		response.NotFound(w, r, "Photo not found")
		return
	}
	f, err := h.svc.OpenDiscrepancyPhoto(r.Context(), caller(r), id, d, idx)
	if err != nil {
		writePartialError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+f.Filename+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}
