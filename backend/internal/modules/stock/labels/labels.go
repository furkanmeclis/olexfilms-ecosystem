// Package labels renders barcode and QR label sheets as HTML for the single
// PDF engine (pdfrender / Gotenberg, TEC-202). A sheet is an A4 grid of
// fixed-size labels; the template decides the symbology (code128 or QR),
// the logo mode, the label size and the number of columns.
//
// Payloads the universal scanner (TEC-203) resolves:
//
//   - unit label, code128: the barcode itself;
//   - unit label, QR: OFW:UNIT:<barcode>;
//   - location label (always QR): OFW:LOC:<full_code>.
package labels

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/boombuler/barcode/code128"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// Template kinds and symbologies (label_templates CHECKs).
const (
	KindUnit     = "unit"
	KindLocation = "location"

	SymbologyCode128 = "code128"
	SymbologyQR      = "qr"

	LogoNone  = "none"
	LogoText  = "text"
	LogoImage = "image"
)

// Scanner payload prefixes.
const (
	LocationPrefix = "OFW:LOC:"
	UnitPrefix     = "OFW:UNIT:"
)

// ErrEmptyCode is returned for a label without a code.
var ErrEmptyCode = errors.New("labels: empty code")

// Template is the print layout of a label sheet.
type Template struct {
	Kind      string
	Symbology string
	LogoMode  string
	LogoText  string
	// LogoImage is a data:image/... URI (validated at save time).
	LogoImage    string
	WidthMm      float64
	HeightMm     float64
	Columns      int
	ShowName     bool
	ShowCodeText bool
}

// DefaultUnit is the built-in unit template (code128, 70x37 mm, 3 columns).
var DefaultUnit = Template{
	Kind: KindUnit, Symbology: SymbologyCode128, LogoMode: LogoNone,
	WidthMm: 70, HeightMm: 37, Columns: 3, ShowName: true, ShowCodeText: true,
}

// DefaultLocation is the built-in location template (QR, 50x50 mm, 4
// columns).
var DefaultLocation = Template{
	Kind: KindLocation, Symbology: SymbologyQR, LogoMode: LogoNone,
	WidthMm: 50, HeightMm: 50, Columns: 4, ShowName: true, ShowCodeText: true,
}

// Item is one label: the code to encode and the lines printed with it.
type Item struct {
	// Code is the barcode (unit) or the full_code (location).
	Code string
	// Title is the product name or the location name.
	Title string
	// Subtitle is the SKU and meters, or the warehouse / room path.
	Subtitle string
}

// Payload is the scanner payload of a label.
func Payload(kind, code string) string {
	switch kind {
	case KindLocation:
		return LocationPrefix + code
	default:
		return code
	}
}

// qrPayload is what the QR code of a label encodes.
func qrPayload(kind, code string) string {
	if kind == KindLocation {
		return LocationPrefix + code
	}
	return UnitPrefix + code
}

