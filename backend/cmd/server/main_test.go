package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/httpserver"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TEC-142 regression: with QUEUE_WORKER_INPROCESS=true main builds the
// worker and hands it to httpserver.New, which wires the remaining task
// processors. That used to panic with "asynq: multiple registrations for
// app:notifications:purge".
func TestInProcessWorkerWithHTTPServerDoesNotPanic(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	var cfg config.Config
	cfg.App.Name, cfg.App.Env, cfg.App.DefaultBrandSlug = "test", "test", "olex"
	cfg.JWT.AccessSecret = strings.Repeat("a", 40)
	cfg.JWT.RefreshSecret = strings.Repeat("b", 40)
	cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL = 15*time.Minute, time.Hour
	cfg.Encryption.Key = "app-dev-encryption-key-32bytes!!"
	cfg.Auth.AdapterSecret = strings.Repeat("s", 40)
	cfg.Auth.FrontendURL = "http://localhost:3000"
	cfg.Redis.Addr = mr.Addr()
	cfg.Queue.Enabled = true
	cfg.Queue.WorkerInProcess = true
	cfg.Queue.Concurrency = 1

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := newInProcessWorker(cfg, log, func(context.Context, int64) error { return nil })

	var srv *httpserver.Server
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("httpserver.New with in-process worker panicked: %v", r)
			}
		}()
		srv, err = httpserver.New(cfg, log, httpserver.Deps{
			DB:      pool,
			Queries: db.New(pool),
			Redis:   rdb,
			Queue:   queue.NewClient(cfg.Redis),
			Worker:  worker,
		})
	}()
	if err != nil {
		t.Fatalf("httpserver.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
