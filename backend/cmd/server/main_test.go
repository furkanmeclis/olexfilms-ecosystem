package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/httpserver"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/hibiken/asynq"
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

// TEC-143: the in-process worker delivers through the notification center
// of the cmd/worker factory: the same channel drivers, with the real
// WhatsApp provider instead of the placeholder.
func TestInProcessDeliveryRegistersWorkerProviders(t *testing.T) {
	var cfg config.Config
	cfg.Encryption.Key = "app-dev-encryption-key-32bytes!!"
	cfg.VAPID.PublicKey, cfg.VAPID.PrivateKey = "pub", "priv"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := db.New(nil)

	inProcess, err := newInProcessDelivery(cfg, nil, q, nil, realtime.NoopPublisher{}, log)
	if err != nil {
		t.Fatalf("newInProcessDelivery: %v", err)
	}
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	// cmd/worker builds its delivery side with this factory.
	worker, _ := notifmodule.NewWithWhatsApp(notifmodule.Deps{Config: cfg, Queries: q, Log: log}, nil, box)

	if got, want := inProcess.Channels(), worker.Channels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("in-process channels = %v, cmd/worker channels = %v", got, want)
	}
	for _, ch := range []string{
		providers.ChannelWhatsApp, notifmodel.ChannelInapp, notifmodel.ChannelEmail,
		notifmodel.ChannelSMS, notifmodel.ChannelExpoPush, notifmodel.ChannelWebPush,
	} {
		if inProcess.Provider(ch) == nil {
			t.Errorf("channel %q has no driver", ch)
		}
	}
	if _, ok := inProcess.Provider(providers.ChannelWhatsApp).(providers.WhatsAppProvider); !ok {
		t.Fatalf("in-process whatsapp driver = %T, want providers.WhatsAppProvider", inProcess.Provider(providers.ChannelWhatsApp))
	}
}

// TEC-143: with QUEUE_WORKER_INPROCESS the API runs the periodic scheduler
// under the worker's leader lock, with the worker's task list. Next to a
// second scheduler (another API, or cmd/worker) only one publishes entries.
func TestInProcessSchedulerSharesWorkerSchedulesAndLock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	var cfg config.Config
	cfg.Redis.Addr = mr.Addr()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if done := startInProcessScheduler(context.Background(), cfg, rdb, false, log); !isClosed(done) {
		t.Fatal("without the in-process worker no scheduler may start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	inProcessDone := startInProcessScheduler(ctx, cfg, rdb, true, log)
	otherDone := make(chan struct{})
	go func() {
		defer close(otherDone)
		queue.RunScheduler(ctx, cfg, rdb, log) // what cmd/worker runs
	}()
	defer func() {
		cancel()
		<-inProcessDone
		<-otherDone
	}()

	want := map[string]int{}
	for _, p := range queue.Schedules() {
		want[p.Type+" "+p.Cron]++
	}
	inspector := asynq.NewInspector(queue.RedisOpt(cfg.Redis))
	defer func() { _ = inspector.Close() }()
	entries := func() map[string]int {
		list, err := inspector.SchedulerEntries()
		if err != nil {
			t.Fatalf("scheduler entries: %v", err)
		}
		got := map[string]int{}
		for _, e := range list {
			got[e.Task.Type()+" "+e.Spec]++
		}
		return got
	}
	// asynq publishes entries on its heartbeat (5 s).
	deadline := time.Now().Add(15 * time.Second)
	for len(entries()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no scheduler published its entries")
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	if got := entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("scheduler entries = %v, want a single scheduler with %v", got, want)
	}
	if holder, err := mr.Get("worker:scheduler:leader"); err != nil || holder == "" {
		t.Fatalf("leader lock not held: %q %v", holder, err)
	}
	for _, typ := range []string{queue.TaskWhatsAppStatusPoll, queue.TaskNotificationPurge, queue.TaskRatesFetch, queue.TaskLogPurgeSweep} {
		found := false
		for k := range want {
			found = found || strings.HasPrefix(k, typ+" ")
		}
		if !found {
			t.Errorf("periodic task %s missing", typ)
		}
	}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
