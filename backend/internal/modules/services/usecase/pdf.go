package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	warrantyuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-196 (F1-05f2): the service PDF. One document per service on the
// letterhead of the organization that performed it: service number, dates
// and status, the customer (phone masked: the document leaves the panel),
// the vehicle (brand / model / year, plate, VIN), every item with its unit
// barcode, amount and applied parts, and the warranty summary with a QR
// code per warranty (TEC-188 helpers). It runs as an I/O engine export job
// (queue "exports", worker-docs) and Gotenberg renders it in the job locale
// (pdfrender skeleton: lang/dir, RTL for ar). The HTML is built in memory
// from already loaded data, so its cost is a few milliseconds; the time
// budget (< 3 s) is Gotenberg's.

// ResourcePDF is the export resource of the service PDF (export_jobs.resource);
// the job is polled and downloaded through /v1/service-pdfs.
const ResourcePDF = "tenant.services.pdf"

// Export query keys written by the HTTP handler after it authorized the
// request. The worker re-checks the scope against the job organization.
const (
	PDFQueryServiceUUID = "service_uuid"
	PDFQueryBrandID     = "brand_id"
)

// errPDFScope: the job's service is outside the job organization's tree.
var errPDFScope = errors.New("services pdf: service is outside the job scope")

// PDFDoc is the data of one service PDF.
type PDFDoc struct {
	ServiceNo    string
	Status       string
	CreatedAt    string
	CompletedAt  string
	CancelReason string
	Dealer       PDFDealer
	Customer     PDFCustomer
	Vehicle      PDFVehicle
	KM           string
	Package      string
	Notes        string
	Items        []PDFItem
	Warranties   []PDFWarranty
	GeneratedAt  time.Time
	// Timezone is the zone GeneratedAt is printed in (TEC-521): the
	// requester's zone from the export job, else the performing
	// organization's, else Europe/Istanbul.
	Timezone string
	// Letterhead is the performing organization's letterhead.
	Letterhead ioengine.Letterhead
}

// PDFDealer is the organization that performed the service.
type PDFDealer struct {
	Name    string
	City    string
	Address string
	Phone   string
	Email   string
}

// PDFCustomer is the service customer; Phone is already masked and Name is
// the anonymized label for an anonymized customer (K19).
type PDFCustomer struct {
	Name  string
	Phone string
}

// PDFVehicle is the vehicle snapshot of the service.
type PDFVehicle struct {
	Brand string
	Model string
	Year  string
	Plate string
	VIN   string
}

// PDFItem is one service item.
type PDFItem struct {
	Product  string
	SKU      string
	Barcode  string
	Kind     string
	Quantity int32
	Meters   string
	Parts    []string
	Notes    string
}

// PDFWarranty is one warranty the completed service issued.
type PDFWarranty struct {
	PublicCode string
	Product    string
	Kind       string
	Status     string
	StartDate  string
	EndDate    string
	VerifyURL  string
}

// PDFService builds service PDFs.
type PDFService struct {
	svc   *Service
	cert  *warrantyuc.CertificateService
	store storage.Driver
	log   *slog.Logger
	now   func() time.Time
}

