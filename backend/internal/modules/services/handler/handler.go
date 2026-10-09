// Package handler serves the /v1/services endpoints (TEC-179, F1-05b).
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	psmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to services.
const (
	CodeInvalidTransition           = "SERVICE_INVALID_TRANSITION"
	CodeNotEditable                 = "SERVICE_NOT_EDITABLE"
	CodeUnitInUse                   = "SERVICE_UNIT_IN_USE"
	CodeUnitNotAvailable            = "SERVICE_UNIT_NOT_AVAILABLE"
	CodeTooManyImages               = "SERVICE_TOO_MANY_IMAGES"
	CodeContractRequired            = "CONTRACT_REQUIRED"
	CodeCertificateApprovalRequired = "CERTIFICATE_APPROVAL_REQUIRED"
	CodeIncomeAlreadyRecorded       = "SERVICE_INCOME_ALREADY_RECORDED"
	CodeIncomeNotRecorded           = "SERVICE_INCOME_NOT_RECORDED"

	// TEC-230: consumption correction.
	CodeCorrectionWindowClosed   = "SERVICE_CORRECTION_WINDOW_CLOSED"
	CodeItemWarrantyActive       = "SERVICE_ITEM_WARRANTY_ACTIVE"
	CodeConsumptionNotReversible = "SERVICE_CONSUMPTION_NOT_REVERSIBLE"
	CodeItemAlreadyCorrected     = "SERVICE_ITEM_ALREADY_CORRECTED"
)

// imageField is the multipart field of the image upload.
const imageField = "image"

// Handler serves service endpoints.
type Handler struct {
	svc     *svcuc.Service
	store   storage.Driver
	exports ExportRequester
}

// New creates the handler; store may be nil (images answer 503).
func New(svc *svcuc.Service, store storage.Driver) *Handler { return &Handler{svc: svc, store: store} }

type ExportRequester interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

func (h *Handler) WithExports(exports ExportRequester) { h.exports = exports }

func caller(r *http.Request) svcuc.Caller {
	f, _ := scopefilter.From(r.Context())
	return svcuc.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *svcuc.ValidationError
	var qe *apiquery.ValidationError
	var incomplete *psmodel.IncompleteError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, svcuc.ErrNotFound):
		response.NotFound(w, r, "Service not found")
	case errors.Is(err, svcuc.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot perform this service action")
	case errors.Is(err, svcuc.ErrInvalidTransition):
		response.Conflict(w, r, CodeInvalidTransition, "The service status does not allow this transition")
	case errors.Is(err, svcuc.ErrNotEditable):
		response.Conflict(w, r, CodeNotEditable, "The service status does not allow this change")
	case errors.Is(err, svcuc.ErrUnitInUse):
		response.Conflict(w, r, CodeUnitInUse, "The unit is already in another open service")
	case errors.Is(err, svcuc.ErrUnitNotAvailable):
		response.Conflict(w, r, CodeUnitNotAvailable, "The unit is not available to this organization")
	case errors.Is(err, svcuc.ErrTooManyImages):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeTooManyImages, "The image limit of the service is reached")
	case errors.As(err, &incomplete):
		psmodel.WriteIncomplete(w, r, incomplete)
	case errors.Is(err, svcuc.ErrContractRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeContractRequired, "An executed intake contract is required for this service transition")
	case errors.Is(err, svcuc.ErrCertificateApprovalRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeCertificateApprovalRequired, "Certificate warning approval is required for this service transition")
	case errors.Is(err, svcuc.ErrIncomeAlreadyRecorded):
		response.Conflict(w, r, CodeIncomeAlreadyRecorded, "Income is already recorded for this service")
	case errors.Is(err, svcuc.ErrIncomeNotRecorded):
		response.Conflict(w, r, CodeIncomeNotRecorded, "Income is not recorded for this service")
	case errors.Is(err, svcuc.ErrCorrectionWindowClosed):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeCorrectionWindowClosed,
			"The consumption of this service can no longer be corrected")
	case errors.Is(err, svcuc.ErrItemWarrantyActive):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeItemWarrantyActive,
			"The item has a warranty that is not void; void it first")
	case errors.Is(err, svcuc.ErrConsumptionNotReversible):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeConsumptionNotReversible,
			"Only a whole consumption can be corrected")
	case errors.Is(err, svcuc.ErrItemAlreadyCorrected):
		response.Conflict(w, r, CodeItemAlreadyCorrected, "The consumption of this item was already corrected")
	default:
		response.InternalErr(w, r, err, "service request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Service not found")
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

