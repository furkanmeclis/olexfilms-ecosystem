package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskQuoteReminder sends the single delayed WhatsApp reminder of a quote.
const TaskQuoteReminder = "quotes:reminder"

// QuoteReminderPayload names the reminder row to process.
type QuoteReminderPayload struct {
	ReminderID int64 `json:"reminder_id"`
}

// QuoteReminderFunc runs one reminder row.
type QuoteReminderFunc func(ctx context.Context, reminderID int64) error

// NewQuoteReminderTask builds a quote reminder task.
func NewQuoteReminderTask(reminderID int64) (*asynq.Task, error) {
	body, err := json.Marshal(QuoteReminderPayload{ReminderID: reminderID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal quote reminder: %w", err)
	}
	return asynq.NewTask(TaskQuoteReminder, body), nil
}

// ParseQuoteReminderPayload decodes a quote reminder payload.
func ParseQuoteReminderPayload(data []byte) (QuoteReminderPayload, error) {
	var payload QuoteReminderPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return QuoteReminderPayload{}, fmt.Errorf("queue: unmarshal quote reminder: %w", err)
	}
	return payload, nil
}

// QuoteReminderTaskID deduplicates one pending task per quote.
func QuoteReminderTaskID(quoteID int64) string { return fmt.Sprintf("quote-reminder-%d", quoteID) }

// QuoteReminderOpts are the enqueue options for the delayed reminder.
func QuoteReminderOpts(quoteID int64, delay time.Duration) []asynq.Option {
	if delay < 0 {
		delay = 0
	}
	return []asynq.Option{
		asynq.Queue(QueueNotifications),
		asynq.ProcessIn(delay),
		asynq.TaskID(QuoteReminderTaskID(quoteID)),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
	}
}

// WithQuoteReminder sets the quotes:reminder processor.
func (w *Worker) WithQuoteReminder(fn QuoteReminderFunc) *Worker {
	w.quoteReminder = fn
	return w
}

func (w *Worker) handleQuoteReminder(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseQuoteReminderPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.quoteReminder == nil {
		w.log.Warn("quote_reminder_handler_missing", "reminder_id", payload.ReminderID)
		return nil
	}
	return w.quoteReminder(ctx, payload.ReminderID)
}