// NewPDF builds the service PDF use case. cert provides the public verify
// URL of a warranty (PUBLIC_FRONTEND_URL/garanti/{code}; nil prints no QR);
// store loads the dealer logo (nil prints the letterhead without logo).
func NewPDF(svc *Service, cert *warrantyuc.CertificateService, store storage.Driver, log *slog.Logger) *PDFService {
	if log == nil {
		log = slog.Default()
	}
	return &PDFService{svc: svc, cert: cert, store: store, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Authorize checks a panel request: the service of the domain brand must be
// inside the caller's services.read scope (else ErrNotFound).
func (p *PDFService) Authorize(ctx context.Context, c Caller, id uuid.UUID) (db.Service, error) {
	return p.svc.getVisible(ctx, c, id)
}

// Build loads the PDF data of an export job and re-checks that the service
// is the job organization's, below it, or in its brand when the job
// organization is the center.
func (p *PDFService) Build(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (PDFDoc, error) {
	id, err := uuid.Parse(strings.TrimSpace(q[PDFQueryServiceUUID]))
	if err != nil {
		return PDFDoc{}, ErrNotFound
	}
	brandID, err := strconv.ParseInt(strings.TrimSpace(q[PDFQueryBrandID]), 10, 64)
	if err != nil || brandID <= 0 {
		return PDFDoc{}, errors.New("services pdf: brand is required")
	}
	jobOrg, err := strconv.ParseInt(strings.TrimSpace(q[ioengine.QueryOrganizationID]), 10, 64)
	if err != nil || jobOrg <= 0 {
		return PDFDoc{}, errPDFScope
	}
	svc, err := p.svc.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PDFDoc{}, ErrNotFound
	}
	if err != nil {
		return PDFDoc{}, fmt.Errorf("services pdf: service: %w", err)
	}
	ok, err := ioengine.JobOrgCovers(ctx, p.svc.q, jobOrg, svc.OrganizationID, svc.BrandID)
	if err != nil {
		return PDFDoc{}, fmt.Errorf("services pdf: %w", err)
	}
	if !ok {
		return PDFDoc{}, errPDFScope
	}
	return p.doc(ctx, svc, loc)
}

func (p *PDFService) doc(ctx context.Context, svc db.Service, loc i18n.Locale) (PDFDoc, error) {
	q := p.svc.q
	// The full view (items with product / unit, warranties) without a
	// caller: no edit flags, nothing else depends on it.
	v, err := p.svc.view(ctx, q, Caller{}, svc)
	if err != nil {
		return PDFDoc{}, err
	}
	org, err := q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PDFDoc{}, fmt.Errorf("services pdf: organization: %w", err)
	}
	zone, err := time.LoadLocation(org.Timezone)
	if err != nil || org.Timezone == "" {
		zone = time.UTC
	}
	date := func(t time.Time) string { return warrantyuc.FormatCertificateDate(t, zone, loc) }
	d := PDFDoc{
		ServiceNo: v.ServiceNo, Status: i18n.Translate(loc, StatusLabelKey(v.Status)),
		CreatedAt: date(v.CreatedAt),
		Dealer: PDFDealer{Name: org.Name, City: org.City, Address: org.Address, Phone: org.Phone,
			Email: org.Email},
		Vehicle:     PDFVehicle{Brand: v.CarBrand.Name, Model: v.CarModel.Name, Plate: deref(v.Plate), VIN: deref(v.VIN)},
		Package:     deref(v.Package),
		Notes:       deref(v.Notes),
		GeneratedAt: p.now(),
		Timezone:    org.Timezone,
	}
	if v.CompletedAt != nil {
		d.CompletedAt = date(*v.CompletedAt)
	}
	if v.Status == StatusCancelled {
		d.CancelReason = deref(v.CancelReason)
	}
	if v.ModelYear != nil {
		d.Vehicle.Year = strconv.Itoa(int(*v.ModelYear))
	}
	// The service snapshot first; a missing value comes from the vehicle
	// (same rule as the warranty certificate).
	if d.Vehicle.Plate == "" || d.Vehicle.VIN == "" || d.Vehicle.Year == "" {
		if veh, err := q.GetVehicleByUUID(ctx, v.VehicleUUID); err == nil {
			if d.Vehicle.Plate == "" {
				d.Vehicle.Plate = strings.TrimSpace(veh.Plate.String)
			}
			if d.Vehicle.VIN == "" {
				d.Vehicle.VIN = strings.TrimSpace(veh.Vin.String)
			}
			if d.Vehicle.Year == "" && veh.ModelYear.Valid {
				d.Vehicle.Year = strconv.Itoa(int(veh.ModelYear.Int16))
			}
		}
	}
	if v.KM != nil {
		d.KM = strconv.FormatInt(int64(*v.KM), 10)
	}
	if v.Customer.Anonymized {
		d.Customer.Name = i18n.Translate(loc, AnonymizedNameKey)
	} else {
		d.Customer.Name = strings.TrimSpace(v.Customer.Name + " " + v.Customer.Surname)
		if ph := deref(v.Customer.Phone); ph != "" {
			d.Customer.Phone = phone.Mask(ph)
		}
	}
	for _, it := range v.Items {
		pi := PDFItem{Product: it.Product.Name, SKU: it.Product.SKU, Barcode: it.Barcode, Kind: it.Kind,
			Meters: deref(it.Meters), Parts: it.AppliedParts, Notes: deref(it.Notes)}
		if it.Quantity != nil {
			pi.Quantity = *it.Quantity
		}
		d.Items = append(d.Items, pi)
	}
	for _, w := range v.Warranties {
		pw := PDFWarranty{PublicCode: w.PublicCode, Product: w.ProductName, Kind: w.ItemKind, Status: w.Status,
			StartDate: date(w.StartAt), EndDate: date(w.EndAt)}
		if p.cert != nil {
			pw.VerifyURL = p.cert.VerifyURL(w.PublicCode)
		}
		d.Warranties = append(d.Warranties, pw)
	}
	settings, err := q.GetAppSettings(ctx)
	if err != nil {
		return PDFDoc{}, fmt.Errorf("services pdf: settings: %w", err)
	}
	d.Letterhead = ioengine.LetterheadFromOrganization(org, settings)
	if p.store != nil {
		if lh, err := ioengine.LoadOrganizationLetterhead(ctx, p.store, org, settings); err != nil {
			p.log.Warn("service_pdf_logo_failed", "error", err)
		} else {
			d.Letterhead = lh
		}
	}
	return d, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// --- Export adapter ------------------------------------------------------------

// PDFAdapter exports the service PDF (panel export job, read only).
type PDFAdapter struct{ svc *PDFService }

// NewPDFAdapter builds the export adapter.
func NewPDFAdapter(svc *PDFService) *PDFAdapter { return &PDFAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *PDFAdapter) Resource() string { return ResourcePDF }

// ExportColumns implements ioengine.ResourceAdapter (generic fallback PDF:
// the item table).
func (a *PDFAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "product", LabelKey: "warranty.certificate.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "barcode", LabelKey: "warranty.certificate.barcode", Type: ioengine.ColumnTypeString},
		{Key: "amount", LabelKey: "services.pdf.amount", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "parts", LabelKey: "services.pdf.parts", Type: ioengine.ColumnTypeString, Weight: 2},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *PDFAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *PDFAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *PDFAdapter) RevertRow(context.Context, string, string, map[string]any) error { return nil }

// Export implements ioengine.ResourceAdapter. Query: service_uuid, brand_id
// and the job organization injected by the worker.
func (a *PDFAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	d, err := a.svc.Build(ctx, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return PDFDataset(d, loc), nil
}

// PDFDataset maps the service PDF to an export dataset.
func PDFDataset(d PDFDoc, loc i18n.Locale) ioengine.Dataset {
	rows := make([]map[string]any, 0, len(d.Items))
	for _, it := range d.Items {
		rows = append(rows, map[string]any{
			"product": it.Product, "barcode": it.Barcode, "amount": itemAmount(it, loc),
			"parts": strings.Join(partLabels(it.Parts, loc), ", "),
		})
	}
	return ioengine.Dataset{
		Resource: ResourcePDF,
		Columns:  (&PDFAdapter{}).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "warranty.certificate.service_no", Value: d.ServiceNo},
			{LabelKey: "services.pdf.status", Value: d.Status},
			{LabelKey: "warranty.certificate.dealer", Value: d.Dealer.Name},
			{LabelKey: "services.pdf.customer", Value: d.Customer.Name},
			{LabelKey: "warranty.certificate.plate", Value: d.Vehicle.Plate},
			{LabelKey: "warranty.certificate.vin", Value: d.Vehicle.VIN},
		},
		Doc: d,
	}
}

// itemAmount: "N m" for a partial cut, "N pcs" for pieces, else the whole unit.
func itemAmount(it PDFItem, loc i18n.Locale) string {
	switch {
	case it.Kind == "partial" && it.Meters != "":
		return it.Meters + " m"
	case it.Quantity > 0:
		return strconv.Itoa(int(it.Quantity)) + " " + i18n.Translate(loc, "services.pdf.pieces")
	}
	return i18n.Translate(loc, "services.pdf.whole")
}

// partLabels translates the applied part keys (services.parts.<key>); an
// unknown key is printed as is.
func partLabels(parts []string, loc i18n.Locale) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key := "services.parts." + part
		if l := i18n.Translate(loc, key); l != key {
			out = append(out, l)
		} else {
			out = append(out, part)
		}
	}
	return out
}

