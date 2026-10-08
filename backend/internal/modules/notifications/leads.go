package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// leadApplicationDispatch maps leads.application_received (TEC-317) to the
// dealer application notification for notify_user_ids (members of the
// receiving organization holding leads.read); the brand is the lead brand.
// No recipient sends nothing.
func leadApplicationDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventLeadDealerApplication, UserIDs: ids,
		Vars: map[string]string{
			"company_name": stringFromPayload(event.Payload, "company_name"),
			"contact_name": stringFromPayload(event.Payload, "contact_name"),
			"location":     stringFromPayload(event.Payload, "location"),
		},
		Payload: map[string]any{
			"lead_uuid": stringFromPayload(event.Payload, "lead_uuid"),
			"routed_by": stringFromPayload(event.Payload, "routed_by"),
		},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}

func leadWebsiteDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventLeadWebsiteReceived, UserIDs: ids,
		Vars: map[string]string{
			"contact_name": stringFromPayload(event.Payload, "contact_name"),
			"phone":        stringFromPayload(event.Payload, "phone"),
			"dealer_code":  stringFromPayload(event.Payload, "dealer_code"),
		},
		Payload: map[string]any{"lead_uuid": stringFromPayload(event.Payload, "lead_uuid")},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}
