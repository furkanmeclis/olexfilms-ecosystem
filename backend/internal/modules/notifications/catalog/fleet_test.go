package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-473: every fleet event ships a default template in every locale (13)
// for in-app and e-mail and names the fleet.
func TestFleetTemplatesCoverEveryLocale(t *testing.T) {
	for _, code := range FleetEvents {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if ev.Module != "fleet" {
			t.Errorf("%s: module %q", code, ev.Module)
		}
		seen := map[string]bool{}
		for _, tpl := range ev.Templates {
			seen[tpl.Channel+"/"+tpl.Language] = true
			if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
				t.Errorf("%s %s/%s: empty text", code, tpl.Channel, tpl.Language)
			}
			if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
				t.Errorf("%s %s/%s: unknown placeholders %v", code, tpl.Channel, tpl.Language, unknown)
			}
			if !strings.Contains(tpl.Subject+tpl.Body, "{{fleet_name}}") {
				t.Errorf("%s %s/%s: misses the fleet", code, tpl.Channel, tpl.Language)
			}
		}
		for _, lang := range msgtemplate.Locales {
			for _, ch := range FleetChannels {
				if !seen[ch+"/"+lang] {
					t.Errorf("%s: no %s template in %s", code, ch, lang)
				}
			}
		}
	}
}
