package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskWarehouseEODReports writes the end-of-day warehouse reports
// (TEC-207): the previous local day of every center / distributor with a
// warehouse, system report plus one per active warehouse. A report that
// already exists is skipped, so the extra runs write nothing.
const TaskWarehouseEODReports = "warehouse:eod_reports"

// Hourly, so every organization time zone gets its report within an hour
// after its local midnight. Minute 23 keeps it clear of the other
// maintenance runs (7, 41, ...).
const warehouseEODCron = "23 * * * *"

// WarehouseEODFunc runs one end-of-day report pass.
type WarehouseEODFunc func(ctx context.Context) error

// NewWarehouseEODTask builds the end-of-day report task.
func NewWarehouseEODTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskWarehouseEODReports, []byte("{}")), nil
}

func warehouseEODOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(15 * time.Minute)}
}

// WithWarehouseEOD sets the warehouse:eod_reports processor.
func (w *Worker) WithWarehouseEOD(fn WarehouseEODFunc) *Worker {
	w.warehouseEOD = fn
	return w
}

func (w *Worker) handleWarehouseEOD(ctx context.Context, _ *asynq.Task) error {
	if w.warehouseEOD == nil {
		w.log.Warn("warehouse_eod_handler_missing")
		return nil
	}
	return w.warehouseEOD(ctx)
}
