package notifications

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

// RegisterEventHandlers attaches Notification Center listeners to the
// platform bus. Each outbox event maps to a catalog event; the outbox event
// id is the delivery idempotency key, so a redelivered event sends nothing
// twice. Titles and bodies come from notification_templates (event x role x
// channel x language); nothing is hard-coded here.
func RegisterEventHandlers(bus events.Bus, svc *notifusecase.Service, log *slog.Logger) {
	if bus == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe("customers.*", func(_ context.Context, event events.Event) error {
		log.Info(
			"notifications_bus_observed",
			"event", event.Name,
			"event_id", event.EventID.String(),
			"entity_type", event.EntityType,
			"customer_event_uuid", event.Payload["customer_event_uuid"],
		)
		return nil
	})
	if svc == nil {
		return
	}
	on := func(name string, build func(events.Event) (notifmodel.DispatchInput, bool)) {
		bus.Subscribe(name, func(ctx context.Context, event events.Event) error {
			in, ok := build(event)
			if !ok {
				return nil
			}
			in.EventID = event.EventID
			in.OrganizationID = event.TenantID
			if _, err := svc.Dispatch(ctx, in); err != nil {
				log.Error("notifications_dispatch_failed", "event", event.Name, "event_id", event.EventID, "error", err)
			}
			return nil // fail-soft: the bus must not block on a notification
		})
	}
	on(events.CustomersAssigned, func(event events.Event) (notifmodel.DispatchInput, bool) {
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventCustomersAssigned, UserIDs: []int64{userID},
			Vars: map[string]string{"customer_name": customerLabel(event)},
			Payload: map[string]any{
				"customer_event_uuid": event.Payload["customer_event_uuid"],
				"customer_uuid":       entityUUIDString(event.EntityUUID),
			},
		}, true
	})
	on(events.CustomersStatusChanged, func(event events.Event) (notifmodel.DispatchInput, bool) {
		to, _ := event.Payload["to"].(string)
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if to != "blocked" || !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventCustomersStatusBlocked, UserIDs: []int64{userID},
			Priority: notifmodel.PriorityHigh,
			Vars:     map[string]string{"customer_name": customerLabel(event)},
			Payload: map[string]any{
				"customer_event_uuid": event.Payload["customer_event_uuid"],
				"customer_uuid":       entityUUIDString(event.EntityUUID),
				"to":                  to,
			},
		}, true
	})
	on(events.ConversationsAssigned, func(event events.Event) (notifmodel.DispatchInput, bool) {
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false // unassign
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventConversationsAssigned, UserIDs: []int64{userID},
			Payload: map[string]any{"conversation_uuid": entityUUIDString(event.EntityUUID), "assigned_user_id": userID},
		}, true
	})
	on(events.AIPipelineEscalated, func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromAIEvent(event)
		return notifmodel.DispatchInput{
			EventCode: catalog.EventAIPipelineEscalated, UserIDs: ids, Priority: notifmodel.PriorityHigh,
			Vars: map[string]string{"intent": stringFromPayload(event.Payload, "intent")},
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"intent":            stringFromPayload(event.Payload, "intent"),
			},
		}, len(ids) > 0
	})
	on(events.AIDraftCreated, func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromAIEvent(event)
		return notifmodel.DispatchInput{
			EventCode: catalog.EventAIDraftPending, UserIDs: ids,
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"draft_uuid":        stringFromPayload(event.Payload, "draft_uuid"),
			},
		}, len(ids) > 0
	})
}

// userIDsFromAIEvent prefers assigned_user_id, else notify_user_ids slice in payload.
func userIDsFromAIEvent(event events.Event) []int64 {
	if id, ok := int64FromPayload(event.Payload, "assigned_user_id"); ok && id > 0 {
		return []int64{id}
	}
	raw, ok := event.Payload["notify_user_ids"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []int64:
		return v
	case []any:
		out := make([]int64, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case int64:
				out = append(out, n)
			case int:
				out = append(out, int64(n))
			case float64:
				out = append(out, int64(n))
			}
		}
		return out
	default:
		return nil
	}
}

func customerLabel(event events.Event) string {
	if name := stringFromPayload(event.Payload, "display_name"); name != "" {
		return name
	}
	if event.EntityUUID != nil {
		return event.EntityUUID.String()
	}
	return "Customer"
}

func entityUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func int64FromPayload(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, _ := payload[key].(string)
	return v
}
