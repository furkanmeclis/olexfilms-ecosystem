package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskGlorianPullCatalog pulls the Glorian hub's categories, products and
// dealers changed since the last run into the glorian brand (TEC-268, K2),
// then mirrors the changed stock items onto units.external_status
// (TEC-269; no ledger movement).
// Only active connections are pulled; the upserts are idempotent, so an
// overlapping or repeated run writes nothing new.
const TaskGlorianPullCatalog = "glorian:pull_catalog"

// Every 15 minutes, offset from the other maintenance runs.
const glorianPullCron = "4,19,34,49 * * * *"

// GlorianPullFunc runs one catalog, dealer and stock item pull pass.
type GlorianPullFunc func(ctx context.Context) error

// NewGlorianPullCatalogTask builds the pull task.
func NewGlorianPullCatalogTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskGlorianPullCatalog, []byte("{}")), nil
}

// No retry: the next run 15 minutes later starts from the last successful
// watermark anyway. Unique for the period so a slow run is not doubled.
func glorianPullOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(10 * time.Minute), asynq.Unique(14 * time.Minute)}
}

// WithGlorianPull sets the glorian:pull_catalog processor.
func (w *Worker) WithGlorianPull(fn GlorianPullFunc) *Worker {
	w.glorianPull = fn
	return w
}

func (w *Worker) handleGlorianPull(ctx context.Context, _ *asynq.Task) error {
	if w.glorianPull == nil {
		w.log.Warn("glorian_pull_handler_missing")
		return nil
	}
	return w.glorianPull(ctx)
}