// Code128PNG encodes content as a code128 PNG with a quiet zone, 3 px per
// module and the given height.
func Code128PNG(content string, height int) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyCode
	}
	bc, err := code128.Encode(content)
	if err != nil {
		return nil, fmt.Errorf("labels: code128: %w", err)
	}
	if height <= 0 {
		height = 80
	}
	const px, quiet = 3, 10
	n := bc.Bounds().Dx()
	width := n*px + 2*quiet*px
	img := image.NewGray(image.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	b := bc.Bounds()
	for x := 0; x < n; x++ {
		if r, _, _, _ := bc.At(b.Min.X+x, b.Min.Y).RGBA(); r >= 0x8000 {
			continue
		}
		for dx := 0; dx < px; dx++ {
			for y := 0; y < height; y++ {
				img.SetGray(quiet*px+x*px+dx, y, color.Gray{Y: 0})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Sheet renders the label grid as a full HTML document (pdfrender.Document)
// for the given language. An item without a code is refused.
func Sheet(lang string, t Template, items []Item) (string, error) {
	t = normalize(t)
	var b strings.Builder
	fmt.Fprintf(&b, `<style>
.sheet{display:grid;grid-template-columns:repeat(%d,%.1fmm);gap:2mm;justify-content:start}
.label{width:%.1fmm;height:%.1fmm;box-sizing:border-box;border:0.2mm dashed #cbd5e1;padding:1.5mm;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:0.8mm;overflow:hidden;break-inside:avoid;text-align:center}
.label .logo{font-weight:700;font-size:7pt;color:var(--primary);max-height:6mm;overflow:hidden}
.label .logo img{max-height:6mm;max-width:%.1fmm}
.label img.code{max-width:100%%;height:auto}
.label img.qr{width:%.1fmm;height:%.1fmm}
.label .text{font-family:"DejaVu Sans Mono",monospace;font-size:7.5pt;letter-spacing:0.3pt;word-break:break-all}
.label .title{font-size:7pt;font-weight:700;line-height:1.2;max-height:8mm;overflow:hidden}
.label .sub{font-size:6pt;color:#475569;line-height:1.2}
</style>
<div class="sheet">`, t.Columns, t.WidthMm, t.WidthMm, t.HeightMm, t.WidthMm-4, qrSide(t), qrSide(t))
	for _, it := range items {
		code := strings.TrimSpace(it.Code)
		if code == "" {
			return "", ErrEmptyCode
		}
		b.WriteString(`<div class="label">`)
		switch t.LogoMode {
		case LogoText:
			b.WriteString(`<div class="logo">` + html.EscapeString(t.LogoText) + `</div>`)
		case LogoImage:
			if strings.HasPrefix(t.LogoImage, "data:image/") {
				b.WriteString(`<div class="logo"><img src="` + html.EscapeString(t.LogoImage) + `" alt=""></div>`)
			}
		}
		switch {
		case t.Kind == KindLocation || t.Symbology == SymbologyQR:
			data, err := pdfrender.QRCodePNG(qrPayload(t.Kind, code), pdfrender.DefaultQRSize)
			if err != nil {
				return "", fmt.Errorf("labels: qr %s: %w", code, err)
			}
			b.WriteString(`<img class="qr" src="` + pdfrender.ImageDataURI("image/png", data) + `" alt="">`)
		default:
			data, err := Code128PNG(code, 80)
			if err != nil {
				return "", err
			}
			b.WriteString(`<img class="code" src="` + pdfrender.ImageDataURI("image/png", data) + `" alt="">`)
		}
		if t.ShowCodeText {
			b.WriteString(`<div class="text">` + html.EscapeString(code) + `</div>`)
		}
		if t.ShowName && it.Title != "" {
			b.WriteString(`<div class="title">` + html.EscapeString(it.Title) + `</div>`)
		}
		if t.ShowName && it.Subtitle != "" {
			b.WriteString(`<div class="sub">` + html.EscapeString(it.Subtitle) + `</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return pdfrender.Document{Lang: lang, Title: "Labels", Body: b.String(), Fonts: pdfrender.FontsEmbedded}.HTML(), nil
}

// qrSide is the QR edge in mm: the smaller label side minus the paddings
// and the text lines.
func qrSide(t Template) float64 {
	side := min(t.WidthMm, t.HeightMm) - 4
	if t.ShowCodeText {
		side -= 4
	}
	if t.ShowName {
		side -= 6
	}
	if t.LogoMode != LogoNone {
		side -= 6
	}
	return max(side, 10)
}

func normalize(t Template) Template {
	if t.Kind == "" {
		t.Kind = KindUnit
	}
	if t.Symbology == "" {
		t.Symbology = SymbologyCode128
	}
	if t.LogoMode == "" {
		t.LogoMode = LogoNone
	}
	if t.WidthMm <= 0 || t.HeightMm <= 0 {
		if t.Kind == KindLocation {
			t.WidthMm, t.HeightMm = DefaultLocation.WidthMm, DefaultLocation.HeightMm
		} else {
			t.WidthMm, t.HeightMm = DefaultUnit.WidthMm, DefaultUnit.HeightMm
		}
	}
	if t.Columns <= 0 {
		t.Columns = 3
	}
	return t
}
