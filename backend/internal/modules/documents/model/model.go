// Package model holds the PDF document kinds, their variable schemas and
// the SourceLoader contract that business modules (F1: services,
// measurements, warranties, orders, contracts) implement.
package model

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
)

// Document kinds (document_templates.kind).
const (
	KindService     = "service"
	KindMeasurement = "measurement"
	KindContract    = "contract"
	KindOrderSlip   = "order_slip"
	KindInvoiceView = "invoice_view"
	KindWarranty    = "warranty"
	KindQuote       = "quote"
)

// Languages a template may be written in (K10: 12 languages + Arabic).
// A missing language falls back to en.
var Languages = []string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "zh_CN", "az", "ar"}

// FallbackLanguage is used when no template exists for the requested one.
const FallbackLanguage = "en"

// IsLanguage reports whether code is a supported template language.
func IsLanguage(code string) bool {
	for _, l := range Languages {
		if l == code {
			return true
		}
	}
	return false
}

// NormalizeLanguage maps "ar-SA", "zh-cn", "TR" to a supported code, or "".
func NormalizeLanguage(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "-", "_"))
	if strings.EqualFold(s, "zh_cn") || strings.EqualFold(s, "zh") {
		return "zh_CN"
	}
	if i := strings.Index(s, "_"); i > 0 {
		s = s[:i]
	}
	s = strings.ToLower(s)
	if IsLanguage(s) {
		return s
	}
	return ""
}

// Variable types.
const (
	// VarText values are HTML-escaped on render.
	VarText = "text"
	// VarHTML values are HTML blocks built by the server (tables, images).
	VarHTML = "html"
)

// Variable is one `{{key}}` a template of a kind may use.
type Variable struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Group    string `json:"group"`
	LabelTR  string `json:"label_tr"`
	LabelEN  string `json:"label_en"`
	SampleTR string `json:"-"`
	SampleEN string `json:"-"`
}

// KindSpec is a document kind with its variable schema.
type KindSpec struct {
	Kind      string     `json:"kind"`
	Variables []Variable `json:"variables"`
}

// RawHTMLKeys returns the keys whose values are inserted unescaped.
func (k KindSpec) RawHTMLKeys() map[string]bool {
	out := map[string]bool{}
	for _, v := range k.Variables {
		if v.Type == VarHTML {
			out[v.Key] = true
		}
	}
	return out
}

// Unknown returns placeholders in html that the kind does not define.
func (k KindSpec) Unknown(html string) []string {
	allowed := map[string]bool{}
	for _, v := range k.Variables {
		allowed[v.Key] = true
	}
	var out []string
	for _, p := range msgtemplate.Placeholders(html) {
		if !allowed[p] {
			out = append(out, p)
		}
	}
	return out
}

// SampleVars returns preview values (tr samples for tr, en otherwise).
func (k KindSpec) SampleVars(locale string) map[string]string {
	out := make(map[string]string, len(k.Variables))
	tr := NormalizeLanguage(locale) == "tr"
	for _, v := range k.Variables {
		if tr {
			out[v.Key] = v.SampleTR
		} else {
			out[v.Key] = v.SampleEN
		}
	}
	for key, build := range sampleBlocks {
		if _, ok := out[key]; ok {
			out[key] = build(tr)
		}
	}
	return out
}

func text(group, key, labelTR, labelEN, sampleTR, sampleEN string) Variable {
	return Variable{Key: key, Type: VarText, Group: group, LabelTR: labelTR, LabelEN: labelEN, SampleTR: sampleTR, SampleEN: sampleEN}
}

func block(group, key, labelTR, labelEN string) Variable {
	return Variable{Key: key, Type: VarHTML, Group: group, LabelTR: labelTR, LabelEN: labelEN}
}

// Company variables are filled by the renderer from the organization
// letterhead (name, contact, logo); a loader may override them.
var companyVars = []Variable{
	text("company", "company_name", "Firma adı", "Company name", "Olex Films İstanbul", "Olex Films Istanbul"),
	text("company", "company_address", "Firma adresi", "Company address", "Atatürk Cd. No:1, Kadıköy / İstanbul", "Ataturk St. No:1, Kadikoy / Istanbul"),
	text("company", "company_phone", "Firma telefonu", "Company phone", "+90 216 555 00 00", "+90 216 555 00 00"),
	text("company", "company_email", "Firma e-postası", "Company e-mail", "info@olexfilms.app", "info@olexfilms.app"),
	text("company", "company_website", "Firma web sitesi", "Company website", "olexfilms.app", "olexfilms.app"),
	block("company", "company_logo", "Firma logosu", "Company logo"),
	text("company", "footer_text", "Alt bilgi", "Footer text", "Olex Films — olexfilms.app", "Olex Films — olexfilms.app"),
}

