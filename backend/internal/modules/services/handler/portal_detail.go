package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-239: portal service detail and the portal service PDF.

// Reuse windows of the portal PDF: a completed render is reused while the
// service is unchanged and the render is younger than portalPDFReuse; a
// queued / processing job younger than portalPDFPending is returned
// instead of queueing a second one.
const (
	portalPDFReuse   = 24 * time.Hour
	portalPDFPending = 15 * time.Minute
)

// PortalExports queues portal export jobs and finds a reusable one.
type PortalExports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	FindPortalServiceJob(ctx context.Context, actorID int64, resource, serviceUUID, locale string,
		notBefore, pendingAfter time.Time) (exportusecase.ExportJobView, bool, error)
}

// PortalDetail serves GET /v1/portal/services/{uuid} and its PDF.
type PortalDetail struct {
	svc     *svcuc.Service
	pdf     *svcuc.PDFService
	exports PortalExports
	now     func() time.Time
}

// NewPortalDetail builds the handler. Without exports the PDF answers 503.
func NewPortalDetail(svc *svcuc.Service, pdf *svcuc.PDFService, exports PortalExports) *PortalDetail {
	return &PortalDetail{svc: svc, pdf: pdf, exports: exports, now: func() time.Time { return time.Now().UTC() }}
}

// portalBrandID is the domain brand (K3/K20); 0 when unresolved.
func portalBrandID(r *http.Request) int64 {
	if br, ok := brandctx.From(r.Context()); ok {
		return br.ID
	}
	return 0
}

// Get (GET /v1/portal/services/{uuid}): a service the signed-in user owns
// (customer or warranty holder) in the domain brand; anything else is 404.
func (h *PortalDetail) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	v, err := h.svc.PortalGet(r.Context(), portalBrandID(r), p.UserInternal, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// PDF (GET /v1/portal/services/{uuid}/pdf): the service PDF of a service the
// user owns. A completed render of the same locale is returned with 200
// (download_url under /v1/portal/exports); otherwise a running job is
// returned, or a new one queued, with 202. ?locale= overrides the user
// language.
func (h *PortalDetail) PDF(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	svc, err := h.pdf.PortalAuthorize(r.Context(), portalBrandID(r), p.UserInternal, id)
	if err != nil {
		writePDFError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(r.URL.Query().Get("locale")); ok {
		locale = string(l)
	}
	now := h.now()
	notBefore := now.Add(-portalPDFReuse)
	if svc.UpdatedAt.Valid && svc.UpdatedAt.Time.After(notBefore) {
		notBefore = svc.UpdatedAt.Time
	}
	job, found, err := h.exports.FindPortalServiceJob(r.Context(), p.UserInternal, svcuc.ResourcePortalPDF,
		svc.Uuid.String(), locale, notBefore, now.Add(-portalPDFPending))
	if err != nil {
		writePDFError(w, r, err)
		return
	}
	if !found {
		job, err = h.exports.RequestExport(r.Context(), p.UserInternal, nil, svcuc.ResourcePortalPDF,
			ioengine.ExportPDF, ioengine.ExportQuery{
				svcuc.PDFQueryServiceUUID:  svc.Uuid.String(),
				svcuc.PDFQueryBrandID:      strconv.FormatInt(svc.BrandID, 10),
				svcuc.PDFQueryPortalUserID: strconv.FormatInt(p.UserInternal, 10),
			}, locale)
		if err != nil {
			writePDFError(w, r, err)
			return
		}
	}
	status := http.StatusAccepted
	if job.Status == "completed" {
		status = http.StatusOK
	}
	response.JSON(w, r, status, job)
}
