package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-473: fleet.* events go to notify_user_ids with the fleet and dealer
// names; no recipient sends nothing; fleet.created / fleet.vehicle_added
// notify nobody.
func TestFleetDispatch(t *testing.T) {
	if len(FleetEventCodes) != 4 {
		t.Fatalf("codes = %v", FleetEventCodes)
	}
	for _, silent := range []string{events.FleetCreated, events.FleetVehicleAdded} {
		if _, ok := FleetEventCodes[silent]; ok {
			t.Fatalf("%s must not notify", silent)
		}
	}
	for name, code := range FleetEventCodes {
		ev := events.New(name).WithPayload(map[string]any{
			"notify_user_ids": []any{float64(5)}, "brand_id": float64(2),
			"fleet_uuid": "f-1", "link_uuid": "l-1", "fleet_name": "Filo A", "dealer_name": "Tech Oto",
		})
		in, ok := fleetDispatcher(code)(ev)
		if !ok || in.EventCode != code || !slices.Equal(in.UserIDs, []int64{5}) {
			t.Fatalf("%s: dispatch = %+v %v", name, in, ok)
		}
		if in.BrandID == nil || *in.BrandID != 2 {
			t.Fatalf("%s: brand = %v", name, in.BrandID)
		}
		if in.Vars["fleet_name"] != "Filo A" || in.Vars["dealer_name"] != "Tech Oto" || in.Payload["link_uuid"] != "l-1" {
			t.Fatalf("%s: vars = %v payload = %v", name, in.Vars, in.Payload)
		}
		if _, ok := fleetDispatcher(code)(events.New(name).WithPayload(map[string]any{})); ok {
			t.Fatalf("%s: no recipient must send nothing", name)
		}
	}
}
