package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-200: every transfers.* event maps to a registered catalog event; the
// dispatch goes to notify_user_ids (as JSON decodes them) with the names
// and the transfer number; no recipient sends nothing.
func TestTransferDispatch(t *testing.T) {
	if len(TransferEventCodes) != 6 {
		t.Fatalf("mapped events = %d, want 6", len(TransferEventCodes))
	}
	for name, code := range TransferEventCodes {
		if _, ok := catalog.Lookup(code); !ok {
			t.Fatalf("%s -> %s: not in the catalog", name, code)
		}
	}
	ev := events.New(events.TransfersShipped).WithPayload(map[string]any{
		"notify_user_ids": []any{float64(7), float64(9), float64(0)},
		"brand_id":        float64(3), "transfer_no": "TR00000001", "transfer_uuid": "u-1",
		"status": "shipped", "from_org_name": "A Oto", "to_org_name": "B Oto",
	})
	in, ok := transferDispatcher(catalog.EventTransferShipped)(ev)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventTransferShipped || !slices.Equal(in.UserIDs, []int64{7, 9}) {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["transfer_no"] != "TR00000001" || in.Vars["sender_name"] != "A Oto" || in.Vars["receiver_name"] != "B Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, ok := transferDispatcher(catalog.EventTransferShipped)(events.New(events.TransfersShipped).WithPayload(map[string]any{
		"notify_user_ids": []int64{},
	})); ok {
		t.Fatal("no recipient must send nothing")
	}
}
