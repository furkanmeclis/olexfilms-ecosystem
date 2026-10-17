// Package certtheme renders the legacy Olex digital certificate pages
// (TEC-772): the dark green cover, the summary with the car part map and the
// applied services table, and the static closing pages (care guide,
// certifications, test reports). The service PDF and the warranty
// certificate put them around their report pages for Olex brand documents;
// other brands keep the plain letterhead document (K20).
package certtheme

import (
	"embed"
	"encoding/base64"
	"html"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// BrandSlug is the brand whose documents carry the certificate pages.
const BrandSlug = "olex"

// ForBrand reports whether documents of the brand get the certificate pages.
func ForBrand(slug string) bool {
	return strings.EqualFold(strings.TrimSpace(slug), BrandSlug)
}

//go:embed assets
var assets embed.FS

// Vehicle is the car on the cover: make (with its catalog logo), model and
// model year.
type Vehicle struct {
	Make     string
	Model    string
	Year     string
	LogoMIME string
	Logo     []byte
}

// Service is one row of the applied services table.
type Service struct {
	Category string
	Code     string
	Product  string
	// Warranty is the printed warranty end ("" prints a dash).
	Warranty string
}

// Input is the data of the certificate pages.
type Input struct {
	Vehicle  Vehicle
	Parts    []string
	Services []Service
}

// Colors of the legacy certificate.
const (
	pageColor   = "#003f26"
	headColor   = "#003a22"
	rowColor    = "#002315"
	borderColor = "#47a27d"
	highlight   = "#f7b500"
	bodyFill    = "#1a8f14"
	windowFill  = "#3db5ff"
)

// pageLocales are the languages of the localized closing pages; others use
// the English ones.
var pageLocales = map[i18n.Locale]string{
	i18n.LocaleTR: "tr", i18n.LocaleEN: "en", i18n.LocaleDE: "de", i18n.LocaleBG: "bg", i18n.LocaleRU: "ru",
}

// CSS is the style block of the certificate pages. The named page "ct" has
// no margin, so the dark pages bleed to the paper edge while the report
// pages keep the Gotenberg margins.
func CSS() string {
	return `<style>@font-face{font-family:"Michroma";font-style:normal;font-weight:400;font-display:block;src:url(data:font/woff2;base64,` +
		base64.StdEncoding.EncodeToString(asset("michroma-400.woff2")) + `) format("woff2")}
@page ct{margin:0}
.ct-page{page:ct;break-after:page;position:relative;overflow:hidden;box-sizing:border-box;width:100%;height:297mm;margin:0;background:` + pageColor + `;color:#fff;font-family:"Michroma","Noto Sans","Noto Sans Arabic","Noto Sans CJK SC",sans-serif;font-weight:400;text-align:center}
.ct-page.ct-summary{height:auto;min-height:297mm;padding-block-end:1cm}
.ct-watermark{position:absolute;top:2cm;inset-inline-start:-150px;width:400px;opacity:.5}
.ct-logo{display:block;width:10cm;margin:10cm auto 5cm}
.ct-summary .ct-logo{width:3.97cm;margin:0 auto 30px;padding-block-start:1cm}
.ct-car{display:inline-block}
.ct-make{display:flex;justify-content:center;align-items:center;gap:10px;font-size:27px;letter-spacing:.5px}
.ct-make-logo{height:30px;object-fit:contain}
.ct-model{display:block;font-size:20px;letter-spacing:.5px}
.ct-map{display:block;width:12cm;margin:20px auto}
.ct-services{margin:20px 1.3cm 0;border:2px solid ` + borderColor + `;border-radius:10px;overflow:hidden;text-align:start}
.ct-services table{width:100%;border-collapse:collapse;margin:0}
.ct-services thead td{background:` + headColor + `;padding:15px 20px}
.ct-services .ct-head{display:flex;justify-content:space-between;align-items:center}
.ct-services .ct-head-logo{height:50px}
.ct-services .ct-head span{font-size:24px;letter-spacing:1px}
.ct-services tbody tr{background:` + rowColor + `;border-block-end:1px solid ` + headColor + `;break-inside:avoid}
.ct-services tbody tr:last-child{border-block-end:0}
.ct-services tbody td{padding:10px;vertical-align:top;width:33%}
.ct-services .ct-label{display:block;font-size:12px}
.ct-services .ct-value{display:block;font-size:14px;overflow-wrap:anywhere}
.ct-services .ct-code{color:` + highlight + `}
.ct-services .ct-check{width:15px;height:15px;vertical-align:middle;margin-inline-start:4px}
.ct-image img{display:block;width:100%;height:100%;object-fit:cover}
</style>`
}

// OpeningPages are the cover and the summary page.
func OpeningPages(in Input, loc i18n.Locale) string {
	t := func(key string) string { return i18n.Translate(loc, key) }
	esc := html.EscapeString
	logo := pdfrender.ImageDataURI("image/png", asset("olex-logo.png"))
	makeLogo := pdfrender.ImageDataURI(in.Vehicle.LogoMIME, in.Vehicle.Logo)
	car := carHTML(in.Vehicle, makeLogo)
	var b strings.Builder
	b.WriteString(`<section class="ct-page ct-cover">`)
	if makeLogo != "" {
		b.WriteString(`<img class="ct-watermark" src="` + makeLogo + `" alt="">`)
	}
	b.WriteString(`<img class="ct-logo" src="` + logo + `" alt="Olex">` + car + `</section>`)

	b.WriteString(`<section class="ct-page ct-summary"><img class="ct-logo" src="` + logo + `" alt="Olex">` + car)
	b.WriteString(`<img class="ct-map" src="` + CarMapDataURI(in.Parts) + `" alt="">`)
	if len(in.Services) > 0 {
		check := pdfrender.ImageDataURI("image/png", asset("check.png"))
		b.WriteString(`<div class="ct-services"><table><thead><tr><td colspan="3"><div class="ct-head"><img class="ct-head-logo" src="` +
			pdfrender.ImageDataURI("image/png", asset("olex-table-logo.png")) + `" alt="Olex"><span>` +
			esc(t("certtheme.applied_services")) + `</span></div></td></tr></thead><tbody>`)
		for _, s := range in.Services {
			warranty := s.Warranty
			if strings.TrimSpace(warranty) == "" {
				warranty = "-"
			}
			b.WriteString(`<tr><td><span class="ct-label">` + esc(t("certtheme.category")) + `</span><span class="ct-value">` +
				esc(s.Category) + `</span></td><td><span class="ct-label">` + esc(t("warranty.certificate.product")))
			if s.Code != "" {
				b.WriteString(` <span class="ct-code" dir="ltr">#` + esc(s.Code) + `</span>`)
			}
			b.WriteString(`</span><span class="ct-value">` + esc(s.Product) + `</span></td><td><span class="ct-label">` +
				esc(t("certtheme.warranty")) + `</span><span class="ct-value">` + esc(warranty) +
				`<img class="ct-check" src="` + check + `" alt=""></span></td></tr>`)
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func carHTML(v Vehicle, makeLogo string) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<div class="ct-car"><div class="ct-make">`)
	if makeLogo != "" {
		b.WriteString(`<img class="ct-make-logo" src="` + makeLogo + `" alt="">`)
	}
	b.WriteString(`<span>` + esc(v.Make) + `</span></div>`)
	model := strings.TrimSpace(v.Model)
	if y := strings.TrimSpace(v.Year); y != "" {
		model = strings.TrimSpace(model + " (" + y + ")")
	}
	if model != "" {
		b.WriteString(`<span class="ct-model">` + esc(model) + `</span>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// ClosingPages are the static care guide and certification pages in the
// document language (English when the language has none).
func ClosingPages(loc i18n.Locale) string {
	code, ok := pageLocales[loc]
	if !ok {
		code = "en"
	}
	var b strings.Builder
	for _, name := range []string{"pages/page4-" + code + ".jpg", "pages/page5-" + code + ".jpg", "pages/page6.jpg"} {
		b.WriteString(`<section class="ct-page ct-image"><img src="` + pdfrender.ImageDataURI("image/jpeg", asset(name)) + `" alt=""></section>`)
	}
	return b.String()
}

var partRE = regexp.MustCompile(`^(body|window)_[a-z_]+$`)

// CarMapDataURI is the legacy car outline with the applied parts filled
// (body parts green, windows blue) as an SVG data URI. Unknown keys are
// ignored; the SVG is a static asset, never user input.
func CarMapDataURI(parts []string) string {
	svg := string(asset("car.svg"))
	for _, p := range parts {
		if !partRE.MatchString(p) {
			continue
		}
		color := bodyFill
		if strings.HasPrefix(p, "window_") {
			color = windowFill
		}
		re := regexp.MustCompile(`(<path[^>]*\sid="` + p + `"[^>]*\sfill=")[^"]*(")`)
		svg = re.ReplaceAllString(svg, `${1}`+color+`${2}`)
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func asset(name string) []byte {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		panic("certtheme: missing asset " + name)
	}
	return b
}
