package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// ShowcaseEventCodes maps each notified showcase.* outbox event to its
// catalog event (TEC-467). showcase.published notifies only after a center
// approval (a direct publish carries no recipients).
var ShowcaseEventCodes = map[string]string{
	events.ShowcaseReviewRequested: catalog.EventShowcaseReviewRequested,
	events.ShowcasePublished:       catalog.EventShowcaseApproved,
	events.ShowcaseRejected:        catalog.EventShowcaseRejected,
}

// showcaseDispatcher maps a showcase.* event to a dispatch for
// notify_user_ids (the center reviewers for a submission, the dealer owners
// for a decision; resolved by the showcase use case). No recipient sends
// nothing.
func showcaseDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		vars := map[string]string{"organization_name": stringFromPayload(event.Payload, "organization_name")}
		if code == catalog.EventShowcaseRejected {
			vars["reason"] = stringFromPayload(event.Payload, "reason")
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids, Vars: vars,
			Payload: map[string]any{
				"organization_uuid": stringFromPayload(event.Payload, "organization_uuid"),
				"status":            stringFromPayload(event.Payload, "status"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
