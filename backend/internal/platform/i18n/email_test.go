package i18n

import (
	"strings"
	"testing"
)

// TEC-1235: every e-mail text exists in every supported locale (written,
// not the en fallback) with the en placeholders.
func TestEmailTextsCoverEveryLocale(t *testing.T) {
	for _, key := range EmailKeys {
		en := enCatalog[key]
		if strings.TrimSpace(en) == "" {
			t.Errorf("%s: missing en", key)
			continue
		}
		for _, l := range Supported {
			v := strings.TrimSpace(catalogs[l][key])
			if v == "" {
				t.Errorf("%s: missing %s", key, l)
				continue
			}
			if params(v) != params(en) {
				t.Errorf("%s/%s: params %q, en %q", key, l, params(v), params(en))
			}
		}
	}
	if got := TranslateParams(LocaleAR, "email.footer.reason", map[string]string{"brand": "Olex"}); !strings.Contains(got, "Olex") || strings.Contains(got, "{{") {
		t.Fatalf("reason = %q", got)
	}
}
