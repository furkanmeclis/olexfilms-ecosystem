package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-221: every task event ships a default template in every locale (13)
// for in-app and e-mail, using only catalog placeholders, naming the task
// and (for the reminders) the due date.
func TestTaskTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	for _, code := range TaskEvents {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if ev.Module != "tasks" || !ev.UserConfigurable {
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
			if !strings.Contains(text, "{{task_title}}") || !strings.Contains(text, "{{subject_name}}") {
				t.Errorf("%s %s/%s: misses the task or the subject", code, tpl.Channel, tpl.Language)
			}
			if code != EventTaskAssigned && !strings.Contains(text, "{{due_date}}") {
				t.Errorf("%s %s/%s: misses the due date", code, tpl.Channel, tpl.Language)
			}
		}
		for _, ch := range []string{ChannelInapp, ChannelEmail} {
			for _, l := range msgtemplate.Locales {
				if !seen[ch+"/"+l] {
					t.Errorf("%s: no %s template in %s", code, ch, l)
				}
			}
		}
	}
}
