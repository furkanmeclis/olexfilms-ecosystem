package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskTasksDueScan writes the center task due date reminders (TEC-221):
// tasks.overdue for open tasks past due_at, tasks.due_soon for open tasks
// due within a day. Each task is stamped per threshold in the event's
// transaction, so the extra runs write nothing.
const TaskTasksDueScan = "tasks:due_scan"

// Hourly, so a "due soon" reminder lands at least ~23 hours ahead and an
// overdue one within an hour of the deadline. Minute 41 keeps it clear of
// the warranty expiry (minute 7) and the other maintenance runs.
const tasksDueScanCron = "41 * * * *"

// TasksDueScanFunc runs one due date scan.
type TasksDueScanFunc func(ctx context.Context) error

// NewTasksDueScanTask builds the due date scan task.
func NewTasksDueScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskTasksDueScan, []byte("{}")), nil
}

func tasksDueScanOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

// WithTasksDueScan sets the tasks:due_scan processor.
func (w *Worker) WithTasksDueScan(fn TasksDueScanFunc) *Worker {
	w.tasksDueScan = fn
	return w
}

func (w *Worker) handleTasksDueScan(ctx context.Context, _ *asynq.Task) error {
	if w.tasksDueScan == nil {
		w.log.Warn("tasks_due_scan_handler_missing")
		return nil
	}
	return w.tasksDueScan(ctx)
}
