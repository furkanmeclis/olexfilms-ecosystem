package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the stock adjustment and return inspection endpoints
// (TEC-1226).
const (
	CodeAdjustmentNotFound      = "ADJUSTMENT_NOT_FOUND"
	CodeAdjustmentPhotoNotFound = "ADJUSTMENT_PHOTO_NOT_FOUND"
	CodeAdjustmentStatus        = "ADJUSTMENT_INVALID_STATUS"
	CodeAdjustmentForbidden     = "ADJUSTMENT_ADJUST_FORBIDDEN"
	CodeAdjustmentSelfApproval  = "ADJUSTMENT_SELF_APPROVAL"
	CodeAdjustmentEmpty         = "ADJUSTMENT_EMPTY"
	CodeAdjustmentPhotoRequired = "ADJUSTMENT_PHOTO_REQUIRED"
	CodeAdjustmentUnit          = "ADJUSTMENT_UNIT_NOT_ALLOWED"
	CodeAdjustmentUnitBusy      = "ADJUSTMENT_UNIT_BUSY"
	CodeAdjustmentStale         = "ADJUSTMENT_STALE"
	CodeStorageUnavailable      = "STORAGE_UNAVAILABLE"
	CodeNotQuarantined          = "UNIT_NOT_QUARANTINED"
)

// Adjustments serves /v1/warehouse/adjustments and
// /v1/warehouse/return-inspections.
type Adjustments struct {
	svc *wh.Adjustments
}

// NewAdjustments builds the handler.
func NewAdjustments(svc *wh.Adjustments) *Adjustments { return &Adjustments{svc: svc} }

func writeAdjustmentError(w http.ResponseWriter, r *http.Request, err error) {
	msg := strings.TrimPrefix(err.Error(), "warehouse: ")
	switch {
	case errors.Is(err, wh.ErrAdjustmentNotFound):
		response.Error(w, r, http.StatusNotFound, CodeAdjustmentNotFound, "Stock adjustment not found")
	case errors.Is(err, wh.ErrAdjustmentPhotoNotFound):
		response.Error(w, r, http.StatusNotFound, CodeAdjustmentPhotoNotFound, "Photo not found")
	case errors.Is(err, wh.ErrNotQuarantined):
		response.Error(w, r, http.StatusNotFound, CodeNotQuarantined, "The unit is not waiting for inspection in this organization")
	case errors.Is(err, wh.ErrAdjustmentStatus):
		response.Conflict(w, r, CodeAdjustmentStatus, "The adjustment's status does not allow this action")
	case errors.Is(err, wh.ErrAdjustmentForbidden):
		response.Error(w, r, http.StatusForbidden, CodeAdjustmentForbidden, "This action needs stock.adjust")
	case errors.Is(err, wh.ErrAdjustmentSelfApproval):
		response.Error(w, r, http.StatusForbidden, CodeAdjustmentSelfApproval,
			"Above the threshold another person approves the adjustment")
	case errors.Is(err, wh.ErrAdjustmentEmpty):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeAdjustmentEmpty, "Add at least one line")
	case errors.Is(err, wh.ErrAdjustmentPhotoNeeded):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeAdjustmentPhotoRequired, "A damage adjustment needs at least one photo")
	case errors.Is(err, wh.ErrAdjustmentUnit):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeAdjustmentUnit, msg)
	case errors.Is(err, wh.ErrAdjustmentUnitBusy):
		response.Conflict(w, r, CodeAdjustmentUnitBusy, msg)
	case errors.Is(err, wh.ErrAdjustmentStale):
		response.Conflict(w, r, CodeAdjustmentStale, msg)
	case errors.Is(err, wh.ErrStorageUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, CodeStorageUnavailable, "File storage is not configured")
	default:
		writeError(w, r, err)
	}
}

func (h *Adjustments) withID(w http.ResponseWriter, r *http.Request, fn func(id uuid.UUID) (any, error)) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := fn(id)
	if errors.Is(err, errHandled) {
		return
	}
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type adjustmentBody struct {
	Type          string  `json:"type"`
	ReasonCode    *string `json:"reason_code"`
	Note          *string `json:"note"`
	WarehouseUUID *string `json:"warehouse_uuid"`
}

func (b adjustmentBody) input() wh.AdjustmentInput {
	return wh.AdjustmentInput{Type: b.Type, ReasonCode: b.ReasonCode, Note: b.Note, WarehouseUUID: b.WarehouseUUID}
}

type adjustmentLineBody struct {
	Barcode      string  `json:"barcode"`
	Quantity     *int32  `json:"quantity"`
	Meters       *string `json:"meters"`
	LocationUUID *string `json:"location_uuid"`
}

type adjustmentCancelBody struct {
	Reason *string `json:"reason"`
}

type adjustmentSettingsBody struct {
	ThresholdQuantity int32  `json:"threshold_quantity"`
	ThresholdMeters   string `json:"threshold_meters"`
}

type inspectionBody struct {
	Barcode      string  `json:"barcode"`
	UnitUUID     *string `json:"unit_uuid"`
	Decision     string  `json:"decision"`
	ReturnReason string  `json:"return_reason"`
	Note         *string `json:"note"`
	LocationUUID *string `json:"location_uuid"`
}

