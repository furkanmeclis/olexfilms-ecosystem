package ioengine

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/jung-kurt/gofpdf/v2"
)

const pdfContentWidth = 190.0

// EncodePDF renders a letterheaded A4 table.
func EncodePDF(ds Dataset, locale string, lh *Letterhead, title string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 18)
	if err := registerPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.AddPage()
	loc := i18n.Normalize(locale)
	colWidths := pdfColumnWidths(ds.Columns, pdfContentWidth)

	y := 12.0
	if lh != nil {
		y = drawPDFLetterhead(pdf, lh, y)
	}
	setPDFFont(pdf, "B", 14)
	pdf.SetXY(10, y)
	pdf.Cell(pdfContentWidth, 8, title)
	y += 10
	setPDFFont(pdf, "", 9)
	pdf.SetXY(10, y)
	pdf.Cell(pdfContentWidth, 5, ExportedAtLabel(locale))
	y += 6
	if len(ds.Info) > 0 {
		y = drawPDFInfo(pdf, ds.Info, y, loc)
	}
	y += 2

	drawPDFHeaderRow(pdf, ds.Columns, colWidths, y, lh, loc)
	y += 6
	setPDFFont(pdf, "", 8)
	for _, row := range ds.Rows {
		if y > 270 {
			pdf.AddPage()
			y = 12
			drawPDFHeaderRow(pdf, ds.Columns, colWidths, y, lh, loc)
			y += 6
			setPDFFont(pdf, "", 8)
		}
		y = drawPDFDataRow(pdf, ds.Columns, colWidths, y, row, loc)
	}
	if len(ds.Totals) > 0 {
		if y > 270 {
			pdf.AddPage()
			y = 12
		}
		setPDFFont(pdf, "B", 8)
		pdf.SetFillColor(241, 245, 249)
		drawPDFRow(pdf, ds.Columns, colWidths, y, ds.Totals, loc, true)
		setPDFFont(pdf, "", 8)
	}
	if lh != nil && lh.FooterText != "" {
		pdf.SetY(-12)
		setPDFFont(pdf, "I", 8)
		pdf.Cell(pdfContentWidth, 5, lh.FooterText)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawPDFHeaderRow(
	pdf *gofpdf.Fpdf,
	columns []Column,
	widths []float64,
	y float64,
	lh *Letterhead,
	loc i18n.Locale,
) {
	pdf.SetFillColor(parseRGB(lh, true))
	pdf.SetTextColor(255, 255, 255)
	setPDFFont(pdf, "B", 8)
	x := 10.0
	for i, c := range columns {
		pdf.SetXY(x, y)
		label := i18n.Translate(loc, c.LabelKey)
		pdf.CellFormat(widths[i], 6, truncateRunes(label, 28), "1", 0, pdfAlign(c), true, 0, "")
		x += widths[i]
	}
	pdf.SetTextColor(0, 0, 0)
}

func drawPDFDataRow(
	pdf *gofpdf.Fpdf,
	columns []Column,
	widths []float64,
	y float64,
	row map[string]any,
	loc i18n.Locale,
) float64 {
	return drawPDFRow(pdf, columns, widths, y, row, loc, false)
}

func drawPDFRow(
	pdf *gofpdf.Fpdf,
	columns []Column,
	widths []float64,
	y float64,
	row map[string]any,
	loc i18n.Locale,
	fill bool,
) float64 {
	x := 10.0
	lineH := 5.0
	texts := make([]string, len(columns))
	maxH := lineH
	for i, c := range columns {
		texts[i] = formatPDFCell(row[c.Key], c, loc)
		if h := pdfMultiCellHeight(pdf, widths[i]-2, lineH, texts[i]); h > maxH {
			maxH = h
		}
	}
	style := "D"
	if fill {
		style = "FD"
	}
	for i, c := range columns {
		pdf.Rect(x, y, widths[i], maxH, style)
		pdf.SetXY(x+1, y+0.5)
		pdf.MultiCell(widths[i]-2, lineH, texts[i], "", pdfAlign(c), false)
		x += widths[i]
	}
	return y + maxH
}

func pdfAlign(c Column) string {
	if c.AlignRight {
		return "R"
	}
	return "L"
}

func drawPDFInfo(pdf *gofpdf.Fpdf, lines []InfoLine, y float64, loc i18n.Locale) float64 {
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		label := i18n.Translate(loc, line.LabelKey) + ": "
		pdf.SetXY(10, y)
		setPDFFont(pdf, "B", 9)
		w := pdf.GetStringWidth(label) + 1
		pdf.Cell(w, 5, label)
		setPDFFont(pdf, "", 9)
		pdf.Cell(pdfContentWidth-w, 5, line.Value)
		y += 5
	}
	return y
}

func pdfMultiCellHeight(pdf *gofpdf.Fpdf, w, lineH float64, txt string) float64 {
	if strings.TrimSpace(txt) == "" {
		return lineH
	}
	return float64(len(pdf.SplitText(txt, w))) * lineH
}

func drawPDFLetterhead(pdf *gofpdf.Fpdf, lh *Letterhead, y float64) float64 {
	if len(lh.LogoBytes) > 0 {
		opt := gofpdf.ImageOptions{ImageType: logoImageType(lh.LogoMIME), ReadDpi: true}
		pdf.RegisterImageOptionsReader("logo", opt, bytes.NewReader(lh.LogoBytes))
		pdf.ImageOptions("logo", 10, y, 18, 0, false, opt, 0, "")
	}
	x := 32.0
	if lh.CompanyName != "" {
		setPDFFont(pdf, "B", 12)
		pdf.SetXY(x, y)
		pdf.Cell(160, 6, lh.CompanyName)
		y += 6
	}
	if lh.Tagline != "" {
		setPDFFont(pdf, "", 9)
		pdf.SetXY(x, y)
		pdf.Cell(160, 5, lh.Tagline)
		y += 5
	}
	meta := strings.Join(filterNonEmpty([]string{lh.Address, lh.Phone, lh.Email}), " · ")
	if meta != "" {
		setPDFFont(pdf, "", 8)
		pdf.SetXY(x, y)
		pdf.Cell(160, 4, meta)
		y += 5
	}
	return y + 4
}

func logoImageType(mime string) string {
	switch mime {
	case "image/png":
		return "PNG"
	case "image/webp":
		return "WEBP"
	default:
		return "JPG"
	}
}

func parseRGB(lh *Letterhead, _ bool) (int, int, int) {
	c := "#0F172A"
	if lh != nil && lh.PrimaryColor != "" {
		c = lh.PrimaryColor
	}
	c = strings.TrimPrefix(strings.TrimSpace(c), "#")
	if len(c) != 6 {
		return 15, 23, 42
	}
	var r, g, b int
	_, _ = fmt.Sscanf(c, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}
