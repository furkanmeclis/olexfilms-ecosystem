package catalog

import (
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-164: the customer welcome ships a WhatsApp template in every locale
// (13), each linking the portal; TEC-1034 adds the in-app row (no link, the
// action opens the portal).
func TestCustomerWelcomeTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventCustomerWelcome)
	if !ok {
		t.Fatal("CUSTOMER_WELCOME not registered")
	}
	if !slices.Equal(ev.DefaultChannels, []string{ChannelInapp, ChannelWhatsApp}) {
		t.Fatalf("default channels = %v, want inapp+whatsapp", ev.DefaultChannels)
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
		want := []string{"{{organization_name}}", "{{customer_name}}"}
		if tpl.Channel == ChannelWhatsApp {
			want = append(want, "{{portal_url}}")
		} else if strings.Contains(tpl.Body, "{{portal_url}}") {
			t.Errorf("%s/%s: in-app body carries the portal link", tpl.Channel, tpl.Language)
		}
		for _, p := range want {
			if !strings.Contains(tpl.Body, p) {
				t.Errorf("%s/%s: body misses %s", tpl.Channel, tpl.Language, p)
			}
		}
	}
	for _, ch := range ev.DefaultChannels {
		for _, l := range msgtemplate.Locales {
			if !seen[ch+"/"+l] {
				t.Errorf("no %s template in %s", ch, l)
			}
		}
	}
}
