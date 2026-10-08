package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const TaskStockForecastDaily = "stock_forecast:daily"

const stockForecastDailyCron = "17 * * * *"

type StockForecastDailyPayload struct {
	OrganizationID int64     `json:"organization_id"`
	ComputedOn     time.Time `json:"computed_on"`
}

type StockForecastDailyFunc func(ctx context.Context, organizationID int64, computedOn time.Time) error

func NewStockForecastDailyTask(organizationID int64, computedOn time.Time) (*asynq.Task, error) {
	body, err := json.Marshal(StockForecastDailyPayload{OrganizationID: organizationID, ComputedOn: computedOn.UTC()})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal stock forecast daily: %w", err)
	}
	return asynq.NewTask(TaskStockForecastDaily, body), nil
}

func ParseStockForecastDailyPayload(data []byte) (StockForecastDailyPayload, error) {
	var payload StockForecastDailyPayload
	if len(data) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return StockForecastDailyPayload{}, fmt.Errorf("queue: unmarshal stock forecast daily: %w", err)
	}
	if payload.OrganizationID < 0 {
		return StockForecastDailyPayload{}, fmt.Errorf("queue: stock forecast organization %d", payload.OrganizationID)
	}
	return payload, nil
}

func stockForecastDailyOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(20 * time.Minute), asynq.Unique(50 * time.Minute)}
}

func (w *Worker) WithStockForecastDaily(fn StockForecastDailyFunc) *Worker {
	w.stockForecastDaily = fn
	return w
}

func (w *Worker) handleStockForecastDaily(ctx context.Context, task *asynq.Task) error {
	if w.stockForecastDaily == nil {
		w.log.Warn("stock_forecast_daily_handler_missing")
		return nil
	}
	payload, err := ParseStockForecastDailyPayload(task.Payload())
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return w.stockForecastDaily(ctx, payload.OrganizationID, payload.ComputedOn)
}
