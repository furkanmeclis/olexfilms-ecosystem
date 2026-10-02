package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// Warranty periodic tasks (TEC-187, F1-06c).
const (
	// TaskWarrantyExpire marks warranties past end_at as expired and writes
	// warranty.expired events.
	TaskWarrantyExpire = "warranty:expire"
	// TaskWarrantyExpiringScan writes the 30 / 7 day warranty.expiring_soon
	// reminders.
	TaskWarrantyExpiringScan = "warranty:expiring_scan"
)

// end_at is the end of the last covered day in each organization's own
// time zone (decision 4), and organizations span several zones, so the
// expiry runs every hour (minute 7): a warranty is expired within an hour
// of its local day end, wherever the organization is. Each run only
// touches active rows past end_at, so the extra runs write nothing.
// Reminders are daily, late morning Istanbul time, so WhatsApp messages
// arrive at a reasonable hour; the <= window catches up a missed day.
const (
	warrantyExpireCron       = "7 * * * *"
	warrantyExpiringScanCron = "17 10 * * *"
)

// WarrantyTaskFunc runs one warranty periodic task.
type WarrantyTaskFunc func(ctx context.Context) error

// NewWarrantyExpireTask builds the expiry task.
func NewWarrantyExpireTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskWarrantyExpire, []byte("{}")), nil
}

// NewWarrantyExpiringScanTask builds the reminder scan task.
func NewWarrantyExpiringScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskWarrantyExpiringScan, []byte("{}")), nil
}

func warrantyTaskOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

// WithWarrantyCron sets the warranty task processors. The task handlers are
// registered once in NewWorkerWithQueues, so a repeated call replaces them.
func (w *Worker) WithWarrantyCron(expire, expiringScan WarrantyTaskFunc) *Worker {
	w.warrantyExpire = expire
	w.warrantyExpiringScan = expiringScan
	return w
}

func (w *Worker) handleWarrantyExpire(ctx context.Context, _ *asynq.Task) error {
	if w.warrantyExpire == nil {
		w.log.Warn("warranty_expire_handler_missing")
		return nil
	}
	return w.warrantyExpire(ctx)
}

func (w *Worker) handleWarrantyExpiringScan(ctx context.Context, _ *asynq.Task) error {
	if w.warrantyExpiringScan == nil {
		w.log.Warn("warranty_expiring_scan_handler_missing")
		return nil
	}
	return w.warrantyExpiringScan(ctx)
}
