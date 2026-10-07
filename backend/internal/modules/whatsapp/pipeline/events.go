package pipeline

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

// ReplyQueue enqueues the debounced AI reply of a conversation
// (queue.WhatsAppAIEnqueuer).
type ReplyQueue interface {
	EnqueueAIReply(ctx context.Context, conversation uuid.UUID) error
}

// RegisterEventHandlers subscribes whatsapp.message.received: every
// inbound message (re)arms the conversation's whatsapp:ai_reply task. It
// runs where the outbox is published (server and worker); a nil queue
// disables it.
func RegisterEventHandlers(bus events.Bus, q ReplyQueue, log *slog.Logger) {
	if bus == nil || q == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.WhatsAppMessageReceived, func(ctx context.Context, ev events.Event) error {
		raw, _ := ev.Payload["conversation_uuid"].(string)
		conv, err := uuid.Parse(raw)
		if err != nil {
			log.WarnContext(ctx, "whatsapp_ai_event_without_conversation", "event_id", ev.EventID)
			return nil
		}
		return q.EnqueueAIReply(ctx, conv)
	})
}