// List (GET /v1/services?q&status&organization_uuid&customer_uuid&vehicle_uuid&created_from&created_to&completed_from&completed_to&sort&limit&offset).
// TEC-377: docs/list-contract.md (svcuc.ListSort, CSV status / organization_uuid).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := svcuc.ParseListFilter(r.URL.Query())
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

// Get (GET /v1/services/{uuid}).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
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

type incomeBody struct {
	Amount        string  `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
	AccountUUID   *string `json:"account_uuid"`
	Description   string  `json:"description"`
}

// RecordIncome (POST /v1/services/{uuid}/income): posts service income.
func (h *Handler) RecordIncome(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b incomeBody
	if !decode(w, r, &b) {
		return
	}
	var account *uuid.UUID
	if b.AccountUUID != nil {
		u, err := uuid.Parse(strings.TrimSpace(*b.AccountUUID))
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "account_uuid", Message: "must be a UUID"}})
			return
		}
		account = &u
	}
	v, err := h.svc.RecordIncome(r.Context(), caller(r), id, svcuc.IncomeInput{
		Amount: b.Amount, PaymentMethod: b.PaymentMethod, AccountUUID: account, Description: b.Description,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

type deleteIncomeBody struct {
	Reason string `json:"reason"`
}

// DeleteIncome (DELETE /v1/services/{uuid}/income): reverses service income.
func (h *Handler) DeleteIncome(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b deleteIncomeBody
	if r.Body != nil && r.ContentLength != 0 {
		if !decode(w, r, &b) {
			return
		}
	}
	v, err := h.svc.DeleteIncome(r.Context(), caller(r), id, b.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Profit (GET /v1/services/{uuid}/profit): returns revenue, cost and margin.
func (h *Handler) Profit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	v, err := h.svc.Profit(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type createBody struct {
	CustomerUUID   string  `json:"customer_uuid"`
	VehicleUUID    string  `json:"vehicle_uuid"`
	KM             *int64  `json:"km"`
	Package        *string `json:"package"`
	Notes          *string `json:"notes"`
	HasMeasurement bool    `json:"has_measurement"`
}

// Create (POST /v1/services): a draft service of the active organization.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var b createBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Create(r.Context(), caller(r), svcuc.CreateInput{
		CustomerUUID: b.CustomerUUID, VehicleUUID: b.VehicleUUID, KM: b.KM,
		Package: b.Package, Notes: b.Notes, HasMeasurement: b.HasMeasurement,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

type updateBody struct {
	KM             svcuc.Optional[int64]  `json:"km"`
	Package        svcuc.Optional[string] `json:"package"`
	Notes          svcuc.Optional[string] `json:"notes"`
	HasMeasurement svcuc.Optional[bool]   `json:"has_measurement"`
	VIN            svcuc.Optional[string] `json:"vin"`
}

// Update (PATCH /v1/services/{uuid}).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b updateBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Update(r.Context(), caller(r), id, svcuc.UpdateInput{
		KM: b.KM, Package: b.Package, Notes: b.Notes, HasMeasurement: b.HasMeasurement,
		VIN: b.VIN,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type itemBody struct {
	Barcode      string       `json:"barcode"`
	ProductUUID  *string      `json:"product_uuid"`
	Kind         string       `json:"kind"`
	Quantity     *int64       `json:"quantity"`
	Meters       *json.Number `json:"meters"`
	AppliedParts []string     `json:"applied_parts"`
	Notes        *string      `json:"notes"`
}

// AddItem (POST /v1/services/{uuid}/items).
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b itemBody
	if !decode(w, r, &b) {
		return
	}
	var meters *string
	if b.Meters != nil {
		m := b.Meters.String()
		meters = &m
	}
	v, err := h.svc.AddItem(r.Context(), caller(r), id, svcuc.ItemInput{
		Barcode: b.Barcode, ProductUUID: b.ProductUUID, Kind: b.Kind, Quantity: b.Quantity,
		Meters: meters, AppliedParts: b.AppliedParts, Notes: b.Notes,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// RemoveItem (DELETE /v1/services/{uuid}/items/{item}).
func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, ok := pathUUID(w, r, "item")
	if !ok {
		return
	}
	v, err := h.svc.RemoveItem(r.Context(), caller(r), id, item)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// StockUnits (GET /v1/services/{uuid}/stock-units?barcode&q&product_uuid&min_meters&limit&offset):
// the stock picker of the service organization (TEC-180).
func (h *Handler) StockUnits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	items, err := h.svc.StockUnits(r.Context(), caller(r), id, svcuc.StockFilter{
		Barcode: v.Get("barcode"), Q: v.Get("q"), ProductUUID: v.Get("product_uuid"), MinMeters: v.Get("min_meters"),
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type transitionBody struct {
	Status string  `json:"status"`
	Note   *string `json:"note"`
}

// Transition (POST /v1/services/{uuid}/transitions).
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b transitionBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Transition(r.Context(), caller(r), id, svcuc.TransitionInput{Status: b.Status, Note: b.Note})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type cancelCompletedBody struct {
	Reason string `json:"reason"`
}

// CancelCompleted (POST /v1/services/{uuid}/cancel-completed): cancels a
// completed service and reverses its warranties, stock consumption and
// accounting rows in one transaction.
func (h *Handler) CancelCompleted(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b cancelCompletedBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.CancelCompleted(r.Context(), caller(r), id, svcuc.CancelCompletedInput{
		Reason: b.Reason,
		Meta:   activity.MetaFromRequest(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type correctionBody struct {
	Reason             string  `json:"reason"`
	ReplacementBarcode *string `json:"replacement_barcode"`
}

// CorrectConsumption (POST /v1/services/{uuid}/items/{item}/consumption-correction,
// TEC-230): the item's unit goes back to stock (ledger return), optionally
// the replacement unit is consumed instead. Center only (services.cancel),
// within svcuc.CorrectionWindow of the completion.
func (h *Handler) CorrectConsumption(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, ok := pathUUID(w, r, "item")
	if !ok {
		return
	}
	var b correctionBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.CorrectConsumption(r.Context(), caller(r), id, item, svcuc.CorrectionInput{
		Reason: b.Reason, ReplacementBarcode: b.ReplacementBarcode,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func tooLarge(w http.ResponseWriter, r *http.Request) {
	response.ErrorWithDetails(w, r, http.StatusRequestEntityTooLarge, response.CodeValidationError, "image must be at most 5 MiB",
		[]response.Detail{{Field: imageField, Message: "image must be at most 5 MiB", Code: "too_large"}})
}

// UploadImage (POST /v1/services/{uuid}/images, multipart "image" and an
// optional "title"): JPEG/PNG/WebP by content sniffing, at most 5 MiB.
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	const limit = storage.MaxProductImageBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			tooLarge(w, r)
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile(imageField)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, imageField+" is required")
		return
	}
	defer func() { _ = file.Close() }()
	if header.Size > storage.MaxProductImageBytes {
		tooLarge(w, r)
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	if n == 0 || header.Size <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, "image file is required")
		return
	}
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "image must be image/jpeg, image/png, or image/webp")
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	c := caller(r)
	slot, err := h.svc.PrepareImage(r.Context(), c, id, ext)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.store.Upload(r.Context(), storage.File{
		Body: io.MultiReader(bytes.NewReader(head[:n]), file), Size: header.Size,
		ContentType: mime, Filename: header.Filename,
	}, slot.ObjectKey); err != nil {
		response.InternalErr(w, r, err, "image upload failed")
		return
	}
	var title *string
	if t := strings.TrimSpace(r.FormValue("title")); t != "" {
		title = &t
	}
	img, err := h.svc.AddImage(r.Context(), c, id, slot.ObjectKey, title)
	if err != nil {
		_ = h.store.Delete(r.Context(), slot.ObjectKey)
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, img)
}

// DownloadImage (GET /v1/services/{uuid}/images/{image}).
func (h *Handler) DownloadImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	img, ok := pathUUID(w, r, "image")
	if !ok {
		return
	}
	key, err := h.svc.ImageObject(r.Context(), caller(r), id, img)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.store == nil {
		response.NotFound(w, r, "Service not found")
		return
	}
	rc, size, err := h.store.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "Service not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// DeleteImage (DELETE /v1/services/{uuid}/images/{image}).
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	img, ok := pathUUID(w, r, "image")
	if !ok {
		return
	}
	key, err := h.svc.RemoveImage(r.Context(), caller(r), id, img)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.store != nil {
		_ = h.store.Delete(r.Context(), key)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ParseCreatedBound parses a created_from / created_to bound: RFC3339 as
// given, or a YYYY-MM-DD day in UTC. A day as the upper bound (end) means
// the whole day, so it moves to the next midnight (exclusive bound).
func ParseCreatedBound(raw string, end bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		if end {
			d = d.Add(24 * time.Hour)
		}
		return &d, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}
