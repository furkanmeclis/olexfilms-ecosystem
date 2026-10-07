package notifications

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

type fakeQuotaRecipients struct {
	ids  []int64
	asks []db.ListAIQuotaNotifyUserIDsParams
}

func (f *fakeQuotaRecipients) ListAIQuotaNotifyUserIDs(_ context.Context, arg db.ListAIQuotaNotifyUserIDsParams) ([]int64, error) {
	f.asks = append(f.asks, arg)
	return f.ids, nil
}

func (f *fakeQuotaRecipients) GetOrganizationByID(_ context.Context, id int64) (db.Organization, error) {
	return db.Organization{ID: id, Uuid: uuid.New(), Name: "Tech Oto", BrandID: 3}, nil
}

// TEC-389: ai.quota.threshold goes to the resolved recipients with the
// organization name and the numbers; 100 % is high priority; a missing
// pool or no recipient sends nothing.
func TestAIQuotaThresholdDispatch(t *testing.T) {
	r := &fakeQuotaRecipients{ids: []int64{4, 5}}
	ev := events.New(events.AIQuotaThreshold).WithPayload(map[string]any{
		"organization_id": float64(9), "pool": "org", "period": "2026-10",
		"threshold": float64(80), "used_tokens": float64(1600000), "quota_tokens": float64(2000000),
	})
	in, ok, err := aiQuotaThresholdDispatch(context.Background(), r, ev)
	if err != nil || !ok {
		t.Fatalf("dispatch ok=%v err=%v", ok, err)
	}
	if in.EventCode != catalog.EventAIQuotaThreshold || !slices.Equal(in.UserIDs, []int64{4, 5}) ||
		in.BrandID == nil || *in.BrandID != 3 || in.Priority == notifmodel.PriorityHigh {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.Vars["organization_name"] != "Tech Oto" || in.Vars["threshold"] != "80" ||
		in.Vars["used_tokens"] != "1600000" || in.Vars["quota_tokens"] != "2000000" || in.Vars["period"] != "2026-10" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if len(r.asks) != 1 || r.asks[0].Pool != "org" || r.asks[0].OrganizationID != 9 {
		t.Fatalf("recipient lookup = %+v", r.asks)
	}
	ev.Payload["threshold"] = float64(100)
	if in, _, _ := aiQuotaThresholdDispatch(context.Background(), r, ev); in.Priority != notifmodel.PriorityHigh {
		t.Fatalf("100 %% priority = %q", in.Priority)
	}
	if _, ok, _ := aiQuotaThresholdDispatch(context.Background(), &fakeQuotaRecipients{}, ev); ok {
		t.Fatal("no recipient must send nothing")
	}
	ev.Payload["pool"] = "other"
	if _, ok, _ := aiQuotaThresholdDispatch(context.Background(), r, ev); ok {
		t.Fatal("unknown pool must send nothing")
	}
}
