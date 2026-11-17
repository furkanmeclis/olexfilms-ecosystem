package usecase

// TEC-1225: the discrepancy record ("kabul tutanağı") PDF of a stock entry,
// rendered synchronously through Gotenberg like the label sheets. The
// document names the supplier, the delivery, the warehouse, the decisions
// with their reasons and the people who took them. No price is printed.

import (
	"context"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
)

// PDFRenderer converts HTML to PDF (satisfied by *pdfrender.Client).
type PDFRenderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// ReceiptPDFDoc is the data of one discrepancy record PDF.
type ReceiptPDFDoc struct {
	Receipt      EntryReceipt
	Organization string
	GeneratedAt  time.Time
	Letterhead   ioengine.Letterhead
}

// ReceiptPDF renders the discrepancy record of an entry in the locale.
func (s *StockEntries) ReceiptPDF(ctx context.Context, c EntryCaller, id uuid.UUID, locale string) ([]byte, string, error) {
	if s.pdf == nil {
		return nil, "", ErrEntryStorage
	}
	rec, err := s.Receipt(ctx, c, id)
	if err != nil {
		return nil, "", err
	}
	org, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return nil, "", err
	}
	d := ReceiptPDFDoc{Receipt: rec, Organization: org.Name, GeneratedAt: time.Now().UTC()}
	if settings, err := s.q.GetAppSettings(ctx); err == nil {
		d.Letterhead = ioengine.LetterheadFromOrganization(org, settings)
		if s.store != nil {
			if lh, err := ioengine.LoadOrganizationLetterhead(ctx, s.store, org, settings); err == nil {
				d.Letterhead = lh
			} else {
				slog.Default().Warn("entry_receipt_logo_failed", "error", err)
			}
		}
	}
	loc := i18n.Normalize(locale)
	out, err := s.pdf.Convert(ctx, pdfrender.Request{HTML: ReceiptPDFHTML(d, loc)})
	if err != nil {
		return nil, "", err
	}
	name := "tutanak-" + id.String()[:8] + ".pdf"
	if rec.ContainerNo != nil {
		name = "tutanak-" + safeFilePart(*rec.ContainerNo) + ".pdf"
	}
	return out, name, nil
}

func safeFilePart(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}

