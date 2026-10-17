package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type EntryImportService interface {
	Upload(context.Context, int64, *int64, string, ioengine.ImportFormat, string, string, io.Reader) (importusecase.ImportJobView, error)
	Sample(context.Context, string, ioengine.ImportFormat, string) ([]byte, string, error)
}
type EntryImport struct {
	entries *wh.StockEntries
	adapter *wh.EntryBarcodeImporter
	imports EntryImportService
}

func NewEntryImport(entries *wh.StockEntries, adapter *wh.EntryBarcodeImporter, imports EntryImportService) *EntryImport {
	return &EntryImport{entries: entries, adapter: adapter, imports: imports}
}
func (h *EntryImport) target(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := pathUUID(w, r)
	if !ok {
		return id, false
	}
	entry, err := h.entries.Get(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return id, false
	}
	if entry.Mode != wh.EntryModeWithExisting || entry.Status != wh.EntryStatusDraft {
		response.Conflict(w, r, CodeStockEntryNotDraft, "Only a draft entry in existing-barcode mode accepts an Excel import")
		return id, false
	}
	if err := h.entries.CheckWrite(r.Context(), entryCaller(r), id); err != nil {
		writeEntryError(w, r, err)
		return id, false
	}
	return id, true
}
func (h *EntryImport) Upload(w http.ResponseWriter, r *http.Request) {
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (10<<20)+(1<<20))
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	policy := strings.TrimSpace(r.FormValue("existing_policy"))
	if policy == "" {
		policy = "none"
	}
	if policy != "none" && policy != "all_existing" && policy != "conflicts_only" {
		response.BadRequest(w, r, response.CodeValidationError, "existing_policy is invalid")
		return
	}
	file, head, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.FormValue("format"))))
	if format == "" {
		format = ioengine.ImportCSV
	}
	switch format {
	case ioengine.ImportCSV, ioengine.ImportTSV, ioengine.ImportXLSX, ioengine.ImportJSON:
	default:
		response.BadRequest(w, r, response.CodeValidationError, "format is invalid")
		return
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = "tr"
	}
	scope := orgctx.MustScope(r.Context())
	orgID := scope.InternalID
	job, err := h.imports.Upload(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, &orgID, wh.EntryBarcodeImportResource, format, locale, head.Filename, file)
	if err != nil {
		writeEntryImportError(w, r, err)
		return
	}
	if err := h.adapter.Bind(r.Context(), job.UUID, id, orgID, policy); err != nil {
		writeEntryImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}
func (h *EntryImport) Sample(w http.ResponseWriter, r *http.Request) {
	id, ok := h.target(w, r)
	if !ok {
		return
	}
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	if format == "" {
		format = ioengine.ImportXLSX
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale == "" {
		locale = "tr"
	}
	data, ct, err := h.imports.Sample(wh.EntryImportContext(r.Context(), id), wh.EntryBarcodeImportResource, format, locale)
	if err != nil {
		writeEntryImportError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"entry-barcodes-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}
func writeEntryImportError(w http.ResponseWriter, r *http.Request, err error) {
	if response.Coded(w, r, err) {
		return
	}
	if errors.Is(err, ioengine.ErrImportRejected) || errors.Is(err, importusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	writeEntryError(w, r, err)
}
