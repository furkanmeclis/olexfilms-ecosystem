package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// publicPDFRateAction is the limiter bucket of the anonymous PDF. It is
// separate from (and tighter than) the lookup: every hit is a Gotenberg
// render.
const publicPDFRateAction = "warranty_public_pdf"

// PDFRenderer converts an HTML document to PDF (pdfrender.Client).
type PDFRenderer interface {
	Configured() bool
	HTMLToPDF(ctx context.Context, html string) ([]byte, error)
}

// WithPDF enables GET /v1/public/warranties/{public_code}/pdf (TEC-248):
// renderer draws the document, frontendURL is the origin of the QR link
// and limit is the per-IP PDF hits per window.
func (h *Public) WithPDF(renderer PDFRenderer, frontendURL string, limit int) *Public {
	h.renderer, h.frontendURL, h.pdfLimit = renderer, frontendURL, limit
	return h
}

// PDF answers the anonymous warranty PDF of the public page. No
// authentication; per-IP limited before anything else. Only an active
// warranty has a PDF: malformed, unknown, another brand's, expired and void
// codes all get the lookup's 404. The document is built from the public
// projection only (masked plate, last four VIN characters, no holder).
// Query: lang (else Accept-Language, else tr), tz (IANA zone of the
// printed dates and issued-at time, else Europe/Istanbul).
func (h *Public) PDF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if !h.allow(w, r, publicPDFRateAction, h.pdfLimit) {
		return
	}
	out, ok := h.find(w, r)
	if !ok {
		return
	}
	if out.Status != "active" {
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
	doc, err := usecase.PublicCertificateHTML(out, usecase.PublicVerifyURL(h.frontendURL, out.PublicCode), zone, loc, h.now())
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
	w.Header().Set("Content-Disposition", `attachment; filename="warranty-`+out.PublicCode+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}
