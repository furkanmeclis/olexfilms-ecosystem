package otp_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
)

// TEC-1233: the OTP text exists in every supported locale and the locale of
// the request picks it (no silent tr/en fallback).
func TestNormalizeLocale(t *testing.T) {
	cases := map[string]string{
		"":      "tr",
		"TR_tr": "tr",
		"de-DE": "de",
		"zh_cn": "zh-CN",
		"ar-AE": "ar",
		"xx":    "en",
	}
	for in, want := range cases {
		if got := otp.NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderMessageEveryLocale(t *testing.T) {
	en := otp.RenderMessage("en", "Olex", "123456", 5, "")
	for _, l := range i18n.Supported {
		msg := otp.RenderMessage(string(l), "Olex", "123456", 5, "KVKK")
		if !strings.Contains(msg, "Olex") || !strings.Contains(msg, "123456") || !strings.Contains(msg, "5") {
			t.Errorf("%s: placeholders not filled: %q", l, msg)
		}
		if strings.ContainsAny(msg, "{}") {
			t.Errorf("%s: unreplaced placeholder: %q", l, msg)
		}
		if !strings.HasSuffix(msg, "\n\nKVKK") {
			t.Errorf("%s: KVKK notice missing: %q", l, msg)
		}
		if l != i18n.LocaleEN && strings.HasPrefix(msg, strings.SplitN(en, "\n", 2)[0]) {
			t.Errorf("%s: falls back to the English text", l)
		}
	}
	if got := otp.RenderMessage("de-DE", "Olex", "1", 5, ""); !strings.Contains(got, "Bestätigungscode") {
		t.Errorf("raw de-DE not normalized: %q", got)
	}
}
