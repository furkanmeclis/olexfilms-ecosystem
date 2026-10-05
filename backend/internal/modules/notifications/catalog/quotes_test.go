package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

func TestQuoteSentTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventQuoteSent)
	if !ok {
		t.Fatal("QUOTE_SENT not registered")
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
		if !strings.Contains(tpl.Body, "{{quote_url}}") || !strings.Contains(tpl.Body, "{{total_amount}}") {
			t.Errorf("%s/%s: body misses quote_url or total_amount", tpl.Channel, tpl.Language)
		}
	}
	for _, l := range msgtemplate.Locales {
		if !seen[ChannelWhatsApp+"/"+l] {
			t.Errorf("no whatsapp template in %s", l)
		}
	}
}