var documentVars = []Variable{
	text("document", "document_number", "Belge no", "Document number", "HZM-2026-000042", "SRV-2026-000042"),
	text("document", "document_date", "Belge tarihi", "Document date", "01.10.2026", "Oct 1, 2026"),
}

var customerVars = []Variable{
	text("customer", "customer_name", "Müşteri adı", "Customer name", "Ahmet Yılmaz", "John Smith"),
	text("customer", "customer_phone", "Müşteri telefonu", "Customer phone", "+90 555 123 45 67", "+90 555 123 45 67"),
	text("customer", "customer_email", "Müşteri e-postası", "Customer e-mail", "ahmet@example.com", "john@example.com"),
}

var vehicleVars = []Variable{
	text("vehicle", "plate", "Plaka", "Plate", "34 ABC 123", "34 ABC 123"),
	text("vehicle", "vehicle", "Araç", "Vehicle", "BMW 320i (2024)", "BMW 320i (2024)"),
}

var totalsVars = []Variable{
	text("totals", "subtotal", "Ara toplam", "Subtotal", "10.416,67 TRY", "TRY 10,416.67"),
	text("totals", "tax_total", "Vergi", "Tax", "2.083,33 TRY", "TRY 2,083.33"),
	text("totals", "total_amount", "Genel toplam", "Total", "12.500,00 TRY", "TRY 12,500.00"),
	text("totals", "currency", "Para birimi", "Currency", "TRY", "TRY"),
}

