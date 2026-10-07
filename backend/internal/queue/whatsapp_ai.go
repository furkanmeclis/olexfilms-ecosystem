package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TEC-396 (F4-02c): the WhatsApp AI reply of one conversation. Every
// whatsapp.message.received enqueues it with ProcessIn(WhatsAppAIDebounce)
// and Unique for the same window, so messages sent within a few seconds of
// each other become one task (one AI turn over all unanswered messages).
const (
	TaskWhatsAppAIReply = "whatsapp:ai_reply"

	// WhatsAppAIDebounce is the debounce window of a conversation.
	WhatsAppAIDebounce = 4 * time.Second
)

// WhatsAppAIReplyPayload names the conversation. It is the uniqueness key:
// it must not carry anything that differs between messages.
type WhatsAppAIReplyPayload struct {
	ConversationUUID uuid.UUID `json:"conversation_uuid"`
}

// WhatsAppAIReplyFunc runs the AI pipeline of one conversation.
type WhatsAppAIReplyFunc func(ctx context.Context, conversation uuid.UUID) error

// NewWhatsAppAIReplyTask builds the task of a conversation.
func NewWhatsAppAIReplyTask(conversation uuid.UUID) (*asynq.Task, error) {
	body, err := json.Marshal(WhatsAppAIReplyPayload{ConversationUUID: conversation})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal %s: %w", TaskWhatsAppAIReply, err)
	}
	return asynq.NewTask(TaskWhatsAppAIReply, body), nil
}

// WhatsAppAIReplyOpts: whatsapp queue, delayed by the debounce window and
// unique for it (a second message within the window joins the pending
// task), two retries.
func WhatsAppAIReplyOpts() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueWhatsApp),
		asynq.ProcessIn(WhatsAppAIDebounce),
		asynq.Unique(WhatsAppAIDebounce),
		asynq.MaxRetry(2),
		asynq.Timeout(3 * time.Minute),
	}
}

// WhatsAppAIEnqueuer enqueues the AI reply task; a duplicate within the
// debounce window is not an error.
type WhatsAppAIEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueueAIReply enqueues whatsapp:ai_reply for a conversation.
func (e WhatsAppAIEnqueuer) EnqueueAIReply(_ context.Context, conversation uuid.UUID) error {
	if e.Client == nil {
		return fmt.Errorf("queue: client is nil")
	}
	task, err := NewWhatsAppAIReplyTask(conversation)
	if err != nil {
		return err
	}
	if _, err := e.Client.Enqueue(task, WhatsAppAIReplyOpts()...); err != nil &&
		!errors.Is(err, asynq.ErrDuplicateTask) && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithWhatsAppAIReply sets the whatsapp:ai_reply processor; a repeated
// call replaces it.
func (w *Worker) WithWhatsAppAIReply(fn WhatsAppAIReplyFunc) *Worker {
	w.whatsAppAIReply = fn
	return w
}

func (w *Worker) handleWhatsAppAIReply(ctx context.Context, task *asynq.Task) error {
	var p WhatsAppAIReplyPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("queue: unmarshal %s: %w: %w", task.Type(), err, asynq.SkipRetry)
	}
	if w.whatsAppAIReply == nil {
		w.log.Warn("whatsapp_ai_reply_handler_missing", "conversation", p.ConversationUUID)
		return nil
	}
	return w.whatsAppAIReply(ctx, p.ConversationUUID)
}
