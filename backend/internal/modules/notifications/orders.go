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

// orderWarehouseNewDispatch tells the seller's warehouse staff that a new
// approved order fell into their warehouse (TEC-1028). The recipients are
// resolved by the orders use case (notify_user_ids: the warehouse members
// holding orders.ship, minus the actor); none sends nothing.
func orderWarehouseNewDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{
		"order_no":       stringFromPayload(event.Payload, "order_no"),
		"buyer_name":     stringFromPayload(event.Payload, "buyer_name"),
		"warehouse_name": stringFromPayload(event.Payload, "warehouse_name"),
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventOrderWarehouseNew, UserIDs: ids, Vars: vars,
		Payload: map[string]any{
			"order_uuid":     stringFromPayload(event.Payload, "order_uuid"),
			"order_no":       vars["order_no"],
			"status":         stringFromPayload(event.Payload, "status"),
			"warehouse_uuid": stringFromPayload(event.Payload, "warehouse_uuid"),
		},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}

// OrderStatusEventCodes maps the order status events that carry recipients
// (notify_user_ids, resolved by the orders use case) to their catalog
// codes: shipped and delivered tell the buyer, received tells the seller.
var OrderStatusEventCodes = map[string]string{
	events.OrdersShipped:   catalog.EventOrderShipped,
	events.OrdersDelivered: catalog.EventOrderDelivered,
	events.OrdersReceived:  catalog.EventOrderReceived,
}

// orderStatusDispatcher builds the dispatch of an order status event; an
// event without recipients (e.g. written before the recipients were
// resolved) sends nothing.
func orderStatusDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
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
			EventCode: code, UserIDs: ids, Vars: vars,
			Payload: map[string]any{
				"order_uuid":      stringFromPayload(event.Payload, "order_uuid"),
				"order_no":        vars["order_no"],
				"status":          stringFromPayload(event.Payload, "status"),
				"tracking_no":     stringFromPayload(event.Payload, "tracking_no"),
				"delivery_method": stringFromPayload(event.Payload, "delivery_method"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