func join(groups ...[]Variable) []Variable {
	var out []Variable
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

var specs = map[string]KindSpec{
	KindService: {Kind: KindService, Variables: join(companyVars, documentVars, customerVars, vehicleVars, []Variable{
		text("service", "technician_name", "Teknisyen", "Technician", "Mehmet Demir", "Michael Doe"),
		text("service", "warranty_code", "Garanti no", "Warranty number", "OLX-W-000042", "OLX-W-000042"),
		block("service", "items_table", "Hizmet kalemleri tablosu", "Service items table"),
		text("service", "notes", "Notlar", "Notes", "Araç teslimde yıkanmıştır.", "Vehicle was washed on delivery."),
		text("totals", "total_amount", "Genel toplam", "Total", "12.500,00 TRY", "TRY 12,500.00"),
	})},
	KindMeasurement: {Kind: KindMeasurement, Variables: join(companyVars, documentVars, customerVars, vehicleVars, []Variable{
		text("measurement", "measured_at", "Ölçüm tarihi", "Measured at", "01.10.2026 14:30", "Oct 1, 2026 2:30 PM"),
		text("measurement", "technician_name", "Ölçen", "Measured by", "Mehmet Demir", "Michael Doe"),
		block("measurement", "measurements_table", "Ölçüm tablosu", "Measurements table"),
		text("measurement", "notes", "Notlar", "Notes", "Kaput bölgesinde boya kalınlığı yüksek.", "Hood paint thickness is high."),
	})},
	KindContract: {Kind: KindContract, Variables: join(companyVars, documentVars, []Variable{
		text("contract", "contract_title", "Sözleşme başlığı", "Contract title", "Bayilik Sözleşmesi", "Dealership Agreement"),
		text("contract", "party_name", "Karşı taraf", "Counterparty", "Tech Oto Ltd. Şti.", "Tech Oto Ltd."),
		text("contract", "signed_at", "İmza tarihi", "Signed at", "01.10.2026", "Oct 1, 2026"),
		block("contract", "contract_body_html", "Sözleşme metni", "Contract body"),
		block("contract", "signature_image", "İmza görseli", "Signature image"),
	})},
	KindOrderSlip: {Kind: KindOrderSlip, Variables: join(companyVars, customerVars, totalsVars, []Variable{
		text("order", "order_number", "Sipariş no", "Order number", "SIP-2026-000042", "ORD-2026-000042"),
		text("order", "order_date", "Sipariş tarihi", "Order date", "01.10.2026", "Oct 1, 2026"),
		text("order", "delivery_address", "Teslimat adresi", "Delivery address", "Organize Sanayi Böl. 5. Cd. No:12, Ankara", "Industrial Zone 5th St. No:12, Ankara"),
		block("order", "items_table", "Sipariş kalemleri tablosu", "Order items table"),
	})},
	KindInvoiceView: {Kind: KindInvoiceView, Variables: join(companyVars, customerVars, totalsVars, []Variable{
		text("invoice", "invoice_number", "Fatura no", "Invoice number", "FTR-2026-000042", "INV-2026-000042"),
		text("invoice", "invoice_date", "Fatura tarihi", "Invoice date", "01.10.2026", "Oct 1, 2026"),
		text("invoice", "due_date", "Vade tarihi", "Due date", "31.10.2026", "Oct 31, 2026"),
		text("invoice", "billing_address", "Fatura adresi", "Billing address", "Bağdat Cd. No:100, Kadıköy / İstanbul", "Bagdat St. No:100, Kadikoy / Istanbul"),
		block("invoice", "items_table", "Fatura kalemleri tablosu", "Invoice items table"),
	})},
	KindWarranty: {Kind: KindWarranty, Variables: join(companyVars, customerVars, vehicleVars, []Variable{
		text("warranty", "warranty_code", "Garanti no", "Warranty number", "OLX-W-000042", "OLX-W-000042"),
		text("warranty", "product_name", "Ürün", "Product", "Olex PPF Ultra 190µ", "Olex PPF Ultra 190µ"),
		text("warranty", "issued_at", "Düzenlenme tarihi", "Issued at", "01.10.2026", "Oct 1, 2026"),
		text("warranty", "valid_until", "Geçerlilik", "Valid until", "01.10.2036", "Oct 1, 2036"),
		text("warranty", "coverage_text", "Kapsam", "Coverage", "Sararma, çatlama ve kabarmaya karşı 10 yıl garanti.", "10-year warranty against yellowing, cracking and blistering."),
		block("warranty", "qr_code", "Doğrulama QR kodu", "Verification QR code"),
		text("warranty", "verify_url", "Doğrulama adresi", "Verification URL", "https://olexfilms.app/garanti/OLX-W-000042", "https://olexfilms.app/garanti/OLX-W-000042"),
	})},
	KindQuote: {Kind: KindQuote, Variables: join(companyVars, documentVars, customerVars, totalsVars, []Variable{
		text("quote", "quote_number", "Teklif no", "Quote number", "Q-000042", "Q-000042"),
		text("quote", "valid_until", "Geçerlilik tarihi", "Valid until", "15.10.2026", "Oct 15, 2026"),
		text("quote", "status", "Durum", "Status", "draft", "draft"),
		block("quote", "items_table", "Teklif kalemleri tablosu", "Quote line items table"),
		text("totals", "discount_total", "İndirim", "Discount", "500,00 TRY", "TRY 500.00"),
	})},
}

func init() {
	// de-duplicate keys (later definitions win) and keep a stable order
	for kind, spec := range specs {
		seen := map[string]int{}
		var vars []Variable
		for _, v := range spec.Variables {
			if i, ok := seen[v.Key]; ok {
				vars[i] = v
				continue
			}
			seen[v.Key] = len(vars)
			vars = append(vars, v)
		}
		spec.Variables = vars
		specs[kind] = spec
	}
}

// Kinds lists the document kinds in display order.
var Kinds = []string{KindService, KindMeasurement, KindContract, KindOrderSlip, KindInvoiceView, KindWarranty, KindQuote}

// Spec returns the schema of a kind.
func Spec(kind string) (KindSpec, bool) {
	s, ok := specs[kind]
	return s, ok
}

// AllSpecs returns every kind spec in display order.
func AllSpecs() []KindSpec {
	out := make([]KindSpec, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, specs[k])
	}
	return out
}