// ReceiptPDFHTML renders the record. Every value is escaped.
func ReceiptPDFHTML(d ReceiptPDFDoc, loc i18n.Locale) string {
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	r := d.Receipt
	lh := d.Letterhead
	title := t("warehouse.receipt.title")
	var b strings.Builder
	b.WriteString(receiptPDFCSS)
	b.WriteString(ioengine.LetterheadHeaderHTML(&lh))
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	meta := func(label, value string, ltr bool) {
		if value == "" {
			value = "—"
		}
		dir := ""
		if ltr {
			dir = ` dir="ltr"`
		}
		b.WriteString(`<tr><th>` + esc(label) + `</th><td` + dir + `>` + esc(value) + `</td></tr>`)
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	meta(t("warehouse.receipt.organization"), d.Organization, false)
	if r.Warehouse != nil {
		meta(t("warehouse.receipt.warehouse"), r.Warehouse.Code+" · "+r.Warehouse.Name, false)
	}
	if r.Supplier != nil {
		sup := r.Supplier.Name
		if r.Supplier.TaxNo != nil {
			sup += " · " + t("warehouse.receipt.tax_no") + " " + *r.Supplier.TaxNo
		}
		meta(t("warehouse.receipt.supplier"), sup, false)
		if r.Supplier.Phone != nil || r.Supplier.Email != nil {
			meta(t("warehouse.receipt.supplier_contact"), strings.Trim(str(r.Supplier.Phone)+" "+str(r.Supplier.Email), " "), true)
		}
	} else {
		meta(t("warehouse.receipt.supplier"), "", false)
	}
	meta(t("warehouse.receipt.container_no"), str(r.ContainerNo), true)
	meta(t("warehouse.receipt.waybill_no"), str(r.WaybillNo), true)
	meta(t("warehouse.receipt.arrival_date"), str(r.ArrivalDate), true)
	meta(t("warehouse.receipt.status"), t("warehouse.receipt.status."+r.Status), false)
	if r.ConfirmedAt != nil {
		meta(t("warehouse.receipt.confirmed_at"), pdfrender.IssuedAt(*r.ConfirmedAt, pdfrender.Zone("", ""), loc), true)
	}
	if r.ConfirmedBy != nil {
		meta(t("warehouse.receipt.confirmed_by"), r.ConfirmedBy.Name, false)
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h2>` + esc(t("warehouse.receipt.summary")) + `</h2><table class="rcpt"><tbody>`)
	sum := func(key string, n int64) {
		b.WriteString(`<tr><td>` + esc(t("warehouse.receipt.summary."+key)) + `</td><td class="num" dir="ltr">` + strconv.FormatInt(n, 10) + `</td></tr>`)
	}
	sum("entered", r.Summary.Entered)
	sum("missing", r.Summary.Missing)
	sum("extras_added", r.Summary.ExtrasAdded)
	sum("extras_rejected", r.Summary.ExtrasRejected)
	sum("damaged_accepted", r.Summary.DamagedAccepted)
	sum("damaged_excluded", r.Summary.DamagedExcluded)
	b.WriteString(`</tbody></table>`)

	b.WriteString(`<h2>` + esc(t("warehouse.receipt.items")) + `</h2>`)
	if len(r.Items) == 0 {
		b.WriteString(`<p class="muted">` + esc(t("warehouse.receipt.no_items")) + `</p>`)
	} else {
		b.WriteString(`<table class="rcpt"><thead><tr>`)
		for _, k := range []string{"kind", "barcode", "product", "quantity", "decision", "reason", "photos"} {
			b.WriteString(`<th>` + esc(t("warehouse.receipt.col."+k)) + `</th>`)
		}
		b.WriteString(`</tr></thead><tbody>`)
		for _, it := range r.Items {
			product := ""
			if it.Product != nil {
				product = it.Product.Name
				if it.Product.SKU != "" {
					product += " (" + it.Product.SKU + ")"
				}
			}
			names := make([]string, 0, len(it.Attachments))
			for _, a := range it.Attachments {
				names = append(names, a.Filename)
			}
			b.WriteString(`<tr><td>` + esc(t("warehouse.receipt.kind."+it.Kind)) + `</td><td dir="ltr">` + esc(it.Barcode) +
				`</td><td>` + esc(product) + `</td><td class="num" dir="ltr">` + strconv.Itoa(int(it.Quantity)) +
				`</td><td>` + esc(t("warehouse.receipt.decision."+it.Decision)) + `</td><td>` + esc(str(it.Reason)) +
				`</td><td>` + esc(strings.Join(names, ", ")) + `</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`<table class="sign"><tr><td>` + esc(t("warehouse.receipt.sign_receiver")) + `</td><td>` +
		esc(t("warehouse.receipt.sign_supplier")) + `</td></tr></table>`)
	b.WriteString(`<p class="muted">` + esc(t("warehouse.receipt.generated_at")) + `: ` +
		esc(pdfrender.IssuedAt(d.GeneratedAt, pdfrender.Zone("", ""), loc)) + `</p>`)
	b.WriteString(ioengine.LetterheadFooterHTMLFor(&lh, string(loc)))
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String(), PrimaryColor: lh.PrimaryColor}.HTML()
}

const receiptPDFCSS = `<style>
table.rcpt{width:100%;border-collapse:collapse;font-size:9pt;margin-block-end:8pt}
table.rcpt th,table.rcpt td{border-block-end:1px solid #e5e7eb;padding:4pt;text-align:start;vertical-align:top}
table.rcpt th{background:#f3f4f6;font-weight:600}
table.rcpt td.num{text-align:end}
table.sign{width:100%;margin-block-start:28pt;font-size:9pt}
table.sign td{width:50%;padding-block-start:36pt;border-block-start:1px solid #9ca3af;text-align:center}
.muted{color:#6b7280;font-size:8pt}
</style>`
