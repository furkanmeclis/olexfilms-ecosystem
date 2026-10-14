package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-317: leads.application_received goes to notify_user_ids with the
// company, contact and location; no recipient sends nothing.
func TestLeadApplicationDispatch(t *testing.T) {
	ev := events.New(events.LeadsApplicationReceived).WithPayload(map[string]any{
		"notify_user_ids": []any{float64(4), float64(5)},
		"brand_id":        float64(2), "lead_uuid": "l-1", "routed_by": "territory",
		"company_name": "Kuzey GmbH", "contact_name": "Max", "location": "Berlin, Germany",
	})
	in, ok := leadApplicationDispatch(ev)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventLeadDealerApplication || !slices.Equal(in.UserIDs, []int64{4, 5}) {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 2 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["company_name"] != "Kuzey GmbH" || in.Vars["contact_name"] != "Max" || in.Vars["location"] != "Berlin, Germany" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, ok := leadApplicationDispatch(events.New(events.LeadsApplicationReceived).WithPayload(map[string]any{})); ok {
		t.Fatal("no recipient must send nothing")
	}
}

// TEC-641: the showcase lead notification links to the lead in the dealer
// panel (push data action_url).
func TestLeadWebsiteDispatchLinksTheLead(t *testing.T) {
	ev := events.New(events.LeadsWebsiteReceived).WithPayload(map[string]any{
		"notify_user_ids": []any{float64(21)}, "brand_id": float64(1), "lead_uuid": "l-8",
		"dealer_code": "demo-istanbul-detailing", "contact_name": "Ayşe", "phone": "+905551234567",
	})
	in, ok := leadWebsiteDispatch(ev)
	if !ok || in.EventCode != catalog.EventLeadWebsiteReceived || !slices.Equal(in.UserIDs, []int64{21}) {
		t.Fatalf("dispatch = %+v, %v", in, ok)
	}
	if in.ActionURL == nil || *in.ActionURL != "/t/demo-istanbul-detailing/leads/l-8" {
		t.Fatalf("action url = %v", in.ActionURL)
	}
	if in.Payload["lead_uuid"] != "l-8" {
		t.Fatalf("payload = %v", in.Payload)
	}
	delete(ev.Payload, "dealer_code")
	if in, _ := leadWebsiteDispatch(ev); in.ActionURL != nil {
		t.Fatalf("no dealer code must leave the link empty, got %q", *in.ActionURL)
	}
}
