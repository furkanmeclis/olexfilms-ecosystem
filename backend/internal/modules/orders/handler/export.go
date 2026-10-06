package handler

// TEC-373 (DT-BE-5): order list export (I/O engine). Same permission and
// scope as GET /v1/orders (orders.read, RequireScope); the job stores the
// resolved scope and the worker re-authorizes it. Poll and download through
// /v1/tenant/exports/{uuid}.

import (
	"context"
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Exports queues export jobs (*exportusecase.Service).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// WithExports enables POST /v1/orders/export (without it: 503).
func (h *Handler) WithExports(e Exports) *Handler {
	h.exports = e
	return h
}

type listExportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// RequestListExport (POST /v1/orders/export, 202): exports the orders of the
// caller's scope with the filters, q and sort of GET /v1/orders.
func (h *Handler) RequestListExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b listExportBody
	if !decode(w, r, &b) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	switch format {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return
	}
	c := caller(r)
	query, err := ord.ListExportQuery(c, b.Query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if query[ord.QuerySide] != "" && !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		writeError(w, r, ord.ErrForbidden)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.Principal.UserInternal, &orgID, ord.ResourceListExport,
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
