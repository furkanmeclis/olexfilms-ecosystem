package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-508: the decision event ships in-app and e-mail templates in all 13
// locales and every locale has its decision labels.
func TestModuleRequestDecidedTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventFeaturesModuleRequestDecided)
	if !ok {
		t.Fatal("event not registered")
	}
	seen := map[string]bool{}
	for _, tpl := range ev.Templates {
		seen[tpl.Channel+"/"+tpl.Language] = true
		if strings.TrimSpace(tpl.Subject) == "" || !strings.Contains(tpl.Body, "{{decision}}") {
			t.Errorf("%s/%s: incomplete template", tpl.Channel, tpl.Language)
		}
		if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
			t.Errorf("%s/%s: unknown placeholders %v", tpl.Channel, tpl.Language, unknown)
		}
	}
	for _, lang := range msgtemplate.Locales {
		for _, ch := range ModuleRequestChannels {
			if !seen[ch+"/"+lang] {
				t.Errorf("no %s template in %s", ch, lang)
			}
		}
		if _, ok := moduleRequestDecisionLabels[lang]; !ok {
			t.Errorf("no decision labels in %s", lang)
		}
	}
	if got := ModuleRequestDecisionLabel("de", "rejected"); got != "abgelehnt" {
		t.Fatalf("de rejected = %q", got)
	}
}
