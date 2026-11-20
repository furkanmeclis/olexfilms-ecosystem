package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-1265, kullanıcı kararı 5 (TEC-1253): the ecosystem owns prices.
//
// TaskGlorianPushPrices sends the USD list price of every product synced
// from a connection to the hub (PUT /products/prices, absolute values).
// A cron runs it for every active connection; the admin "push prices"
// button queues it for one connection. ConnectionID 0 means every active
// glorian connection.
const TaskGlorianPushPrices = "glorian:push_prices"

// Price push cron: every 15 minutes, after the barcode push.
const glorianPricesCron = "13,28,43,58 * * * *"

// GlorianPushPricesPayload names the connection of a price push.
type GlorianPushPricesPayload struct {
	ConnectionID int64 `json:"connection_id"`
}

// GlorianPushPricesFunc pushes the prices of one connection (0: every
// active one).
type GlorianPushPricesFunc func(ctx context.Context, connectionID int64) error

// NewGlorianPushPricesTask builds the price push task of a connection.
func NewGlorianPushPricesTask(connectionID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianPushPricesPayload{ConnectionID: connectionID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian price push: %w", err)
	}
	return asynq.NewTask(TaskGlorianPushPrices, body), nil
}

// NewGlorianPushPricesAllTask is the cron form (every active connection).
func NewGlorianPushPricesAllTask() (*asynq.Task, error) { return NewGlorianPushPricesTask(0) }

// GlorianPushPricesOpts are the options of a manual push: retried with
// backoff (the call is idempotent), unique for a minute against double
// clicks.
func GlorianPushPricesOpts() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.MaxRetry(5),
		asynq.Timeout(5 * time.Minute),
		asynq.Unique(time.Minute),
	}
}

// Cron run: no retry (the next run 15 minutes later catches up), unique
// for the period.
func glorianPricesCronOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(10 * time.Minute), asynq.Unique(14 * time.Minute)}
}

// WithGlorianPrices sets the glorian:push_prices processor.
func (w *Worker) WithGlorianPrices(push GlorianPushPricesFunc) *Worker {
	w.glorianPrices = push
	return w
}

func (w *Worker) handleGlorianPrices(ctx context.Context, task *asynq.Task) error {
	var payload GlorianPushPricesPayload
	if len(task.Payload()) > 0 {
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("queue: unmarshal glorian price push: %w", err)
		}
	}
	if w.glorianPrices == nil {
		w.log.Warn("glorian_prices_handler_missing", "connection_id", payload.ConnectionID)
		return nil
	}
	return w.glorianPrices(ctx, payload.ConnectionID)
}
