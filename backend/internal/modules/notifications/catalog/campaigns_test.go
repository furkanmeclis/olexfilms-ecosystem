package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-406: every campaign approval event ships a default template in every
// locale (13) for in-app and e-mail, names the campaign and, for a
// rejection or a change request, carries the reason.
func TestCampaignTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	for _, code := range CampaignEvents {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if ev.Module != "campaigns" || !ev.UserConfigurable {
			t.Errorf("%s: module %q configurable %v", code, ev.Module, ev.UserConfigurable)
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
			text := tpl.Subject + tpl.Body
			if !strings.Contains(text, "{{campaign_name}}") {
				t.Errorf("%s %s/%s: misses the campaign", code, tpl.Channel, tpl.Language)
			}
			needReason := code == EventCampaignRejected || code == EventCampaignChangesRequested
			if needReason != strings.Contains(text, "{{reason}}") {
				t.Errorf("%s %s/%s: reason placeholder present = %v", code, tpl.Channel, tpl.Language, !needReason)
			}
			if code == EventCampaignApprovalRequested && !strings.Contains(text, "{{organization_name}}") {
				t.Errorf("%s %s/%s: misses the organization", code, tpl.Channel, tpl.Language)
			}
		}
		for _, ch := range CampaignChannels {
			for _, l := range msgtemplate.Locales {
				if !seen[ch+"/"+l] {
					t.Errorf("%s: no %s template in %s", code, ch, l)
				}
			}
		}
	}
}
