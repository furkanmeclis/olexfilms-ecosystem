package handler

// TEC-377 (DT-BE-7): warranty list export (I/O engine). Same permission and
// scope as GET /v1/warranties (warranties.read, RequireScope); the job
// stores the resolved scope and the worker re-authorizes it. Poll and
// download through /v1/tenant/exports/{uuid}.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// ListExports queues export jobs (*exportusecase.Service).
type ListExports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// ListExport serves POST /v1/warranties/export.
type ListExport struct {
	exports ListExports
}

// NewListExport creates the handler (nil exports: 503).
func NewListExport(exports ListExports) *ListExport { return &ListExport{exports: exports} }

type listExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// Request (POST /v1/warranties/export, 202): exports the warranties of the
// caller's scope with the filters, q and sort of GET /v1/warranties.
func (h *ListExport) Request(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b listExportBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	switch format {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return
	}
	c := panelCaller(r)
	query, err := usecase.ListExportQuery(c, b.Query)
	if errors.Is(err, usecase.ErrExportScope) {
		response.Forbidden(w, r, "This scope cannot export warranties")
		return
	}
	if err != nil {
		writeListError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.Principal.UserInternal, &orgID, usecase.ResourceListExport,
		format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeListError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
