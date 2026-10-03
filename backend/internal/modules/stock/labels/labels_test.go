package labels

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestCode128PNG(t *testing.T) {
	data, err := Code128PNG("OLEX-00000123", 60)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dy() != 60 || img.Bounds().Dx() < 100 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	if _, err := Code128PNG("  ", 60); err != ErrEmptyCode {
		t.Fatalf("empty: %v", err)
	}
}

func TestSheetUnitCode128(t *testing.T) {
	doc, err := Sheet("tr", DefaultUnit, []Item{
		{Code: "OLEX-00000001", Title: "Olex PPF <Gloss>", Subtitle: "SKU-1 · 15.00 m"},
		{Code: "OLEX-00000002", Title: "Olex PPF", Subtitle: "SKU-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`lang="tr"`, `grid-template-columns:repeat(3,70.0mm)`,
		`<img class="code" src="data:image/png;base64,`,
		`<div class="text">OLEX-00000001</div>`, `<div class="text">OLEX-00000002</div>`,
		`Olex PPF &lt;Gloss&gt;`, `SKU-1 · 15.00 m`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if strings.Count(doc, `<div class="label">`) != 2 {
		t.Fatalf("label count = %d", strings.Count(doc, `<div class="label">`))
	}
	if strings.Contains(doc, "OFW:") {
		t.Fatal("code128 unit labels carry no OFW payload text")
	}
}

func TestSheetLocationQRAndLogo(t *testing.T) {
	tpl := DefaultLocation
	tpl.LogoMode = LogoText
	tpl.LogoText = "Olex & Co"
	doc, err := Sheet("ar", tpl, []Item{{Code: "IST-R1-A-01-03", Title: "Bin 03", Subtitle: "IST / R1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`dir="rtl"`, `<img class="qr"`, `<div class="logo">Olex &amp; Co</div>`, `IST-R1-A-01-03`} {
		if !strings.Contains(doc, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if Payload(KindLocation, "IST-R1-A-01-03") != "OFW:LOC:IST-R1-A-01-03" || Payload(KindUnit, "X") != "X" {
		t.Fatal("payload")
	}
	if _, err := Sheet("en", DefaultUnit, []Item{{Code: ""}}); err != ErrEmptyCode {
		t.Fatalf("empty code: %v", err)
	}
}

func TestSheetUnitQRUsesUnitPrefix(t *testing.T) {
	tpl := DefaultUnit
	tpl.Symbology = SymbologyQR
	tpl.LogoMode = LogoImage
	tpl.LogoImage = "data:image/png;base64,iVBORw0KGgo="
	doc, err := Sheet("en", tpl, []Item{{Code: "OLEX-00000009"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, `<img class="qr"`) || !strings.Contains(doc, `<div class="logo"><img src="data:image/png;base64,iVBORw0KGgo="`) {
		t.Fatal("qr unit sheet")
	}
	if qrPayload(KindUnit, "A") != "OFW:UNIT:A" {
		t.Fatal("unit qr payload")
	}
}
