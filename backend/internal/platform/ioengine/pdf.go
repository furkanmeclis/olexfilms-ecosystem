package ioengine

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// ErrPDFRendererRequired is returned when a PDF export is encoded without a
// Gotenberg client: table PDFs have a single engine (design §6, TEC-139).
var ErrPDFRendererRequired = errors.New("ioengine: pdf export needs the gotenberg renderer")

// PDFConverter renders one HTML request to PDF (satisfied by
// *pdfrender.Client).
type PDFConverter interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// pdfLandscapeColumns is the column count above which the table is laid out
// on landscape A4.
const pdfLandscapeColumns = 7

// EncodePDF renders a letterheaded table through Gotenberg (TEC-139). The
// HTML skeleton carries lang/dir (rtl for ar) and the Noto fonts, so Arabic
// shaping, bidi and CJK glyphs come from Chromium.
func EncodePDF(
	ctx context.Context,
	conv PDFConverter,
	ds Dataset,
	locale string,
	lh *Letterhead,
	title string,
) ([]byte, error) {
	if conv == nil {
		return nil, ErrPDFRendererRequired
	}
	return conv.Convert(ctx, ExportPDFRequest(ds, locale, lh, title))
}

// ExportPDFRequest builds the Gotenberg request of a table export: the
// document, a footer with the letterhead footer text and page numbers, and
// landscape orientation for wide tables.
func ExportPDFRequest(ds Dataset, locale string, lh *Letterhead, title string) pdfrender.Request {
	footer := ""
	if lh != nil {
		footer = strings.TrimSpace(lh.FooterText)
	}
	return pdfrender.Request{
		HTML:       ExportTableHTML(ds, locale, lh, title),
		FooterHTML: pdfrender.FooterHTML(string(i18n.Normalize(locale)), footer),
		Landscape:  len(ds.Columns) > pdfLandscapeColumns,
	}
}

// ExportTableHTML lays out a dataset as an HTML document on the pdfrender
// skeleton: letterhead header, title, export timestamp, info lines, the
// data table (column widths from the column weights, numeric columns end
// aligned) and the totals row. Every value is escaped.
func ExportTableHTML(ds Dataset, locale string, lh *Letterhead, title string) string {
	loc := i18n.Normalize(locale)
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(LetterheadHeaderHTML(lh))
	b.WriteString(`<h1 class="doc-title">`)
	b.WriteString(esc(title))
	b.WriteString(`</h1><p class="muted">`)
	b.WriteString(esc(ExportedAtLabel(locale)))
	b.WriteString(`</p>`)
	if info := exportInfoHTML(ds.Info, loc); info != "" {
		b.WriteString(info)
	}
	b.WriteString(`<table class="doc-table export-table"><colgroup>`)
	for _, w := range pdfColumnWidths(ds.Columns, 100) {
		fmt.Fprintf(&b, `<col style="width:%.2f%%">`, w)
	}
	b.WriteString(`</colgroup><thead><tr>`)
	for _, c := range ds.Columns {
		b.WriteString(exportCellOpen("th", c))
		b.WriteString(esc(i18n.Translate(loc, c.LabelKey)))
		b.WriteString(`</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, row := range ds.Rows {
		writeExportRow(&b, ds.Columns, row, loc)
	}
	b.WriteString(`</tbody>`)
	if len(ds.Totals) > 0 {
		b.WriteString(`<tfoot>`)
		writeExportRow(&b, ds.Columns, ds.Totals, loc)
		b.WriteString(`</tfoot>`)
	}
	b.WriteString(`</table>`)
	b.WriteString(LetterheadFooterHTML(lh))
	color := ""
	if lh != nil {
		color = lh.PrimaryColor
	}
	body := `<style>` + exportTableCSS + `</style>` + b.String()
	return pdfrender.Document{Lang: string(loc), Title: title, Body: body, PrimaryColor: color}.HTML()
}

// exportTableCSS narrows the skeleton table for dense lists: header in the
// primary color, smaller text, long values wrap instead of being cut.
const exportTableCSS = `table.export-table{table-layout:fixed;font-size:8pt}
table.export-table th,table.export-table td{padding:2pt 4pt;overflow-wrap:anywhere}
table.export-table thead th{background:var(--primary);color:#fff}
table.export-table tfoot td{background:#f1f5f9;font-weight:700}`

func writeExportRow(b *strings.Builder, columns []Column, row map[string]any, loc i18n.Locale) {
	b.WriteString(`<tr>`)
	for _, c := range columns {
		b.WriteString(exportCellOpen("td", c))
		b.WriteString(pdfrender.EscapeText(formatCell(row[c.Key], c, loc)))
		b.WriteString(`</td>`)
	}
	b.WriteString(`</tr>`)
}

func exportCellOpen(tag string, c Column) string {
	if c.AlignRight {
		return `<` + tag + ` class="num">`
	}
	return `<` + tag + `>`
}

func exportInfoHTML(lines []InfoLine, loc i18n.Locale) string {
	var b strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		b.WriteString(`<tr><th>`)
		b.WriteString(html.EscapeString(i18n.Translate(loc, line.LabelKey)))
		b.WriteString(`</th><td>`)
		b.WriteString(html.EscapeString(line.Value))
		b.WriteString(`</td></tr>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<table class="doc-meta">` + b.String() + `</table>`
}
