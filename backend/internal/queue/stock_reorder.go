package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskStockReorderCheck (TEC-1229): the warehouse low stock check of the
// reorder rules; notifies on the ok -> low edge.
const TaskStockReorderCheck = "stock_reorder:check"

const stockReorderCheckCron = "17 * * * *"

// StockReorderCheckFunc runs one check of every organization with rules.
type StockReorderCheckFunc func(ctx context.Context) error

// NewStockReorderCheckTask builds the task.
func NewStockReorderCheckTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskStockReorderCheck, []byte("{}")), nil
}

func stockReorderCheckOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(10 * time.Minute)}
}

// WithStockReorderCheck registers the handler.
func (w *Worker) WithStockReorderCheck(fn StockReorderCheckFunc) *Worker {
	w.stockReorderCheck = fn
	return w
}

func (w *Worker) handleStockReorderCheck(ctx context.Context, _ *asynq.Task) error {
	if w.stockReorderCheck == nil {
		w.log.Warn("stock_reorder_check_handler_missing")
		return nil
	}
	return w.stockReorderCheck(ctx)
}
