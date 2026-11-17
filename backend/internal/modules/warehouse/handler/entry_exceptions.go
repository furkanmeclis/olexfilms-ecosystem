package handler

// TEC-1225: receiving exceptions (discrepancy record, attachments, line
// details, undo, label print step).
import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the receiving exceptions.
const (
	CodeStockEntryDiscrepancyRequired = "STOCK_ENTRY_DISCREPANCY_REQUIRED"
	CodeStockEntryUndoBlocked         = "STOCK_ENTRY_UNDO_BLOCKED"
	CodeStockEntryNotConfirmed        = "STOCK_ENTRY_NOT_CONFIRMED"
	CodeStockEntryUndoImport          = "STOCK_ENTRY_UNDO_IMPORT"
	CodeStockEntryCancelled           = "STOCK_ENTRY_CANCELLED"
	CodeStockEntryAttachmentInUse     = "STOCK_ENTRY_ATTACHMENT_IN_USE"
	CodeStockAdjustRequired           = "STOCK_ADJUST_REQUIRED"
	CodeStorageUnavailable            = "STORAGE_UNAVAILABLE"
)

// DiscrepancyRequiredData is the data of a STOCK_ENTRY_DISCREPANCY_REQUIRED error.
type DiscrepancyRequiredData struct {
	Missing []string `json:"missing"`
	Extras  []string `json:"extras"`
}

// UndoBlockedData is the data of a STOCK_ENTRY_UNDO_BLOCKED error.
type UndoBlockedData struct {
	Blockers []wh.UndoBlocker `json:"blockers"`
}

func writeExceptionError(w http.ResponseWriter, r *http.Request, err error) {
	var dr *wh.DiscrepancyRequiredError
	var ub *wh.UndoBlockedError
	switch {
	case errors.As(err, &dr):
		details := make([]response.Detail, 0, len(dr.Missing)+len(dr.Extras))
		for _, b := range dr.Missing {
			details = append(details, response.Detail{Field: "missing", Message: b, Code: "missing"})
		}
		for _, b := range dr.Extras {
			details = append(details, response.Detail{Field: "extras", Message: b, Code: "extra"})
		}
		response.ErrorWithData(w, r, http.StatusConflict, CodeStockEntryDiscrepancyRequired,
			"Missing lines and extra barcodes need a decision before confirming", details,
			DiscrepancyRequiredData{Missing: nonNil(dr.Missing), Extras: nonNil(dr.Extras)})
	case errors.As(err, &ub):
		details := make([]response.Detail, 0, len(ub.Blockers))
		for _, b := range ub.Blockers {
			details = append(details, response.Detail{Field: "blockers", Message: b.Barcode, Code: b.MovementType})
		}
		response.ErrorWithData(w, r, http.StatusConflict, CodeStockEntryUndoBlocked,
			"Some units moved after the entry; the entry cannot be undone", details, UndoBlockedData{Blockers: ub.Blockers})
	case errors.Is(err, wh.ErrEntryNotConfirmed):
		response.Conflict(w, r, CodeStockEntryNotConfirmed, "Only a confirmed stock entry can be undone")
	case errors.Is(err, wh.ErrEntryUndoImport):
		response.Conflict(w, r, CodeStockEntryUndoImport, "An import entry is undone through its import job")
	case errors.Is(err, wh.ErrEntryCancelled):
		response.Conflict(w, r, CodeStockEntryCancelled, "The stock entry is cancelled")
	case errors.Is(err, wh.ErrEntryAttachmentInUse):
		response.Conflict(w, r, CodeStockEntryAttachmentInUse, "The attachment is evidence of the discrepancy record")
	case errors.Is(err, wh.ErrEntryAttachmentNotFound):
		response.NotFound(w, r, "Attachment not found")
	case errors.Is(err, wh.ErrEntryAdjustForbidden):
		response.Error(w, r, http.StatusForbidden, CodeStockAdjustRequired, "Undoing a stock entry needs the stock adjust permission")
	case errors.Is(err, wh.ErrEntryStorage):
		response.ServiceUnavailable(w, r, CodeStorageUnavailable, "File storage or PDF rendering is not configured")
	default:
		writeError(w, r, err)
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type decisionBody struct {
	Barcode         string      `json:"barcode"`
	Kind            string      `json:"kind"`
	Decision        string      `json:"decision"`
	Reason          string      `json:"reason"`
	AttachmentUUIDs []uuid.UUID `json:"attachment_uuids"`
	LocationUUID    *uuid.UUID  `json:"location_uuid"`
}

type confirmBody struct {
	Decisions []decisionBody `json:"decisions"`
}

// decodeConfirm reads the optional confirm body.
func decodeConfirm(w http.ResponseWriter, r *http.Request) (wh.ConfirmInput, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return wh.ConfirmInput{}, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return wh.ConfirmInput{}, true
	}
	var b confirmBody
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return wh.ConfirmInput{}, false
	}
	in := wh.ConfirmInput{Decisions: make([]wh.DiscrepancyDecision, 0, len(b.Decisions))}
	for _, d := range b.Decisions {
		in.Decisions = append(in.Decisions, wh.DiscrepancyDecision{
			Barcode: d.Barcode, Kind: d.Kind, Decision: d.Decision, Reason: d.Reason,
			AttachmentUUIDs: d.AttachmentUUIDs, LocationUUID: d.LocationUUID,
		})
	}
	return in, true
}

