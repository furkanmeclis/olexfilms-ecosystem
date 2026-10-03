package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TransferEventCodes maps each transfers.* outbox event to its catalog
// event (TEC-200).
var TransferEventCodes = map[string]string{
	events.TransfersRequested: catalog.EventTransferRequested,
	events.TransfersApproved:  catalog.EventTransferApproved,
	events.TransfersRejected:  catalog.EventTransferRejected,
	events.TransfersShipped:   catalog.EventTransferShipped,
	events.TransfersReceived:  catalog.EventTransferReceived,
	events.TransfersCancelled: catalog.EventTransferCancelled,
}

// transferDispatcher maps a transfers.* event to a dispatch for the
// recipients the transfer use case resolved (notify_user_ids); the brand
// is the request brand (K20). No recipient sends nothing.
func transferDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		vars := map[string]string{
			"transfer_no":   stringFromPayload(event.Payload, "transfer_no"),
			"sender_name":   stringFromPayload(event.Payload, "from_org_name"),
			"receiver_name": stringFromPayload(event.Payload, "to_org_name"),
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids, Vars: vars,
			Payload: map[string]any{
				"transfer_uuid": stringFromPayload(event.Payload, "transfer_uuid"),
				"transfer_no":   vars["transfer_no"],
				"status":        stringFromPayload(event.Payload, "status"),
				"kind":          stringFromPayload(event.Payload, "kind"),
			},
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}

// userIDsFromPayload reads a list of user ids (in-process []int64 or the
// JSON-decoded []any of numbers); non-positive entries are dropped.
func userIDsFromPayload(payload map[string]any, key string) []int64 {
	var out []int64
	add := func(n int64) {
		if n > 0 {
			out = append(out, n)
		}
	}
	switch v := payload[key].(type) {
	case []int64:
		for _, n := range v {
			add(n)
		}
	case []any:
		for _, item := range v {
			switch n := item.(type) {
			case int64:
				add(n)
			case int:
				add(int64(n))
			case float64:
				add(int64(n))
			}
		}
	}
	return out
}
