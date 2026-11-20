package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskExportCleanup deletes the stored files of export jobs past expires_at
// and marks the jobs expired (TEC-1259). Exports can hold personal data
// (customer data exports, lists), so the file must not outlive the job.
const TaskExportCleanup = "app:exports:cleanup"

const exportCleanupCron = "17 * * * *"

// ExportCleanupFunc runs one cleanup pass and returns the jobs expired.
type ExportCleanupFunc func(ctx context.Context) (int, error)

// NewExportCleanupTask builds the hourly cleanup task.
func NewExportCleanupTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskExportCleanup, []byte("{}")), nil
}

func exportCleanupOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(15 * time.Minute)}
}

// WithExportCleanup sets the export file cleanup processor.
func (w *Worker) WithExportCleanup(fn ExportCleanupFunc) *Worker {
	w.exportCleanup = fn
	return w
}

func (w *Worker) handleExportCleanup(ctx context.Context, _ *asynq.Task) error {
	if w.exportCleanup == nil {
		w.log.Warn("export_cleanup_handler_missing")
		return nil
	}
	n, err := w.exportCleanup(ctx)
	if err == nil && n > 0 {
		w.log.Info("export_files_expired", "jobs", n)
	}
	return err
}
