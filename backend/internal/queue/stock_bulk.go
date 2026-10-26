package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskStockBulk runs or undoes one bulk barcode job (TEC-1008). The
// handler is idempotent: it resumes the job's pending items.
const TaskStockBulk = "stock:bulk"

// StockBulkPayload names the job.
type StockBulkPayload struct {
	JobID int64 `json:"job_id"`
	Undo  bool  `json:"undo"`
}

// StockBulkFunc processes a job.
type StockBulkFunc func(ctx context.Context, jobID int64, undo bool) error

// NewStockBulkTask builds the task.
func NewStockBulkTask(jobID int64, undo bool) (*asynq.Task, error) {
	body, err := json.Marshal(StockBulkPayload{JobID: jobID, Undo: undo})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal stock bulk: %w", err)
	}
	return asynq.NewTask(TaskStockBulk, body), nil
}

// StockBulkOpts: default queue, one task per job and mode, long timeout.
func StockBulkOpts(jobID int64, undo bool) []asynq.Option {
	mode := "run"
	if undo {
		mode = "undo"
	}
	return []asynq.Option{
		asynq.TaskID(fmt.Sprintf("stock-bulk-%d-%s", jobID, mode)),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Minute),
	}
}

// EnqueueStockBulk queues a job run or undo.
func (c *Client) EnqueueStockBulk(_ context.Context, jobID int64, undo bool) error {
	task, err := NewStockBulkTask(jobID, undo)
	if err != nil {
		return err
	}
	_, err = c.Enqueue(task, StockBulkOpts(jobID, undo)...)
	return err
}

// WithStockBulk sets the stock:bulk processor.
func (w *Worker) WithStockBulk(fn StockBulkFunc) *Worker {
	w.stockBulk = fn
	return w
}

func (w *Worker) handleStockBulk(ctx context.Context, task *asynq.Task) error {
	var p StockBulkPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("queue: unmarshal stock bulk: %w", err)
	}
	if w.stockBulk == nil {
		w.log.Warn("stock_bulk_handler_missing", "job_id", p.JobID)
		return nil
	}
	return w.stockBulk(ctx, p.JobID, p.Undo)
}
