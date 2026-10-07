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

// TEC-395 (F4-02d): WhatsApp conversation messages. Outgoing messages are
// stored first (status queued) and sent by whatsapp:send; inbound media is
// copied to object storage by whatsapp:media_store. Both run on the
// whatsapp queue of the critical worker group.
const (
	TaskWhatsAppSend       = "whatsapp:send"
	TaskWhatsAppMediaStore = "whatsapp:media_store"
	// TaskWhatsAppQueueSweep re-enqueues messages whose send task was lost.
	TaskWhatsAppQueueSweep = "app:whatsapp:queue_sweep"

	QueueWhatsApp = "whatsapp"

	// WhatsAppSendMaxRetry: one try plus three retries, then failed.
	WhatsAppSendMaxRetry = 3

	whatsAppQueueSweepCron = "@every 2m"
)

// WhatsAppMessagePayload names the message of a send / media task.
type WhatsAppMessagePayload struct {
	MessageID   int64     `json:"message_id"`
	MessageUUID uuid.UUID `json:"message_uuid"`
}

// WhatsAppMessageFunc processes one message; final is true on the last
// retry (the processor then records the failure instead of retrying).
type WhatsAppMessageFunc func(ctx context.Context, messageID int64, final bool) error

// WhatsAppQueueSweepFunc runs one stale queue sweep.
type WhatsAppQueueSweepFunc func(ctx context.Context) (int, error)

func newWhatsAppMessageTask(typ string, id int64, u uuid.UUID) (*asynq.Task, error) {
	body, err := json.Marshal(WhatsAppMessagePayload{MessageID: id, MessageUUID: u})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal %s: %w", typ, err)
	}
	return asynq.NewTask(typ, body), nil
}

// WhatsAppSendOpts: whatsapp queue, task id = message uuid (one pending
// send per message), three retries.
func WhatsAppSendOpts(u uuid.UUID) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueWhatsApp),
		asynq.TaskID("wa-send-" + u.String()),
		asynq.MaxRetry(WhatsAppSendMaxRetry),
		asynq.Timeout(2 * time.Minute),
	}
}

// WhatsAppMediaStoreOpts: whatsapp queue, one pending task per message.
func WhatsAppMediaStoreOpts(u uuid.UUID) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueWhatsApp),
		asynq.TaskID("wa-media-" + u.String()),
		asynq.MaxRetry(3),
		asynq.Timeout(3 * time.Minute),
	}
}

// NewWhatsAppQueueSweepTask builds the periodic sweep task.
func NewWhatsAppQueueSweepTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskWhatsAppQueueSweep, []byte("{}"), asynq.MaxRetry(0)), nil
}

// WhatsAppEnqueuer implements the WhatsApp use case's SendQueue. A task id
// conflict (the task is already pending) is not an error.
type WhatsAppEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueueSend enqueues whatsapp:send for a queued message.
func (e WhatsAppEnqueuer) EnqueueSend(_ context.Context, id int64, u uuid.UUID) error {
	return e.enqueue(TaskWhatsAppSend, id, u, WhatsAppSendOpts(u))
}

// EnqueueMediaStore enqueues whatsapp:media_store for an inbound message.
func (e WhatsAppEnqueuer) EnqueueMediaStore(_ context.Context, id int64, u uuid.UUID) error {
	return e.enqueue(TaskWhatsAppMediaStore, id, u, WhatsAppMediaStoreOpts(u))
}

func (e WhatsAppEnqueuer) enqueue(typ string, id int64, u uuid.UUID, opts []asynq.Option) error {
	if e.Client == nil {
		return fmt.Errorf("queue: client is nil")
	}
	task, err := newWhatsAppMessageTask(typ, id, u)
	if err != nil {
		return err
	}
	if _, err := e.Client.Enqueue(task, opts...); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithWhatsAppMessaging sets the send, media and sweep processors; a
// repeated call replaces them.
func (w *Worker) WithWhatsAppMessaging(send, media WhatsAppMessageFunc, sweep WhatsAppQueueSweepFunc) *Worker {
	w.whatsAppSend, w.whatsAppMedia, w.whatsAppSweep = send, media, sweep
	return w
}

// finalAttempt reports whether the running task has no retry left.
func finalAttempt(ctx context.Context) bool {
	n, ok1 := asynq.GetRetryCount(ctx)
	max, ok2 := asynq.GetMaxRetry(ctx)
	return ok1 && ok2 && n >= max
}

func (w *Worker) handleWhatsAppMessage(fn WhatsAppMessageFunc, name string) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		var p WhatsAppMessagePayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("queue: unmarshal %s: %w: %w", task.Type(), err, asynq.SkipRetry)
		}
		if fn == nil {
			w.log.Warn(name+"_handler_missing", "message_id", p.MessageID)
			return nil
		}
		return fn(ctx, p.MessageID, finalAttempt(ctx))
	}
}

func (w *Worker) handleWhatsAppSend(ctx context.Context, task *asynq.Task) error {
	return w.handleWhatsAppMessage(w.whatsAppSend, "whatsapp_send")(ctx, task)
}

func (w *Worker) handleWhatsAppMediaStore(ctx context.Context, task *asynq.Task) error {
	return w.handleWhatsAppMessage(w.whatsAppMedia, "whatsapp_media_store")(ctx, task)
}

func (w *Worker) handleWhatsAppQueueSweep(ctx context.Context, _ *asynq.Task) error {
	if w.whatsAppSweep == nil {
		return nil
	}
	n, err := w.whatsAppSweep(ctx)
	if err == nil && n > 0 {
		w.log.Info("whatsapp_queue_requeued", "messages", n)
	}
	return err
}
