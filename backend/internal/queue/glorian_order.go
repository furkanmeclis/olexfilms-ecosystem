package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-271 (F2-02f): Glorian order outbound.

// TaskGlorianOrderOutbound brings the hub orders of one local order in line
// with it: creates the order_outbounds rows when the order is ready, sends
// POST /orders and then ship / receive / cancel in order. Enqueued by the
// orders.* outbox listener.
const TaskGlorianOrderOutbound = "glorian:order_outbound"

// TaskGlorianOrderReplay replays the held order outbounds of a connection
// (0: every connection). The cron run is the safety net that sends a held
// order once its dealer link or the connection is in place.
const TaskGlorianOrderReplay = "glorian:order_outbound_replay"

// glorianOrderReplayCron runs after the dealer pull (4,19,34,49).
const glorianOrderReplayCron = "8,23,38,53 * * * *"

// GlorianOrderOutboundPayload names the local order.
type GlorianOrderOutboundPayload struct {
	OrderID int64 `json:"order_id"`
}

// GlorianOrderReplayPayload names the connection (0: every connection).
type GlorianOrderReplayPayload struct {
	ConnectionID int64 `json:"connection_id"`
}

// GlorianOrderOutboundFunc syncs the outbounds of one order.
type GlorianOrderOutboundFunc func(ctx context.Context, orderID int64) error

// GlorianOrderReplayFunc replays the held outbounds of a connection.
type GlorianOrderReplayFunc func(ctx context.Context, connectionID int64) error

// NewGlorianOrderOutboundTask builds the outbound task of an order.
func NewGlorianOrderOutboundTask(orderID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianOrderOutboundPayload{OrderID: orderID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian order outbound: %w", err)
	}
	return asynq.NewTask(TaskGlorianOrderOutbound, body), nil
}

// NewGlorianOrderReplayTask builds the replay task of a connection.
func NewGlorianOrderReplayTask(connectionID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianOrderReplayPayload{ConnectionID: connectionID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian order replay: %w", err)
	}
	return asynq.NewTask(TaskGlorianOrderReplay, body), nil
}

// NewGlorianOrderReplayAllTask is the cron form of the replay task.
func NewGlorianOrderReplayAllTask() (*asynq.Task, error) { return NewGlorianOrderReplayTask(0) }

// GlorianOrderOutboundTaskID dedupes a redelivered orders.* event: one task
// per order and status.
func GlorianOrderOutboundTaskID(orderID int64, status string) string {
	return fmt.Sprintf("glorian-order-%d-%s", orderID, status)
}

// GlorianOrderOutboundOpts: retried with backoff on a transient failure
// (hub 5xx, transport); every run re-reads the order and the hub order, so
// a retry or a late task never repeats or reorders a step.
func GlorianOrderOutboundOpts(orderID int64, status string) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.TaskID(GlorianOrderOutboundTaskID(orderID, status)),
		asynq.MaxRetry(10),
		asynq.Timeout(2 * time.Minute),
	}
}

// GlorianOrderReplayOpts: an on-demand replay of a connection.
func GlorianOrderReplayOpts() []asynq.Option {
	return []asynq.Option{asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

// Cron run: no retry (the next run catches up), unique for the period.
func glorianOrderReplayCronOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(10 * time.Minute), asynq.Unique(14 * time.Minute)}
}

// WithGlorianOrderOutbound sets the glorian:order_outbound and
// glorian:order_outbound_replay processors.
func (w *Worker) WithGlorianOrderOutbound(sync GlorianOrderOutboundFunc, replay GlorianOrderReplayFunc) *Worker {
	w.glorianOrder = sync
	w.glorianReplay = replay
	return w
}

func (w *Worker) handleGlorianOrderOutbound(ctx context.Context, task *asynq.Task) error {
	var payload GlorianOrderOutboundPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("queue: unmarshal glorian order outbound: %w", err)
	}
	if w.glorianOrder == nil {
		w.log.Warn("glorian_order_outbound_handler_missing", "order_id", payload.OrderID)
		return nil
	}
	return w.glorianOrder(ctx, payload.OrderID)
}

func (w *Worker) handleGlorianOrderReplay(ctx context.Context, task *asynq.Task) error {
	var payload GlorianOrderReplayPayload
	if len(task.Payload()) > 0 {
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("queue: unmarshal glorian order replay: %w", err)
		}
	}
	if w.glorianReplay == nil {
		w.log.Warn("glorian_order_replay_handler_missing", "connection_id", payload.ConnectionID)
		return nil
	}
	return w.glorianReplay(ctx, payload.ConnectionID)
}
