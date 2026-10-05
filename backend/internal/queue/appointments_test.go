package queue

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/hibiken/asynq"
)

func TestAppointmentReminderHandler(t *testing.T) {
	w := NewWorker(config.Config{Redis: config.RedisConfig{Addr: "127.0.0.1:0"}}, nil, nil)
	starts := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	task, err := NewAppointmentReminderTask(42, starts, "24h")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.mux.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unwired: %v", err)
	}
	var gotID int64
	var gotStarts time.Time
	var gotKind string
	w.WithAppointmentReminder(func(_ context.Context, id int64, s time.Time, kind string) error {
		gotID, gotStarts, gotKind = id, s, kind
		return nil
	})
	if err := w.mux.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if gotID != 42 || !gotStarts.Equal(starts) || gotKind != "24h" {
		t.Fatalf("payload = %d %s %s", gotID, gotStarts, gotKind)
	}
	if err := w.mux.ProcessTask(context.Background(), asynq.NewTask(TaskAppointmentReminder, []byte("x"))); err == nil {
		t.Fatal("bad payload: want error")
	}
}
