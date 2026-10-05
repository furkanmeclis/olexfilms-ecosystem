package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

func TestMeasurementDiffTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventMeasurementDiffCheckRequired)
	if !ok {
		t.Fatalf("%s not registered", EventMeasurementDiffCheckRequired)
	}
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	seen := map[string]bool{}
	for _, tpl := range ev.Templates {
		seen[tpl.Channel+"/"+tpl.Language] = true
		if strings.TrimSpace(tpl.Subject) == "" || strings.TrimSpace(tpl.Body) == "" {
			t.Errorf("%s/%s: empty text", tpl.Channel, tpl.Language)
		}
		if tpl.Role != RoleDealer || tpl.Channel != ChannelInapp || tpl.Format != "text" {
			t.Errorf("%s/%s: role %q channel %q format %q", tpl.Channel, tpl.Language, tpl.Role, tpl.Channel, tpl.Format)
		}
		if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
			t.Errorf("%s/%s: unknown placeholders %v", tpl.Channel, tpl.Language, unknown)
		}
		if !strings.Contains(tpl.Subject+tpl.Body, "{{service_no}}") ||
			!strings.Contains(tpl.Subject+tpl.Body, "{{deviation_count}}") {
			t.Errorf("%s/%s: misses service_no or deviation_count", tpl.Channel, tpl.Language)
		}
	}
	for _, lang := range msgtemplate.Locales {
		if !seen[ChannelInapp+"/"+lang] {
			t.Errorf("no inapp template in %s", lang)
		}
	}
}
