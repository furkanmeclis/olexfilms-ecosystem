package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAIActionSweep expires AI confirmation cards past their 30 minute TTL
// and fails actions stuck executing for over 10 minutes (TEC-387).
const TaskAIActionSweep = "ai:actions:sweep"

// Every 5 minutes: a card is shown expired at most 5 minutes late (a late
// confirmation is refused by the use case anyway).
const aiActionSweepCron = "*/5 * * * *"

// AIActionSweepFunc runs one sweep (ai/usecase.Actions.SweepTask).
type AIActionSweepFunc func(ctx context.Context) error

// NewAIActionSweepTask builds the sweep task.
func NewAIActionSweepTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskAIActionSweep, []byte("{}")), nil
}

func aiActionSweepOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(2 * time.Minute), asynq.Unique(4 * time.Minute)}
}

// WithAIActionSweep sets the AI action sweep processor.
func (w *Worker) WithAIActionSweep(fn AIActionSweepFunc) *Worker {
	w.aiActionSweep = fn
	return w
}

func (w *Worker) handleAIActionSweep(ctx context.Context, _ *asynq.Task) error {
	if w.aiActionSweep == nil {
		w.log.Warn("ai_action_sweep_handler_missing")
		return nil
	}
	return w.aiActionSweep(ctx)
}
