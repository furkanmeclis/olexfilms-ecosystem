package handler

// TEC-161: KVKK/GDPR anonymization and personal data export endpoints.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodePanelAccount: the customer is also an organization user.
const CodePanelAccount = "CUSTOMER_HAS_PANEL_ACCOUNT"

// Exports is the export job service used by the data export endpoints
// (*exportusecase.Service).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (exportusecase.ExportJobView, error)
	DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error)
	GetPortalJob(ctx context.Context, jobUUID uuid.UUID, actorID int64) (exportusecase.ExportJobView, error)
	DownloadPortal(ctx context.Context, jobUUID uuid.UUID, actorID int64) (io.ReadCloser, string, string, error)
}

// WithExports enables the data export endpoints (without it they answer 503).
func (h *Handler) WithExports(e Exports) *Handler {
	h.exports = e
	return h
}

// AnonymizeCustomer (POST /v1/customers/{uuid}/anonymize): irreversible,
// idempotent (a second call answers 200 with changed=false).
func (h *Handler) AnonymizeCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.AnonymizeCustomer(r.Context(), caller(r), id, activity.MetaFromRequest(r))
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

type dataExportBody struct {
	Format string `json:"format"`
	Locale string `json:"locale"`
}

func dataExportFormat(raw string) (ioengine.ExportFormat, error) {
	f := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(raw)))
	switch f {
	case ioengine.ExportJSON, ioengine.ExportPDF:
		return f, nil
	default:
		return "", &cu.ValidationError{Field: "format", Message: "must be json or pdf"}
	}
}

func exportLocale(r *http.Request, override string) string {
	if l, ok := i18n.Parse(override); ok {
		return string(l)
	}
	return string(i18n.FromContext(r.Context()).Locale)
}

// RequestDataExport (POST /v1/customers/{uuid}/data-export, 202): the center
// exports a customer's personal data (JSON or PDF job).
func (h *Handler) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b dataExportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := dataExportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c := caller(r)
	if _, err := h.svc.ResolveExportTarget(r.Context(), c, id); err != nil {
		writePrivacyError(w, r, err)
		return
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.UserID, &orgID, cu.ResourceDataExport, format,
		ioengine.ExportQuery{cu.QueryCustomerUUID: id.String()}, exportLocale(r, b.Locale))
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	if err := h.svc.AuditExportRequest(r.Context(), c.UserID, c.Org.UUID, id, job.UUID, string(format), "panel",
		activity.MetaFromRequest(r)); err != nil {
		response.InternalErr(w, r, err, "audit failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, panelExportJob(job))
}

// GetDataExport (GET /v1/customer-data-exports/{uuid}).
func (h *Handler) GetDataExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.panelJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, panelExportJob(job))
}

// DownloadDataExport (GET /v1/customer-data-exports/{uuid}/download).
func (h *Handler) DownloadDataExport(w http.ResponseWriter, r *http.Request) {
	job, ok := h.panelJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	h.record(r, "customers.data_export_downloaded", "customers", &job.UUID, map[string]any{"channel": "panel"})
	sendFile(w, file, ct, filename)
}

func (h *Handler) panelJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && job.Resource != cu.ResourceDataExport {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writeExportError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

// panelExportJob points the download link at the customers route
// (customers.anonymize), not the generic tenant exports route.
func panelExportJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/customer-data-exports/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}

// --- Portal (the customer's own data) ----------------------------------------

// RequestPortalDataExport (POST /v1/portal/me/data-export, 202): the signed-in
// customer exports their own data in the domain brand (K20).
func (h *Handler) RequestPortalDataExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b dataExportBody
	if !decode(w, r, &b) {
		return
	}
	format, err := dataExportFormat(b.Format)
	if err != nil {
		writeError(w, r, err)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	query := ioengine.ExportQuery{cu.QueryCustomerUUID: p.UserID.String()}
	if br, ok := brandctx.From(r.Context()); ok && br.ID > 0 {
		query[cu.QueryBrandID] = strconv.FormatInt(br.ID, 10)
	}
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, nil, cu.ResourcePortalDataExport, format,
		query, exportLocale(r, b.Locale))
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	if err := h.svc.AuditExportRequest(r.Context(), p.UserInternal, uuid.Nil, p.UserID, job.UUID, string(format), "portal",
		activity.MetaFromRequest(r)); err != nil {
		response.InternalErr(w, r, err, "audit failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

// GetPortalExport (GET /v1/portal/exports/{uuid}): own portal jobs only.
func (h *Handler) GetPortalExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	job, err := h.exports.GetPortalJob(r.Context(), id, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}

// DownloadPortalExport (GET /v1/portal/exports/{uuid}/download).
func (h *Handler) DownloadPortalExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadPortal(r.Context(), id, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		writeExportError(w, r, err)
		return
	}
	h.record(r, "customers.data_export_downloaded", "customers", &id, map[string]any{"channel": "portal"})
	sendFile(w, file, ct, filename)
}

func sendFile(w http.ResponseWriter, file io.ReadCloser, ct, filename string) {
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", attachment(filename))
	_, _ = io.Copy(w, file)
}

func writePrivacyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, cu.ErrPanelAccount) {
		response.Conflict(w, r, CodePanelAccount, "The customer is also an organization user; detach the panel account first")
		return
	}
	writeError(w, r, err)
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
