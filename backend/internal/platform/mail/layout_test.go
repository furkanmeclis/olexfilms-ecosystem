package mail

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the layout golden files")

// TEC-1235: golden HTML + text of representative frames. Regenerate with
// go test ./internal/platform/mail -run TestLayoutGolden -update
func TestLayoutGolden(t *testing.T) {
	cases := map[string]Layout{
		"ltr_logo_button_secondary": {
			Lang: "tr", Dir: "ltr", Title: "Size bir görev atandı: Aylık ziyaret",
			BrandName: "Olex Films", LogoURL: "https://cdn.example.test/logo.png", Color: "#0f766e",
			BodyHTML:    "<p>Tech Oto ile ilgili <strong>Aylık ziyaret</strong> görevi size atandı.</p><ul><li>Son tarih: 31.10.2026</li></ul>",
			BodyText:    "Tech Oto ile ilgili Aylık ziyaret görevi size atandı.\n\n- Son tarih: 31.10.2026",
			ButtonLabel: "Görüntüle", ButtonURL: "https://app.example.test/tasks/42?a=1&b=2",
			Secondary: "Buton çalışmıyorsa bu bağlantıyı açın:", SecondaryURL: "https://app.example.test/tasks/42?a=1&b=2",
			Reason: "Bu e-postayı Olex Films hesabınızla ilgili bir bildirim olduğu için aldınız.",
		},
		"rtl_ar_button": {
			Lang: "ar", Dir: "rtl", Title: "تم إسناد مهمة إليك",
			BrandName: "Olex Films", Color: "#111827",
			BodyHTML:    "<p>افتح اللوحة للاطلاع على التفاصيل.</p>",
			ButtonLabel: "عرض", ButtonURL: "https://app.example.test/tasks/42",
			Reason: "تلقيت هذه الرسالة بسبب إشعار يخص حسابك.",
		},
		"legacy_no_logo": {
			Lang: "en", Title: "Test notification", BrandName: "Olex <Films>", Color: "red;}body{display:none",
			BodyHTML: "<p>This is a test email.</p>",
		},
		"preheader_unsubscribe_light_brand": {
			Lang: "de", Dir: "ltr", Title: "Herbstaktion", BrandName: "Olex Films", Color: "#fde047",
			BodyHTML:    `<p>Jetzt <a href="https://x.test/a">ansehen</a>.</p>`,
			Preheader:   "  20 % Rabatt\n bis 30. November  ",
			ButtonLabel: "Details", ButtonURL: "https://app.example.test/campaigns/autumn",
			UnsubscribeText: "Sie möchten keine Kampagnennachrichten erhalten?", UnsubscribeLabel: "Abmelden",
			UnsubscribeURL: "https://app.example.test/unsubscribe/tok",
		},
	}
	for name, l := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := l.RenderHTML()
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name+".html", got)
			golden(t, name+".txt", l.RenderText())
		})
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs; run go test ./internal/platform/mail -run TestLayoutGolden -update\n--- got ---\n%s", name, got)
	}
}

func TestLayoutEscapesAndGuards(t *testing.T) {
	l := Layout{
		Lang: `tr"><script>`, Title: "<b>x</b>", BrandName: "A&B", Color: "#fff;background:url(x)",
		BodyHTML: "<p>ok</p>", ButtonLabel: "Aç", ButtonURL: "javascript:alert(1)",
		Secondary: "<i>s</i>", UnsubscribeLabel: "Çık", UnsubscribeURL: "data:text/html,x",
		LogoURL: "javascript:x",
	}
	html, err := l.RenderHTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script>", "<b>x</b>", "<i>s</i>", "javascript:", "data:text", "url(x)"} {
		if strings.Contains(html, bad) {
			t.Errorf("html contains %q", bad)
		}
	}
	if !strings.Contains(html, "A&amp;B") || !strings.Contains(html, "background-color:"+DefaultColor) {
		t.Error("brand not escaped or color not sanitized")
	}
	if strings.Contains(html, "v:roundrect") {
		t.Error("unsafe button url rendered")
	}
}

func TestPreheaderDerivedFromBody(t *testing.T) {
	long := strings.Repeat("kelime ", 40)
	l := Layout{BodyHTML: "<p>" + long + "</p>"}
	p := l.PreheaderText()
	if !strings.HasSuffix(p, "…") || len([]rune(p)) > PreheaderMax+1 || strings.Contains(p, "<") {
		t.Fatalf("preheader = %q", p)
	}
	if got := (Layout{BodyText: "Kısa  metin\n\nikinci"}).PreheaderText(); got != "Kısa metin ikinci" {
		t.Fatalf("short preheader = %q", got)
	}
}

func TestOnColorContrast(t *testing.T) {
	for bg, want := range map[string]string{
		"#111827": "#ffffff", "#0f766e": "#ffffff", "#fde047": "#111827", "#fff": "#111827", "red": "#ffffff",
	} {
		if got := onColor(bg); got != want {
			t.Errorf("onColor(%s) = %s, want %s", bg, got, want)
		}
	}
}

func TestBrandName(t *testing.T) {
	if BrandName(" ") != DefaultBrandName || BrandName("X") != "X" {
		t.Fatal("brand fallback")
	}
}
