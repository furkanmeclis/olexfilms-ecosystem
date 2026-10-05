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
		"form_url": "https://olexfilms.app/s/AbCdEfGhIj",
		"plate":    "34 ABC 123", "service_no": "DS00000001", "service_uuid": "u-1",
	})
	in, ok := serviceReviewDispatch(ev)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventServiceReviewRequest || len(in.UserIDs) != 1 || in.UserIDs[0] != 42 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 || in.ActionURL == nil || *in.ActionURL != "https://olexfilms.app/s/AbCdEfGhIj" {
		t.Fatalf("brand/action = %v %v", in.BrandID, in.ActionURL)
	}
	if in.Vars["review_url"] != "https://g.page/r/x/review" || in.Vars["form_url"] != "https://olexfilms.app/s/AbCdEfGhIj" ||
		in.Vars["organization_name"] != "Tech Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, ok := serviceReviewDispatch(events.New(events.ServiceReviewRequested).WithPayload(map[string]any{
		"customer_user_id": int64(42),
	})); ok {
		t.Fatal("no form_url: want no dispatch")
	}
	if _, ok := serviceReviewDispatch(events.New(events.ServiceReviewRequested).WithPayload(map[string]any{
		"form_url": "https://olexfilms.app/s/AbCdEfGhIj",
	})); ok {
		t.Fatal("no customer: want no dispatch")
	}
}

func TestServiceReviewLowScoreDispatch(t *testing.T) {
	ev := events.New(events.ServiceReviewLowScore).WithPayload(map[string]any{
		"brand_id": int64(4), "notify_user_ids": []any{float64(10), float64(11)},
		"review_uuid": "review-1", "service_uuid": "service-1", "service_no": "DS000001",
		"organization_name": "Dealer One", "platform_rating": "5", "product_rating": "1", "min_rating": "1",
	})
	in, ok := serviceReviewLowScoreDispatch(ev)
	if !ok || in.EventCode != catalog.EventServiceReviewLowScore || len(in.UserIDs) != 2 {
		t.Fatalf("dispatch = %+v, %v", in, ok)
	}
	if in.BrandID == nil || *in.BrandID != 4 || in.Vars["product_rating"] != "1" || in.Payload["service_no"] != "DS000001" {
		t.Fatalf("payload = %+v", in)
	}
}

func TestAppointmentDispatch(t *testing.T) {
	ev := events.New(events.AppointmentReminder).WithPayload(map[string]any{
		"customer_user_id": int64(42), "brand_id": int64(3), "appointment_uuid": "u-1",
		"organization_name": "Tech Oto", "starts_at": "2026-10-06T09:00:00Z", "plate": "34 ABC 123",
	})
	in, ok := appointmentDispatcher(catalog.EventAppointmentReminder)(ev)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventAppointmentReminder || len(in.UserIDs) != 1 || in.UserIDs[0] != 42 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["organization_name"] != "Tech Oto" || in.Vars["starts_at"] != "2026-10-06T09:00:00Z" ||
		in.Vars["plate"] != "34 ABC 123" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, ok := appointmentDispatcher(catalog.EventAppointmentCreated)(events.New(events.AppointmentCreated)); ok {
		t.Fatal("no customer: want no dispatch")
	}
}

// TEC-164: customer.created (as the outbox redelivers it: JSON numbers)
// becomes the welcome dispatch with the portal link; no phone, no message.
func TestCustomerWelcomeDispatch(t *testing.T) {
	payload := map[string]any{
		"customer_user_id": float64(42), "brand_id": float64(3), "customer_uuid": "u-1",
		"customer_name": "Ahmet Yilmaz", "organization_name": "Tech Oto",
		"portal_url": "https://olexfilms.app/portal", "has_phone": true,
	}
	in, ok := customerWelcomeDispatch(events.New(events.CustomerCreated).WithPayload(payload))
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventCustomerWelcome || len(in.UserIDs) != 1 || in.UserIDs[0] != 42 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 || in.ActionURL == nil || *in.ActionURL != "https://olexfilms.app/portal" {
		t.Fatalf("brand/action = %v %v", in.BrandID, in.ActionURL)
	}
	if in.Vars["portal_url"] != "https://olexfilms.app/portal" || in.Vars["customer_name"] != "Ahmet Yilmaz" ||
		in.Vars["organization_name"] != "Tech Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	noPhone := map[string]any{}
	for k, v := range payload {
		noPhone[k] = v
	}
	noPhone["has_phone"] = false
	if _, ok := customerWelcomeDispatch(events.New(events.CustomerCreated).WithPayload(noPhone)); ok {
		t.Fatal("no phone: want no dispatch")
	}
	if _, ok := customerWelcomeDispatch(events.New(events.CustomerCreated).WithPayload(map[string]any{
		"customer_user_id": int64(42), "has_phone": true,
	})); ok {
		t.Fatal("no portal_url: want no dispatch")
	}
}