// List (GET /v1/warehouse/adjustments?status&type&source&warehouse_uuid&uuid&created_from&created_to&q&sort&limit&offset).
func (h *Adjustments) List(w http.ResponseWriter, r *http.Request) {
	f, err := wh.ParseAdjustmentListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), countCaller(r), f)
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Create (POST /v1/warehouse/adjustments).
func (h *Adjustments) Create(w http.ResponseWriter, r *http.Request) {
	var b adjustmentBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Create(r.Context(), countCaller(r), b.input())
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// Get (GET /v1/warehouse/adjustments/{uuid}).
func (h *Adjustments) Get(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Get(r.Context(), countCaller(r), id) })
}

// Update (PATCH /v1/warehouse/adjustments/{uuid}): draft header.
func (h *Adjustments) Update(w http.ResponseWriter, r *http.Request) {
	var b adjustmentBody
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		if !decode(w, r, &b) {
			return nil, errHandled
		}
		return h.svc.Update(r.Context(), countCaller(r), id, b.input())
	})
}

// AddLine (POST /v1/warehouse/adjustments/{uuid}/lines).
func (h *Adjustments) AddLine(w http.ResponseWriter, r *http.Request) {
	var b adjustmentLineBody
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		if !decode(w, r, &b) {
			return nil, errHandled
		}
		return h.svc.AddLine(r.Context(), countCaller(r), id, wh.AdjustmentLineInput{
			Barcode: b.Barcode, Quantity: b.Quantity, Meters: b.Meters, LocationUUID: b.LocationUUID,
		})
	})
}

// DeleteLine (DELETE /v1/warehouse/adjustments/{uuid}/lines/{line_uuid}).
func (h *Adjustments) DeleteLine(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		lineID, err := uuid.Parse(r.PathValue("line_uuid"))
		if err != nil {
			return nil, wh.ErrAdjustmentNotFound
		}
		return h.svc.DeleteLine(r.Context(), countCaller(r), id, lineID)
	})
}

// Submit (POST /v1/warehouse/adjustments/{uuid}/submit).
func (h *Adjustments) Submit(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Submit(r.Context(), countCaller(r), id) })
}

// Approve (POST /v1/warehouse/adjustments/{uuid}/approve).
func (h *Adjustments) Approve(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Approve(r.Context(), countCaller(r), id) })
}

// Cancel (POST /v1/warehouse/adjustments/{uuid}/cancel).
func (h *Adjustments) Cancel(w http.ResponseWriter, r *http.Request) {
	var b adjustmentCancelBody
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		if r.ContentLength != 0 && !decode(w, r, &b) {
			return nil, errHandled
		}
		return h.svc.Cancel(r.Context(), countCaller(r), id, b.Reason)
	})
}

// AddPhoto (POST /v1/warehouse/adjustments/{uuid}/photos, multipart "file").
func (h *Adjustments) AddPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, wh.MaxAdjustmentPhotoBytes+(1<<20))
	if err := r.ParseMultipartForm(wh.MaxAdjustmentPhotoBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	v, err := h.svc.AddPhoto(r.Context(), countCaller(r), id, wh.AdjustmentPhotoInput{Body: file, Size: header.Size})
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// DeletePhoto (DELETE /v1/warehouse/adjustments/{uuid}/photos/{photo_uuid}).
func (h *Adjustments) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		photoID, err := uuid.Parse(r.PathValue("photo_uuid"))
		if err != nil {
			return nil, wh.ErrAdjustmentPhotoNotFound
		}
		return h.svc.DeletePhoto(r.Context(), countCaller(r), id, photoID)
	})
}

// Photo (GET /v1/warehouse/adjustments/{uuid}/photos/{photo_uuid}).
func (h *Adjustments) Photo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(r.PathValue("photo_uuid"))
	if err != nil {
		writeAdjustmentError(w, r, wh.ErrAdjustmentPhotoNotFound)
		return
	}
	f, err := h.svc.Photo(r.Context(), countCaller(r), id, photoID)
	if err != nil {
		writeAdjustmentError(w, r, err)
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

// Settings (GET /v1/warehouse/adjustment-settings).
func (h *Adjustments) Settings(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Settings(r.Context(), countCaller(r))
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// UpdateSettings (PUT /v1/warehouse/adjustment-settings).
func (h *Adjustments) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var b adjustmentSettingsBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.UpdateSettings(r.Context(), countCaller(r), wh.AdjustmentSettings{
		ThresholdQuantity: b.ThresholdQuantity, ThresholdMeters: b.ThresholdMeters,
	})
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Quarantine (GET /v1/warehouse/return-inspections?q&limit&offset): units
// waiting for inspection.
func (h *Adjustments) Quarantine(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Quarantine(r.Context(), countCaller(r), q.Q, q.Limit, q.Offset)
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// QuarantineSummary (GET /v1/warehouse/return-inspections/summary).
func (h *Adjustments) QuarantineSummary(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.QuarantineSummary(r.Context(), countCaller(r))
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Inspect (POST /v1/warehouse/return-inspections).
func (h *Adjustments) Inspect(w http.ResponseWriter, r *http.Request) {
	var b inspectionBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Inspect(r.Context(), countCaller(r), wh.InspectionInput{
		Barcode: b.Barcode, UnitUUID: b.UnitUUID, Decision: b.Decision, ReturnReason: b.ReturnReason,
		Note: b.Note, LocationUUID: b.LocationUUID,
	})
	if err != nil {
		writeAdjustmentError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// errHandled: the response is already written (decode failed).
var errHandled = errors.New("handled")
