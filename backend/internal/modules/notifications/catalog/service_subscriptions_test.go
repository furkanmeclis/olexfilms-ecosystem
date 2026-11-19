package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

func TestServiceSubscriptionTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 15 {
		t.Fatalf("locales = %d, want 15", len(msgtemplate.Locales))
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

// TEC-1235: every locale is written (no en copies) and tr keeps its
// diacritics.
func TestServiceSubscriptionTextsTranslated(t *testing.T) {
	for _, code := range serviceSubscriptionEvents {
		texts := serviceSubscriptionTexts[code]
		for _, lang := range msgtemplate.Locales {
			tx, ok := texts[lang]
			if !ok {
				t.Errorf("%s: %s not written", code, lang)
				continue
			}
			if lang != "en" && (tx.subject == texts["en"].subject || tx.body == texts["en"].body) {
				t.Errorf("%s/%s: still the en text", code, lang)
			}
		}
		if !strings.ContainsAny(texts["tr"].subject+texts["tr"].body, "ğüşıöçİ") {
			t.Errorf("%s/tr: no Turkish characters", code)
		}
	}
}
