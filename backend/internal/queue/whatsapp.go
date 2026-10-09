package queue

import (
	"context"

	"github.com/hibiken/asynq"
)

// TaskWhatsAppStatusPoll refreshes the wuzapi session status every minute,
// reconnects a dropped websocket and raises logout alarms (TEC-92).
const TaskWhatsAppStatusPoll = "app:whatsapp:status_poll"

const whatsAppPollCron = "@every 1m"

// WhatsAppPollFunc runs one status poll.
type WhatsAppPollFunc func(ctx context.Context) error

// NewWhatsAppStatusPollTask builds the periodic poll task.
func NewWhatsAppStatusPollTask() *asynq.Task {
	// No retries: the next minute's poll replaces a failed one.
	return asynq.NewTask(TaskWhatsAppStatusPoll, []byte("{}"), asynq.MaxRetry(0))
}

// WithWhatsAppPoll sets the poll processor. The task handler itself is
// registered once in NewWorkerWithQueues, so a repeated call replaces fn.
func (w *Worker) WithWhatsAppPoll(fn WhatsAppPollFunc) *Worker {
	w.pollWhatsApp = fn
	return w
}

func (w *Worker) handleWhatsAppPoll(ctx context.Context, _ *asynq.Task) error {
	if w.pollWhatsApp == nil {
		w.log.Warn("whatsapp_poll_handler_missing")
		return nil
	}
	return w.pollWhatsApp(ctx)
}
