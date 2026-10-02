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
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-196: service PDF (export job -> poll -> download), same flow as the
// warranty certificate (TEC-188).

// Exports queues and serves export jobs (exports usecase).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
	GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (exportusecase.ExportJobView, error)
	DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error)
}

// PDF serves the service PDF endpoints.
type PDF struct {
	svc     *svcuc.PDFService
	exports Exports
}

// NewPDF builds the handler. Without exports every request answers 503.
func NewPDF(svc *svcuc.PDFService, exports Exports) *PDF { return &PDF{svc: svc, exports: exports} }

type pdfBody struct {
	Locale string `json:"locale"`
}

// pdfJob points the download link of a service PDF job at the services
// route (services.read).
func pdfJob(j exportusecase.ExportJobView) exportusecase.ExportJobView {
	if j.Download != nil {
		u := "/v1/service-pdfs/" + j.UUID.String() + "/download"
		j.Download = &u
	}
	return j
}

// Request (POST /v1/services/{uuid}/pdf, 202) queues the PDF of a service
// in the caller's services.read scope (out of scope = 404). The optional
// body {"locale"} overrides the user language. Poll GET
// /v1/service-pdfs/{job} and download from download_url.
func (h *PDF) Request(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b pdfBody
	if r.Body != nil && r.ContentLength != 0 {
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
			return
		}
	}
	c := caller(r)
	svc, err := h.svc.Authorize(r.Context(), c, id)
	if err != nil {
		writePDFError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.Principal.UserInternal, &orgID, svcuc.ResourcePDF,
		ioengine.ExportPDF, ioengine.ExportQuery{
			svcuc.PDFQueryServiceUUID: svc.Uuid.String(),
			svcuc.PDFQueryBrandID:     strconv.FormatInt(svc.BrandID, 10),
		}, locale)
	if err != nil {
		writePDFError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, pdfJob(job))
}

// Get (GET /v1/service-pdfs/{uuid}): a service PDF job of the active
// organization.
func (h *PDF) Get(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, pdfJob(job))
}

// Download (GET /v1/service-pdfs/{uuid}/download).
func (h *PDF) Download(w http.ResponseWriter, r *http.Request) {
	job, ok := h.orgJob(w, r)
	if !ok {
		return
	}
	file, ct, filename, err := h.exports.DownloadOrg(r.Context(), job.UUID, orgctx.MustScope(r.Context()).InternalID)
	if err != nil {
		writePDFError(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", attachment(filename))
	_, _ = io.Copy(w, file)
}

func (h *PDF) orgJob(w http.ResponseWriter, r *http.Request) (exportusecase.ExportJobView, bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return exportusecase.ExportJobView{}, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Export job not found")
		return exportusecase.ExportJobView{}, false
	}
	job, err := h.exports.GetOrgJob(r.Context(), id, orgctx.MustScope(r.Context()).InternalID)
	if err == nil && job.Resource != svcuc.ResourcePDF {
		err = exportusecase.ErrNotFound
	}
	if err != nil {
		writePDFError(w, r, err)
		return exportusecase.ExportJobView{}, false
	}
	return job, true
}

func writePDFError(w http.ResponseWriter, r *http.Request, err error) {
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
