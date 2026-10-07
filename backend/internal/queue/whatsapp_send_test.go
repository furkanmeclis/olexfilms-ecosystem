package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type fakeEnqueueClient struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (c *fakeEnqueueClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	c.tasks = append(c.tasks, task)
	c.opts = append(c.opts, opts)
	return &asynq.TaskInfo{}, c.err
}

// whatsapp:send runs on the whatsapp queue, deduplicated by the message
// uuid, with three retries; an already pending task is not an error.
func TestWhatsAppEnqueuer(t *testing.T) {
	c := &fakeEnqueueClient{}
	e := WhatsAppEnqueuer{Client: c}
	u := uuid.New()
	if err := e.EnqueueSend(context.Background(), 42, u); err != nil {
		t.Fatal(err)
	}
	if c.tasks[0].Type() != TaskWhatsAppSend {
		t.Fatalf("type %s", c.tasks[0].Type())
	}
	var p WhatsAppMessagePayload
	if err := json.Unmarshal(c.tasks[0].Payload(), &p); err != nil || p.MessageID != 42 || p.MessageUUID != u {
		t.Fatalf("payload %+v %v", p, err)
	}
	want := map[asynq.OptionType]any{
		asynq.QueueOpt: QueueWhatsApp, asynq.TaskIDOpt: "wa-send-" + u.String(), asynq.MaxRetryOpt: WhatsAppSendMaxRetry,
	}
	for _, o := range c.opts[0] {
		if v, ok := want[o.Type()]; ok {
			if o.Value() != v {
				t.Errorf("option %v = %v, want %v", o.Type(), o.Value(), v)
			}
			delete(want, o.Type())
		}
	}
	if len(want) != 0 || WhatsAppSendMaxRetry != 3 {
		t.Fatalf("missing options %v (max retry %d)", want, WhatsAppSendMaxRetry)
	}

	c.err = asynq.ErrTaskIDConflict
	if err := e.EnqueueMediaStore(context.Background(), 42, u); err != nil {
		t.Fatalf("task id conflict must be ignored: %v", err)
	}
	if c.tasks[1].Type() != TaskWhatsAppMediaStore {
		t.Fatalf("type %s", c.tasks[1].Type())
	}
	if DefaultQueues()[QueueWhatsApp] == 0 {
		t.Fatal("whatsapp queue is not consumed")
	}
}

// The handler passes final = true only on the last retry.
func TestWhatsAppSendHandlerFinalFlag(t *testing.T) {
	var finals []bool
	w := &Worker{}
	w.WithWhatsAppMessaging(func(_ context.Context, id int64, final bool) error {
		if id != 7 {
			t.Fatalf("id %d", id)
		}
		finals = append(finals, final)
		return nil
	}, nil, nil)
	task, _ := newWhatsAppMessageTask(TaskWhatsAppSend, 7, uuid.New())
	if err := w.handleWhatsAppSend(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(finals) != 1 || finals[0] {
		t.Fatalf("finals = %v (no retry metadata = not final)", finals)
	}
}

type deferErr struct{ d time.Duration }

func (e deferErr) Error() string             { return "deferred" }
func (e deferErr) RetryAfter() time.Duration { return e.d }

// A RetryAfterError (also wrapped) is not a failure and waits its own
// delay (at least a second, plus under a second of jitter); other errors
// fail and use the default backoff.
func TestTaskRetryDelayAndIsFailure(t *testing.T) {
	wrapped := fmt.Errorf("send: %w", deferErr{d: 20 * time.Second})
	if IsTaskFailure(wrapped) || !IsTaskFailure(errors.New("boom")) {
		t.Fatal("IsTaskFailure: deferral must not be a failure, other errors must")
	}
	if d := TaskRetryDelay(0, wrapped, nil); d < 20*time.Second || d >= 21*time.Second {
		t.Fatalf("deferral delay = %s", d)
	}
	if d := TaskRetryDelay(0, deferErr{}, nil); d < time.Second || d >= 2*time.Second {
		t.Fatalf("zero deferral delay = %s, want ≥1s", d)
	}
	task, _ := newWhatsAppMessageTask(TaskWhatsAppSend, 1, uuid.New())
	if d := TaskRetryDelay(1, errors.New("boom"), task); d < 15*time.Second {
		t.Fatalf("default backoff = %s", d)
	}
}
