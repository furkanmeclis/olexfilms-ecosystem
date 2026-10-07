package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-467: showcase.* events go to notify_user_ids with the organization
// name; a rejection carries the note; a direct publish (no recipient) sends
// nothing.
func TestShowcaseDispatch(t *testing.T) {
	if len(ShowcaseEventCodes) != 3 {
		t.Fatalf("codes = %v", ShowcaseEventCodes)
	}
	for name, code := range ShowcaseEventCodes {
		ev := events.New(name).WithPayload(map[string]any{
			"notify_user_ids": []any{float64(5)}, "brand_id": float64(2),
			"organization_uuid": "o-1", "organization_name": "Tech Oto", "status": "rejected", "reason": "Logo eksik",
		})
		in, ok := showcaseDispatcher(code)(ev)
		if !ok || in.EventCode != code || !slices.Equal(in.UserIDs, []int64{5}) || in.BrandID == nil || *in.BrandID != 2 {
			t.Fatalf("%s: dispatch = %+v", name, in)
		}
		if in.Vars["organization_name"] != "Tech Oto" || in.Payload["organization_uuid"] != "o-1" {
			t.Fatalf("%s: vars = %v payload = %v", name, in.Vars, in.Payload)
		}
		if _, has := in.Vars["reason"]; has != (code == catalog.EventShowcaseRejected) {
			t.Fatalf("%s: reason var = %v", name, in.Vars)
		}
		if _, ok := showcaseDispatcher(code)(events.New(name).WithPayload(map[string]any{"notify_user_ids": []any{}})); ok {
			t.Fatalf("%s: no recipient must send nothing", name)
		}
	}
}
