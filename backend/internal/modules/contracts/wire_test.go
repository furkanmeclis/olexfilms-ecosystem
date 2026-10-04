package contracts

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

type fakeQueue struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (f *fakeQueue) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, task)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return nil, f.err
	}
	return &asynq.TaskInfo{}, nil
}

func option(opts []asynq.Option, typ asynq.OptionType) (any, bool) {
	for _, o := range opts {
		if o.Type() == typ {
			return o.Value(), true
		}
	}
	return nil, false
}

func executed(id int64) events.Event {
	return events.New(events.ContractExecuted).WithEntity("contract", &id, nil)
}

// TEC-288: contract.executed reaches a registered handler that enqueues one
// worker-docs contract:pdf task per instance, deduplicated by task id.
func TestContractExecutedEnqueuesDocsPDFTask(t *testing.T) {
	q := &fakeQueue{}
	bus := events.NewBus(nil)
	RegisterEventHandlers(bus, q, nil)
	if n := bus.HandlerCount(events.ContractExecuted); n != 1 {
		t.Fatalf("contract.executed handlers = %d, want 1", n)
	}
	if err := bus.Publish(context.Background(), executed(42)); err != nil {
		t.Fatal(err)
	}
	if len(q.tasks) != 1 || q.tasks[0].Type() != queue.TaskContractPDF {
		t.Fatalf("tasks = %+v", q.tasks)
	}
	p, err := queue.ParseContractPDFPayload(q.tasks[0].Payload())
	if err != nil || p.InstanceID != 42 {
		t.Fatalf("payload = %+v (%v)", p, err)
	}
	if v, ok := option(q.opts[0], asynq.QueueOpt); !ok || v.(string) != queue.QueueDocs {
		t.Fatalf("queue = %v", v)
	}
	if v, ok := option(q.opts[0], asynq.TaskIDOpt); !ok || v.(string) != queue.ContractPDFTaskID(42) {
		t.Fatalf("task id = %v", v)
	}
}

func TestContractExecutedHandlerConflictAndErrors(t *testing.T) {
	h := ExecutedPDFHandler(&fakeQueue{err: asynq.ErrTaskIDConflict}, nil)
	if err := h(context.Background(), executed(1)); err != nil {
		t.Fatalf("pending task should count as done: %v", err)
	}
	boom := errors.New("redis down")
	if err := ExecutedPDFHandler(&fakeQueue{err: boom}, nil)(context.Background(), executed(1)); !errors.Is(err, boom) {
		t.Fatalf("enqueue error = %v, want %v", err, boom)
	}
	if err := ExecutedPDFHandler(nil, nil)(context.Background(), executed(1)); err != nil {
		t.Fatalf("nil queue: %v", err)
	}
	q := &fakeQueue{}
	if err := ExecutedPDFHandler(q, nil)(context.Background(), events.New(events.ContractExecuted)); err != nil || len(q.tasks) != 0 {
		t.Fatalf("event without entity: err=%v tasks=%d", err, len(q.tasks))
	}
}
