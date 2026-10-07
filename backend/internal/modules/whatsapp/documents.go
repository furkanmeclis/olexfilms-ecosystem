package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// DocumentAdapter is an export adapter whose PDF is a styled document (the
// service PDF and warranty certificate adapters).
type DocumentAdapter interface {
	ioengine.ResourceAdapter
	ioengine.DocumentRenderer
}

// HTMLToPDF converts HTML to PDF (*pdfrender.Client).
type HTMLToPDF interface {
	HTMLToPDF(ctx context.Context, html string) ([]byte, error)
}

// DocumentRenderer renders the WhatsApp document helpers' PDFs with the
// same adapters, titles and Gotenberg path as the export pipeline
// (TEC-395). Keys are usecase.Document* kinds.
type DocumentRenderer struct {
	Adapters map[string]DocumentAdapter
	PDF      HTMLToPDF
}

// NewDocumentRenderer wires the service PDF and warranty certificate
// adapters.
func NewDocumentRenderer(servicePDF, certificate DocumentAdapter, pdf HTMLToPDF) *DocumentRenderer {
	return &DocumentRenderer{
		Adapters: map[string]DocumentAdapter{
			usecase.DocumentServicePDF:          servicePDF,
			usecase.DocumentWarrantyCertificate: certificate,
		},
		PDF: pdf,
	}
}

var documentFileNames = map[string]string{
	usecase.DocumentServicePDF:          "service",
	usecase.DocumentWarrantyCertificate: "warranty_certificate",
}

// Render implements usecase.DocumentRenderer. ref.OrganizationID is the job
// organization the adapters re-check the service against.
func (r *DocumentRenderer) Render(ctx context.Context, kind string, ref usecase.DocumentRef) (usecase.RenderedDocument, error) {
	a, ok := r.Adapters[kind]
	if !ok || a == nil {
		return usecase.RenderedDocument{}, fmt.Errorf("%w: document kind %q", usecase.ErrInvalidRequest, kind)
	}
	if r.PDF == nil {
		return usecase.RenderedDocument{}, errors.New("whatsapp: pdf renderer not configured")
	}
	loc := i18n.Normalize(ref.Locale)
	q := ioengine.ExportQuery{
		"service_uuid":                ref.ServiceUUID.String(),
		"brand_id":                    strconv.FormatInt(ref.BrandID, 10),
		ioengine.QueryOrganizationID: strconv.FormatInt(ref.OrganizationID, 10),
	}
	ds, err := a.Export(ctx, q, loc)
	if err != nil {
		return usecase.RenderedDocument{}, err
	}
	html, err := a.DocumentHTML(ds, string(loc), nil, ioengine.ExportTitle(string(loc), a.Resource()))
	if err != nil {
		return usecase.RenderedDocument{}, err
	}
	data, err := r.PDF.HTMLToPDF(ctx, html)
	if err != nil {
		return usecase.RenderedDocument{}, err
	}
	name := fmt.Sprintf("%s_%s.pdf", documentFileNames[kind], time.Now().UTC().Format("20060102"))
	return usecase.RenderedDocument{Data: data, FileName: name}, nil
}
