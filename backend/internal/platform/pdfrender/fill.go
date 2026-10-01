package pdfrender

import (
	"encoding/base64"
	"html"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// Fill substitutes {{key}} placeholders. Every value is HTML-escaped (line
// breaks become <br>) except keys listed in rawHTML: those are HTML blocks
// built by the server (tables, logo, QR) with the helpers below, which escape
// their own cells. Unknown placeholders render empty, as in msgtemplate.
func Fill(tpl string, vars map[string]string, rawHTML map[string]bool) string {
	esc := make(map[string]string, len(vars))
	for k, v := range vars {
		key := strings.ToLower(k)
		if rawHTML[key] {
			esc[key] = v
			continue
		}
		esc[key] = EscapeText(v)
	}
	return msgtemplate.Render(tpl, esc)
}

// EscapeText HTML-escapes plain text and keeps line breaks.
func EscapeText(s string) string {
	s = html.EscapeString(s)
	if strings.Contains(s, "\n") {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "<br>")
	}
	return s
}

// Column is one table column.
type Column struct {
	Label   string
	Numeric bool
}

// Table builds an escaped <table class="doc-table"> block.
func Table(cols []Column, rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<table class="doc-table"><thead><tr>`)
	for _, c := range cols {
		if c.Numeric {
			b.WriteString(`<th class="num">`)
		} else {
			b.WriteString(`<th>`)
		}
		b.WriteString(html.EscapeString(c.Label))
		b.WriteString(`</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, row := range rows {
		b.WriteString(`<tr>`)
		for i, cell := range row {
			if i < len(cols) && cols[i].Numeric {
				b.WriteString(`<td class="num">`)
			} else {
				b.WriteString(`<td>`)
			}
			b.WriteString(EscapeText(cell))
			b.WriteString(`</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

// ImageDataURI encodes image bytes as a data: URI (the only image source
// the document CSP allows). Non-image MIME types return "".
func ImageDataURI(mime string, data []byte) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if len(data) == 0 || !strings.HasPrefix(mime, "image/") || strings.ContainsAny(mime, "\"'<> ;") {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// ImageTag builds an <img> block from bytes ("" when there is no image).
func ImageTag(mime string, data []byte, alt string) string {
	uri := ImageDataURI(mime, data)
	if uri == "" {
		return ""
	}
	return `<img src="` + uri + `" alt="` + html.EscapeString(alt) + `">`
}
