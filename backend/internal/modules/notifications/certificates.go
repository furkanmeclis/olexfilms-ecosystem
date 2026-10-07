package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

var CertificateEventCodes = map[string]string{
	events.CertificatesExpiring: catalog.EventCertificateExpiring,
	events.CertificateExpired:   catalog.EventCertificateExpired,
}

func certificateDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids,
			Vars: map[string]string{
				"type_name":         stringFromPayload(event.Payload, "type_name"),
				"organization_name": stringFromPayload(event.Payload, "organization_name"),
				"expires_date":      stringFromPayload(event.Payload, "expires_date"),
			},
			Payload: map[string]any{
				"certificate_uuid": stringFromPayload(event.Payload, "certificate_uuid"),
				"expires_at":       stringFromPayload(event.Payload, "expires_at"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
