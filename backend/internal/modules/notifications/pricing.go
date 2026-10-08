package notifications

import (
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// PricingEventCodes maps the pricing outbox events to notification codes
// (TEC-506).
var PricingEventCodes = map[string]string{
	events.PricingRecommendedPublished: catalog.EventPricingRecommendedPublished,
	events.PricingDisciplineDigest:     catalog.EventPricingDisciplineDigest,
}

func payloadText(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprint(v)
	}
}

func pricingDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		in := notifmodel.DispatchInput{EventCode: code, UserIDs: ids, Priority: notifmodel.PriorityNormal}
		switch code {
		case catalog.EventPricingRecommendedPublished:
			in.Vars = map[string]string{
				"price_count":    payloadText(event.Payload, "price_count"),
				"currencies":     payloadText(event.Payload, "currencies"),
				"effective_from": payloadText(event.Payload, "effective_from"),
			}
			in.Payload = map[string]any{"batch_id": payloadText(event.Payload, "batch_id")}
		default:
			in.Vars = map[string]string{
				"snapshot_date": payloadText(event.Payload, "snapshot_date"),
				"org_count":     payloadText(event.Payload, "org_count"),
				"threshold_pct": payloadText(event.Payload, "threshold_pct"),
				"org_names":     payloadText(event.Payload, "org_names"),
			}
			in.Payload = map[string]any{"snapshot_date": payloadText(event.Payload, "snapshot_date")}
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
