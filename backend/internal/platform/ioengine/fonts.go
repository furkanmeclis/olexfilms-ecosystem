package ioengine

import (
	"embed"
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"
)

//go:embed fonts/DejaVuSansCondensed.ttf fonts/DejaVuSansCondensed-Bold.ttf fonts/DejaVuSansCondensed-Oblique.ttf
var dejaVuFonts embed.FS

const pdfFontFamily = "DejaVu"

func registerPDFFonts(pdf *gofpdf.Fpdf) error {
	specs := []struct {
		style string
		file  string
	}{
		{"", "fonts/DejaVuSansCondensed.ttf"},
		{"B", "fonts/DejaVuSansCondensed-Bold.ttf"},
		{"I", "fonts/DejaVuSansCondensed-Oblique.ttf"},
	}
	for _, spec := range specs {
		data, err := dejaVuFonts.ReadFile(spec.file)
		if err != nil {
			return fmt.Errorf("pdf font %s: %w", spec.file, err)
		}
		// gofpdf mutates font bytes; copy before registering (see gofpdf#316).
		cpy := append([]byte(nil), data...)
		pdf.AddUTF8FontFromBytes(pdfFontFamily, spec.style, cpy)
	}
	return nil
}

func setPDFFont(pdf *gofpdf.Fpdf, style string, size float64) {
	pdf.SetFont(pdfFontFamily, style, size)
}
