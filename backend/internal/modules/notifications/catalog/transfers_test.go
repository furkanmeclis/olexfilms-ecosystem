package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-200: every transfer event ships a default template in every locale
// (13) for in-app and WhatsApp, using only catalog placeholders and naming
// the transfer number.
func TestTransferTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	for _, code := range TransferEvents {
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
			if tpl.Role != RoleGeneric || tpl.Format != "text" {
				t.Errorf("%s %s/%s: role %q format %q", code, tpl.Channel, tpl.Language, tpl.Role, tpl.Format)
			}
			if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
				t.Errorf("%s %s/%s: unknown placeholders %v", code, tpl.Channel, tpl.Language, unknown)
			}
			if !strings.Contains(tpl.Subject+tpl.Body, "{{transfer_no}}") {
				t.Errorf("%s %s/%s: misses the transfer number", code, tpl.Channel, tpl.Language)
			}
		}
		for _, ch := range []string{ChannelInapp, ChannelWhatsApp} {
			for _, l := range msgtemplate.Locales {
				if !seen[ch+"/"+l] {
					t.Errorf("%s: no %s template in %s", code, ch, l)
				}
			}
		}
	}
}
