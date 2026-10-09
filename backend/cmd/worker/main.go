package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	accountingposting "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	appointmentreminder "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/reminder"
	contractsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts"
	efficiencymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	measurementsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	performancemodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance"
	performanceusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	pricingmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	servicecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	servicereview "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyclaimsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	whatsappmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp"
	wapipeline "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/pipeline"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/workerapp"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	if on, err := errtrack.Init(errtrack.OptionsFromEnv(cfg.App.Env, "worker")); err != nil {
		log.Warn("errtrack_init_failed", "error", err)
	} else if on {
		log.Info("errtrack_enabled", "release", errtrack.OptionsFromEnv(cfg.App.Env, "worker").Release)
	}
	defer errtrack.Flush(2 * time.Second)
	if !cfg.Queue.Enabled {
		log.Error("queue_disabled")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		os.Exit(1)
	}

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(pool)
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		log.Error("encryption_init_failed", "error", err)
		os.Exit(1)
	}
	// TEC-143: same factory as the in-process worker and the API server.
	notifSvc, waSvc := notifmodule.NewWithWhatsApp(notifmodule.Deps{
		Config: cfg, Queries: queries, Realtime: publisher,
		Mail: mail.NewSMTPSender(cfg.SMTP), SMS: sms.Noop{Log: log}, Log: log,
	}, pool, secretBox)
	if err := notifusecase.SyncCatalog(ctx, queries); err != nil {
		log.Warn("notification_catalog_sync_failed", "error", err)
	}

	reviewQueue := queue.NewClient(cfg.Redis)
	defer func() { _ = reviewQueue.Close() }()
	eventBus := events.NewBus(log)
	outboxStore := outbox.NewStore(pool, queries)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log,
		notifmodule.WithAnnouncementFanout(queries, reviewQueue),
		notifmodule.WithAIQuotaRecipients(queries)) // TEC-389
	// TEC-186: service.completed opens one warranty per service item.
	warrantymodule.RegisterEventHandlers(eventBus, pool, queries, cfg.Auth.FrontendURL, log)
	// TEC-336: approved claims open and track their re-application service.
	warrantyclaimsusecase.RegisterEventHandlers(eventBus, pool, queries, outbox.NewStore(pool, queries), log)
	// TEC-337: a completed re-application service books the claim's warranty
	// cost, product refund and labor (rates from the stored table).
	warrantyclaimsusecase.RegisterAccountingHandlers(eventBus, pool,
		accountingposting.New(queries, outboxStore, fxrates.New(queries, nil, log)),
		sysconfig.New(queries, sysconfig.NoCache{}), log)
	// TEC-308: an approved early cancellation books its fee on both ledgers.
	servicecatalogusecase.RegisterAccountingHandlers(eventBus, servicecatalogusecase.New(queries).
		WithLifecycle(pool, nil, nil, nil).
		WithAccounting(accountingposting.New(queries, outboxStore, fxrates.New(queries, nil, log))), log)
	// TEC-192: service.completed schedules the delayed review request.
	servicereview.RegisterEventHandlers(eventBus, reviewQueue, cfg.Services.ReviewRequestDelay, log)
	// TEC-352: service.reviewed opens low-score tasks and dealer notifications.
	servicereview.RegisterProcessingHandlers(eventBus, servicereview.NewProcessor(pool, queries, outboxStore, log))
	// TEC-325: appointment.created/rescheduled schedules 24h and 2h reminders.
	appointmentreminder.RegisterEventHandlers(eventBus, reviewQueue, log)
	// TEC-270: glorian stock entries/placements and exits schedule the push.
	glorian.RegisterEventHandlers(eventBus, queries, reviewQueue, log)
	// TEC-288: contract.executed enqueues the worker-docs contract:pdf task.
	contractsmodule.RegisterEventHandlers(eventBus, reviewQueue, log)
	// TEC-296: service events compute the before/after measurement match.
	measurementsmodule.RegisterEventHandlers(eventBus, pool, queries, log)
	// TEC-488: efficiency facts are historical projections, independent of
	// the read-side feature gate.
	efficiencymodule.RegisterEventHandlers(eventBus, queries, log)
	// TEC-506: recommended prices: the publication event queues the price
	// list PDFs (docs queue). The hourly tick is bound in workerapp.
	recommendedSvc := pricingusecase.NewRecommended(pool, queries, outboxStore, sysconfig.New(queries, sysconfig.NoCache{}), log)
	recommendedSvc.SetPriceListQueue(queue.PriceListEnqueuer{Client: reviewQueue})
	pricingmodule.RegisterEventHandlers(eventBus, recommendedSvc)
	// TEC-492: performance.computed evaluates weak-dealer rules.
	performancemodule.RegisterEventHandlers(eventBus,
		performanceusecase.New(pool, queries, outboxStore).WithPanelURL(cfg.Auth.FrontendURL), log)
	// TEC-396: whatsapp.message.received arms the debounced whatsapp:ai_reply.
	wapipeline.RegisterEventHandlers(eventBus, queue.WhatsAppAIEnqueuer{Client: reviewQueue}, log)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	persist := logging.Attach(log, logsusecase.New(queries))
	log = persist.Logger()
	defer persist.Close()

	queues, err := parseWorkerQueues(os.Getenv("WORKER_QUEUES"))
	if err != nil {
		log.Error("worker_queues_invalid", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer func() { _ = rdb.Close() }()

	// TEC-527: every task processor comes from the factory the in-process
	// worker (QUEUE_WORKER_INPROCESS) uses too.
	worker := queue.NewWorkerWithQueues(cfg, log, notifSvc.Deliver, queues)
	handlers, err := workerapp.Register(worker, workerapp.Deps{
		Config: cfg, Pool: pool, Queries: queries, Storage: store, Realtime: publisher,
		Redis: rdb, Queue: reviewQueue, Notifications: notifSvc, WhatsApp: waSvc, Log: log,
	})
	if err != nil {
		log.Error("worker_handlers_failed", "error", err)
		os.Exit(1)
	}
	// TEC-209: service / warranty / vehicle outbox events refresh the indexes.
	indexsync.Register(eventBus, queries, handlers.SearchIndexer, log)
	if handlers.SearchIndexer.Enabled() {
		go handlers.SearchIndexer.Bootstrap(ctx)
	}
	// TEC-394: user / membership / organization events drop the WhatsApp
	// identity cache.
	whatsappmodule.RegisterIdentityInvalidation(eventBus, rdb, cfg.App.Env, log)
	// TEC-392 (F4-01j): claim events arm the AI first triage (default queue).
	warrantyclaimsusecase.RegisterTriageHandlers(eventBus, queue.WarrantyTriageEnqueuer{Client: reviewQueue}, log)

	healthPath := os.Getenv("WORKER_HEALTH_FILE")
	if healthPath == "" {
		healthPath = "/tmp/worker.health"
	}
	go runHealthFile(ctx, rdb, healthPath, 15*time.Second, log)

	// Periodic tasks: only workers with SCHEDULER_ENABLED (default true) run
	// for the scheduler, and a Redis lease keeps a single leader among them
	// (shared with the in-process worker, TEC-143).
	schedulerDone := make(chan struct{})
	if queue.SchedulerEnabled() {
		go func() {
			defer close(schedulerDone)
			queue.RunScheduler(ctx, cfg, rdb, log)
		}()
	} else {
		close(schedulerDone)
	}

	log.Info(
		"worker_started",
		"app", cfg.App.Name,
		"env", cfg.App.Env,
		"concurrency", cfg.Queue.Concurrency,
		"queues", queues,
		"scheduler", queue.SchedulerEnabled(),
	)

	if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
		log.Error("notification_reclaim_failed", "error", err)
	} else if n > 0 {
		log.Info("notification_reclaim_completed", "count", n)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("worker_shutdown_signal")
		worker.Shutdown()
		<-schedulerDone
	case err := <-errCh:
		if err != nil {
			log.Error("worker_failed", "error", err)
			errtrack.Capture(context.Background(), errtrack.ModuleUnknown, err, errtrack.Tags{errtrack.TagComponent: "worker"})
			errtrack.Flush(2 * time.Second)
			os.Exit(1)
		}
	}
	log.Info("worker_stopped")
}
