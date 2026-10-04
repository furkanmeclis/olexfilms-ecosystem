package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

func TestServiceSubscriptionTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	for _, code := range serviceSubscriptionEvents {
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
			if !strings.Contains(tpl.Subject+tpl.Body, "{{item_name}}") {
				t.Errorf("%s %s/%s: misses item name", code, tpl.Channel, tpl.Language)
			}
		}
		for _, ch := range ServiceSubscriptionChannels {
			for _, lang := range msgtemplate.Locales {
				if !seen[ch+"/"+lang] {
					t.Errorf("%s: no %s template in %s", code, ch, lang)
				}
			}
		}
	}
}
