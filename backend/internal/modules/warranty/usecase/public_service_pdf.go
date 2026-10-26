package usecase

import (
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/certtheme"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// PublicServiceCertificateHTML renders the anonymous certificate of a
// service (TEC-1005): one document with every warranty of the service,
// built from the public projection only (masked plate, last four VIN
// characters, no holder, no dealer contact). An Olex service carries the
// legacy certificate pages (TEC-772) with the applied parts on the car map.
// verifyURL is the service page the QR points to.
func PublicServiceCertificateHTML(p PublicService, verifyURL string, zone *time.Location, loc i18n.Locale, generatedAt time.Time) (string, error) {
	if zone == nil {
		zone = pdfrender.Zone()
	}
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	title := i18n.ResourceLabel(loc, ResourceCertificate)

	vehicle := ioengine.JoinNonEmpty(" ", p.Vehicle.BrandName, p.Vehicle.ModelName)
	if p.Vehicle.ModelYear != nil {
		vehicle += " (" + strconv.Itoa(*p.Vehicle.ModelYear) + ")"
	}
	plate, vin := "", ""
	if p.Vehicle.PlateMasked != nil {
		plate = *p.Vehicle.PlateMasked
	}
	if p.Vehicle.VINLast4 != nil {
		vin = "…" + *p.Vehicle.VINLast4
	}

	var b strings.Builder
	b.WriteString(certificateCSS)
	if certtheme.ForBrand(p.Brand.Slug) {
		car := certtheme.Vehicle{Make: p.Vehicle.BrandName, Model: p.Vehicle.ModelName}
		if p.Vehicle.ModelYear != nil {
			car.Year = strconv.Itoa(*p.Vehicle.ModelYear)
		}
		in := certtheme.Input{Vehicle: car}
		for _, w := range p.Warranties {
			if w.Status == "void" {
				continue
			}
			for _, part := range w.Parts {
				if !slices.Contains(in.Parts, part) {
					in.Parts = append(in.Parts, part)
				}
			}
			in.Services = append(in.Services, certtheme.Service{Category: w.Category, Code: w.PublicCode,
				Product: w.Product.Name, Warranty: FormatCertificateDate(w.EndAt, zone, loc)})
		}
		b.WriteString(certtheme.CSS())
		b.WriteString(certtheme.OpeningPages(in, loc))
	}
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	for _, row := range [][2]string{
		{t("warranty.certificate.service_no"), p.ServiceNo},
		{t("warranty.certificate.service_date"), FormatCertificateDate(p.ServiceDate, zone, loc)},
		{t("warranty.certificate.dealer"), ioengine.JoinNonEmpty(" · ", p.Dealer.Name, p.Dealer.City)},
		{t("warranty.certificate.vehicle"), vehicle},
		{t("warranty.certificate.plate"), plate},
		{t("warranty.certificate.vin"), vin},
	} {
		if strings.TrimSpace(row[1]) == "" {
			continue
		}
		b.WriteString(`<tr><th>` + esc(row[0]) + `</th><td>` + esc(row[1]) + `</td></tr>`)
	}
	b.WriteString(`</table><h2>` + esc(t("warranty.certificate.warranties")) + `</h2>`)

	qr, err := pdfrender.QRCodeImageTag(verifyURL, t("warranty.certificate.verify")+" "+p.ServiceNo)
	if err != nil {
		return "", fmt.Errorf("warranty public pdf: qr: %w", err)
	}
	b.WriteString(`<section class="wc-card"><div class="wc-body"><table class="doc-meta">`)
	b.WriteString(`<tr><th>` + esc(t("warranty.certificate.product")) + `</th><th>` + esc(t("warranty.certificate.start")) +
		`</th><th>` + esc(t("warranty.certificate.end")) + `</th><th>` + esc(t("warranty.certificate.status")) + `</th></tr>`)
	for _, w := range p.Warranties {
		product := w.Product.Name
		if w.Category != "" {
			product = w.Category + " · " + product
		}
		b.WriteString(`<tr><td>` + esc(product) + ` <span class="muted" dir="ltr">#` + esc(w.PublicCode) + `</span></td><td>` +
			esc(FormatCertificateDate(w.StartAt, zone, loc)) + `</td><td>` + esc(FormatCertificateDate(w.EndAt, zone, loc)) +
			`</td><td>` + esc(t("warranty.certificate.status."+w.Status)) + `</td></tr>`)
	}
	b.WriteString(`</table></div><div class="wc-qr">` + qr + `<p class="wc-url" dir="ltr">` + esc(verifyURL) + `</p></div></section>`)
	b.WriteString(`<p class="muted">` + esc(t("warranty.certificate.verify_hint")) + `</p>`)
	b.WriteString(`<h2>` + esc(t("warranty.certificate.terms_title")) + `</h2><ol class="wc-terms">`)
	for _, line := range strings.Split(t("warranty.certificate.terms"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString(`<li>` + esc(line) + `</li>`)
		}
	}
	b.WriteString(`</ol><p class="muted">` + esc(t("warranty.certificate.generated_at")) + `: ` +
		esc(pdfrender.IssuedAt(generatedAt, zone)) + `</p>`)
	if certtheme.ForBrand(p.Brand.Slug) {
		b.WriteString(certtheme.ClosingPages(loc))
	}
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String()}.HTML(), nil
}
