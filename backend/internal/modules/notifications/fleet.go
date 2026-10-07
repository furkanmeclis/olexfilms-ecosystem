package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// FleetEventCodes maps each notified fleet.* outbox event to its catalog
// event (TEC-473).
var FleetEventCodes = map[string]string{
	events.FleetLinkRequested: catalog.EventFleetLinkRequested,
	events.FleetLinked:        catalog.EventFleetLinkAccepted,
	events.FleetLinkRejected:  catalog.EventFleetLinkRejected,
	events.FleetUserInvited:   catalog.EventFleetUserInvited,
}

// fleetDispatcher maps a fleet.* event to a dispatch for notify_user_ids
// (the fleet users for a link request, the requesting dealer user for a
// decision, the invited user; resolved by the fleet use case). No
// recipient sends nothing.
func fleetDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids,
			Vars: map[string]string{
				"fleet_name":  stringFromPayload(event.Payload, "fleet_name"),
				"dealer_name": stringFromPayload(event.Payload, "dealer_name"),
			},
			Payload: map[string]any{
				"fleet_uuid": stringFromPayload(event.Payload, "fleet_uuid"),
				"link_uuid":  stringFromPayload(event.Payload, "link_uuid"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
