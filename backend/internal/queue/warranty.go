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
	// TaskWarrantyRepairScan opens the warranties of completed services that
	// the service.completed listener missed (TEC-194).
	TaskWarrantyRepairScan = "warranty:repair_scan"
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

// The repair scan runs once a night (Istanbul), clear of the hourly expiry
// (minute 7), the inventory drift scan (03:47) and the morning reminders,
// so a repaired warranty is in place before the reminder pass.
const warrantyRepairScanCron = "27 4 * * *"

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

// NewWarrantyRepairScanTask builds the repair scan task.
func NewWarrantyRepairScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskWarrantyRepairScan, []byte("{}")), nil
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

// WithWarrantyRepairScan sets the warranty:repair_scan processor (TEC-194).
func (w *Worker) WithWarrantyRepairScan(fn WarrantyTaskFunc) *Worker {
	w.warrantyRepairScan = fn
	return w
}

func (w *Worker) handleWarrantyRepairScan(ctx context.Context, _ *asynq.Task) error {
	if w.warrantyRepairScan == nil {
		w.log.Warn("warranty_repair_scan_handler_missing")
		return nil
	}
	return w.warrantyRepairScan(ctx)
}
