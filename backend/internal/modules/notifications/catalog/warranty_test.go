package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-187: both warranty events ship a default template in every locale
// (13) for every default channel, using only catalog placeholders.
func TestWarrantyTemplatesCoverEveryLocale(t *testing.T) {
	for _, code := range []string{EventWarrantyExpiringSoon, EventWarrantyExpired} {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if len(msgtemplate.Locales) != 13 {
			t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
		}
		seen := map[string]bool{}
		for _, tpl := range ev.Templates {
			seen[tpl.Channel+"/"+tpl.Language] = true
			if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
				t.Errorf("%s %s/%s: empty text", code, tpl.Channel, tpl.Language)
			}
			if tpl.Role != RoleGeneric || tpl.Format != "text" {
				t.Errorf("%s %s/%s: role %q format %q", code, tpl.Channel, tpl.Language, tpl.Role, tpl.Format)
			}
			if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
				t.Errorf("%s %s/%s: unknown placeholders %v", code, tpl.Channel, tpl.Language, unknown)
			}
			if !strings.Contains(tpl.Body, "{{plate}}") || !strings.Contains(tpl.Body, "{{end_date}}") {
				t.Errorf("%s %s/%s: body misses plate or end date", code, tpl.Channel, tpl.Language)
			}
			if code == EventWarrantyExpiringSoon && !strings.Contains(tpl.Subject+tpl.Body, "{{days}}") {
				t.Errorf("%s %s/%s: misses days", code, tpl.Channel, tpl.Language)
			}
		}
		for _, ch := range ev.DefaultChannels {
			for _, l := range msgtemplate.Locales {
				if !seen[ch+"/"+l] {
					t.Errorf("%s: no %s template in %s", code, ch, l)
				}
			}
		}
		hasWA := false
		for _, ch := range ev.DefaultChannels {
			hasWA = hasWA || ch == ChannelWhatsApp
		}
		if !hasWA {
			t.Errorf("%s: WhatsApp is not a default channel", code)
		}
	}
}
