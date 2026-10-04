package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-317: the dealer application event ships in-app and e-mail templates
// in every locale (13), using only catalog placeholders.
func TestLeadApplicationTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventLeadDealerApplication)
	if !ok {
		t.Fatal("not registered")
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
		if !strings.Contains(tpl.Subject, "{{company_name}}") {
			t.Errorf("%s/%s: subject misses the company", tpl.Channel, tpl.Language)
		}
	}
	for _, ch := range []string{ChannelInapp, ChannelEmail} {
		for _, l := range msgtemplate.Locales {
			if !seen[ch+"/"+l] {
				t.Errorf("no %s template in %s", ch, l)
			}
		}
	}
}
