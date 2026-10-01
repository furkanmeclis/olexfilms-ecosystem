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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/httpserver"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
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
		notifSvc := notifmodule.NewService(notifmodule.Deps{
			Config: cfg, Queries: queries, Queue: queueClient, Realtime: publisher,
			Mail: mail.NewSMTPSender(cfg.SMTP), SMS: sms.Noop{Log: log}, Log: log,
		})
		if cfg.Queue.WorkerInProcess {
			worker = newInProcessWorker(cfg, log, notifSvc.Deliver)
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
	log.Info("server_stopped")
}

// newInProcessWorker builds the QUEUE_WORKER_INPROCESS worker. It only binds
// notification delivery: every other processor (export, import, bulk, log and
// notification purge, rates, WhatsApp poll, search, docs) is wired by
// httpserver.New from its own services. Wiring them here as well registered
// app:notifications:purge twice and panicked at startup (TEC-142).
func newInProcessWorker(cfg config.Config, log *slog.Logger, deliver queue.DeliverNotificationFunc) *queue.Worker {
	return queue.NewWorker(cfg, log, deliver)
}
