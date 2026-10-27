package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// orderCreatedBySellerDispatch tells the dealer's members that the center
// opened an approved order for them (TEC-1024). The recipients are resolved
// by the orders use case (notify_user_ids); none sends nothing.
func orderCreatedBySellerDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{
		"order_no":    stringFromPayload(event.Payload, "order_no"),
		"seller_name": stringFromPayload(event.Payload, "seller_name"),
		"buyer_name":  stringFromPayload(event.Payload, "buyer_name"),
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventOrderCreatedBySeller, UserIDs: ids, Vars: vars,
		Payload: map[string]any{
			"order_uuid": stringFromPayload(event.Payload, "order_uuid"),
			"order_no":   vars["order_no"],
			"status":     stringFromPayload(event.Payload, "status"),
		},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}
