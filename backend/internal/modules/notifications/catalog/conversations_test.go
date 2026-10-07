package catalog

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-398: conversation.inbound ships in-app and web push templates in all
// 13 locales, naming the contact and the message preview.
func TestConversationInboundTemplatesCoverEveryLocale(t *testing.T) {
	ev, ok := Lookup(EventConversationInbound)
	if !ok {
		t.Fatal("conversation.inbound not registered")
	}
	if ev.Module != "conversations" || !ev.UserConfigurable {
		t.Fatalf("module %q configurable %v", ev.Module, ev.UserConfigurable)
	}
	seen := map[string]bool{}
	for _, tpl := range ev.Templates {
		seen[tpl.Channel+"/"+tpl.Language] = true
		if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
			t.Errorf("%s/%s: unknown placeholders %v", tpl.Channel, tpl.Language, unknown)
		}
		text := tpl.Subject + tpl.Body
		if !strings.Contains(text, "{{contact_name}}") || !strings.Contains(text, "{{preview}}") {
			t.Errorf("%s/%s: misses the contact or the preview", tpl.Channel, tpl.Language)
		}
	}
	for _, ch := range []string{ChannelInapp, ChannelWebPush} {
		for _, l := range msgtemplate.Locales {
			if !seen[ch+"/"+l] {
				t.Errorf("no %s template in %s", ch, l)
			}
		}
	}
}
