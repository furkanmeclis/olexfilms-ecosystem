package queue

import (
	"context"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/hibiken/asynq"
)

// TEC-192: service:review_request is registered on the worker mux and
// hands the payload's service id to the processor; unwired it is a no-op.
func TestServiceReviewRequestHandler(t *testing.T) {
	w := NewWorker(config.Config{Redis: config.RedisConfig{Addr: "127.0.0.1:0"}}, nil, nil)
	task, err := NewServiceReviewRequestTask(42)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.mux.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unwired: %v", err)
	}
	var got int64
	w.WithServiceReviewRequest(func(_ context.Context, id int64) error { got = id; return nil })
	if err := w.mux.ProcessTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("service id = %d, want 42", got)
	}
	if err := w.mux.ProcessTask(context.Background(), asynq.NewTask(TaskServiceReviewRequest, []byte("x"))); err == nil {
		t.Fatal("bad payload: want error")
	}
}
