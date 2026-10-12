package usecase

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
)

type captureEnqueuer struct {
	task *asynq.Task
	opts []asynq.Option
}

func (c *captureEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	c.task, c.opts = task, opts
	return &asynq.TaskInfo{}, nil
}

// TEC-632: with a queue the delivery goes through asynq (never inline), on
// the notifications queue, once per notification and with a bounded retry.
func TestScheduleDeliverEnqueuesWithBoundedRetry(t *testing.T) {
	enq := &captureEnqueuer{}
	s := New(nil, enq, nil, nil)
	if s.syncMode {
		t.Fatal("service with a queue must not run in sync mode")
	}
	if err := s.scheduleDeliver(context.Background(), 42); err != nil {
		t.Fatalf("scheduleDeliver: %v", err)
	}
	if enq.task == nil || enq.task.Type() != queue.TaskNotificationDeliver {
		t.Fatalf("task = %v, want %s", enq.task, queue.TaskNotificationDeliver)
	}
	got := map[asynq.OptionType]any{}
	for _, o := range enq.opts {
		got[o.Type()] = o.Value()
	}
	if got[asynq.QueueOpt] != queue.QueueNotifications {
		t.Errorf("queue = %v, want %s", got[asynq.QueueOpt], queue.QueueNotifications)
	}
	if got[asynq.TaskIDOpt] != "notification:42" {
		t.Errorf("task id = %v, want notification:42", got[asynq.TaskIDOpt])
	}
	if got[asynq.MaxRetryOpt] != DeliverMaxRetry {
		t.Errorf("max retry = %v, want %d", got[asynq.MaxRetryOpt], DeliverMaxRetry)
	}
}

func TestNewWithoutQueueIsSyncMode(t *testing.T) {
	if s := New(nil, nil, nil, nil); !s.syncMode {
		t.Fatal("service without a queue delivers inline (sync mode)")
	}
}