// Preview blocks for html variables.
var sampleBlocks = map[string]func(tr bool) string{
	"company_logo": func(bool) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="36" viewBox="0 0 120 36"><rect width="120" height="36" rx="6" fill="#0F172A"/><text x="60" y="23" font-size="14" font-family="sans-serif" fill="#fff" text-anchor="middle">LOGO</text></svg>`
	},
	"items_table": func(tr bool) string {
		if tr {
			return pdfrender.Table([]pdfrender.Column{{Label: "Hizmet / Ürün"}, {Label: "Adet", Numeric: true}, {Label: "Birim fiyat", Numeric: true}, {Label: "Tutar", Numeric: true}}, [][]string{
				{"Tam araç PPF kaplama", "1", "10.000,00 TRY", "10.000,00 TRY"},
				{"Cam filmi", "1", "2.500,00 TRY", "2.500,00 TRY"},
			})
		}
		return pdfrender.Table([]pdfrender.Column{{Label: "Service / Product"}, {Label: "Qty", Numeric: true}, {Label: "Unit price", Numeric: true}, {Label: "Amount", Numeric: true}}, [][]string{
			{"Full vehicle PPF", "1", "TRY 10,000.00", "TRY 10,000.00"},
			{"Window film", "1", "TRY 2,500.00", "TRY 2,500.00"},
		})
	},
	"measurements_table": func(tr bool) string {
		cols := []pdfrender.Column{{Label: "Panel"}, {Label: "Boya (µm)", Numeric: true}}
		rows := [][]string{{"Kaput", "142"}, {"Sol ön çamurluk", "118"}, {"Tavan", "121"}}
		if !tr {
			cols = []pdfrender.Column{{Label: "Panel"}, {Label: "Paint (µm)", Numeric: true}}
			rows = [][]string{{"Hood", "142"}, {"Front left fender", "118"}, {"Roof", "121"}}
		}
		return pdfrender.Table(cols, rows)
	},
	"contract_body_html": func(tr bool) string {
		if tr {
			return "<h2>1. Taraflar</h2><p>Bu sözleşme aşağıda bilgileri bulunan taraflar arasında imzalanmıştır.</p><h2>2. Konu</h2><p>Sözleşmenin konusu bayilik koşullarının belirlenmesidir.</p>"
		}
		return "<h2>1. Parties</h2><p>This agreement is signed between the parties listed below.</p><h2>2. Subject</h2><p>The subject of this agreement is the dealership terms.</p>"
	},
	"signature_image": func(bool) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" width="140" height="40" viewBox="0 0 140 40"><path d="M5 30 C 30 5, 40 40, 65 20 S 110 10, 135 25" stroke="#1e3a8a" stroke-width="2" fill="none"/></svg>`
	},
	"qr_code": func(bool) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80" viewBox="0 0 8 8"><rect width="8" height="8" fill="#fff"/><path d="M0 0h3v3H0zM5 0h3v3H5zM0 5h3v3H0zM4 4h1v1H4zM6 5h1v2H6zM4 6h1v2H4z" fill="#000"/></svg>`
	},
}

// ErrSourceNotFound is returned by loaders when the source does not exist or
// the viewer may not see it (the API answers 404 either way).
var ErrSourceNotFound = errors.New("documents: source not found")

// Viewer is who asks for a document. System is set for the worker, which
// re-loads a source that was authorized at request time.
type Viewer struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
	System         bool
}

// Source is the data of one business record for a document.
type Source struct {
	OrganizationID int64
	BrandID        int64
	// Version must change whenever the rendered data changes (e.g.
	// updated_at in nanoseconds); it is part of the cache key.
	Version string
	// Vars holds text and html-block values keyed like the kind's
	// variables. Build html blocks with pdfrender.Table / ImageTag.
	Vars map[string]string
	// Title is used as the PDF title and download file name.
	Title string
}

// SourceLoader loads one source record for a document kind. Business
// modules register one per kind; the loader owns read permission and scope
// (return ErrSourceNotFound when the viewer may not see the record).
type SourceLoader interface {
	SourceType() string
	Load(ctx context.Context, viewer Viewer, sourceID string, locale string) (Source, error)
}

// TemplateView is the API shape of a template version.
type TemplateView struct {
	UUID        uuid.UUID  `json:"uuid"`
	Kind        string     `json:"kind"`
	BrandUUID   *uuid.UUID `json:"brand_uuid,omitempty"`
	BrandSlug   *string    `json:"brand_slug,omitempty"`
	Language    string     `json:"language"`
	Name        string     `json:"name"`
	Version     int32      `json:"version"`
	Status      string     `json:"status"`
	IsActive    bool       `json:"is_active"`
	HTML        *string    `json:"html,omitempty"`
	LexicalJSON any        `json:"lexical_json,omitempty"`
	Variables   []string   `json:"variables"`
	ContentHash string     `json:"content_hash"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Template statuses (derived).
const (
	StatusDraft      = "draft"
	StatusActive     = "active"
	StatusSuperseded = "superseded"
)

// RenderView is the API shape of a rendered document.
type RenderView struct {
	UUID        uuid.UUID  `json:"uuid"`
	Kind        string     `json:"kind"`
	SourceType  string     `json:"source_type"`
	SourceID    string     `json:"source_id"`
	Locale      string     `json:"locale"`
	Status      string     `json:"status"`
	SizeBytes   *int64     `json:"size_bytes,omitempty"`
	SHA256      *string    `json:"sha256,omitempty"`
	Error       *string    `json:"error,omitempty"`
	DownloadURL *string    `json:"download_url,omitempty"`
	RenderedAt  *time.Time `json:"rendered_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SortedKeys returns map keys sorted (helper for stable output).
func SortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
