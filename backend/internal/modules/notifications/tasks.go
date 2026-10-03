package notifications

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TaskEventCodes maps each notified tasks.* outbox event to its catalog
// event (TEC-221).
var TaskEventCodes = map[string]string{
	events.TasksAssigned: catalog.EventTaskAssigned,
	events.TasksDueSoon:  catalog.EventTaskDueSoon,
	events.TasksOverdue:  catalog.EventTaskOverdue,
}

// taskDispatcher maps a tasks.* event to a dispatch for notify_user_ids
// (resolved by the task use case or tasks:due_scan); the brand is the task
// brand (K20). No recipient (self-assignment, nobody to notify) sends
// nothing.
func taskDispatcher(code string) func(events.Event) (notifmodel.DispatchInput, bool) {
	return func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromPayload(event.Payload, "notify_user_ids")
		if len(ids) == 0 {
			return notifmodel.DispatchInput{}, false
		}
		vars := map[string]string{
			"task_title":   stringFromPayload(event.Payload, "title"),
			"subject_name": stringFromPayload(event.Payload, "subject_org_name"),
		}
		if code != catalog.EventTaskAssigned {
			vars["due_date"] = stringFromPayload(event.Payload, "due_date")
		}
		in := notifmodel.DispatchInput{
			EventCode: code, UserIDs: ids, Vars: vars,
			Payload: map[string]any{
				"task_uuid": stringFromPayload(event.Payload, "task_uuid"),
				"priority":  stringFromPayload(event.Payload, "priority"),
				"due_at":    stringFromPayload(event.Payload, "due_at"),
			},
		}
		if code == catalog.EventTaskOverdue {
			in.Priority = notifmodel.PriorityHigh
		}
		if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
			in.BrandID = &brand
		}
		return in, true
	}
}
