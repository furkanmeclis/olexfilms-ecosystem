package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// stockReorderDispatcher maps stock.reorder_low (TEC-1229) to the
// STOCK_REORDER_LOW notification of the listed users; a meter threshold
// event (measure "meter", TEC-1243) maps to STOCK_REORDER_METERS_LOW.
func stockReorderDispatcher(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	code := catalog.EventStockReorderLow
	measure := stringFromPayload(event.Payload, "measure")
	if measure == "meter" {
		code = catalog.EventStockReorderMetersLow
	}
	in := notifmodel.DispatchInput{
		EventCode: code, UserIDs: ids,
		Vars: map[string]string{
			"product_name":       stringFromPayload(event.Payload, "product_name"),
			"warehouse_name":     stringFromPayload(event.Payload, "warehouse_name"),
			"available_quantity": stringFromPayload(event.Payload, "available_quantity"),
			"available_meters":   stringFromPayload(event.Payload, "available_meters"),
			"threshold":          stringFromPayload(event.Payload, "threshold"),
		},
		Priority: notifmodel.PriorityHigh,
		Payload: map[string]any{
			"rule_uuid":      stringFromPayload(event.Payload, "rule_uuid"),
			"product_uuid":   stringFromPayload(event.Payload, "product_uuid"),
			"warehouse_uuid": stringFromPayload(event.Payload, "warehouse_uuid"),
			"level":          stringFromPayload(event.Payload, "level"),
			"measure":        measure,
		},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}
