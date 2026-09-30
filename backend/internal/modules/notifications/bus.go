package notifications

import (
	"context"
	"log/slog"

	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

// RegisterEventHandlers attaches Notification Center listeners to the platform bus.
// Handlers enqueue via Service.Enqueue only — never SMTP.
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
	bus.Subscribe(events.CustomersAssigned, func(ctx context.Context, event events.Event) error {
		return enqueueAssigned(ctx, svc, event, log)
	})
	bus.Subscribe(events.CustomersStatusChanged, func(ctx context.Context, event events.Event) error {
		return enqueueStatusBlocked(ctx, svc, event, log)
	})
	bus.Subscribe(events.ConversationsAssigned, func(ctx context.Context, event events.Event) error {
		return enqueueConversationAssigned(ctx, svc, event, log)
	})
	bus.Subscribe(events.AIPipelineEscalated, func(ctx context.Context, event events.Event) error {
		return enqueueAIEscalated(ctx, svc, event, log)
	})
	bus.Subscribe(events.AIDraftCreated, func(ctx context.Context, event events.Event) error {
		return enqueueAIDraftPending(ctx, svc, event, log)
	})
}

func enqueueAssigned(ctx context.Context, svc *notifusecase.Service, event events.Event, log *slog.Logger) error {
	userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
	if !ok || userID <= 0 {
		return nil
	}
	customerName := customerLabel(event)
	tid := event.TenantID
	_, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
		TenantID:     tid,
		UserID:       &userID,
		Channels:     []string{notifmodel.ChannelInapp},
		Priority:     notifmodel.PriorityNormal,
		Title:        "Müşteri atandı",
		Body:         customerName + " size atandı.",
		TemplateCode: "customers.assigned",
		TemplateVars: map[string]string{"customer_name": customerName},
		SourceEvent:  events.CustomersAssigned,
		Payload: map[string]any{
			"customer_event_uuid": event.Payload["customer_event_uuid"],
			"customer_uuid":       entityUUIDString(event.EntityUUID),
		},
	})
	if err != nil {
		log.Error("notifications_enqueue_assigned_failed", "error", err)
	}
	return nil // fail-soft
}

func enqueueConversationAssigned(ctx context.Context, svc *notifusecase.Service, event events.Event, log *slog.Logger) error {
	userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
	if !ok || userID <= 0 {
		return nil // unassign — no inapp
	}
	tid := event.TenantID
	_, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
		TenantID:     tid,
		UserID:       &userID,
		Channels:     []string{notifmodel.ChannelInapp},
		Priority:     notifmodel.PriorityNormal,
		Title:        "Konuşma atandı",
		Body:         "Bir konuşma size atandı.",
		TemplateCode: "conversations.assigned",
		TemplateVars: map[string]string{},
		SourceEvent:  events.ConversationsAssigned,
		Payload: map[string]any{
			"conversation_uuid": entityUUIDString(event.EntityUUID),
			"assigned_user_id":  userID,
		},
	})
	if err != nil {
		log.Error("notifications_enqueue_conversation_assigned_failed", "error", err)
	}
	return nil // fail-soft
}

func enqueueStatusBlocked(ctx context.Context, svc *notifusecase.Service, event events.Event, log *slog.Logger) error {
	to, _ := event.Payload["to"].(string)
	if to != "blocked" {
		return nil
	}
	userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
	if !ok || userID <= 0 {
		return nil
	}
	customerName := customerLabel(event)
	tid := event.TenantID
	_, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
		TenantID:     tid,
		UserID:       &userID,
		Channels:     []string{notifmodel.ChannelInapp},
		Priority:     notifmodel.PriorityHigh,
		Title:        "Müşteri engellendi",
		Body:         customerName + " durumu blocked olarak güncellendi.",
		TemplateCode: "customers.status_blocked",
		TemplateVars: map[string]string{"customer_name": customerName},
		SourceEvent:  events.CustomersStatusChanged,
		Payload: map[string]any{
			"customer_event_uuid": event.Payload["customer_event_uuid"],
			"customer_uuid":       entityUUIDString(event.EntityUUID),
			"to":                  to,
		},
	})
	if err != nil {
		log.Error("notifications_enqueue_blocked_failed", "error", err)
	}
	return nil
}

func enqueueAIEscalated(ctx context.Context, svc *notifusecase.Service, event events.Event, log *slog.Logger) error {
	userIDs := userIDsFromAIEvent(event)
	if len(userIDs) == 0 {
		return nil
	}
	tid := event.TenantID
	for _, userID := range userIDs {
		uid := userID
		_, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
			TenantID:     tid,
			UserID:       &uid,
			Channels:     []string{notifmodel.ChannelInapp},
			Priority:     notifmodel.PriorityHigh,
			Title:        "AI escalate",
			Body:         "Bir konuşma AI tarafından ekibe iletildi.",
			TemplateCode: "ai.pipeline.escalated",
			TemplateVars: map[string]string{},
			SourceEvent:  events.AIPipelineEscalated,
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"intent":            stringFromPayload(event.Payload, "intent"),
			},
		})
		if err != nil {
			log.Error("notifications_enqueue_ai_escalated_failed", "error", err)
		}
	}
	return nil
}

func enqueueAIDraftPending(ctx context.Context, svc *notifusecase.Service, event events.Event, log *slog.Logger) error {
	userIDs := userIDsFromAIEvent(event)
	if len(userIDs) == 0 {
		return nil
	}
	tid := event.TenantID
	for _, userID := range userIDs {
		uid := userID
		_, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
			TenantID:     tid,
			UserID:       &uid,
			Channels:     []string{notifmodel.ChannelInapp},
			Priority:     notifmodel.PriorityNormal,
			Title:        "AI draft bekliyor",
			Body:         "İncelemeniz için yeni bir AI yanıt taslağı oluşturuldu.",
			TemplateCode: "ai.draft.pending",
			TemplateVars: map[string]string{},
			SourceEvent:  events.AIDraftCreated,
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"draft_uuid":        stringFromPayload(event.Payload, "draft_uuid"),
			},
		})
		if err != nil {
			log.Error("notifications_enqueue_ai_draft_failed", "error", err)
		}
	}
	return nil
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
