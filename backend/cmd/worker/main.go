package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	whatsappmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
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
	notifSvc := notifmodule.NewService(notifmodule.Deps{
		Config: cfg, Queries: queries, Realtime: publisher,
		Mail: mail.NewSMTPSender(cfg.SMTP), SMS: sms.Noop{Log: log}, Log: log,
	})
	if err := notifusecase.SyncCatalog(ctx, queries); err != nil {
		log.Warn("notification_catalog_sync_failed", "error", err)
	}

	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	activityRec := activity.NewRecorder(queries, log)
	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		catalogusecase.NewSearchAdapter(queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// TEC-145: product import/export runs here; the import reindexes products.
	catalogSvc := catalogusecase.New(queries, searchIndexer)
	ioReg := ioengine.NewRegistry(
		catalogusecase.NewIOAdapter(catalogSvc, queries),
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	pdfClient := pdfrender.NewWithOptions(cfg.Gotenberg.URL, pdfrender.Options{MaxConnsPerHost: cfg.Queue.Concurrency})
	exportSvc.SetDocumentPDF(pdfClient)
	// Business modules (F1) register their SourceLoader per kind on docSvc.
	docSvc := docusecase.New(pool, queries, store, pdfClient, nil, pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(queries)
	ratesSvc := fxrates.New(queries, fxrates.NewFetcher(cfg.Rates.TCMBURL, cfg.Rates.ECBURL), log)

	persist := logging.Attach(log, logsSvc)
	log = persist.Logger()
	defer persist.Close()

	queues, err := parseWorkerQueues(os.Getenv("WORKER_QUEUES"))
	if err != nil {
		log.Error("worker_queues_invalid", "error", err)
		os.Exit(1)
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		log.Error("encryption_init_failed", "error", err)
		os.Exit(1)
	}
	waSvc := whatsappmodule.NewService(cfg.Wuzapi, pool, queries, secretBox, notifSvc, log)
	notifSvc.RegisterProvider(providers.WhatsAppProvider{WA: waSvc.Provider()})

	worker := queue.NewWorkerWithQueues(cfg, log, notifSvc.Deliver, queues).
		WithWhatsAppPoll(waSvc.PollStatus).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithNotificationPurge(notifSvc.PurgeExpired).
		WithDocsRender(docSvc.ProcessRender).
		WithRatesFetch(ratesSvc.FetchTask).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		)

	if searchIndexer.Enabled() {
		go searchIndexer.Bootstrap(ctx)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer func() { _ = rdb.Close() }()

	healthPath := os.Getenv("WORKER_HEALTH_FILE")
	if healthPath == "" {
		healthPath = "/tmp/worker.health"
	}
	go runHealthFile(ctx, rdb, healthPath, 15*time.Second, log)

	// Periodic tasks: only workers with SCHEDULER_ENABLED (default true) run
	// for the scheduler, and a Redis lease keeps a single leader among them.
	schedulerDone := make(chan struct{})
	if os.Getenv("SCHEDULER_ENABLED") != "false" {
		hostname, _ := os.Hostname()
		lock := &leaderLock{
			rdb: rdb,
			key: schedulerLockKey,
			id:  fmt.Sprintf("%s:%d", hostname, os.Getpid()),
			ttl: 30 * time.Second,
		}
		go func() {
			defer close(schedulerDone)
			runAsLeader(ctx, lock, 10*time.Second, log, func() func() {
				scheduler, err := queue.StartScheduler(cfg, log)
				if err == nil {
					err = queue.RegisterWhatsAppPoll(scheduler)
				}
				if err == nil {
					err = queue.RegisterNotificationPurge(scheduler)
				}
				if err == nil {
					err = scheduler.Start()
				}
				if err != nil {
					log.Error("scheduler_failed", "error", err)
					errtrack.CaptureTask(ctx, errtrack.TaskInfo{
						Type: "scheduler", Queue: queue.QueueMaintenance, Scheduler: true,
					}, err)
					return func() {}
				}
				return scheduler.Shutdown
			})
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
		"scheduler", os.Getenv("SCHEDULER_ENABLED") != "false",
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
