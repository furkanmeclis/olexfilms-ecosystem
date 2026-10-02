package notifications

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-187: a warranty cron event (as the outbox redelivers it: numbers as
// int64) becomes a dispatch to the holder with the brand and template vars.
func TestWarrantyDispatch(t *testing.T) {
	ev := events.New(events.WarrantyExpiringSoon).WithPayload(map[string]any{
		"holder_user_id": int64(42), "brand_id": int64(3), "days": int64(7),
		"plate": "34 ABC 123", "product_name": "PPF", "end_date": "2027-03-31",
		"organization_name": "Tech Oto", "verify_url": "https://olexfilms.app/garanti/abc",
		"warranty_uuid": "u-1", "public_code": "abc",
	})
	in, ok := warrantyDispatch(ev, catalog.EventWarrantyExpiringSoon, 7)
	if !ok {
		t.Fatal("dispatch skipped")
	}
	if in.EventCode != catalog.EventWarrantyExpiringSoon || len(in.UserIDs) != 1 || in.UserIDs[0] != 42 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["days"] != "7" || in.Vars["plate"] != "34 ABC 123" || in.Vars["end_date"] != "2027-03-31" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if in.ActionURL == nil || *in.ActionURL != "https://olexfilms.app/garanti/abc" {
		t.Fatalf("action url = %v", in.ActionURL)
	}

	expired, ok := warrantyDispatch(events.New(events.WarrantyExpired).WithPayload(map[string]any{
		"holder_user_id": int64(42),
	}), catalog.EventWarrantyExpired, 0)
	if !ok || expired.Vars["days"] != "" || expired.ActionURL != nil {
		t.Fatalf("expired dispatch = %+v", expired)
	}

	if _, ok := warrantyDispatch(events.New(events.WarrantyExpired), catalog.EventWarrantyExpired, 0); ok {
		t.Fatal("an event without holder must be skipped")
	}
}
