package handler

// TEC-373 (DT-BE-5): unit list export (I/O engine). Same permission and
// scope as GET /v1/stock/organizations/{uuid}/units (stock.read,
// RequireScope); the job stores the resolved scope and the worker
// re-authorizes it. Poll and download through /v1/tenant/exports/{uuid}.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Exports queues export jobs (*exportusecase.Service).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// WithExports enables POST /v1/stock/organizations/{uuid}/units/export
// (without it: 503).
func (h *Handler) WithExports(e Exports) *Handler {
	h.exports = e
	return h
}

type unitsExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// RequestUnitsExport (POST /v1/stock/organizations/{uuid}/units/export,
// 202): exports the organization's units with the filters, q and sort of
// the unit list.
func (h *Handler) RequestUnitsExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	var b unitsExportBody
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
	query, err := h.svc.UnitsExportQuery(r.Context(), f, id, b.Query)
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, stockusecase.ResourceUnitsExport,
		format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
