package ioengine

import (
	"html"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// LetterheadHeaderHTML is the document header of styled export PDFs
// (pdfrender skeleton classes doc-header / doc-logo / doc-company): the
// legacy warehouse letterhead bar (TEC-772) with the logo on a white plate
// and the company name, tagline, address, phone · e-mail and website. Every
// value is escaped; nil returns "".
func LetterheadHeaderHTML(lh *Letterhead) string {
	if lh == nil {
		return ""
	}
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<header class="doc-header"><div class="doc-logo">`)
	b.WriteString(pdfrender.ImageTag(lh.LogoMIME, lh.LogoBytes, lh.CompanyName))
	b.WriteString(`</div><div class="doc-company"><strong>`)
	b.WriteString(esc(lh.CompanyName))
	b.WriteString(`</strong>`)
	for _, line := range []string{lh.Tagline, lh.Address, JoinNonEmpty(" · ", lh.Phone, lh.Email), lh.Website} {
		if strings.TrimSpace(line) != "" {
			b.WriteString(`<br>`)
			b.WriteString(esc(line))
		}
	}
	b.WriteString(`</div></header>`)
	return b.String()
}

// LetterheadFooterHTML is LetterheadFooterHTMLFor in Turkish, the language
// of the legacy letterhead.
func LetterheadFooterHTML(lh *Letterhead) string {
	return LetterheadFooterHTMLFor(lh, string(i18n.LocaleTR))
}

// LetterheadFooterHTMLFor is the closing letterhead block of the legacy
// documents (TEC-772): the footer text, address · phone · e-mail and the
// tax office / tax number (labels in the document locale) on the start
// side, the website on the end side. It returns "" when the letterhead has
// none of them.
func LetterheadFooterHTMLFor(lh *Letterhead, locale string) string {
	if lh == nil {
		return ""
	}
	loc := i18n.Normalize(locale)
	tax := JoinNonEmpty(" · ",
		labeled(i18n.Translate(loc, "letterhead.tax_office"), lh.TaxOffice),
		labeled(i18n.Translate(loc, "letterhead.tax_number"), lh.TaxNumber))
	var start []string
	for _, line := range []string{lh.FooterText, JoinNonEmpty(" · ", lh.Address, lh.Phone, lh.Email), tax} {
		if s := strings.TrimSpace(line); s != "" {
			start = append(start, html.EscapeString(s))
		}
	}
	website := strings.TrimSpace(lh.Website)
	if len(start) == 0 && website == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<footer class="doc-footer"><div>`)
	b.WriteString(strings.Join(start, `<br>`))
	b.WriteString(`</div>`)
	if website != "" {
		b.WriteString(`<div class="doc-footer-end">`)
		b.WriteString(html.EscapeString(website))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</footer>`)
	return b.String()
}

// labeled is "label: value", or "" for an empty value.
func labeled(label, value string) string {
	if value = strings.TrimSpace(value); value == "" {
		return ""
	}
	return label + ": " + value
}

// JoinNonEmpty joins the trimmed non-empty parts with sep.
func JoinNonEmpty(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}