// Receipt (GET /v1/warehouse/stock-entries/{uuid}/receipt).
func (h *Entries) Receipt(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Receipt(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReceiptPDF (GET /v1/warehouse/stock-entries/{uuid}/receipt.pdf?locale=).
func (h *Entries) ReceiptPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(r.URL.Query().Get("locale")); ok {
		locale = string(l)
	}
	pdf, name, err := h.svc.ReceiptPDF(r.Context(), entryCaller(r), id, locale)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// AddAttachment (POST /v1/warehouse/stock-entries/{uuid}/attachments,
// multipart: file, kind).
func (h *Entries) AddAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, wh.MaxEntryAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(wh.MaxEntryAttachmentBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid multipart body (max 15 MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "is required"}})
		return
	}
	defer func() { _ = file.Close() }()
	out, err := h.svc.AddAttachment(r.Context(), entryCaller(r), id, wh.AttachmentInput{
		Kind: r.FormValue("kind"), Filename: header.Filename, Body: file, Size: header.Size,
	})
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func attachmentUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("attachment_uuid"))
	if err != nil {
		response.NotFound(w, r, "Attachment not found")
		return uuid.Nil, false
	}
	return id, true
}

// Attachment (GET /v1/warehouse/stock-entries/{uuid}/attachments/{attachment_uuid}).
func (h *Entries) Attachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	att, ok := attachmentUUID(w, r)
	if !ok {
		return
	}
	rc, size, meta, err := h.svc.AttachmentFile(r.Context(), entryCaller(r), id, att)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", meta.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": meta.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// DeleteAttachment (DELETE /v1/warehouse/stock-entries/{uuid}/attachments/{attachment_uuid}).
func (h *Entries) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	att, ok := attachmentUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAttachment(r.Context(), entryCaller(r), id, att); err != nil {
		writeEntryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type lineDetailsBody struct {
	Lot            *string `json:"lot"`
	ProductionDate *string `json:"production_date"`
	BoxNo          *string `json:"box_no"`
}

// UpdateLine (PATCH /v1/warehouse/stock-entries/{uuid}/lines/{line_uuid}).
func (h *Entries) UpdateLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	lid, ok := lineUUID(w, r)
	if !ok {
		return
	}
	var b lineDetailsBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateLine(r.Context(), entryCaller(r), id, lid, wh.LineDetailsInput{
		Lot: b.Lot, ProductionDate: b.ProductionDate, BoxNo: b.BoxNo,
	})
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UndoCheck (GET /v1/warehouse/stock-entries/{uuid}/undo-check).
func (h *Entries) UndoCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CheckUndo(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type undoBody struct {
	Reason string `json:"reason"`
}

// Undo (POST /v1/warehouse/stock-entries/{uuid}/undo; stock.adjust + step-up).
func (h *Entries) Undo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b undoBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Undo(r.Context(), entryCaller(r), id, b.Reason)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type labelPrintBody struct {
	Count int `json:"count"`
}

// LabelPrint (POST /v1/warehouse/stock-entries/{uuid}/label-prints): the
// timeline step of a label print.
func (h *Entries) LabelPrint(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b labelPrintBody
	if !decode(w, r, &b) {
		return
	}
	if err := h.svc.RecordLabelPrint(r.Context(), entryCaller(r), id, b.Count); err != nil {
		writeEntryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
