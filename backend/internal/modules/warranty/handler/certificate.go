package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-188: warranty certificate PDF of a service (export job -> download).

// CodeNoActiveWarranty: the service has no active warranty to print (409).
const CodeNoActiveWarranty = "NO_ACTIVE_WARRANTY"

// Exports queues and serves export jobs (exports usecase).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (exportusecase.ExportJobView, error)
	DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error)
}

// Certificate serves the warranty certificate endpoints.
type Certificate struct {
	svc     *usecase.CertificateService
	exports Exports
}

// NewCertificate builds the handler. Without exports every request answers
// 503.
func NewCertificate(svc *usecase.CertificateService, exports Exports) *Certificate {
	return &Certificate{svc: svc, exports: exports}
}

type certificateBody struct {
	Locale string `json:"locale"`
}

func decodeCertificateBody(w http.ResponseWriter, r *http.Request) (certificateBody, bool) {
	var b certificateBody
	if r.Body == nil || r.ContentLength == 0 {
		return b, true
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
		return b, false
	}
	return b, true
}

// certificateLocale is the body locale when supported, else the request
// locale (user preference / Accept-Language).
func certificateLocale(r *http.Request, override string) string {
	if l, ok := i18n.Parse(override); ok {
		return string(l)
	}
	return string(i18n.FromContext(r.Context()).Locale)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

// certificateJob points the download link of a panel certificate job at
// the warranty route (warranties.read).
func certificateJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/warranty-certificates/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}

// RequestTenant (POST /v1/services/{uuid}/warranty-certificate, 202) queues
// the certificate PDF of a service in the caller's warranties.read scope.
// Poll GET /v1/warranty-certificates/{job} and download from download_url.
func (h *Certificate) RequestTenant(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	b, ok := decodeCertificateBody(w, r)
	if !ok {
		return
	}
	f, ok := scopefilter.From(r.Context())
	if !ok {
		response.Forbidden(w, r, "")
		return
	}
	org := orgctx.MustScope(r.Context())
	svc, err := h.svc.AuthorizeTenant(r.Context(), f, org.BrandID, id)
	if err != nil {
		writeCertificateError(w, r, err)
		return
	}
	query := ioengine.ExportQuery{
		usecase.QueryServiceUUID: svc.Uuid.String(),
		usecase.QueryBrandID:     strconv.FormatInt(svc.BrandID, 10),
	}
	p := authctx.MustPrincipal(r.Context())
	orgID := org.InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, usecase.ResourceCertificate,
		ioengine.ExportPDF, query, certificateLocale(r, b.Locale))
	if err != nil {
		writeCertificateError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, certificateJob(job))
}

// GetTenant (GET /v1/warranty-certificates/{uuid}): a certificate job of
// the active organization.
func (h *Certificate) GetTenant(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, certificateJob(job))
}

// DownloadTenant (GET /v1/warranty-certificates/{uuid}/download).
func (h *Certificate) DownloadTenant(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writeCertificateError(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", attachment(filename))
	_, _ = io.Copy(w, file)
}

func (h *Certificate) orgJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && job.Resource != usecase.ResourceCertificate {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writeCertificateError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

// RequestPortal (POST /v1/portal/services/{uuid}/warranty-certificate, 202):
// the signed-in customer's certificate of a service whose active warranty
// they hold (domain brand, K20). Poll and download through
// /v1/portal/exports/{job}.
func (h *Certificate) RequestPortal(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	b, ok := decodeCertificateBody(w, r)
	if !ok {
		return
	}
	br, ok := brandctx.From(r.Context())
	if !ok || br.ID <= 0 {
		response.NotFound(w, r, "Service not found")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	svc, err := h.svc.AuthorizePortal(r.Context(), p.UserInternal, br.ID, id)
	if err != nil {
		writeCertificateError(w, r, err)
		return
	}
	query := ioengine.ExportQuery{
		usecase.QueryServiceUUID:  svc.Uuid.String(),
		usecase.QueryBrandID:      strconv.FormatInt(svc.BrandID, 10),
		usecase.QueryHolderUserID: strconv.FormatInt(p.UserInternal, 10),
	}
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, nil, usecase.ResourcePortalCertificate,
		ioengine.ExportPDF, query, certificateLocale(r, b.Locale))
	if err != nil {
		writeCertificateError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

func writeCertificateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrCertificateNotFound):
		response.NotFound(w, r, "Service not found")
	case errors.Is(err, usecase.ErrNoActiveWarranty):
		response.Conflict(w, r, CodeNoActiveWarranty, "The service has no active warranty")
	case errors.Is(err, exportusecase.ErrNotFound):
		response.NotFound(w, r, "Export job not found")
	case errors.Is(err, exportusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "warranty certificate failed")
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
