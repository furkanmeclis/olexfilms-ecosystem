package i18n

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in     string
		want   Locale
		wantOK bool
	}{
		{"tr", LocaleTR, true},
		{" EN ", LocaleEN, true},
		{"tr-TR", LocaleTR, true},
		{"tr_TR", LocaleTR, true},
		{"en-US", LocaleEN, true},
		{"ar-AE", LocaleAR, true},
		{"zh-CN", LocaleZhCN, true},
		{"zh_CN", LocaleZhCN, true},
		{"zh-cn", LocaleZhCN, true},
		{"zh", LocaleZhCN, true},
		{"zh-Hans-CN", LocaleZhCN, true},
		{"uk", LocaleUK, true},
		{"az-Latn-AZ", LocaleAZ, true},
		{"", "", false},
		{"xx", "", false},
		{"klingon", "", false},
		{"*", "", false},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("Parse(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestNormalizeFallsBackToTR(t *testing.T) {
	for in, want := range map[string]Locale{
		"":      LocaleTR,
		"xx":    LocaleTR,
		"en":    LocaleEN,
		"de-DE": LocaleDE,
		"zh_CN": LocaleZhCN,
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSupportedIsCanonical(t *testing.T) {
	if len(Supported) != 13 {
		t.Fatalf("supported = %d, want 13", len(Supported))
	}
	for _, l := range Supported {
		if !IsSupported(string(l)) {
			t.Errorf("IsSupported(%q) = false", l)
		}
		if got, ok := Parse(string(l)); !ok || got != l {
			t.Errorf("Parse(%q) = %q, %v", l, got, ok)
		}
		if _, ok := catalogs[l]; !ok {
			t.Errorf("no catalog for %q", l)
		}
	}
	if IsSupported("zh_CN") || IsSupported("tr-TR") {
		t.Error("IsSupported must only accept canonical codes")
	}
	if Dir(LocaleAR) != "rtl" || Dir(LocaleTR) != "ltr" {
		t.Error("Dir")
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	tests := []struct {
		header string
		want   Locale
		wantOK bool
	}{
		{"de-DE,de;q=0.9,en;q=0.8", LocaleDE, true},
		{"ja-JP,ja;q=0.9,en;q=0.5", LocaleEN, true},
		{"en;q=0.5,ar;q=0.9", LocaleAR, true},
		{"fr;q=0,es", LocaleES, true},
		{"zh-CN,zh;q=0.9", LocaleZhCN, true},
		{"*", "", false},
		{"ja", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseAcceptLanguage(tt.header)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ParseAcceptLanguage(%q) = %q, %v; want %q, %v", tt.header, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestValidTimezone(t *testing.T) {
	for tz, want := range map[string]bool{
		"Asia/Dubai":      true,
		"Europe/Istanbul": true,
		"UTC":             true,
		"":                false,
		"Local":           false,
		"Mars/Olympus":    false,
		" Asia/Dubai":     false,
	} {
		if got := ValidTimezone(tz); got != want {
			t.Errorf("ValidTimezone(%q) = %v, want %v", tz, got, want)
		}
	}
}

func TestResolveChain(t *testing.T) {
	tests := []struct {
		name   string
		in     Sources
		wantL  Locale
		wantTZ string
	}{
		{"defaults", Sources{}, LocaleTR, DefaultTimezone},
		{"accept-language only", Sources{AcceptLanguage: "de-DE,de;q=0.9"}, LocaleDE, DefaultTimezone},
		{"unsupported accept-language", Sources{AcceptLanguage: "ja"}, LocaleTR, DefaultTimezone},
		{"center beats accept-language", Sources{CenterLocale: "en", CenterTimezone: "Europe/London", AcceptLanguage: "de"}, LocaleEN, "Europe/London"},
		{"org beats center", Sources{OrgLocale: "ar", OrgTimezone: "Asia/Dubai", CenterLocale: "tr", CenterTimezone: "Europe/Istanbul"}, LocaleAR, "Asia/Dubai"},
		{"user beats org", Sources{UserLocale: "zh-CN", UserTimezone: "Asia/Shanghai", OrgLocale: "ar", OrgTimezone: "Asia/Dubai"}, LocaleZhCN, "Asia/Shanghai"},
		{"null user inherits org", Sources{OrgLocale: "de", OrgTimezone: "Europe/Berlin", AcceptLanguage: "fr"}, LocaleDE, "Europe/Berlin"},
		{"user timezone only", Sources{UserTimezone: "Asia/Dubai", OrgLocale: "en"}, LocaleEN, "Asia/Dubai"},
		{"legacy org value", Sources{OrgLocale: "tr-TR"}, LocaleTR, DefaultTimezone},
		{"invalid stored values skipped", Sources{UserLocale: "xx", UserTimezone: "Mars/Olympus", OrgLocale: "it", OrgTimezone: "Europe/Rome"}, LocaleIT, "Europe/Rome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.in)
			if got.Locale != tt.wantL || got.Timezone != tt.wantTZ {
				t.Fatalf("Resolve = %q / %q, want %q / %q", got.Locale, got.Timezone, tt.wantL, tt.wantTZ)
			}
		})
	}
}

func TestTranslateFallsBackToEN(t *testing.T) {
	// A key only en has (not translated yet) resolves to the en label.
	const onlyEN = "test.only_en_label"
	enCatalog[onlyEN] = "English only"
	t.Cleanup(func() { delete(enCatalog, onlyEN) })
	if got := Translate(LocaleDE, onlyEN); got != "English only" {
		t.Fatalf("de %s = %q, want en value", onlyEN, got)
	}
	if got := Translate(LocaleAR, "missing.key"); got != "missing.key" {
		t.Fatalf("missing key = %q", got)
	}
	if got := Translate(LocaleTR, "users.email"); got != "E-posta" {
		t.Fatalf("tr users.email = %q", got)
	}
}
