package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-389: ai.quota.threshold ships a default template in every locale (13)
// for in-app and e-mail, naming the threshold and the organization with
// known placeholders only.
func TestAIQuotaTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventAIQuotaThreshold)
	if !ok {
		t.Fatal("ai.quota.threshold not registered")
	}
	if ev.Module != "ai" || !ev.UserConfigurable {
		t.Fatalf("module %q configurable %v", ev.Module, ev.UserConfigurable)
	}
	seen := map[string]bool{}
	for _, tpl := range ev.Templates {
		seen[tpl.Channel+"/"+tpl.Language] = true
		if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
			t.Errorf("%s/%s: empty text", tpl.Channel, tpl.Language)
		}
		if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
			t.Errorf("%s/%s: unknown placeholders %v", tpl.Channel, tpl.Language, unknown)
		}
		text := tpl.Subject + tpl.Body
		for _, p := range []string{"{{threshold}}", "{{organization_name}}", "{{period}}"} {
			if !strings.Contains(text, p) {
				t.Errorf("%s/%s: misses %s", tpl.Channel, tpl.Language, p)
			}
		}
	}
	for _, ch := range AIQuotaChannels {
		for _, l := range msgtemplate.Locales {
			if !seen[ch+"/"+l] {
				t.Errorf("no %s template in %s", ch, l)
			}
		}
	}
}
