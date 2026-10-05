package notifications

import (
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

func measurementDiffDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	count, _ := int64FromPayload(event.Payload, "deviation_count")
	vars := map[string]string{
		"service_no":      stringFromPayload(event.Payload, "service_no"),
		"deviation_count": strconv.FormatInt(count, 10),
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventMeasurementDiffCheckRequired,
		UserIDs:   ids,
		Vars:      vars,
		Payload: map[string]any{
			"service_uuid":    stringFromPayload(event.Payload, "service_uuid"),
			"deviation_count": count,
		},
		Priority: notifmodel.PriorityHigh,
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}
