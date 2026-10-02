package ioengine

import (
	"html"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// LetterheadHeaderHTML is the document header of styled export PDFs
// (pdfrender skeleton classes doc-header / doc-logo / doc-company): the logo
// and the company name with tagline, address, phone · e-mail and website.
// Every value is escaped; nil returns "".
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

// LetterheadFooterHTML is the footer paragraph with the letterhead footer
// text ("" when there is none).
func LetterheadFooterHTML(lh *Letterhead) string {
	if lh == nil || strings.TrimSpace(lh.FooterText) == "" {
		return ""
	}
	return `<p class="doc-footer">` + html.EscapeString(lh.FooterText) + `</p>`
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
