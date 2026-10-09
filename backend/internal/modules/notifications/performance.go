package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

var PerformanceEventCodes = map[string]string{
	events.PerformanceWeakDealer:  catalog.EventPerformanceWeakDealer,
	events.PerformanceBelowTarget: catalog.EventPerformanceBelowTarget,
}

func performanceDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids,
			Vars: map[string]string{
				"dealer_name":  stringFromPayload(event.Payload, "dealer_name"),
				"rule_name":    stringFromPayload(event.Payload, "rule_name"),
				"period":       stringFromPayload(event.Payload, "period"),
				"metric":       stringFromPayload(event.Payload, "metric"),
				"metric_value": stringFromPayload(event.Payload, "metric_value"),
				"threshold":    stringFromPayload(event.Payload, "threshold"),
			},
			Payload: map[string]any{
				"rule_uuid":   stringFromPayload(event.Payload, "rule_uuid"),
				"dealer_uuid": stringFromPayload(event.Payload, "dealer_uuid"),
				"period":      stringFromPayload(event.Payload, "period"),
			},
			ActionURL: performanceStringPtr(stringFromPayload(event.Payload, "panel_url")),
		}
		if code == catalog.EventPerformanceWeakDealer {
			in.Priority = notifmodel.PriorityHigh
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}

func performanceStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
