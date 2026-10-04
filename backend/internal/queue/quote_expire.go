package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskQuoteExpire marks quotes whose valid_until date passed as expired.
const TaskQuoteExpire = "quotes:expire"

// Daily at 02:17 Europe/Istanbul, after date rollover and away from other jobs.
const quoteExpireCron = "17 2 * * *"

// QuoteExpireFunc runs one quote expiry scan.
type QuoteExpireFunc func(ctx context.Context) error

// NewQuoteExpireTask builds the quote expiry task.
func NewQuoteExpireTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskQuoteExpire, []byte("{}")), nil
}

func quoteExpireOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(5 * time.Minute)}
}

// WithQuoteExpire sets the quote expiry processor.
func (w *Worker) WithQuoteExpire(fn QuoteExpireFunc) *Worker {
	w.quoteExpire = fn
	return w
}

func (w *Worker) handleQuoteExpire(ctx context.Context, _ *asynq.Task) error {
	if w.quoteExpire == nil {
		w.log.Warn("quote_expire_handler_missing")
		return nil
	}
	return w.quoteExpire(ctx)
}
