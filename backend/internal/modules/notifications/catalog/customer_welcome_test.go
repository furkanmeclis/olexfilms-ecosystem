package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-164: the customer welcome ships a WhatsApp template in every locale
// (13), each linking the portal.
func TestCustomerWelcomeTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventCustomerWelcome)
	if !ok {
		t.Fatal("CUSTOMER_WELCOME not registered")
	}
	if len(ev.DefaultChannels) != 1 || ev.DefaultChannels[0] != ChannelWhatsApp {
		t.Fatalf("default channels = %v, want whatsapp", ev.DefaultChannels)
	}
	seen := map[string]bool{}
	for _, tpl := range ev.Templates {
		seen[tpl.Channel+"/"+tpl.Language] = true
		if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
			t.Errorf("%s/%s: empty text", tpl.Channel, tpl.Language)
		}
		if tpl.Role != RoleGeneric || tpl.Format != "text" {
			t.Errorf("%s/%s: role %q format %q", tpl.Channel, tpl.Language, tpl.Role, tpl.Format)
		}
		if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
			t.Errorf("%s/%s: unknown placeholders %v", tpl.Channel, tpl.Language, unknown)
		}
		for _, p := range []string{"{{portal_url}}", "{{organization_name}}", "{{customer_name}}"} {
			if !strings.Contains(tpl.Body, p) {
				t.Errorf("%s/%s: body misses %s", tpl.Channel, tpl.Language, p)
			}
		}
	}
	for _, l := range msgtemplate.Locales {
		if !seen[ChannelWhatsApp+"/"+l] {
			t.Errorf("no whatsapp template in %s", l)
		}
	}
}
