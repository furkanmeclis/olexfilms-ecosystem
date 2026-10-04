package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskContractPDF renders the immutable PDF of one executed contract
// (TEC-288, F3-01d). It is enqueued on the docs queue when contract.executed
// is consumed; the handler is idempotent through contract_instances.pdf_key.
const TaskContractPDF = "contract:pdf"

// ContractPDFPayload names the contract instance to render.
type ContractPDFPayload struct {
	InstanceID int64 `json:"instance_id"`
}

// ContractPDFFunc renders the PDF of one executed contract instance.
type ContractPDFFunc func(ctx context.Context, instanceID int64) error

// NewContractPDFTask builds the contract PDF task.
func NewContractPDFTask(instanceID int64) (*asynq.Task, error) {
	body, err := json.Marshal(ContractPDFPayload{InstanceID: instanceID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal contract pdf: %w", err)
	}
	return asynq.NewTask(TaskContractPDF, body), nil
}

// ParseContractPDFPayload decodes a contract PDF payload.
func ParseContractPDFPayload(data []byte) (ContractPDFPayload, error) {
	var payload ContractPDFPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ContractPDFPayload{}, fmt.Errorf("queue: unmarshal contract pdf: %w", err)
	}
	return payload, nil
}

// ContractPDFTaskID is the Asynq task id of a contract's PDF render: a
// redelivered contract.executed event finds the pending task and enqueues
// no second one (ErrTaskIDConflict).
func ContractPDFTaskID(instanceID int64) string {
	return fmt.Sprintf("contract-pdf-%d", instanceID)
}

// ContractPDFOpts are the enqueue options: docs queue (worker-docs),
// deduplicated by task id, retried on Gotenberg/storage failures.
func ContractPDFOpts(instanceID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.TaskID(ContractPDFTaskID(instanceID)),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
}

// WithContractPDF sets the contract:pdf processor; a repeated call replaces it.
func (w *Worker) WithContractPDF(fn ContractPDFFunc) *Worker {
	w.contractPDF = fn
	return w
}

func (w *Worker) handleContractPDF(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseContractPDFPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.contractPDF == nil {
		w.log.Warn("contract_pdf_handler_missing", "instance_id", payload.InstanceID)
		return nil
	}
	return w.contractPDF(ctx, payload.InstanceID)
}
