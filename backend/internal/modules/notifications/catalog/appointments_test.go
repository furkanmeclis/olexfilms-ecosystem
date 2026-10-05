package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

func TestAppointmentTemplatesCoverEveryLocale(t *testing.T) {
	for _, code := range []string{EventAppointmentCreated, EventAppointmentCancelled, EventAppointmentReminder} {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if len(ev.DefaultChannels) != 1 || ev.DefaultChannels[0] != ChannelWhatsApp {
			t.Fatalf("%s channels = %v, want whatsapp", code, ev.DefaultChannels)
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
			if !strings.Contains(tpl.Body, "{{starts_at}}") || !strings.Contains(tpl.Body, "{{organization_name}}") {
				t.Errorf("%s %s/%s: body misses starts_at or organization_name", code, tpl.Channel, tpl.Language)
			}
		}
		for _, l := range msgtemplate.Locales {
			if !seen[ChannelWhatsApp+"/"+l] {
				t.Errorf("%s: no whatsapp template in %s", code, l)
			}
		}
	}
}
