package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// ServiceLookup resolves a public code to its service page (TEC-1005).
type ServiceLookup interface {
	LookupService(ctx context.Context, brand usecase.PublicWarrantyBrand, brandID int64, code string) (usecase.PublicService, error)
}

// Service answers GET /v1/public/service-warranties/{code} (TEC-1005): the
// service page of a service number, warranty public code or legacy alias.
// Same per-IP limit and 404 rules as the warranty lookup.
func (h *Public) Service(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if !h.allow(w, r, publicRateAction, h.limit) {
		return
	}
	out, ok := h.findService(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ServicePDF answers GET /v1/public/service-warranties/{code}/pdf: one
// document with every warranty of the service, offered while one of them
// is active (else 404). PDF bucket and query as the warranty PDF.
func (h *Public) ServicePDF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if !h.allow(w, r, publicPDFRateAction, h.pdfLimit) {
		return
	}
	out, ok := h.findService(w, r)
	if !ok {
		return
	}
	if !out.HasActive() {
		response.NotFound(w, r, notFoundMessage)
		return
	}
	if h.renderer == nil || !h.renderer.Configured() {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "PDF rendering is not configured")
		return
	}
	loc, ok := i18n.Parse(r.URL.Query().Get("lang"))
	if !ok {
		if loc, ok = i18n.ParseAcceptLanguage(r.Header.Get("Accept-Language")); !ok {
			loc = i18n.DefaultLocale
		}
	}
	zone := pdfrender.Zone(strings.TrimSpace(r.URL.Query().Get("tz")))
	doc, err := usecase.PublicServiceCertificateHTML(out, usecase.PublicVerifyURL(h.frontendURL, out.ServiceNo), zone, loc, h.now())
	if err != nil {
		response.InternalErr(w, r, err, "warranty pdf failed")
		return
	}
	pdf, err := h.renderer.HTMLToPDF(r.Context(), doc)
	if err != nil {
		response.InternalErr(w, r, err, "warranty pdf failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="warranty-`+out.ServiceNo+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func (h *Public) findService(w http.ResponseWriter, r *http.Request) (usecase.PublicService, bool) {
	code := r.PathValue("code")
	b, ok := brandctx.From(r.Context())
	svc, isSvc := h.lookup.(ServiceLookup)
	if !ok || !isSvc || !usecase.ValidPublicCode(code) {
		response.NotFound(w, r, notFoundMessage)
		return usecase.PublicService{}, false
	}
	out, err := svc.LookupService(r.Context(), usecase.PublicWarrantyBrand{Name: b.Name, Slug: b.Slug}, b.ID, code)
	if err != nil {
		if errors.Is(err, usecase.ErrPublicNotFound) {
			response.NotFound(w, r, notFoundMessage)
			return usecase.PublicService{}, false
		}
		response.InternalErr(w, r, err, "warranty lookup failed")
		return usecase.PublicService{}, false
	}
	return out, true
}
