package handler

// TEC-761: warehouse export jobs (I/O engine). Same permission and scope as
// the lists they export (warehouse.read); the job stores the resolved scope
// and the worker re-authorizes it. Poll and download through
// /v1/tenant/exports/{uuid}.

import (
	"context"
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// ExportRequester queues export jobs (*exportusecase.Service).
type ExportRequester interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// Exports serves the warehouse export endpoints.
type Exports struct {
	x       *wh.Exporter
	exports ExportRequester
}

// NewExports builds the handler; without exports the endpoints answer 503.
func NewExports(x *wh.Exporter, exports ExportRequester) *Exports {
	return &Exports{x: x, exports: exports}
}

type exportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// Stock entry list (POST /v1/warehouse/stock-entries/export, 202).
func (h *Exports) Entries(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceEntriesExport)
}

// Counts list (POST /v1/warehouse/stock-counts/export, 202).
func (h *Exports) Counts(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceCountsExport)
}

// Transfers list (POST /v1/warehouse/transfers/export, 202).
func (h *Exports) Transfers(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceTransfersExport)
}

// TransferSummary (POST /v1/warehouse/transfers/summary/export, 202): the
// per product transfer aggregate report.
func (h *Exports) TransferSummary(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceTransferSummaryExport)
}

// Locations (POST /v1/warehouse/locations/export, 202).
func (h *Exports) Locations(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceLocationsExport)
}

// EODReports list (POST /v1/warehouse/eod-reports/export, 202).
func (h *Exports) EODReports(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, wh.ResourceEODListExport)
}

// CountReport (POST /v1/warehouse/stock-counts/{uuid}/export, 202): the
// lines of a completed count with their differences.
func (h *Exports) CountReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		writeCountError(w, r, wh.ErrCountNotFound)
		return
	}
	b, format, ok := h.body(w, r)
	if !ok {
		return
	}
	query, err := h.x.CountReportExportQuery(r.Context(), countCaller(r), id)
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	h.request(w, r, wh.ResourceCountReportExport, format, query, b.Locale)
}

func (h *Exports) list(w http.ResponseWriter, r *http.Request, resource string) {
	b, format, ok := h.body(w, r)
	if !ok {
		return
	}
	query, err := h.x.ListExportQuery(caller(r), resource, b.Query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.request(w, r, resource, format, query, b.Locale)
}

func (h *Exports) body(w http.ResponseWriter, r *http.Request) (exportBody, ioengine.ExportFormat, bool) {
	var b exportBody
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return b, "", false
	}
	if !decode(w, r, &b) {
		return b, "", false
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	switch format {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return b, "", false
	}
	return b, format, true
}

func (h *Exports) request(w http.ResponseWriter, r *http.Request, resource string, format ioengine.ExportFormat,
	query ioengine.ExportQuery, rawLocale string) {
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(rawLocale); ok {
		locale = string(l)
	}
	orgID := caller(r).Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, &orgID,
		resource, format, query, locale)
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
