package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-270 (F2-02e): Glorian barcode push.
//
// TaskGlorianPushBarcodes bulk-upserts the barcodes entered or placed into
// a center bin since the connection's last successful push
// (location=center, status=available). The stock.entry / stock.placement
// listener enqueues one task per connection and debounce window, so the
// units placed together go out in one bulk call; a 15 minute cron runs it
// for every active connection as a safety net. ConnectionID 0 means every
// active glorian connection.
const TaskGlorianPushBarcodes = "glorian:push_barcodes"

// TaskGlorianPatchStockItem sends PATCH /stock-items/by-barcode for one
// exit, transfer or shipment movement of a synced unit.
const TaskGlorianPatchStockItem = "glorian:patch_stock_item"

// Push cron: every 15 minutes, between the pull runs.
const glorianPushCron = "11,26,41,56 * * * *"

// GlorianPushDebounce is the listener's window: every placement of the
// same connection inside one window lands in the same task.
const GlorianPushDebounce = 10 * time.Second

// GlorianPushBarcodesPayload names the connection of a push task.
type GlorianPushBarcodesPayload struct {
	ConnectionID int64 `json:"connection_id"`
}

// GlorianPatchStockItemPayload names the movement of a PATCH task.
type GlorianPatchStockItemPayload struct {
	MovementID int64 `json:"movement_id"`
}

// GlorianPushBarcodesFunc pushes one connection (0: every active one).
type GlorianPushBarcodesFunc func(ctx context.Context, connectionID int64) error

// GlorianPatchStockItemFunc sends the PATCH of one movement.
type GlorianPatchStockItemFunc func(ctx context.Context, movementID int64) error

// NewGlorianPushBarcodesTask builds the push task of a connection.
func NewGlorianPushBarcodesTask(connectionID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianPushBarcodesPayload{ConnectionID: connectionID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian push: %w", err)
	}
	return asynq.NewTask(TaskGlorianPushBarcodes, body), nil
}

// NewGlorianPushAllTask is the cron form of the push task (every active
// connection).
func NewGlorianPushAllTask() (*asynq.Task, error) { return NewGlorianPushBarcodesTask(0) }

// NewGlorianPatchStockItemTask builds the PATCH task of a movement.
func NewGlorianPatchStockItemTask(movementID int64) (*asynq.Task, error) {
	body, err := json.Marshal(GlorianPatchStockItemPayload{MovementID: movementID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal glorian patch: %w", err)
	}
	return asynq.NewTask(TaskGlorianPatchStockItem, body), nil
}

// GlorianPushBarcodesTaskID is the task id of a connection's push in the
// debounce window holding now: a second placement in the same window finds
// the scheduled task (ErrTaskIDConflict) and enqueues nothing.
func GlorianPushBarcodesTaskID(connectionID int64, now time.Time) string {
	return fmt.Sprintf("glorian-push-%d-%d", connectionID, now.UTC().Truncate(GlorianPushDebounce).Unix())
}

// GlorianPushBarcodesOpts schedules the push at the end of the debounce
// window holding now. A failed push (hub 5xx, transport) is retried with
// Asynq's backoff; the upsert is idempotent by barcode.
func GlorianPushBarcodesOpts(connectionID int64, now time.Time) []asynq.Option {
	end := now.UTC().Truncate(GlorianPushDebounce).Add(GlorianPushDebounce)
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.TaskID(GlorianPushBarcodesTaskID(connectionID, now)),
		asynq.ProcessAt(end),
		asynq.MaxRetry(8),
		asynq.Timeout(5 * time.Minute),
	}
}

// GlorianPatchStockItemTaskID dedupes a redelivered movement event.
func GlorianPatchStockItemTaskID(movementID int64) string {
	return fmt.Sprintf("glorian-patch-%d", movementID)
}

// GlorianPatchStockItemOpts: retried with backoff; a hub that does not know
// the barcode yet (bulk push still pending) answers 404 and is retried too.
func GlorianPatchStockItemOpts(movementID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueMaintenance),
		asynq.TaskID(GlorianPatchStockItemTaskID(movementID)),
		asynq.MaxRetry(10),
		asynq.Timeout(time.Minute),
	}
}

// Cron run: no retry (the next run 15 minutes later catches up), unique
// for the period.
func glorianPushCronOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(10 * time.Minute), asynq.Unique(14 * time.Minute)}
}

// WithGlorianPush sets the glorian:push_barcodes and
// glorian:patch_stock_item processors.
func (w *Worker) WithGlorianPush(push GlorianPushBarcodesFunc, patch GlorianPatchStockItemFunc) *Worker {
	w.glorianPush = push
	w.glorianPatch = patch
	return w
}

func (w *Worker) handleGlorianPush(ctx context.Context, task *asynq.Task) error {
	var payload GlorianPushBarcodesPayload
	if len(task.Payload()) > 0 {
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("queue: unmarshal glorian push: %w", err)
		}
	}
	if w.glorianPush == nil {
		w.log.Warn("glorian_push_handler_missing", "connection_id", payload.ConnectionID)
		return nil
	}
	return w.glorianPush(ctx, payload.ConnectionID)
}

func (w *Worker) handleGlorianPatch(ctx context.Context, task *asynq.Task) error {
	var payload GlorianPatchStockItemPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("queue: unmarshal glorian patch: %w", err)
	}
	if w.glorianPatch == nil {
		w.log.Warn("glorian_patch_handler_missing", "movement_id", payload.MovementID)
		return nil
	}
	return w.glorianPatch(ctx, payload.MovementID)
}
