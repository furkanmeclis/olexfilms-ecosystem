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

// TEC-192: service.review_requested goes to the service customer with the
// dealer's review link; without a customer or a link nothing is sent.
func TestServiceReviewDispatch(t *testing.T) {
	ev := events.New(events.ServiceReviewRequested).WithPayload(map[string]any{
		"customer_user_id": int64(42), "brand_id": int64(3),
		"organization_name": "Tech Oto", "review_url": "https://g.page/r/x/review",
		"plate": "34 ABC 123", "service_no": "DS00000001", "service_uuid": "u-1",
	})
	in, ok := serviceReviewDispatch(ev)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventServiceReviewRequest || len(in.UserIDs) != 1 || in.UserIDs[0] != 42 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 || in.ActionURL == nil || *in.ActionURL != "https://g.page/r/x/review" {
		t.Fatalf("brand/action = %v %v", in.BrandID, in.ActionURL)
	}
	if in.Vars["review_url"] != "https://g.page/r/x/review" || in.Vars["organization_name"] != "Tech Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, ok := serviceReviewDispatch(events.New(events.ServiceReviewRequested).WithPayload(map[string]any{
		"customer_user_id": int64(42),
	})); ok {
		t.Fatal("no review_url: want no dispatch")
	}
	if _, ok := serviceReviewDispatch(events.New(events.ServiceReviewRequested).WithPayload(map[string]any{
		"review_url": "https://g.page/r/x/review",
	})); ok {
		t.Fatal("no customer: want no dispatch")
	}
}
