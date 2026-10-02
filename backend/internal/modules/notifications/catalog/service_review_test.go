package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-192: the review request ships a WhatsApp template in every locale
// (13), linking the dealer's google_business_url.
func TestServiceReviewTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventServiceReviewRequest)
	if !ok {
		t.Fatal("SERVICE_REVIEW_REQUEST not registered")
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
		if !strings.Contains(tpl.Body, "{{review_url}}") || !strings.Contains(tpl.Body, "{{organization_name}}") {
			t.Errorf("%s/%s: body misses review_url or organization_name", tpl.Channel, tpl.Language)
		}
	}
	for _, l := range msgtemplate.Locales {
		if !seen[ChannelWhatsApp+"/"+l] {
			t.Errorf("no whatsapp template in %s", l)
		}
	}
}