// DocumentHTML implements ioengine.DocumentRenderer.
func (a *PDFAdapter) DocumentHTML(ds ioengine.Dataset, locale string, _ *ioengine.Letterhead, title string) (string, error) {
	d, ok := ds.Doc.(PDFDoc)
	if !ok {
		return "", errors.New("services pdf: dataset carries no service")
	}
	if ds.Timezone != "" {
		d.Timezone = ds.Timezone
	}
	return PDFHTML(d, i18n.Normalize(locale), title)
}

// PDFHTML renders the service document. Every value is escaped; QR codes
// are PNG data: URIs (the document CSP allows no other source).
func PDFHTML(d PDFDoc, loc i18n.Locale, title string) (string, error) {
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	var b strings.Builder
	b.Grow(8192 + 2048*len(d.Items) + 4096*len(d.Warranties))
	lh := d.Letterhead
	meta := func(rows [][2]string, ltr map[int]bool) {
		b.WriteString(`<table class="doc-meta">`)
		for i, row := range rows {
			if strings.TrimSpace(row[1]) == "" {
				continue
			}
			dir := ""
			if ltr[i] {
				dir = ` dir="ltr"`
			}
			b.WriteString(`<tr><th>` + esc(row[0]) + `</th><td` + dir + `>` + esc(row[1]) + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}
	b.WriteString(servicePDFCSS)
	b.WriteString(ioengine.LetterheadHeaderHTML(&lh))
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><div class="sp-grid"><section>`)
	meta([][2]string{
		{t("warranty.certificate.service_no"), d.ServiceNo},
		{t("services.pdf.status"), d.Status},
		{t("services.pdf.created_at"), d.CreatedAt},
		{t("services.pdf.completed_at"), d.CompletedAt},
		{t("services.pdf.cancel_reason"), d.CancelReason},
		{t("warranty.certificate.dealer"), ioengine.JoinNonEmpty(" · ", d.Dealer.Name, d.Dealer.City)},
		{t("warranty.certificate.dealer_contact"), ioengine.JoinNonEmpty(" · ", d.Dealer.Address, d.Dealer.Phone, d.Dealer.Email)},
	}, map[int]bool{0: true})
	b.WriteString(`</section><section>`)
	vehicle := ioengine.JoinNonEmpty(" ", d.Vehicle.Brand, d.Vehicle.Model)
	if d.Vehicle.Year != "" {
		vehicle += " (" + d.Vehicle.Year + ")"
	}
	km := ""
	if d.KM != "" {
		km = d.KM + " km"
	}
	meta([][2]string{
		{t("services.pdf.customer"), d.Customer.Name},
		{t("services.pdf.phone"), d.Customer.Phone},
		{t("warranty.certificate.vehicle"), vehicle},
		{t("warranty.certificate.plate"), d.Vehicle.Plate},
		{t("warranty.certificate.vin"), d.Vehicle.VIN},
		{t("services.pdf.km"), km},
	}, map[int]bool{1: true, 3: true, 4: true})
	b.WriteString(`</section></div>`)
	if d.Package != "" || d.Notes != "" {
		meta([][2]string{{t("services.pdf.package"), d.Package}, {t("services.pdf.notes"), d.Notes}}, nil)
	}

	b.WriteString(`<h2>` + esc(t("services.pdf.items")) + `</h2>`)
	if len(d.Items) == 0 {
		b.WriteString(`<p class="muted">` + esc(t("services.pdf.items_empty")) + `</p>`)
	} else {
		b.WriteString(`<table class="sp-items"><thead><tr><th>#</th><th>` + esc(t("warranty.certificate.product")) +
			`</th><th>` + esc(t("warranty.certificate.barcode")) + `</th><th>` + esc(t("services.pdf.amount")) +
			`</th><th>` + esc(t("services.pdf.parts")) + `</th></tr></thead><tbody>`)
		for i, it := range d.Items {
			b.WriteString(`<tr><td>` + strconv.Itoa(i+1) + `</td><td><strong>` + esc(it.Product) + `</strong>`)
			if it.SKU != "" {
				b.WriteString(`<br><span class="muted" dir="ltr">` + esc(it.SKU) + `</span>`)
			}
			if it.Notes != "" {
				b.WriteString(`<br><span class="muted">` + esc(it.Notes) + `</span>`)
			}
			b.WriteString(`</td><td dir="ltr" class="sp-mono">` + esc(it.Barcode) + `</td><td>` +
				esc(t("warranty.certificate.kind."+it.Kind)) + `<br>` + esc(itemAmount(it, loc)) + `</td><td>`)
			b.WriteString(esc(strings.Join(partLabels(it.Parts, loc), ", ")))
			b.WriteString(`</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}

	b.WriteString(`<h2>` + esc(t("services.pdf.warranties")) + `</h2>`)
	if len(d.Warranties) == 0 {
		key := "services.pdf.no_warranty_pending"
		if d.CompletedAt != "" {
			key = "services.pdf.no_warranty"
		}
		b.WriteString(`<p class="muted">` + esc(t(key)) + `</p>`)
	} else {
		for _, w := range d.Warranties {
			b.WriteString(`<section class="sp-wc"><div class="sp-wc-body">`)
			meta([][2]string{
				{t("warranty.certificate.code"), w.PublicCode},
				{t("warranty.certificate.product"), w.Product},
				{t("warranty.certificate.kind"), t("warranty.certificate.kind." + w.Kind)},
				{t("services.pdf.status"), t("services.pdf.warranty_status." + w.Status)},
				{t("warranty.certificate.start"), w.StartDate},
				{t("warranty.certificate.end"), w.EndDate},
			}, map[int]bool{0: true})
			b.WriteString(`</div>`)
			if w.VerifyURL != "" {
				qr, err := pdfrender.QRCodeImageTag(w.VerifyURL, t("warranty.certificate.verify")+" "+w.PublicCode)
				if err != nil {
					return "", fmt.Errorf("services pdf: qr %s: %w", w.PublicCode, err)
				}
				b.WriteString(`<div class="sp-wc-qr">` + qr + `<p class="sp-url" dir="ltr">` + esc(w.VerifyURL) + `</p></div>`)
			}
			b.WriteString(`</section>`)
		}
		b.WriteString(`<p class="muted">` + esc(t("warranty.certificate.verify_hint")) + `</p>`)
	}
	b.WriteString(`<p class="muted">` + esc(t("warranty.certificate.generated_at")) + `: ` +
		esc(pdfrender.IssuedAt(d.GeneratedAt, pdfrender.Zone(d.Timezone))) + `</p>`)
	b.WriteString(ioengine.LetterheadFooterHTML(&lh))
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: lh.PrimaryColor}.HTML(), nil
}

// servicePDFCSS lays out the service document with logical properties, so
// the layout mirrors in RTL. The skeleton allows inline styles.
const servicePDFCSS = `<style>
.sp-grid{display:flex;gap:16pt}
.sp-grid>section{flex:1;min-width:0}
table.sp-items{width:100%;border-collapse:collapse;font-size:9pt;margin-block-end:8pt}
table.sp-items th,table.sp-items td{border-block-end:1px solid #e5e7eb;padding:4pt;text-align:start;vertical-align:top}
table.sp-items thead th{background:#f3f4f6;border-block-end:2px solid var(--primary)}
table.sp-items tr{break-inside:avoid}
.sp-mono{font-family:monospace;word-break:break-all}
.sp-wc{display:flex;gap:12pt;align-items:flex-start;border:1px solid #d1d5db;border-inline-start:4px solid var(--primary);padding:6pt;margin-block-end:8pt;break-inside:avoid}
.sp-wc-body{flex:1}
.sp-wc-body table.doc-meta{margin:0}
.sp-wc-qr{width:80pt;text-align:center}
.sp-wc-qr img{width:72pt;height:72pt;background:#fff}
.sp-url{font-size:6pt;word-break:break-all;color:#6b7280;margin:2pt 0 0}
</style>`

var (
	_ ioengine.ResourceAdapter  = (*PDFAdapter)(nil)
	_ ioengine.DocumentRenderer = (*PDFAdapter)(nil)
)
