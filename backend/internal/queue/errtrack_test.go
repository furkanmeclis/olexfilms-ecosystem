package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack/errtracktest"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

func initErrtrack(t *testing.T) *errtracktest.Receiver {
	t.Helper()
	rcv := errtracktest.NewReceiver(t)
	tr := sentry.NewHTTPSyncTransport()
	tr.Timeout = 5 * time.Second
	if on, err := errtrack.Init(errtrack.Options{DSN: rcv.DSN(), ServiceName: "worker", Transport: tr}); err != nil || !on {
		t.Fatalf("init: %v %v", on, err)
	}
	t.Cleanup(func() { _, _ = errtrack.Init(errtrack.Options{}) })
	return rcv
}

// A failing asynq task reaches the receiver with task type, queue and module.
func TestFailedTaskIsReported(t *testing.T) {
	rcv := initErrtrack(t)
	mr := miniredis.RunT(t)
	cfg := config.Config{Redis: config.RedisConfig{Addr: mr.Addr()}}
	cfg.Queue.Concurrency = 1

	deliver := func(context.Context, int64) error { return errors.New("smtp unreachable for ali@example.com") }
	w := NewWorkerWithQueues(cfg, nil, deliver, map[string]int{QueueNotifications: 1})
	if err := w.server.Start(w.mux); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(w.Shutdown)

	client := asynq.NewClient(RedisOpt(cfg.Redis))
	t.Cleanup(func() { _ = client.Close() })
	task, err := NewNotificationDeliverTask(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Enqueue(task, asynq.Queue(QueueNotifications), asynq.MaxRetry(0)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	events := rcv.WaitEvents(1, 10*time.Second)
	if len(events) == 0 {
		t.Fatal("no event for failed task")
	}
	ev := events[0]
	for k, v := range map[string]string{
		"task_type": TaskNotificationDeliver,
		"queue":     QueueNotifications,
		"module":    "notification",
		"component": "worker",
		"max_retry": "0",
	} {
		if ev.Tags[k] != v {
			t.Errorf("tag %s = %q, want %q (tags=%v)", k, ev.Tags[k], v, ev.Tags)
		}
	}
	if ex := ev.Exceptions(); len(ex) == 0 || ex[len(ex)-1].Value != "smtp unreachable for [email]" {
		t.Errorf("exception = %+v", ex)
	}
}

// Scheduler enqueue failures are reported with the scheduler component.
func TestSchedulerErrorIsReported(t *testing.T) {
	rcv := initErrtrack(t)
	task, err := NewLogPurgeSweepTask()
	if err != nil {
		t.Fatal(err)
	}
	SchedulerErrorHandler(nil)(task, []asynq.Option{asynq.Queue(QueueMaintenance)}, errors.New("redis: connection refused"))
	errtrack.Flush(2 * time.Second)
	events := rcv.WaitEvents(1, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	for k, v := range map[string]string{
		"task_type": TaskLogPurgeSweep,
		"queue":     QueueMaintenance,
		"component": "scheduler",
		"module":    "unknown",
	} {
		if ev.Tags[k] != v {
			t.Errorf("tag %s = %q, want %q", k, ev.Tags[k], v)
		}
	}
}
