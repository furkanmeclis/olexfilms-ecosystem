package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-406: campaigns.* events go to notify_user_ids with the campaign and
// organization names; rejections and change requests carry the reason; no
// recipient sends nothing.
func TestCampaignDispatch(t *testing.T) {
	if len(CampaignEventCodes) != 4 {
		t.Fatalf("codes = %v", CampaignEventCodes)
	}
	for name, code := range CampaignEventCodes {
		ev := events.New(name).WithPayload(map[string]any{
			"notify_user_ids": []any{float64(7), float64(8)}, "brand_id": float64(3),
			"campaign_uuid": "c-1", "campaign_name": "Kış", "organization_name": "Tech Oto",
			"status": "rejected", "reason": "Görsel eksik",
		})
		in, ok := campaignDispatcher(code)(ev)
		if !ok {
			t.Fatalf("%s: want a dispatch", name)
		}
		if in.EventCode != code || !slices.Equal(in.UserIDs, []int64{7, 8}) {
			t.Fatalf("%s: dispatch = %+v", name, in)
		}
		if in.BrandID == nil || *in.BrandID != 3 {
			t.Fatalf("%s: brand = %v", name, in.BrandID)
		}
		if in.Vars["campaign_name"] != "Kış" || in.Vars["organization_name"] != "Tech Oto" || in.Payload["campaign_uuid"] != "c-1" {
			t.Fatalf("%s: vars = %v payload = %v", name, in.Vars, in.Payload)
		}
		withReason := code == catalog.EventCampaignRejected || code == catalog.EventCampaignChangesRequested
		if _, has := in.Vars["reason"]; has != withReason {
			t.Fatalf("%s: reason var = %v", name, in.Vars)
		}
		if _, ok := campaignDispatcher(code)(events.New(name).WithPayload(map[string]any{})); ok {
			t.Fatalf("%s: no recipient must send nothing", name)
		}
	}
}
