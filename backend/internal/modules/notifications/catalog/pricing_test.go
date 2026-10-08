package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-506: both pricing events ship an in-app and e-mail template in each of
// the 13 locales with only known placeholders.
func TestPricingTemplatesCoverEveryLocale(t *testing.T) {
	for _, code := range PricingEvents {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		seen := map[string]bool{}
		for _, tpl := range ev.Templates {
			seen[tpl.Channel+"/"+tpl.Language] = true
			if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
				t.Errorf("%s %s/%s: empty text", code, tpl.Channel, tpl.Language)
			}
			if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
				t.Errorf("%s %s/%s: unknown placeholders %v", code, tpl.Channel, tpl.Language, unknown)
			}
		}
		if len(msgtemplate.Locales) != 13 {
			t.Fatalf("locales = %d", len(msgtemplate.Locales))
		}
		for _, lang := range msgtemplate.Locales {
			for _, ch := range PricingChannels {
				if !seen[ch+"/"+lang] {
					t.Errorf("%s: no %s template in %s", code, ch, lang)
				}
			}
		}
	}
}
