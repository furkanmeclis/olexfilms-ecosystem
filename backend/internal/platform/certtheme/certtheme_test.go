package certtheme

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestForBrand(t *testing.T) {
	for slug, want := range map[string]bool{"olex": true, " OLEX ": true, "glorian": false, "": false} {
		if got := ForBrand(slug); got != want {
			t.Errorf("ForBrand(%q) = %v", slug, got)
		}
	}
}

func decodeMap(t *testing.T, uri string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCarMapDataURIFillsAppliedParts(t *testing.T) {
	svg := decodeMap(t, CarMapDataURI([]string{"body_kaput", "window_on_cam", `body_x" onload="x`, "unknown"}))
	if !strings.Contains(svg, `id="body_kaput" fill="`+bodyFill+`"`) {
		t.Error("hood not filled green")
	}
	if !strings.Contains(svg, `id="window_on_cam" fill="`+windowFill+`"`) {
		t.Error("windscreen not filled blue")
	}
	if !strings.Contains(svg, `id="body_tavan" fill="#efefef"`) {
		t.Error("roof changed although not applied")
	}
	if strings.Contains(svg, "onload") {
		t.Error("part key reached the svg")
	}
}

func TestOpeningPages(t *testing.T) {
	in := Input{
		Vehicle:  Vehicle{Make: "BMW <M>", Model: "320i", Year: "2024", LogoMIME: "image/png", Logo: []byte{0x89, 'P', 'N', 'G'}},
		Parts:    []string{"body_kaput"},
		Services: []Service{{Category: "PPF", Code: "OLX-1", Product: "Ultimate <Gloss>", Warranty: "02.10.2036"}, {Product: "Cam filmi"}},
	}
	h := OpeningPages(in, i18n.LocaleTR)
	for _, want := range []string{`class="ct-page ct-cover"`, `class="ct-page ct-summary"`, `class="ct-watermark"`,
		"BMW &lt;M&gt;", "320i (2024)", "Ultimate &lt;Gloss&gt;", `<span class="ct-code" dir="ltr">#OLX-1</span>`,
		"Uygulanan Hizmetler", "Kategori", "02.10.2036", `<span class="ct-value">-<img`} {
		if !strings.Contains(h, want) {
			t.Errorf("opening pages miss %q", want)
		}
	}
	if strings.Contains(h, "<M>") || strings.Contains(h, "<Gloss>") {
		t.Error("unescaped value")
	}
	if ar := OpeningPages(in, i18n.LocaleAR); !strings.Contains(ar, "الخدمات المطبقة") {
		t.Error("ar labels missing")
	}
	// No services table and no make logo without data; QR-like plain
	// <img src= tags never come from the theme (export tests count QRs).
	bare := OpeningPages(Input{Vehicle: Vehicle{Make: "BMW"}}, i18n.LocaleEN)
	if strings.Contains(bare, "ct-services") || strings.Contains(bare, "ct-watermark") || strings.Contains(h+bare, `<img src="`) {
		t.Error("unexpected services table, watermark or unclassed image")
	}
}

func TestClosingPagesLocalized(t *testing.T) {
	tr := ClosingPages(i18n.LocaleTR)
	if n := strings.Count(tr, `class="ct-page ct-image"`); n != 3 {
		t.Fatalf("closing pages = %d", n)
	}
	page := func(name string) string {
		return base64.StdEncoding.EncodeToString(asset(name))[:64]
	}
	if !strings.Contains(tr, page("pages/page4-tr.jpg")) || !strings.Contains(tr, page("pages/page6.jpg")) {
		t.Error("tr pages not used")
	}
	// Arabic has no localized pages: English ones.
	if ar := ClosingPages(i18n.LocaleAR); !strings.Contains(ar, page("pages/page4-en.jpg")) {
		t.Error("ar does not fall back to the English pages")
	}
}

func TestCSSUsesNamedFullBleedPage(t *testing.T) {
	css := CSS()
	for _, want := range []string{"@page ct{margin:0}", `font-family:"Michroma"`, "page:ct", "inset-inline-start"} {
		if !strings.Contains(css, want) {
			t.Errorf("css misses %q", want)
		}
	}
}
