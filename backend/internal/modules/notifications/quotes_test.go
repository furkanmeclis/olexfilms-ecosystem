package notifications

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

type fakeQuoteSender struct {
	dispatched []notifmodel.DispatchInput
	enqueued   []notifmodel.EnqueueInput
}

func (f *fakeQuoteSender) Dispatch(_ context.Context, in notifmodel.DispatchInput) (notifmodel.DispatchResult, error) {
	f.dispatched = append(f.dispatched, in)
	return notifmodel.DispatchResult{}, nil
}

func (f *fakeQuoteSender) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	f.enqueued = append(f.enqueued, in)
	return nil, nil
}

// TEC-1034: quote.sent reaches the customer user through Dispatch (in-app +
// WhatsApp); a candidate phone keeps its direct WhatsApp, a lead without a
// user only gets WhatsApp.
func TestQuoteSentNotify(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"quote_uuid": "q-1", "display_no": "TK000001", "brand_id": float64(3),
			"recipient_phone": "+905551112233", "recipient_user_id": float64(42),
			"recipient_phone_is_customer": true, "recipient_name": "Ayşe", "organization_name": "Tech Oto",
			"quote_url": "https://olexfilms.app/s/AbCdEfGhIj", "total_amount": "100.00 TRY",
		}
	}
	org := int64(7)
	send := func(p map[string]any) *fakeQuoteSender {
		f := &fakeQuoteSender{}
		ev := events.New(events.QuoteSent).WithTenant(org).WithPayload(p)
		if err := quoteSentNotify(context.Background(), f, ev); err != nil {
			t.Fatal(err)
		}
		return f
	}

	f := send(base())
	if len(f.dispatched) != 1 || len(f.enqueued) != 0 {
		t.Fatalf("customer phone: dispatched %d enqueued %d", len(f.dispatched), len(f.enqueued))
	}
	in := f.dispatched[0]
	if in.EventCode != catalog.EventQuoteSent || !slices.Equal(in.UserIDs, []int64{42}) || in.Channels != nil {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.ActionURL == nil || *in.ActionURL != "https://olexfilms.app/s/AbCdEfGhIj" || in.BrandID == nil || *in.BrandID != 3 ||
		in.OrganizationID == nil || *in.OrganizationID != org || in.Vars["total_amount"] != "100.00 TRY" {
		t.Fatalf("dispatch = %+v", in)
	}

	p := base()
	p["recipient_phone_is_customer"] = false
	f = send(p)
	if len(f.dispatched) != 1 || !slices.Equal(f.dispatched[0].Channels, []string{notifmodel.ChannelInapp}) {
		t.Fatalf("candidate phone dispatch = %+v", f.dispatched)
	}
	if len(f.enqueued) != 1 || *f.enqueued[0].Recipient != "+905551112233" || f.enqueued[0].UserID == nil {
		t.Fatalf("candidate phone enqueue = %+v", f.enqueued)
	}

	p = base()
	delete(p, "recipient_user_id")
	delete(p, "recipient_phone_is_customer")
	f = send(p)
	if len(f.dispatched) != 0 || len(f.enqueued) != 1 || f.enqueued[0].UserID != nil {
		t.Fatalf("no user: dispatched %d enqueued %+v", len(f.dispatched), f.enqueued)
	}
}
