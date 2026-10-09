package workerapp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/redis/go-redis/v9"
)

// testDeps wires the factory without live services: constructors only store
// their dependencies, nothing connects until a task runs.
func testDeps(t *testing.T) (config.Config, Deps) {
	t.Helper()
	var cfg config.Config
	cfg.Encryption.Key = "app-dev-encryption-key-32bytes!!"
	cfg.Redis.Addr = "127.0.0.1:0"
	cfg.Auth.FrontendURL = "http://localhost:3000"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := db.New(nil)
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	notif, wa := notifmodule.NewWithWhatsApp(notifmodule.Deps{Config: cfg, Queries: q, Log: log}, nil, box)
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	t.Cleanup(func() { _ = rdb.Close() })
	qc := queue.NewClient(cfg.Redis)
	t.Cleanup(func() { _ = qc.Close() })
	return cfg, Deps{
		Config: cfg, Queries: q, Redis: rdb, Queue: qc,
		Notifications: notif, WhatsApp: wa, Log: log,
	}
}

// TEC-527: the factory cmd/worker and the in-process worker share binds a
// processor for every task type the worker handles, so none is dropped
// with a "handler missing" warning. A task type added to the worker without
// a processor here fails this test.
func TestRegisterBindsEveryTaskType(t *testing.T) {
	cfg, deps := testDeps(t)
	w := queue.NewWorker(cfg, deps.Log, nil)
	h, err := Register(w, deps)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if h == nil || h.SearchIndexer == nil {
		t.Fatal("Register must return the search indexer")
	}
	if unbound := w.Unbound(); len(unbound) > 0 {
		t.Fatalf("task types without a processor: %v", unbound)
	}
}

// TEC-527: a worker that consumes only some queues (WORKER_QUEUES) is bound
// the same way: the queue split decides what it consumes, not what it binds.
func TestRegisterBindsQueueSubsetWorker(t *testing.T) {
	cfg, deps := testDeps(t)
	w := queue.NewWorkerWithQueues(cfg, deps.Log, nil, map[string]int{queue.QueueDocs: 1})
	if _, err := Register(w, deps); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if unbound := w.Unbound(); len(unbound) > 0 {
		t.Fatalf("task types without a processor: %v", unbound)
	}
}
