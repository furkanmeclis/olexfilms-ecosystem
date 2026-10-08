package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

var StockForecastEventCodes = map[string]string{
	events.StockForecastLow: catalog.EventStockForecastLow,
}

func stockForecastDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids,
			Vars: map[string]string{
				"product_name":      stringFromPayload(event.Payload, "product_name"),
				"organization_name": stringFromPayload(event.Payload, "organization_name"),
				"days_left":         stringFromPayload(event.Payload, "days_left"),
				"status":            stringFromPayload(event.Payload, "status"),
			},
			Priority: notifmodel.PriorityHigh,
			Payload: map[string]any{
				"forecast_uuid": stringFromPayload(event.Payload, "forecast_uuid"),
				"product_uuid":  stringFromPayload(event.Payload, "product_uuid"),
				"status":        stringFromPayload(event.Payload, "status"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
