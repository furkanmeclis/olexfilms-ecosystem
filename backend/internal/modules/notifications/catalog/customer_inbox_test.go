package catalog

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// TEC-1034: every customer facing event writes the in-app row of the
// portal/mobile inbox next to WhatsApp, in all 13 locales.
func TestCustomerEventsReachTheInbox(t *testing.T) {
	for _, code := range []string{
		EventAppointmentCreated, EventAppointmentCancelled, EventAppointmentReminder,
		EventServiceReviewRequest, EventCustomerWelcome, EventWarrantyClaimResult, EventQuoteSent,
	} {
		ev, ok := Lookup(code)
		if !ok {
			t.Fatalf("%s not registered", code)
		}
		if !slices.Equal(ev.DefaultChannels, []string{ChannelInapp, ChannelWhatsApp}) {
			t.Errorf("%s channels = %v, want inapp+whatsapp", code, ev.DefaultChannels)
		}
		seen := map[string]bool{}
		for _, tpl := range ev.Templates {
			seen[tpl.Channel+"/"+tpl.Language] = true
			if unknown := msgtemplate.Unknown(ev.Spec(), tpl.Subject, tpl.Body); len(unknown) > 0 {
				t.Errorf("%s %s/%s: unknown placeholders %v", code, tpl.Channel, tpl.Language, unknown)
			}
		}
		for _, l := range msgtemplate.Locales {
			if !seen[ChannelInapp+"/"+l] {
				t.Errorf("%s: no inapp template in %s", code, l)
			}
		}
	}
}
