package review

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
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

func completed(serviceID int64) events.Event {
	id := serviceID
	return events.New(events.ServiceCompleted).WithEntity("service", &id, nil).
		WithPayload(map[string]any{"service_id": float64(serviceID)})
}

// TEC-192: service.completed enqueues one service:review_request task for
// the service, delayed with ProcessIn by the configured delay, on the
// notifications queue, deduplicated by task id.
func TestSchedulerEnqueuesDelayedTask(t *testing.T) {
	q := &fakeQueue{}
	s := NewScheduler(q, 90*time.Minute, nil)
	if err := s.HandleServiceCompleted(context.Background(), completed(77)); err != nil {
		t.Fatal(err)
	}
	if len(q.tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(q.tasks))
	}
	if q.tasks[0].Type() != queue.TaskServiceReviewRequest {
		t.Fatalf("type = %s", q.tasks[0].Type())
	}
	p, err := queue.ParseServiceReviewRequestPayload(q.tasks[0].Payload())
	if err != nil || p.ServiceID != 77 {
		t.Fatalf("payload = %+v (%v)", p, err)
	}
	if v, ok := option(q.opts[0], asynq.ProcessInOpt); !ok || v.(time.Duration) != 90*time.Minute {
		t.Fatalf("ProcessIn = %v (%v)", v, ok)
	}
	if v, ok := option(q.opts[0], asynq.QueueOpt); !ok || v.(string) != queue.QueueNotifications {
		t.Fatalf("queue = %v", v)
	}
	if v, ok := option(q.opts[0], asynq.TaskIDOpt); !ok || v.(string) != queue.ServiceReviewRequestTaskID(77) {
		t.Fatalf("task id = %v", v)
	}
}

func TestSchedulerDefaultDelayIs24h(t *testing.T) {
	q := &fakeQueue{}
	if err := NewScheduler(q, 0, nil).HandleServiceCompleted(context.Background(), completed(5)); err != nil {
		t.Fatal(err)
	}
	if v, ok := option(q.opts[0], asynq.ProcessInOpt); !ok || v.(time.Duration) != 24*time.Hour {
		t.Fatalf("ProcessIn = %v", v)
	}
}

func TestSchedulerConflictAndErrors(t *testing.T) {
	// A pending task with the same id: redelivered event, nothing to do.
	q := &fakeQueue{err: asynq.ErrTaskIDConflict}
	if err := NewScheduler(q, time.Hour, nil).HandleServiceCompleted(context.Background(), completed(5)); err != nil {
		t.Fatalf("conflict: %v", err)
	}
	// Redis down: the error goes back to the outbox for a redelivery.
	q = &fakeQueue{err: errors.New("redis down")}
	if err := NewScheduler(q, time.Hour, nil).HandleServiceCompleted(context.Background(), completed(5)); err == nil {
		t.Fatal("want enqueue error")
	}
	// No queue (no worker) or no service id: skipped, no error.
	if err := NewScheduler(nil, time.Hour, nil).HandleServiceCompleted(context.Background(), completed(5)); err != nil {
		t.Fatalf("nil queue: %v", err)
	}
	q = &fakeQueue{}
	if err := NewScheduler(q, time.Hour, nil).HandleServiceCompleted(context.Background(), events.New(events.ServiceCompleted)); err != nil || len(q.tasks) != 0 {
		t.Fatalf("no service: err=%v tasks=%d", err, len(q.tasks))
	}
}

func TestLowScoreDescriptionOmitsAnonymousCustomer(t *testing.T) {
	d := db.GetServiceReviewProcessingDetailsRow{
		OrganizationName: "Dealer One",
		ServiceNo:        "DS000001",
		PlatformRating:   5,
		ProductRating:    1,
		MinRating:        1,
		IsAnonymous:      true,
		CustomerName:     "Ada",
		CustomerSurname:  "Lovelace",
		Comment:          pgtype.Text{String: "too many bubbles", Valid: true},
	}
	got := lowScoreDescription(d)
	if strings.Contains(got, "Ada") || strings.Contains(got, "Lovelace") || strings.Contains(got, "Customer:") {
		t.Fatalf("anonymous customer leaked into task description:\n%s", got)
	}
	if !strings.Contains(got, "DS000001") || !strings.Contains(got, "Product rating: 1") {
		t.Fatalf("description misses review context:\n%s", got)
	}

	d.IsAnonymous = false
	got = lowScoreDescription(d)
	if !strings.Contains(got, "Customer: Ada Lovelace") {
		t.Fatalf("non-anonymous customer missing:\n%s", got)
	}
}
