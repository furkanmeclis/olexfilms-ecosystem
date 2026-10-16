package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	warehouseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type locationImportService interface {
	Upload(context.Context, int64, *int64, string, ioengine.ImportFormat, string, string, io.Reader) (importusecase.ImportJobView, error)
	Sample(context.Context, string, ioengine.ImportFormat, string) ([]byte, string, error)
}

type LocationImport struct {
	imports   locationImportService
	warehouse *warehouseusecase.Service
}

func NewLocationImport(imports locationImportService, warehouse *warehouseusecase.Service) *LocationImport {
	return &LocationImport{imports: imports, warehouse: warehouse}
}

func (h *LocationImport) authorized(w http.ResponseWriter, r *http.Request) bool {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Warehouse not found")
		return false
	}
	if _, err = h.warehouse.GetWarehouse(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return false
	}
	return true
}
func (h *LocationImport) Upload(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid import form")
		return
	}
	file, head, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "File is required")
		return
	}
	defer func() { _ = file.Close() }()
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.FormValue("format"))))
	if format == "" {
		format = ioengine.ImportXLSX
	}
	switch format {
	case ioengine.ImportCSV, ioengine.ImportTSV, ioengine.ImportXLSX, ioengine.ImportJSON:
	default:
		response.BadRequest(w, r, response.CodeValidationError, "Invalid format")
		return
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = "tr"
	}
	org := orgctx.MustScope(r.Context())
	principal := authctx.MustPrincipal(r.Context())
	orgID := org.InternalID
	job, err := h.imports.Upload(r.Context(), principal.UserInternal, &orgID, warehouseusecase.LocationImportResource, format, locale, head.Filename, file)
	if err != nil {
		writeLocationImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}
func (h *LocationImport) Sample(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
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
	data, ct, err := h.imports.Sample(r.Context(), warehouseusecase.LocationImportResource, format, locale)
	if err != nil {
		writeLocationImportError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"warehouse-locations-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}

func writeLocationImportError(w http.ResponseWriter, r *http.Request, err error) {
	if response.Coded(w, r, err) {
		return
	}
	if errors.Is(err, importusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	response.InternalErr(w, r, err, "Warehouse import failed")
}
