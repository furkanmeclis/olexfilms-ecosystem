package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-273 (F2-02h): on-demand Glorian tasks of the admin API.

// TaskGlorianReconcile finishes a reconcile sync run the admin endpoint
// opened (the drift report of one connection).
const TaskGlorianReconcile = "glorian:reconcile"

// TaskGlorianOutboundReplayOne replays one order outbound (held or
// failed).
const TaskGlorianOutboundReplayOne = "glorian:order_outbound_replay_one"

// GlorianReconcilePayload names the running sync run.
type GlorianReconcilePayload struct {
	RunID int64 `json:"run_id"`
}

// GlorianOutboundReplayOnePayload names the outbound.
type GlorianOutboundReplayOnePayload struct {
	OutboundID int64 `json:"outbound_id"`
}

// GlorianReconcileFunc finishes one reconcile run.
type GlorianReconcileFunc func(ctx context.Context, runID int64) error

// GlorianOutboundReplayOneFunc replays one outbound.
type GlorianOutboundReplayOneFunc func(ctx context.Context, outboundID int64) error

// NewGlorianReconcileTask builds the reconcile task of a run.
func NewGlorianReconcileTask(runID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianReconcilePayload{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian reconcile: %w", err)
	}
	return asynq.NewTask(TaskGlorianReconcile, body), nil
}

// NewGlorianOutboundReplayOneTask builds the replay task of an outbound.
func NewGlorianOutboundReplayOneTask(outboundID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianOutboundReplayOnePayload{OutboundID: outboundID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian outbound replay: %w", err)
	}
	return asynq.NewTask(TaskGlorianOutboundReplayOne, body), nil
}

// GlorianReconcileOpts: the outcome is written on the run, so no retry.
func GlorianReconcileOpts(runID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.TaskID(fmt.Sprintf("glorian-reconcile-%d", runID)),
		asynq.MaxRetry(0),
		asynq.Timeout(15 * time.Minute),
	}
}

// GlorianOutboundReplayOneTaskID dedupes a double click: one pending
// replay per outbound.
func GlorianOutboundReplayOneTaskID(outboundID int64) string {
	return fmt.Sprintf("glorian-outbound-replay-%d", outboundID)
}

// GlorianOutboundReplayOneOpts: retried like the order outbound (every run
// re-reads the order and the hub order).
func GlorianOutboundReplayOneOpts(outboundID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.TaskID(GlorianOutboundReplayOneTaskID(outboundID)),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
}

// WithGlorianAdmin sets the glorian:reconcile and
// glorian:order_outbound_replay_one processors.
func (w *Worker) WithGlorianAdmin(reconcile GlorianReconcileFunc, replayOne GlorianOutboundReplayOneFunc) *Worker {
	w.glorianReconcile = reconcile
	w.glorianReplayOne = replayOne
	return w
}

func (w *Worker) handleGlorianReconcile(ctx context.Context, task *asynq.Task) error {
	var payload GlorianReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("queue: unmarshal glorian reconcile: %w", err)
	}
	if w.glorianReconcile == nil {
		w.log.Warn("glorian_reconcile_handler_missing", "run_id", payload.RunID)
		return nil
	}
	return w.glorianReconcile(ctx, payload.RunID)
}

func (w *Worker) handleGlorianOutboundReplayOne(ctx context.Context, task *asynq.Task) error {
	var payload GlorianOutboundReplayOnePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("queue: unmarshal glorian outbound replay: %w", err)
	}
	if w.glorianReplayOne == nil {
		w.log.Warn("glorian_outbound_replay_handler_missing", "outbound_id", payload.OutboundID)
		return nil
	}
	return w.glorianReplayOne(ctx, payload.OutboundID)
}
