package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-1024: the center-opened order event ships a template in every locale
// for in-app and WhatsApp, using only catalog placeholders.
func TestOrderTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventOrderCreatedBySeller)
	if !ok {
		t.Fatal("ORDER_CREATED_BY_SELLER not registered")
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
		if !strings.Contains(tpl.Subject+tpl.Body, "{{order_no}}") {
			t.Errorf("%s/%s: misses the order number", tpl.Channel, tpl.Language)
		}
	}
	for _, ch := range OrderChannels {
		for _, l := range msgtemplate.Locales {
			if !seen[ch+"/"+l] {
				t.Errorf("missing %s/%s", ch, l)
			}
		}
	}
}
