package contracts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// Enqueuer schedules an Asynq task (*queue.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// RegisterEventHandlers subscribes the executed-contract PDF scheduler to the
// platform bus: contract.executed enqueues the worker-docs contract:pdf task.
// Every process that drains the outbox registers it; the task id keeps a
// redelivered event from enqueuing a second task, and the task itself is
// idempotent through contract_instances.pdf_key.
func RegisterEventHandlers(bus events.Bus, q Enqueuer, log *slog.Logger) {
	if bus == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.ContractExecuted, ExecutedPDFHandler(q, log))
}

// ExecutedPDFHandler is the contract.executed bus handler. An enqueue
// failure is returned; a task that is already pending counts as done.
func ExecutedPDFHandler(q Enqueuer, log *slog.Logger) events.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(_ context.Context, ev events.Event) error {
		if ev.EntityID == nil || *ev.EntityID <= 0 {
			log.Warn("contract_pdf_no_instance", "event_id", ev.EventID.String())
			return nil
		}
		id := *ev.EntityID
		if q == nil {
			log.Warn("contract_pdf_queue_missing", "instance_id", id)
			return nil
		}
		task, err := queue.NewContractPDFTask(id)
		if err != nil {
			return err
		}
		if _, err := q.Enqueue(task, queue.ContractPDFOpts(id)...); err != nil {
			if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
				return nil
			}
			return fmt.Errorf("contract pdf: enqueue instance %d: %w", id, err)
		}
		return nil
	}
}
