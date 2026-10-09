package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/cache"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/httpserver"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/workerapp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	if on, err := errtrack.Init(errtrack.OptionsFromEnv(cfg.App.Env, "server")); err != nil {
		log.Warn("errtrack_init_failed", "error", err)
	} else if on {
		log.Info("errtrack_enabled", "release", errtrack.OptionsFromEnv(cfg.App.Env, "server").Release)
	}
	defer errtrack.Flush(2 * time.Second)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Error("redis_init_failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		os.Exit(1)
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := store.Ping(pingCtx); err != nil {
		pingCancel()
		log.Error("storage_ping_failed", "error", err)
		os.Exit(1)
	}
	pingCancel()

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(db)
	logsSvc := logsusecase.New(queries)
	persist := logging.Attach(log, logsSvc)
	log = persist.Logger()
	defer persist.Close()

	eventBus := events.NewBus(log)
	var queueClient *queue.Client
	var worker *queue.Worker
	if cfg.Queue.Enabled {
		queueClient = queue.NewClient(cfg.Redis)
		if cfg.Queue.WorkerInProcess {
			notifSvc, waSvc, err := newInProcessDelivery(cfg, db, queries, queueClient, publisher, log)
			if err != nil {
				log.Error("encryption_init_failed", "error", err)
				os.Exit(1)
			}
			worker, err = newInProcessWorker(cfg, log, workerapp.Deps{
				Config: cfg, Pool: db, Queries: queries, Storage: store, Realtime: publisher,
				Redis: rdb, Queue: queueClient, Notifications: notifSvc, WhatsApp: waSvc, Log: log,
			})
			if err != nil {
				log.Error("worker_handlers_failed", "error", err)
				os.Exit(1)
			}
			if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
				log.Error("notification_reclaim_failed", "error", err)
			} else if n > 0 {
				log.Info("notification_reclaim_completed", "count", n)
			}
		}
	}

	srv, err := httpserver.New(cfg, log, httpserver.Deps{
		DB:       db,
		Queries:  queries,
		Redis:    rdb,
		Queue:    queueClient,
		Storage:  store,
		Realtime: publisher,
		Worker:   worker,
		Events:   eventBus,
	})
	if err != nil {
		log.Error("httpserver_init_failed", "error", err)
		os.Exit(1)
	}

	schedulerDone := startInProcessScheduler(ctx, cfg, rdb, worker != nil, log)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown_signal_received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server_failed", "error", err)
			errtrack.Capture(context.Background(), errtrack.ModuleUnknown, err, errtrack.Tags{errtrack.TagComponent: "server"})
			errtrack.Flush(2 * time.Second)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown_failed", "error", err)
		os.Exit(1)
	}
	stop()
	<-schedulerDone
	log.Info("server_stopped")
}

// newInProcessWorker builds the QUEUE_WORKER_INPROCESS worker with every
// task processor cmd/worker binds, from the same factory (workerapp), so
// each task the scheduler or the API enqueues has a handler here too
// (TEC-527). httpserver.New only starts and stops it.
func newInProcessWorker(cfg config.Config, log *slog.Logger, deps workerapp.Deps) (*queue.Worker, error) {
	w := queue.NewWorker(cfg, log, nil)
	if _, err := workerapp.Register(w, deps); err != nil {
		return nil, err
	}
	return w, nil
}

// newInProcessDelivery builds the notification center the in-process worker
// delivers with, from the factory cmd/worker uses, so the WhatsApp provider
// (and every other channel driver) is the same in both modes (TEC-143).
func newInProcessDelivery(cfg config.Config, pool *pgxpool.Pool, queries *db.Queries, q notifusecase.Enqueuer, publisher realtime.Publisher, log *slog.Logger) (*notifusecase.Service, *whatsappusecase.Service, error) {
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, nil, err
	}
	svc, wa := notifmodule.NewWithWhatsApp(notifmodule.Deps{
		Config: cfg, Queries: queries, Queue: q, Realtime: publisher,
		Mail: mail.NewSMTPSender(cfg.SMTP), SMS: sms.Noop{Log: log}, Log: log,
	}, pool, box)
	return svc, wa, nil
}

// startInProcessScheduler runs the periodic scheduler next to the in-process
// worker: the same task list and Redis leader lock as cmd/worker, so with
// several API instances (or an API next to a worker) only one schedules
// (TEC-143). SCHEDULER_ENABLED=false turns it off as on the worker. The
// returned channel closes once the scheduler stopped (after ctx ends).
func startInProcessScheduler(ctx context.Context, cfg config.Config, rdb redis.UniversalClient, inProcess bool, log *slog.Logger) <-chan struct{} {
	done := make(chan struct{})
	if !inProcess || !queue.SchedulerEnabled() {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		queue.RunScheduler(ctx, cfg, rdb, log)
	}()
	return done
}
