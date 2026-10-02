package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskInventoryRebuild replays stock_movements and reports projection drift
// (TEC-156). The task only scans; repairs run from cmd/inventory-rebuild
// -apply.
const TaskInventoryRebuild = "app:inventory:rebuild"

// Nightly, after the business day (Europe/Istanbul).
const inventoryRebuildCron = "47 3 * * *"

// InventoryRebuildPayload scopes the scan; 0 scans every organization.
type InventoryRebuildPayload struct {
	OrganizationID int64 `json:"organization_id"`
}

// InventoryRebuildFunc scans one scope and reports the drift.
type InventoryRebuildFunc func(ctx context.Context, organizationID int64) error

// NewInventoryRebuildTask builds the scan task.
func NewInventoryRebuildTask(organizationID int64) (*asynq.Task, error) {
	body, err := json.Marshal(InventoryRebuildPayload{OrganizationID: organizationID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskInventoryRebuild, body, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute)), nil
}

// ParseInventoryRebuildPayload decodes the task payload.
func ParseInventoryRebuildPayload(data []byte) (InventoryRebuildPayload, error) {
	var p InventoryRebuildPayload
	if len(data) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("queue: inventory rebuild payload: %w", err)
	}
	if p.OrganizationID < 0 {
		return p, fmt.Errorf("queue: inventory rebuild organization %d", p.OrganizationID)
	}
	return p, nil
}

// WithInventoryRebuild sets the scan processor.
func (w *Worker) WithInventoryRebuild(fn InventoryRebuildFunc) *Worker {
	w.inventoryRebuild = fn
	return w
}

func (w *Worker) handleInventoryRebuild(ctx context.Context, task *asynq.Task) error {
	if w.inventoryRebuild == nil {
		w.log.Warn("inventory_rebuild_handler_missing")
		return nil
	}
	p, err := ParseInventoryRebuildPayload(task.Payload())
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return w.inventoryRebuild(ctx, p.OrganizationID)
}

func newNightlyInventoryRebuildTask() (*asynq.Task, error) { return NewInventoryRebuildTask(0) }
