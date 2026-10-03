package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeEODReportNotFound is the 404 code of the end-of-day report endpoints.
const CodeEODReportNotFound = "EOD_REPORT_NOT_FOUND"

// EODExports queues and serves export jobs (exports usecase).
type EODExports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (exportusecase.ExportJobView, error)
	DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error)
}

// EOD serves /v1/warehouse/eod-reports (TEC-207).
type EOD struct {
	svc     *wh.EOD
	pdf     *wh.EODPDF
	exports EODExports
}

// NewEOD builds the handler. Without exports the PDF endpoints answer 503.
func NewEOD(svc *wh.EOD, pdf *wh.EODPDF, exports EODExports) *EOD {
	return &EOD{svc: svc, pdf: pdf, exports: exports}
}

func writeEODError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, wh.ErrEODReportNotFound):
		response.Error(w, r, http.StatusNotFound, CodeEODReportNotFound, "End-of-day report not found")
	case errors.Is(err, exportusecase.ErrNotFound):
		response.NotFound(w, r, "Export job not found")
	case errors.Is(err, exportusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		writeError(w, r, err)
	}
}

type eodGenerateBody struct {
	Date          string  `json:"date"`
	WarehouseUUID *string `json:"warehouse_uuid"`
}

type eodPDFBody struct {
	Locale string `json:"locale"`
}

func optQuery(r *http.Request, key string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	return &v
}

// List (GET /v1/warehouse/eod-reports?warehouse_uuid&scope&date_from&date_to&limit&offset).
func (h *EOD) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	items, total, err := h.svc.List(r.Context(), caller(r), wh.EODListInput{
		WarehouseUUID: optQuery(r, "warehouse_uuid"), Scope: strings.TrimSpace(v.Get("scope")),
		DateFrom: v.Get("date_from"), DateTo: v.Get("date_to"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Generate (POST /v1/warehouse/eod-reports): builds (or rebuilds) the
// report of a day now (manual run).
func (h *EOD) Generate(w http.ResponseWriter, r *http.Request) {
	var b eodGenerateBody
	if r.ContentLength != 0 {
		if !decode(w, r, &b) {
			return
		}
	}
	out, err := h.svc.Generate(r.Context(), wh.EODCaller{Caller: caller(r), Principal: authctx.MustPrincipal(r.Context())},
		wh.EODGenerateInput{Date: b.Date, WarehouseUUID: b.WarehouseUUID})
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Get (GET /v1/warehouse/eod-reports/{uuid}).
func (h *EOD) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		writeEODError(w, r, wh.ErrEODReportNotFound)
		return
	}
	out, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// eodPDFJob points the download link at the warehouse route.
func eodPDFJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/warehouse/eod-report-pdfs/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}

// RequestPDF (POST /v1/warehouse/eod-reports/{uuid}/pdf, 202) queues the
// report PDF (worker-docs). The optional body {"locale"} overrides the
// user language. Poll GET /v1/warehouse/eod-report-pdfs/{job}.
func (h *EOD) RequestPDF(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil || h.pdf == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		writeEODError(w, r, wh.ErrEODReportNotFound)
		return
	}
	var b eodPDFBody
	if r.Body != nil && r.ContentLength != 0 {
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
			return
		}
	}
	c := caller(r)
	rep, err := h.pdf.Authorize(r.Context(), c, id)
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, &orgID,
		wh.ResourceEODPDF, ioengine.ExportPDF, ioengine.ExportQuery{wh.EODPDFQueryReportUUID: rep.UUID.String()}, locale)
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, eodPDFJob(job))
}

// GetPDF (GET /v1/warehouse/eod-report-pdfs/{uuid}).
func (h *EOD) GetPDF(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, eodPDFJob(job))
}

// DownloadPDF (GET /v1/warehouse/eod-report-pdfs/{uuid}/download).
func (h *EOD) DownloadPDF(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writeEODError(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", eodAttachment(filename))
	_, _ = io.Copy(w, file)
}

func (h *EOD) orgJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	if _, err := wh.EODGuard(caller(r)); err != nil {
		writeEODError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Export job not found")
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && job.Resource != wh.ResourceEODPDF {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writeEODError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

func eodAttachment(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
}
