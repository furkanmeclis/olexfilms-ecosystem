package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	documents "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OrderSlipDoc contains only the authorized order and its two parties.
type OrderSlipDoc struct {
	ShippingDoc
	CreatedAt                                          time.Time
	Status, Currency, Subtotal, TaxTotal, Total, Notes string
}

// OrderSlip renders a fresh PDF synchronously, without an export job or cache.
// Scope is checked before loading parties, templates, or invoking the renderer.
func (s *Service) OrderSlip(ctx context.Context, c Caller, id uuid.UUID, lang, timezone string) ([]byte, string, error) {
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("orders: order slip: %w", err)
	}
	if !visible(c, o) {
		return nil, "", ErrNotFound
	}
	if s.pdf == nil {
		return nil, "", ErrDocumentUnavailable
	}
	if lang == "" {
		lang = string(i18n.FromContext(ctx).Locale)
	}
	loc := i18n.Normalize(lang)
	ctx = i18n.WithAcceptLanguage(ctx, string(loc))
	shipping, err := s.shippingDoc(ctx, o)
	if err != nil {
		return nil, "", err
	}
	shipping.Zone = pdfrender.Zone(timezone, shipping.Zone.String())
	d := OrderSlipDoc{ShippingDoc: shipping, CreatedAt: o.CreatedAt.Time,
		Status: i18n.Translate(loc, "orders.status."+o.Status), Currency: o.Currency,
		Subtotal: numericText(o.Subtotal, 2), TaxTotal: numericText(o.TaxTotal, 2), Total: numericText(o.Total, 2), Notes: o.Note.String}
	lh := d.Letterhead
	vars := map[string]string{
		"company_name": lh.CompanyName, "company_address": lh.Address, "company_phone": lh.Phone, "company_email": lh.Email, "company_website": lh.Website, "footer_text": lh.FooterText,
		"company_logo":  pdfrender.ImageTag(lh.LogoMIME, lh.LogoBytes, lh.CompanyName),
		"customer_name": d.Buyer.Name, "customer_phone": d.Buyer.Phone, "customer_email": d.Buyer.Email, "delivery_address": d.Buyer.Address,
		"order_number": d.OrderNo, "order_date": pdfrender.IssuedAt(d.CreatedAt, d.Zone), "items_table": orderSlipItemsHTML(d),
		"subtotal": d.Subtotal + " " + d.Currency, "tax_total": d.TaxTotal + " " + d.Currency, "total_amount": d.Total + " " + d.Currency, "currency": d.Currency,
		"order_status": d.Status, "notes": d.Notes, "seller_tax": d.Seller.TaxNo, "buyer_tax": d.Buyer.TaxNo, "issued_at": pdfrender.IssuedAt(d.GeneratedAt, d.Zone),
	}
	templates := documents.New(nil, s.q, s.store, s.pdf, nil, pdfrender.FontsEmbedded, s.log)
	body, err := templates.TemplateHTML(ctx, model.KindOrderSlip, o.BrandID, string(loc), vars, i18n.Translate(loc, "orders.pdf.order_slip"), lh.PrimaryColor)
	if errors.Is(err, documents.ErrNotFound) {
		body = OrderSlipHTML(d)
	} else if err != nil {
		return nil, "", fmt.Errorf("orders: order slip template: %w", err)
	}
	data, err := s.pdf.Convert(ctx, pdfrender.Request{HTML: body, FooterHTML: pdfrender.FooterHTML(string(loc), lh.CompanyName)})
	if err != nil {
		return nil, "", fmt.Errorf("orders: render order slip: %w", err)
	}
	return data, "order-slip-" + o.OrderNo + ".pdf", nil
}

func orderSlipItemsHTML(d OrderSlipDoc) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<table class="doc-table"><thead><tr>`)
	for _, key := range []string{"product", "sku_barcode", "amount", "unit_price", "discount", "vat", "line_total"} {
		b.WriteString(`<th>` + esc(i18n.Translate(d.Locale, "orders.pdf."+key)) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, l := range d.Lines {
		b.WriteString(`<tr>`)
		// Orders store effective prices and aggregate tax only; do not infer historical discounts or VAT rates.
		for _, v := range []string{l.Name, ioengine.JoinNonEmpty(" / ", l.SKU, strings.Join(l.Barcodes, ", ")), l.Amount, l.UnitPrice + " " + d.Currency, "—", "—", l.LineTotal + " " + d.Currency} {
			b.WriteString(`<td>` + esc(v) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

// OrderSlipHTML renders the built-in layout; every business value is escaped.
func OrderSlipHTML(d OrderSlipDoc) string {
	t := func(key string) string { return i18n.Translate(d.Locale, "orders.pdf."+key) }
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(ioengine.LetterheadHeaderHTML(&d.Letterhead))
	b.WriteString(`<h1 class="doc-title">` + esc(t("order_slip")) + `</h1><table class="doc-meta">`)
	for _, m := range [][2]string{{t("order_no"), d.OrderNo}, {t("order_date"), pdfrender.IssuedAt(d.CreatedAt, d.Zone)}, {t("status"), d.Status}, {t("generated_at"), pdfrender.IssuedAt(d.GeneratedAt, d.Zone)}} {
		b.WriteString(`<tr><th>` + esc(m[0]) + `</th><td>` + esc(m[1]) + `</td></tr>`)
	}
	b.WriteString(`</table><table><tr>`)
	for _, p := range []struct {
		label string
		party ShippingParty
	}{{t("seller"), d.Seller}, {t("buyer"), d.Buyer}} {
		b.WriteString(`<td style="width:50%;vertical-align:top"><h2>` + esc(p.label) + `</h2><strong>` + esc(p.party.Name) + `</strong>`)
		for _, v := range []string{p.party.Address, p.party.Phone, p.party.Email} {
			if v != "" {
				b.WriteString(`<br>` + esc(v))
			}
		}
		if p.party.TaxNo != "" {
			b.WriteString(`<br>` + esc(t("tax_no")) + `: ` + esc(p.party.TaxNo))
		}
		b.WriteString(`</td>`)
	}
	b.WriteString(`</tr></table>` + orderSlipItemsHTML(d) + `<table class="doc-totals">`)
	for _, m := range [][2]string{{t("subtotal"), d.Subtotal}, {t("vat"), d.TaxTotal}, {t("total"), d.Total}} {
		b.WriteString(`<tr><th>` + esc(m[0]) + `</th><td dir="ltr">` + esc(m[1]+" "+d.Currency) + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	if d.Notes != "" {
		b.WriteString(`<h2>` + esc(t("notes")) + `</h2><p style="white-space:pre-wrap">` + esc(d.Notes) + `</p>`)
	}
	b.WriteString(`<div class="doc-sign"><div>` + esc(t("seller_signature")) + `</div><div>` + esc(t("buyer_signature")) + `</div></div>`)
	b.WriteString(ioengine.LetterheadFooterHTMLFor(&d.Letterhead, string(d.Locale)))
	return pdfrender.Document{Lang: string(d.Locale), Title: t("order_slip") + " " + d.OrderNo, Body: b.String(), PrimaryColor: d.Letterhead.PrimaryColor}.HTML()
}
