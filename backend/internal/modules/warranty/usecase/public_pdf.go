package usecase

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// TEC-248 (F2-04b): the anonymous warranty PDF of /garanti/{public_code}.
// It is built only from the public projection (PublicWarranty), so it
// carries exactly what the public page shows: a masked plate, the last four
// VIN characters, no holder and no dealer contact. The full certificate
// (TEC-188) stays behind the panel / portal.

// PublicCertificateHTML renders the public certificate of one warranty.
// verifyURL is the public page the QR points to; dates are printed as
// calendar days in zone and generatedAt as wall-clock time in zone (nil =
// Europe/Istanbul, TEC-521).
func PublicCertificateHTML(w PublicWarranty, verifyURL string, zone *time.Location, loc i18n.Locale, generatedAt time.Time) (string, error) {
	if zone == nil {
		zone = pdfrender.Zone()
	}
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	title := i18n.ResourceLabel(loc, ResourceCertificate)

	vehicle := ioengine.JoinNonEmpty(" ", w.Vehicle.BrandName, w.Vehicle.ModelName)
	if w.Vehicle.ModelYear != nil {
		vehicle += " (" + strconv.Itoa(*w.Vehicle.ModelYear) + ")"
	}
	plate, vin := "", ""
	if w.Vehicle.PlateMasked != nil {
		plate = *w.Vehicle.PlateMasked
	}
	if w.Vehicle.VINLast4 != nil {
		vin = "…" + *w.Vehicle.VINLast4
	}

	var b strings.Builder
	b.WriteString(certificateCSS)
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	for _, row := range [][2]string{
		{t("warranty.certificate.dealer"), ioengine.JoinNonEmpty(" · ", w.Dealer.Name, w.Dealer.City)},
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

	qr, err := pdfrender.QRCodeImageTag(verifyURL, t("warranty.certificate.verify")+" "+w.PublicCode)
	if err != nil {
		return "", fmt.Errorf("warranty public pdf: qr: %w", err)
	}
	b.WriteString(`<section class="wc-card"><div class="wc-body"><table class="doc-meta">`)
	for _, row := range [][2]string{
		{t("warranty.certificate.code"), w.PublicCode},
		{t("warranty.certificate.product"), w.Product.Name},
		{t("warranty.certificate.start"), FormatCertificateDate(w.StartAt, zone, loc)},
		{t("warranty.certificate.end"), FormatCertificateDate(w.EndAt, zone, loc)},
	} {
		b.WriteString(`<tr><th>` + esc(row[0]) + `</th><td>` + esc(row[1]) + `</td></tr>`)
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
	return pdfrender.Document{Lang: string(loc), Title: title, Body: b.String()}.HTML(), nil
}

// PublicVerifyURL is the public page of a code under the frontend origin.
func PublicVerifyURL(frontendURL, publicCode string) string {
	return verifyURL(strings.TrimRight(frontendURL, "/"), publicCode)
}
