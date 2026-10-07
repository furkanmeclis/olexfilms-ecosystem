package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-398: an inbound message of an assigned conversation notifies the
// assignee (conversation.inbound, in-app + web push by default); an
// unassigned conversation notifies nobody.
func TestConversationInboundDispatch(t *testing.T) {
	ev, ok := catalog.Lookup(catalog.EventConversationInbound)
	if !ok || !slices.Equal(ev.DefaultChannels, []string{catalog.ChannelInapp, catalog.ChannelWebPush}) {
		t.Fatalf("catalog event = %+v (%v)", ev, ok)
	}
	in, ok := conversationInboundDispatch(events.New(events.WhatsAppMessageReceived).WithPayload(map[string]any{
		"conversation_uuid": "c-1", "message_uuid": "m-1", "assigned_user_id": float64(42),
		"contact_name": "Ahmet", "preview": "Merhaba",
	}))
	if !ok || in.EventCode != catalog.EventConversationInbound || !slices.Equal(in.UserIDs, []int64{42}) {
		t.Fatalf("dispatch = %+v (%v)", in, ok)
	}
	if in.Vars["contact_name"] != "Ahmet" || in.Vars["preview"] != "Merhaba" || in.Payload["conversation_uuid"] != "c-1" {
		t.Fatalf("vars/payload = %v %v", in.Vars, in.Payload)
	}
	if _, ok := conversationInboundDispatch(events.New(events.WhatsAppMessageReceived).WithPayload(map[string]any{
		"conversation_uuid": "c-1",
	})); ok {
		t.Fatal("unassigned conversation must notify nobody")
	}
}
