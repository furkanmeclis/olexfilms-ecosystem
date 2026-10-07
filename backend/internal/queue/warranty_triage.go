package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TEC-392 (F4-01j): AI first triage of a warranty claim. A
// warranty_claim.status_changed to dealer_review / center_review enqueues
// it on the default queue with the claim uuid as task id, so a repeated
// event (or both outbox publishers) enqueue it once.
const TaskWarrantyClaimTriage = "warranty_claim:ai_triage"

// WarrantyClaimTriagePayload names the claim.
type WarrantyClaimTriagePayload struct {
	ClaimUUID uuid.UUID `json:"claim_uuid"`
}

// WarrantyClaimTriageFunc runs the triage of one claim.
type WarrantyClaimTriageFunc func(ctx context.Context, claim uuid.UUID) error

// NewWarrantyClaimTriageTask builds the task of a claim.
func NewWarrantyClaimTriageTask(claim uuid.UUID) (*asynq.Task, error) {
	body, err := json.Marshal(WarrantyClaimTriagePayload{ClaimUUID: claim})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal %s: %w", TaskWarrantyClaimTriage, err)
	}
	return asynq.NewTask(TaskWarrantyClaimTriage, body), nil
}

// WarrantyClaimTriageOpts: default queue, task id = claim uuid, two retries.
func WarrantyClaimTriageOpts(claim uuid.UUID) []asynq.Option {
	return []asynq.Option{
		asynq.Queue("default"),
		asynq.TaskID(TaskWarrantyClaimTriage + ":" + claim.String()),
		asynq.MaxRetry(2),
		asynq.Timeout(3 * time.Minute),
	}
}

// WarrantyTriageEnqueuer enqueues the triage task; a task already queued
// for the claim is not an error.
type WarrantyTriageEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueueClaimTriage enqueues warranty_claim:ai_triage for a claim.
func (e WarrantyTriageEnqueuer) EnqueueClaimTriage(_ context.Context, claim uuid.UUID) error {
	if e.Client == nil {
		return fmt.Errorf("queue: client is nil")
	}
	task, err := NewWarrantyClaimTriageTask(claim)
	if err != nil {
		return err
	}
	if _, err := e.Client.Enqueue(task, WarrantyClaimTriageOpts(claim)...); err != nil &&
		!errors.Is(err, asynq.ErrDuplicateTask) && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithWarrantyClaimTriage sets the warranty_claim:ai_triage processor; a
// repeated call replaces it.
func (w *Worker) WithWarrantyClaimTriage(fn WarrantyClaimTriageFunc) *Worker {
	w.warrantyClaimTriage = fn
	return w
}

func (w *Worker) handleWarrantyClaimTriage(ctx context.Context, task *asynq.Task) error {
	var p WarrantyClaimTriagePayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("queue: unmarshal %s: %w: %w", task.Type(), err, asynq.SkipRetry)
	}
	if w.warrantyClaimTriage == nil {
		w.log.Warn("warranty_claim_triage_handler_missing", "claim", p.ClaimUUID)
		return nil
	}
	return w.warrantyClaimTriage(ctx, p.ClaimUUID)
}
