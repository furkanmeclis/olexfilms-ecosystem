package labels

// ZPL output (TEC-1230): Zebra ZPL II for thermal printers, 203 dpi
// (8 dots/mm). One ^XA..^XZ block per label; the layout follows the HTML
// label: logo text, barcode or QR, code text, name and subtitle, centered.
// An image logo is not converted to ZPL graphics (^GF) and is left out.

import (
	"fmt"
	"strings"
)

// ZPLDotsPerMm is the printer resolution the ZPL output targets (203 dpi).
const ZPLDotsPerMm = 8

// ZPLContentType is the media type of the ZPL download.
const ZPLContentType = "application/vnd.zebra.zpl; charset=utf-8"

// IsZPL reports whether a rendered label document is ZPL (not PDF).
func IsZPL(data []byte) bool {
	return len(data) >= 3 && string(data[:3]) == "^XA"
}

// ZPL renders the labels as ZPL II text. An item without a code is refused.
func ZPL(t Template, items []Item) ([]byte, error) {
	t = normalize(t)
	w := int(t.WidthMm * ZPLDotsPerMm)
	h := int(t.HeightMm * ZPLDotsPerMm)
	const margin = 16
	inner := w - 2*margin
	var b strings.Builder
	for _, it := range items {
		code := strings.TrimSpace(it.Code)
		if code == "" {
			return nil, ErrEmptyCode
		}
		// Text sizes scale with the label height (37 mm → ~22 dots).
		font := max(18, min(36, h/14))
		small := max(16, font-4)
		// Reserve the text lines, the rest goes to the symbol.
		reserved := 0
		if t.LogoMode == LogoText && t.LogoText != "" {
			reserved += font + 6
		}
		if t.ShowCodeText {
			reserved += font + 6
		}
		if t.ShowName && it.Title != "" {
			reserved += 2*font + 6
		}
		if t.ShowName && it.Subtitle != "" {
			reserved += small + 6
		}
		symbol := max(40, h-2*margin-reserved)

		b.WriteString("^XA^CI28")
		fmt.Fprintf(&b, "^PW%d^LL%d^LH0,0", w, h)
		y := margin
		if t.LogoMode == LogoText && t.LogoText != "" {
			fmt.Fprintf(&b, "^FO%d,%d^A0N,%d,%d^FB%d,1,0,C^FH^FD%s^FS", margin, y, font, font, inner, zplField(t.LogoText))
			y += font + 6
		}
		if t.Kind == KindLocation || t.Symbology == SymbologyQR {
			payload := qrPayload(t.Kind, code)
			// A short payload fits a version 2-4 symbol (25-33 modules).
			modules := 33
			if len(payload) > 50 {
				modules = 45
			}
			mag := max(1, min(10, min(symbol, inner)/modules))
			x := max(margin, (w-modules*mag)/2)
			// ^BQ adds about 10 dots above the symbol.
			fmt.Fprintf(&b, "^FO%d,%d^BQN,2,%d^FDQA,%s^FS", x, max(0, y-10), mag, zplData(payload))
			y += modules*mag + 6
		} else {
			module := 2
			if code128Width(code, 2) > inner {
				module = 1
			}
			bw := code128Width(code, module)
			x := max(margin, (w-bw)/2)
			fmt.Fprintf(&b, "^FO%d,%d^BY%d^BCN,%d,N,N,N^FD%s^FS", x, y, module, symbol, zplData(code))
			y += symbol + 6
		}
		if t.ShowCodeText {
			fmt.Fprintf(&b, "^FO%d,%d^A0N,%d,%d^FB%d,1,0,C^FH^FD%s^FS", margin, y, font, font, inner, zplField(code))
			y += font + 6
		}
		if t.ShowName && it.Title != "" {
			fmt.Fprintf(&b, "^FO%d,%d^A0N,%d,%d^FB%d,2,0,C^FH^FD%s^FS", margin, y, font, font, inner, zplField(it.Title))
			y += 2*font + 6
		}
		if t.ShowName && it.Subtitle != "" {
			fmt.Fprintf(&b, "^FO%d,%d^A0N,%d,%d^FB%d,1,0,C^FH^FD%s^FS", margin, y, small, small, inner, zplField(it.Subtitle))
		}
		b.WriteString("^XZ\n")
	}
	return []byte(b.String()), nil
}

// code128Width estimates the code128 symbol width in dots: start, data,
// checksum (11 modules each), stop (13) and the 10-module quiet zones.
func code128Width(code string, module int) int {
	return (11*(len(code)+2) + 13 + 20) * module
}

// zplField escapes text for a ^FH field: the ZPL control characters and the
// hex escape character become _XX sequences.
func zplField(s string) string {
	r := strings.NewReplacer("_", "_5F", "^", "_5E", "~", "_7E", "\n", " ", "\r", " ")
	return r.Replace(s)
}

// zplData strips the control characters from barcode data (barcodes and
// payloads never carry them; ^FH is not used for symbols).
func zplData(s string) string {
	r := strings.NewReplacer("^", "", "~", "", "\n", "", "\r", "")
	return r.Replace(s)
}
