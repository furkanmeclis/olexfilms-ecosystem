package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Importer stores uploaded import files (imports usecase).
type Importer interface {
	Upload(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ImportFormat, locale string, filename string, r io.Reader) (importusecase.ImportJobView, error)
	Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error)
}

// Import serves the TEC-158 stock import upload. Preview, confirm and undo
// run on /v1/tenant/imports/{uuid} (ioengine flow, import queue).
type Import struct {
	imports Importer
}

// NewImport creates the stock import handler.
func NewImport(imports Importer) *Import { return &Import{imports: imports} }

const maxImportFile = 10 << 20

// Upload serves POST /v1/stock/import (multipart: file, format, locale).
// Only the brand center imports units (K14).
func (h *Import) Upload(w http.ResponseWriter, r *http.Request) {
	org := orgctx.MustScope(r.Context())
	if org.OrgType != "center" {
		response.Forbidden(w, r, "Only the brand center imports stock")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFile+(1<<20))
	if err := r.ParseMultipartForm(maxImportFile); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
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
	orgID := org.InternalID
	job, err := h.imports.Upload(r.Context(), p.UserInternal, &orgID, stockusecase.ImportResource, format, locale, header.Filename, file)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}

// Sample serves GET /v1/stock/import/sample.
func (h *Import) Sample(w http.ResponseWriter, r *http.Request) {
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	if format == "" {
		format = ioengine.ImportXLSX
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale == "" {
		locale = "tr"
	}
	data, ct, err := h.imports.Sample(r.Context(), stockusecase.ImportResource, format, locale)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"stock-import-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}

func writeImportError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, importusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "unexpected error")
	}
}
