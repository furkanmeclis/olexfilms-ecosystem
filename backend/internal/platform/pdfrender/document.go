package pdfrender

import (
	"html"
	"regexp"
	"strings"
)

// CSP is the Content-Security-Policy of every rendered document: no network,
// no scripts; images and fonts only as data: URIs, inline styles allowed.
// Gotenberg runs Chromium with JavaScript disabled as a second layer.
const CSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:"

// Document is the skeleton around a template body.
type Document struct {
	// Lang is the document language (BCP 47 / app locale, e.g. tr, en, ar).
	Lang  string
	Title string
	// Body is the filled, sanitized template HTML.
	Body string
	// PrimaryColor (#RRGGBB) is exposed to templates as var(--primary).
	PrimaryColor string
	Fonts        FontMode
}

var rtlLanguages = map[string]bool{"ar": true, "fa": true, "he": true, "ur": true}

// IsRTL reports whether a locale is written right to left.
func IsRTL(locale string) bool {
	l := strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	return rtlLanguages[l]
}

// Dir returns "rtl" or "ltr" for a locale.
func Dir(locale string) string {
	if IsRTL(locale) {
		return "rtl"
	}
	return "ltr"
}

var colorRE = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

// HTML assembles the full document: lang/dir, CSP meta, A4 print styles
// (logical properties, so RTL mirrors without extra rules) and fonts.
func (d Document) HTML() string {
	lang := strings.ReplaceAll(strings.TrimSpace(d.Lang), "_", "-")
	if lang == "" {
		lang = "en"
	}
	rtl := IsRTL(lang)
	color := "#0F172A"
	if colorRE.MatchString(strings.TrimSpace(d.PrimaryColor)) {
		color = strings.TrimSpace(d.PrimaryColor)
	}
	var b strings.Builder
	b.Grow(len(d.Body) + 4096)
	b.WriteString("<!DOCTYPE html>\n<html lang=\"")
	b.WriteString(html.EscapeString(lang))
	b.WriteString("\" dir=\"")
	b.WriteString(Dir(lang))
	b.WriteString("\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta http-equiv=\"Content-Security-Policy\" content=\"")
	b.WriteString(CSP)
	b.WriteString("\">\n<title>")
	b.WriteString(html.EscapeString(d.Title))
	b.WriteString("</title>\n<style>\n")
	b.WriteString(fontCSS(d.Fonts, rtl, d.Body))
	b.WriteString(":root{--primary:")
	b.WriteString(color)
	b.WriteString("}\n")
	b.WriteString(baseCSS)
	b.WriteString("</style>\n</head>\n<body>\n")
	b.WriteString(d.Body)
	b.WriteString("\n</body>\n</html>\n")
	return b.String()
}

// FooterHTML builds a Gotenberg footer with an optional text and page
// numbers. Footers render in a separate context, so they carry their own
// minimal style (system fonts only).
func FooterHTML(locale, text string) string {
	lang := html.EscapeString(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	return "<!DOCTYPE html><html lang=\"" + lang + "\" dir=\"" + Dir(locale) + "\"><head><meta charset=\"utf-8\">" +
		"<style>html,body{margin:0;width:100%}body{font-family:'Noto Sans','Noto Sans Arabic',sans-serif;font-size:7pt;color:#6b7280}" +
		"table{width:100%;border-collapse:collapse;padding:0 0.4in;box-sizing:border-box}td{padding:0 0.4in}.n{text-align:end}</style></head><body>" +
		"<table><tr><td>" + html.EscapeString(text) + "</td>" +
		"<td class=\"n\"><span class=\"pageNumber\"></span> / <span class=\"totalPages\"></span></td></tr></table></body></html>"
}

const baseCSS = `@page{size:A4}
html{-webkit-print-color-adjust:exact;print-color-adjust:exact}
body{margin:0;font-family:"Noto Sans","Noto Sans Arabic","Noto Sans CJK SC","DejaVu Sans",sans-serif;font-size:10.5pt;line-height:1.45;color:#111827;text-align:start}
h1,h2,h3{line-height:1.25;color:var(--primary)}
h1.doc-title{font-size:18pt;margin:14pt 0 8pt}
h2{font-size:12pt;margin:14pt 0 6pt}
p{margin:0 0 6pt}
img{max-width:100%}
.doc-header{display:flex;justify-content:space-between;align-items:flex-start;gap:16pt;border-block-end:2px solid var(--primary);padding-block-end:8pt}
.doc-logo img{max-height:48pt;max-width:160pt}
.doc-company{text-align:end;font-size:9pt}
table{border-collapse:collapse;width:100%;margin-block-end:8pt}
thead{display:table-header-group}
tr{break-inside:avoid}
table.doc-meta th,table.doc-meta td,table.doc-table th,table.doc-table td{border:1px solid #d1d5db;padding:4pt 6pt;text-align:start;vertical-align:top}
table.doc-meta th,table.doc-table th{background:#f3f4f6;font-weight:700}
table.doc-table td.num,table.doc-table th.num{text-align:end;white-space:nowrap}
table.doc-totals{width:auto;margin-inline-start:auto}
table.doc-totals th,table.doc-totals td{padding:2pt 6pt;text-align:end}
table.doc-totals tr:last-child{font-weight:700;border-block-start:1px solid #6b7280}
.doc-total{text-align:end;font-weight:700;font-size:12pt}
.doc-sign{display:flex;justify-content:space-between;gap:24pt;margin-block-start:36pt;break-inside:avoid}
.doc-sign>div{flex:1;border-block-start:1px solid #6b7280;padding-block-start:4pt;text-align:center;font-size:9pt}
.doc-sign img{max-height:48pt}
.doc-verify{display:flex;align-items:center;gap:12pt;margin-block-start:12pt}
.doc-verify img,.doc-verify svg{width:80pt;height:80pt}
.doc-footer{margin-block-start:24pt;font-size:8pt;color:#6b7280;text-align:center}
.muted{color:#6b7280}
.doc-photos{display:grid;grid-template-columns:repeat(3,1fr);gap:8pt;margin-block-end:8pt}
.doc-photos figure{margin:0;break-inside:avoid;border:1px solid #d1d5db;padding:4pt}
.doc-photos figcaption{font-size:8.5pt;margin-block-end:3pt}
.doc-photos img{width:100%;height:auto}
`
