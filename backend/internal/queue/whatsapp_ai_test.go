package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type recordingClient struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (c *recordingClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	c.tasks, c.opts = append(c.tasks, task), append(c.opts, opts)
	return &asynq.TaskInfo{}, c.err
}

// whatsapp:ai_reply is debounced per conversation: delayed by 4 s and
// unique for the same window on the whatsapp queue; a duplicate within
// the window is not an error.
func TestWhatsAppAIReplyDebounce(t *testing.T) {
	c := &recordingClient{}
	conv := uuid.New()
	if err := (WhatsAppAIEnqueuer{Client: c}).EnqueueAIReply(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	if len(c.tasks) != 1 || c.tasks[0].Type() != TaskWhatsAppAIReply {
		t.Fatalf("tasks = %+v", c.tasks)
	}
	var p WhatsAppAIReplyPayload
	if err := json.Unmarshal(c.tasks[0].Payload(), &p); err != nil || p.ConversationUUID != conv {
		t.Fatalf("payload %s %v", c.tasks[0].Payload(), err)
	}
	var queue string
	var delay, unique time.Duration
	for _, o := range c.opts[0] {
		switch o.Type() {
		case asynq.QueueOpt:
			queue = o.Value().(string)
		case asynq.ProcessInOpt:
			delay = o.Value().(time.Duration)
		case asynq.UniqueOpt:
			unique = o.Value().(time.Duration)
		}
	}
	if queue != QueueWhatsApp || delay != WhatsAppAIDebounce || unique != WhatsAppAIDebounce {
		t.Fatalf("queue %q delay %s unique %s", queue, delay, unique)
	}

	c.err = asynq.ErrDuplicateTask
	if err := (WhatsAppAIEnqueuer{Client: c}).EnqueueAIReply(context.Background(), conv); err != nil {
		t.Fatalf("duplicate within the window: %v", err)
	}
}
