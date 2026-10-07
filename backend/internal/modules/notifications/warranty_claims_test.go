package notifications

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

func TestWarrantyClaimDispatch(t *testing.T) {
	base := map[string]any{
		"brand_id": int64(2), "customer_user_id": float64(9), "notify_user_ids": []any{float64(7), float64(0)},
		"claim_uuid": "claim-1", "warranty_uuid": "warranty-1", "claim_no": "42",
		"from": "center_review", "to": "approved", "organization_name": "Tech Oto",
		"product_name": "PPF", "plate": "34 ABC 123",
	}
	in, ok := warrantyClaimDispatch(events.New(events.WarrantyClaimStatusChanged).WithPayload(base))
	if !ok || in.EventCode != catalog.EventWarrantyClaimResult || len(in.UserIDs) != 1 || in.UserIDs[0] != 9 {
		t.Fatalf("result dispatch = %+v, %v", in, ok)
	}
	if in.BrandID == nil || *in.BrandID != 2 || in.Vars["claim_no"] != "42" || in.Vars["to"] != "approved" {
		t.Fatalf("result vars = %+v", in)
	}

	openedPayload := map[string]any{}
	for k, v := range base {
		openedPayload[k] = v
	}
	openedPayload["action"] = "opened"
	openedPayload["to"] = "open"
	opened, ok := warrantyClaimDispatch(events.New(events.WarrantyClaimStatusChanged).WithPayload(openedPayload))
	if !ok || opened.EventCode != catalog.EventWarrantyClaimOpened {
		t.Fatalf("opened dispatch = %+v, %v", opened, ok)
	}

	reopenedPayload := map[string]any{}
	for k, v := range base {
		reopenedPayload[k] = v
	}
	reopenedPayload["action"] = "reopened"
	reopenedPayload["from"] = "closed"
	reopenedPayload["to"] = "approved"
	reopenedPayload["reason"] = "iptal düzeltildi"
	reopened, ok := warrantyClaimDispatch(events.New(events.WarrantyClaimStatusChanged).WithPayload(reopenedPayload))
	if !ok || reopened.EventCode != catalog.EventWarrantyClaimReopened || len(reopened.UserIDs) != 1 || reopened.UserIDs[0] != 7 {
		t.Fatalf("reopened dispatch = %+v, %v", reopened, ok)
	}
	if reopened.Vars["reason"] != "iptal düzeltildi" {
		t.Fatalf("reopened vars = %+v", reopened.Vars)
	}
}
