package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-175 (F1-07e): cari statement, balance report and their export jobs.

// Exports queues and serves export jobs (exports usecase).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (exportusecase.ExportJobView, error)
	DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error)
}

// WithExports enables the export endpoints (without it they answer 503).
func (h *Handler) WithExports(e Exports) *Handler {
	h.exports = e
	return h
}

func accountingResource(resource string) bool {
	return resource == acc.ResourceCariStatement || resource == acc.ResourceBalances ||
		acc.IsReportResource(resource) // TEC-346
}

// accountingJob points the download link of an accounting export job at the
// accounting route (accounting.read), which every accounting role holds.
func accountingJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/accounting/exports/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}

// GetStatement (GET /v1/accounting/cari/{uuid}/statement?organization_uuid&
// from&to&locale).
func (h *Handler) GetStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	var p acc.StatementPeriod
	if p.From, ok = queryDate(w, r, "from"); !ok {
		return
	}
	if p.To, ok = queryDate(w, r, "to"); !ok {
		return
	}
	c := caller(r)
	loc := h.svc.ResolveLocale(r.Context(), c, r.URL.Query().Get("locale"))
	st, err := h.svc.GetStatement(r.Context(), c, org, id, p, loc)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// GetBalances (GET /v1/accounting/reports/balances?organization_uuid&as_of).
func (h *Handler) GetBalances(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	asOf, ok := queryDate(w, r, "as_of")
	if !ok {
		return
	}
	rep, err := h.svc.GetBalanceReport(r.Context(), caller(r), org, asOf)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

type exportBody struct {
	Format           string     `json:"format"`
	OrganizationUUID *uuid.UUID `json:"organization_uuid"`
	From             string     `json:"from"`
	To               string     `json:"to"`
	AsOf             string     `json:"as_of"`
	Locale           string     `json:"locale"`
	// Group is the P&L grouping (TEC-346; month or category).
	Group string `json:"group"`
}

func bodyDate(field, raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, &acc.ValidationError{Field: field, Message: "must be a date (YYYY-MM-DD)"}
	}
	return &d, nil
}

func exportFormat(raw string) (ioengine.ExportFormat, error) {
	f := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(raw)))
	switch f {
	case ioengine.ExportPDF, ioengine.ExportXLSX, ioengine.ExportCSV:
		return f, nil
	default:
		return "", &acc.ValidationError{Field: "format", Message: "must be pdf, xlsx or csv"}
	}
}

// ExportStatement queues a statement export job
// (POST /v1/accounting/cari/{uuid}/statement/export, 202). Poll and download
// through /v1/accounting/exports/{uuid}.
func (h *Handler) ExportStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b exportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := exportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var p acc.StatementPeriod
	if p.From, err = bodyDate("from", b.From); err != nil {
		writeError(w, r, err)
		return
	}
	if p.To, err = bodyDate("to", b.To); err != nil {
		writeError(w, r, err)
		return
	}
	if p.From != nil && p.To != nil && p.To.Before(*p.From) {
		writeError(w, r, &acc.ValidationError{Field: "to", Message: "must not be before from"})
		return
	}
	c := caller(r)
	book, err := h.svc.ResolveCari(r.Context(), c, b.OrganizationUUID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := ioengine.ExportQuery{
		acc.QueryBookOrganizationID: strconv.FormatInt(book.ID, 10),
		acc.QueryCariUUID:           id.String(),
	}
	if p.From != nil {
		query[acc.QueryFrom] = p.From.Format(time.DateOnly)
	}
	if p.To != nil {
		query[acc.QueryTo] = p.To.Format(time.DateOnly)
	}
	h.queueExport(w, r, c, acc.ResourceCariStatement, format, query, b.Locale)
}

// ExportBalances queues a balance report export job
// (POST /v1/accounting/reports/balances/export, 202).
func (h *Handler) ExportBalances(w http.ResponseWriter, r *http.Request) {
	var b exportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := exportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	asOf, err := bodyDate("as_of", b.AsOf)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c := caller(r)
	book, err := h.svc.ResolveBook(r.Context(), c, b.OrganizationUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := ioengine.ExportQuery{acc.QueryBookOrganizationID: strconv.FormatInt(book.ID, 10)}
	if asOf != nil {
		query[acc.QueryAsOf] = asOf.Format(time.DateOnly)
	}
	h.queueExport(w, r, c, acc.ResourceBalances, format, query, b.Locale)
}

func (h *Handler) queueExport(w http.ResponseWriter, r *http.Request, c acc.Caller, resource string,
	format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	loc := h.svc.ResolveLocale(r.Context(), c, locale)
	p := authctx.MustPrincipal(r.Context())
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, resource, format, query, string(loc))
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, accountingJob(job))
}

// GetExport (GET /v1/accounting/exports/{uuid}): an accounting export job of
// the active organization.
func (h *Handler) GetExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, accountingJob(job))
}

// DownloadExport (GET /v1/accounting/exports/{uuid}/download).
func (h *Handler) DownloadExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", attachment(filename))
	_, _ = io.Copy(w, file)
}

func (h *Handler) orgJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && !accountingResource(job.Resource) {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writeExportError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

func writeExportError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, exportusecase.ErrNotFound):
		response.NotFound(w, r, "Export job not found")
	case errors.Is(err, exportusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		writeError(w, r, err)
	}
}

func attachment(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
}
