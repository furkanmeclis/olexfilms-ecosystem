package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskStaffPaymentsPostDue books the planned staff payments whose paid_on
// has arrived in their organization's time zone (TEC-381). Each payment is
// locked and re-checked and its ledger row is source-keyed, so the extra
// runs write nothing.
const TaskStaffPaymentsPostDue = "accounting:staff_payments_post_due"

// Hourly, so every organization time zone books its payments of the day
// within an hour after its local midnight. Minute 53 keeps it clear of the
// other maintenance runs (7, 23, 41, ...).
const staffPaymentsPostDueCron = "53 * * * *"

// StaffPaymentsPostDueFunc runs one posting pass.
type StaffPaymentsPostDueFunc func(ctx context.Context) error

// NewStaffPaymentsPostDueTask builds the posting task.
func NewStaffPaymentsPostDueTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskStaffPaymentsPostDue, []byte("{}")), nil
}

func staffPaymentsPostDueOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

// WithStaffPaymentsPostDue sets the accounting:staff_payments_post_due
// processor.
func (w *Worker) WithStaffPaymentsPostDue(fn StaffPaymentsPostDueFunc) *Worker {
	w.staffPaymentsPostDue = fn
	return w
}

func (w *Worker) handleStaffPaymentsPostDue(ctx context.Context, _ *asynq.Task) error {
	if w.staffPaymentsPostDue == nil {
		w.log.Warn("staff_payments_post_due_handler_missing")
		return nil
	}
	return w.staffPaymentsPostDue(ctx)
}
