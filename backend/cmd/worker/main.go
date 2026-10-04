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
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	announcementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/announcements/usecase"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	measurementsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	servicereview "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	warehouseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
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

	reviewQueue := queue.NewClient(cfg.Redis)
	defer func() { _ = reviewQueue.Close() }()
	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log,
		notifmodule.WithAnnouncementFanout(queries, reviewQueue))
	// TEC-186: service.completed opens one warranty per service item.
	warrantymodule.RegisterEventHandlers(eventBus, pool, queries, cfg.Auth.FrontendURL, log)
	// TEC-192: service.completed schedules the delayed review request.
	servicereview.RegisterEventHandlers(eventBus, reviewQueue, cfg.Services.ReviewRequestDelay, log)
	// TEC-270: glorian stock entries/placements and exits schedule the push.
	glorian.RegisterEventHandlers(eventBus, queries, reviewQueue, log)
	// TEC-296: service events compute the before/after measurement match.
	measurementsmodule.RegisterEventHandlers(eventBus, pool, queries, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	activityRec := activity.NewRecorder(queries, log)
	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		catalogusecase.NewSearchAdapter(queries),
		customersusecase.NewSearchAdapter(queries), // TEC-164
		// TEC-209: services, warranties, vehicles (plate / VIN).
		servicesusecase.NewSearchAdapter(queries),
		warrantyusecase.NewSearchAdapter(queries),
		customersusecase.NewVehicleSearchAdapter(queries),
		// TEC-210: organizations (dealer code), orders, stock units (barcode).
		orgusecase.NewSearchAdapter(queries),
		ordersusecase.NewSearchAdapter(queries),
		stockusecase.NewSearchAdapter(queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// TEC-209: service / warranty / vehicle outbox events refresh the indexes.
	indexsync.Register(eventBus, queries, searchIndexer, log)
	// TEC-145: product import/export runs here; the import reindexes products.
	catalogSvc := catalogusecase.New(queries, searchIndexer)
	// Exports only read the ledger: no poster, no feature checker.
	accountingSvc := accountingusecase.New(pool, queries, nil, nil)
	customersExportSvc := customersusecase.New(pool, queries, nil, nil)
	warrantyCert := warrantymodule.NewCertificate(queries, store, cfg.Auth.FrontendURL, log)
	// TEC-196: the service PDF only reads (no outbox).
	servicePDF := servicesusecase.NewPDF(servicesusecase.New(pool, queries, nil), warrantyCert, store, log)
	// TEC-207: end-of-day reports (cron on worker-core, PDF on worker-docs).
	eodSvc := warehouseusecase.NewEOD(pool, queries)
	ioReg := ioengine.NewRegistry(
		catalogusecase.NewIOAdapter(catalogSvc, queries),
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
		// TEC-175: cari statement and balance report exports (read only).
		accountingusecase.NewStatementAdapter(accountingSvc),
		accountingusecase.NewBalancesAdapter(accountingSvc),
		// TEC-161: personal data export (read only; identity numbers stay
		// masked, so no PII key is needed here).
		customersusecase.NewDataExportAdapter(customersExportSvc),
		customersusecase.NewPortalDataExportAdapter(customersExportSvc),
		// TEC-164: customer list export (read only, same masking as the list).
		customersusecase.NewListExportAdapter(customersExportSvc),
		// TEC-158: stock import batches are applied here (import queue).
		stockusecase.NewImporter(pool, queries, outboxStore),
		// TEC-188: warranty certificate PDF (panel and portal, read only).
		warrantyusecase.NewCertificateAdapter(warrantyCert),
		warrantyusecase.NewPortalCertificateAdapter(warrantyCert),
		// TEC-196: service PDF (read only).
		servicesusecase.NewPDFAdapter(servicePDF),
		// TEC-207: end-of-day report PDF (read only).
		warehouseusecase.NewEODPDFAdapter(warehouseusecase.NewEODPDF(eodSvc, store, log)),
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
		// TEC-212: tenant resources with undo.
		bulkadapters.NewCatalogProducts(queries),
		bulkadapters.NewTasks(queries),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log).
		WithPool(pool).WithUndoWindow(sysconfig.New(queries, sysconfig.NoCache{}).BulkUndoWindowHours)
	logsSvc := logsusecase.New(queries)
	ratesSvc := fxrates.New(queries, fxrates.NewFetcher(cfg.Rates.TCMBURL, cfg.Rates.ECBURL), log)
	warrantyCron := warrantymodule.NewCron(pool, queries, cfg.Auth.FrontendURL)
	// TEC-190: only the transfer expiry of the customers service runs here.
	transferExpirer := customersusecase.New(pool, queries, nil, nil)
	transferExpirer.SetOutbox(outbox.NewStore(pool, queries))

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
	glorianPusher := glorian.NewPusher(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log)
	glorianOrders := glorian.NewOrderOutbounder(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log)
	waSvc := whatsappmodule.NewService(cfg.Wuzapi, pool, queries, secretBox, notifSvc, log)
	notifSvc.RegisterProvider(providers.WhatsAppProvider{WA: waSvc.Provider()})

	worker := queue.NewWorkerWithQueues(cfg, log, notifSvc.Deliver, queues).
		WithWhatsAppPoll(waSvc.PollStatus).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithNotificationPurge(notifSvc.PurgeExpired).
		WithAnnouncementDispatch(func(ctx context.Context, payload queue.AnnouncementDispatchPayload) error {
			return announcementsusecase.DispatchBatch(ctx, notifSvc, payload)
		}).
		WithDocsRender(docSvc.ProcessRender).
		WithRatesFetch(ratesSvc.FetchTask).
		WithWarrantyCron(warrantyCron.ExpireTask, warrantyCron.ExpiringScanTask).
		WithWarrantyRepairScan(warrantymodule.NewRepairScanner(pool, queries, cfg.Auth.FrontendURL, cfg.Warranty.RepairScanDays, log).Task).
		// TEC-190: expire pending vehicle transfers (5 min).
		WithVehicleTransferExpire(transferExpirer.ExpireTransfersTask).
		// TEC-192: delayed Google review request of a completed service.
		WithServiceReviewRequest(servicereview.NewTaskSender(pool, queries, log).Task).
		// TEC-156: nightly projection drift scan; report only, no repair.
		WithInventoryRebuild(stockrebuild.New(pool, queries).ScanTask(log)).
		// TEC-221: hourly center task due date reminders.
		WithTasksDueScan(tasksusecase.NewCron(pool, queries, outbox.NewStore(pool, queries)).DueScanTask).
		// TEC-207: hourly end-of-day warehouse reports (previous local day).
		WithWarehouseEOD(eodSvc.DailyTask(log)).
		// TEC-268: Glorian catalog and dealer pull (active connections only).
		WithGlorianPull(glorian.NewPuller(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), searchIndexer, log).Task).
		// TEC-270: Glorian barcode bulk push and outbound PATCH by barcode.
		WithGlorianPush(glorianPusher.PushTask, glorianPusher.PatchTask).
		// TEC-271: Glorian order outbound (POST /orders, ship/receive/cancel) and held replay.
		WithGlorianOrderOutbound(glorianOrders.OrderTask, glorianOrders.ReplayTask).
		// TEC-273: admin-triggered reconcile run and single outbound replay.
		WithGlorianAdmin(
			glorian.NewReconciler(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log).ReconcileTask,
			glorianOrders.ReplayOneTask,
		).
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
