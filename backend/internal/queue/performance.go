package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const TaskPerformanceDaily = "performance:daily"
const performanceDailyCron = "37 * * * *"

type PerformanceDailyPayload struct {
	OrganizationID int64     `json:"organization_id"`
	ComputedAt     time.Time `json:"computed_at"`
}

type PerformanceDailyFunc func(ctx context.Context, organizationID int64, computedAt time.Time) error

func NewPerformanceDailyTask(organizationID int64, computedAt time.Time) (*asynq.Task, error) {
	body, err := json.Marshal(PerformanceDailyPayload{OrganizationID: organizationID, ComputedAt: computedAt.UTC()})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal performance daily: %w", err)
	}
	return asynq.NewTask(TaskPerformanceDaily, body), nil
}

func ParsePerformanceDailyPayload(data []byte) (PerformanceDailyPayload, error) {
	var payload PerformanceDailyPayload
	if len(data) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return PerformanceDailyPayload{}, fmt.Errorf("queue: unmarshal performance daily: %w", err)
	}
	if payload.OrganizationID < 0 {
		return PerformanceDailyPayload{}, fmt.Errorf("queue: performance organization %d", payload.OrganizationID)
	}
	return payload, nil
}

func performanceDailyOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(20 * time.Minute), asynq.Unique(50 * time.Minute)}
}

func (w *Worker) WithPerformanceDaily(fn PerformanceDailyFunc) *Worker {
	w.performanceDaily = fn
	return w
}

func (w *Worker) handlePerformanceDaily(ctx context.Context, task *asynq.Task) error {
	if w.performanceDaily == nil {
		w.log.Warn("performance_daily_handler_missing")
		return nil
	}
	payload, err := ParsePerformanceDailyPayload(task.Payload())
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return w.performanceDaily(ctx, payload.OrganizationID, payload.ComputedAt)
}
