package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

var ServiceSubscriptionEventCodes = map[string]string{
	events.ServiceSubscriptionAssigned:        catalog.EventServiceSubscriptionAssigned,
	events.ServiceSubscriptionCancelRequested: catalog.EventServiceSubscriptionCancelRequested,
	events.ServiceSubscriptionCancelled:       catalog.EventServiceSubscriptionCancelled,
	events.ServiceSubscriptionCancelRejected:  catalog.EventServiceSubscriptionCancelRejected,
}

func serviceSubscriptionDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		vars := map[string]string{}
		for _, k := range []string{"item_name", "organization_name", "ends_on", "reason", "cancellation_fee", "currency"} {
			vars[k] = stringFromPayload(event.Payload, k)
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids, Vars: vars,
			Payload: map[string]any{
				"subscription_uuid": stringFromPayload(event.Payload, "subscription_uuid"),
				"item_uuid":         stringFromPayload(event.Payload, "item_uuid"),
				"status":            stringFromPayload(event.Payload, "status"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
