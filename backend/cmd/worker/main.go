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
	accountingposting "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	announcementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/announcements/usecase"
	appointmentreminder "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/reminder"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	campaignsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/usecase"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	certificatesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/certificates/usecase"
	contractsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts"
	contractsrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	contractsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	measurementsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements"
	measurementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	oauthmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	servicereview "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	shorturlsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	warehouseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	warrantyclaimsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	whatsappmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp"
	wapipeline "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/pipeline"
	whatsapprepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/anthropic"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
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
	// TEC-396: whatsapp.message.received arms the debounced whatsapp:ai_reply.
	wapipeline.RegisterEventHandlers(eventBus, queue.WhatsAppAIEnqueuer{Client: reviewQueue}, log)
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
	warrantyClaimsSvc := warrantyclaimsusecase.New(pool, queries, store, outboxStore)
	ioReg := ioengine.NewRegistry(
		catalogusecase.NewIOAdapter(catalogSvc, queries),
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
		// TEC-365: platform organizations list export (read only).
		orgusecase.NewListExportAdapter(orgusecase.New(pool, queries)),
		// TEC-175: cari statement and balance report exports (read only).
		accountingusecase.NewStatementAdapter(accountingSvc),
		accountingusecase.NewBalancesAdapter(accountingSvc),
		// TEC-346: P&L, margin, cari aging and staff cost report exports.
		accountingusecase.NewPnlAdapter(accountingSvc),
		accountingusecase.NewMarginAdapter(accountingSvc),
		accountingusecase.NewCariAgingAdapter(accountingSvc),
		accountingusecase.NewStaffCostAdapter(accountingSvc),
		// TEC-379: ledger entry list export.
		accountingusecase.NewEntriesAdapter(accountingSvc),
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
		// TEC-352: service reviews list export (read only).
		servicesusecase.NewReviewsExportAdapter(servicesusecase.New(pool, queries, nil)),
		// TEC-207: end-of-day report PDF (read only).
		warehouseusecase.NewEODPDFAdapter(warehouseusecase.NewEODPDF(eodSvc, store, log)),
		// TEC-338: warranty claim reports and CSV/XLSX exports.
		warrantyclaimsusecase.NewFailureRateAdapter(warrantyClaimsSvc),
		warrantyclaimsusecase.NewByDealerAdapter(warrantyClaimsSvc),
		warrantyclaimsusecase.NewPartsAdapter(warrantyClaimsSvc),
		// TEC-371: lead list export (read only).
		leadsusecase.NewListExportAdapter(leadsusecase.New(pool, queries, nil)),
		// TEC-373: order list and stock unit list exports (read only).
		ordersusecase.NewListExportAdapter(ordersusecase.New(pool, queries, nil, nil)),
		stockusecase.NewUnitsExportAdapter(stockusecase.New(queries)),
		// TEC-377: service and warranty list exports (read only).
		servicesusecase.NewListExportAdapter(servicesusecase.New(pool, queries, nil)),
		warrantyusecase.NewListExportAdapter(warrantyusecase.NewReader(pool, queries, nil, cfg.Auth.FrontendURL)),
		// TEC-389: AI usage report export (read only).
		aiusecase.NewUsageExportAdapter(aiusecase.NewAdmin(airepo.New(pool), llm.ModelsFromConfig(cfg.AI), nil)),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	pdfClient := pdfrender.NewWithOptions(cfg.Gotenberg.URL, pdfrender.Options{MaxConnsPerHost: cfg.Queue.Concurrency})
	exportSvc.SetDocumentPDF(pdfClient)
	// Business modules (F1) register their SourceLoader per kind on docSvc.
	docSvc := docusecase.New(pool, queries, store, pdfClient, nil, pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)
	// TEC-314: quote PDF source (worker-docs renders) and daily quote expiry.
	leadsSvc := leadsusecase.New(pool, queries, nil)
	leadsSvc.SetQuoteSenders(outboxStore, shorturlsmodule.NewLinker(queries, cfg.Auth.FrontendURL), reviewQueue)
	if err := docSvc.RegisterLoader(docmodel.KindQuote, leadsSvc); err != nil {
		log.Error("documents_loader_failed", "kind", docmodel.KindQuote, "error", err)
		os.Exit(1)
	}
	contractsSvc := contractsusecase.New(contractsrepo.New(pool, queries),
		contractsusecase.WithStorage(store),
		contractsusecase.WithPDFRenderer(pdfClient),
		contractsusecase.WithOutbox(outboxStore),
	)
	_ = docSvc.RegisterLoader(docmodel.KindContract, contractsSvc.ContractDocumentLoader())
	// TEC-298: measurement PDF (worker-docs renders, pdf_key caches it).
	measurementPDF := measurementsusecase.NewPDF(pool, queries, store, pdfClient, reviewQueue,
		pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)
	_ = docSvc.RegisterLoader(docmodel.KindMeasurement, measurementPDF.DocumentLoader())
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
		// TEC-365: platform organizations (status change, extend access).
		bulkadapters.NewOrganizations(queries),
		// TEC-212: tenant resources with undo.
		bulkadapters.NewCatalogProducts(queries),
		bulkadapters.NewTasks(queries),
		// TEC-369: catalog categories (tenant) and vehicle catalog (platform).
		bulkadapters.NewCatalogCategories(queries),
		bulkadapters.NewVehicleBrands(queries),
		bulkadapters.NewVehicleModels(queries),
		// TEC-371: leads (assign, set status).
		leadsusecase.NewBulkAdapter(queries),
		// TEC-398: conversations (close, assign, AI mode).
		whatsappusecase.NewBulkAdapter(queries),
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
	featureSvc := features.New(pool, queries, nil, log)
	certificatesCron := certificatesusecase.NewCron(pool, queries, outboxStore, featureSvc, sysconfig.New(queries, sysconfig.NoCache{}), log)

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
		// TEC-288: executed contract PDF (docs queue).
		WithContractPDF(contractsSvc.GenerateExecutedPDF).
		// TEC-298: measurement PDF (docs queue).
		WithMeasurementPDF(measurementPDF.GeneratePDF).
		WithRatesFetch(ratesSvc.FetchTask).
		WithWarrantyCron(warrantyCron.ExpireTask, warrantyCron.ExpiringScanTask).
		WithWarrantyRepairScan(warrantymodule.NewRepairScanner(pool, queries, cfg.Auth.FrontendURL, cfg.Warranty.RepairScanDays, log).Task).
		// TEC-190: expire pending vehicle transfers (5 min).
		WithVehicleTransferExpire(transferExpirer.ExpireTransfersTask).
		// TEC-192: delayed Google review request of a completed service.
		WithServiceReviewRequest(servicereview.NewTaskSender(pool, queries, cfg.Auth.FrontendURL, log).Task).
		// TEC-325: appointment WhatsApp reminders and no-show conversion.
		WithAppointmentReminder(appointmentreminder.NewTaskSender(pool, queries, log).Task).
		WithAppointmentNoShowScan(appointmentreminder.NewTaskNoShowScanner(pool, queries, featureSvc, log).Task).
		// TEC-156: nightly projection drift scan; report only, no repair.
		WithInventoryRebuild(stockrebuild.New(pool, queries).ScanTask(log)).
		// TEC-221: hourly center task due date reminders.
		// TEC-314: daily quote expiry (valid_until passed).
		WithQuoteExpire(leadsSvc.ExpireDueQuotesTask).
		WithQuoteReminder(leadsSvc.QuoteReminderTask).
		// TEC-400: hourly MCP OAuth cleanup (expired rows, abandoned clients).
		WithOAuthCleanup(oauthmodule.New(pool, featureSvc, nil, cfg.Auth.FrontendURL, log).Cleanup).
		WithTasksDueScan(tasksusecase.NewCron(pool, queries, outbox.NewStore(pool, queries)).DueScanTask).
		// TEC-207: hourly end-of-day warehouse reports (previous local day).
		WithWarehouseEOD(eodSvc.DailyTask(log)).
		// TEC-381: hourly booking of planned staff payments on their paid_on.
		WithStaffPaymentsPostDue(accountingusecase.New(pool, queries,
			accountingposting.New(queries, outboxStore, fxrates.New(queries, nil, log)), nil).PostDueStaffPaymentsTask).
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
		// TEC-393: 90-day retention of WhatsApp conversation AI runs.
		WithConversationAIRunPurge(whatsapprepo.New(pool).PurgeExpiredAIRuns).
		// TEC-387: AI confirmation card expiry and stale run cleanup.
		WithAIActionSweep(aiusecase.NewActions(airepo.New(pool), nil, nil, log).SweepTask).
		WithCertificateExpiryScan(certificatesCron.ExpiryScanTask).
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
	// TEC-394: user / membership / organization events drop the WhatsApp
	// identity cache.
	whatsappmodule.RegisterIdentityInvalidation(eventBus, rdb, cfg.App.Env, log)

	// TEC-395: WhatsApp outgoing queue, inbound media storage, receipts.
	waMsgs := whatsappmodule.NewMessaging(waSvc, pool, queries, whatsappmodule.MessagingDeps{
		Storage: store, Queue: queue.WhatsAppEnqueuer{Client: reviewQueue},
		Limiter:       ratelimit.New(rdb, cfg.App.Env),
		SendPerMinute: sysconfig.New(queries, sysconfig.NoCache{}).WhatsAppSendPerMinute,
		Publisher:     publisher,
	}, log)
	waMsgs.SetDocuments(whatsappmodule.NewDocumentRenderer(
		servicesusecase.NewPDFAdapter(servicePDF), warrantyusecase.NewCertificateAdapter(warrantyCert), pdfClient))
	worker.WithWhatsAppMessaging(waMsgs.ProcessSend, waMsgs.StoreInboundMedia, func(ctx context.Context) (int, error) {
		return waMsgs.RequeueStale(ctx, 2*time.Minute)
	})
	// TEC-392 (F4-01j): AI first triage of warranty claims (default queue).
	warrantyclaimsusecase.RegisterTriageHandlers(eventBus, queue.WarrantyTriageEnqueuer{Client: reviewQueue}, log)
	var triagePhotos llm.ObjectReader
	if store != nil {
		triagePhotos = store
	}
	worker.WithWarrantyClaimTriage(warrantyclaimsusecase.NewTriage(warrantyclaimsusecase.TriageDeps{
		Conn: pool, Provider: anthropic.NewFromConfig(cfg.AI), Models: llm.ModelsFromConfig(cfg.AI),
		Features: featureSvc, Storage: triagePhotos, Log: log,
	}).Auto)
	// TEC-396 (F4-02c): WhatsApp AI pipeline (whatsapp queue).
	worker.WithWhatsAppAIReply(newWhatsAppAIPipeline(whatsAppAIDeps{
		cfg: cfg, pool: pool, queries: queries, features: featureSvc, activity: activityRec,
		notifier: notifSvc, messaging: waMsgs, downloader: waSvc.MediaDownloader(), store: store,
		rdb: rdb, log: log,
	}).Process)

	// TEC-407 (F4-04d): campaign scheduler tick and recipient sends.
	campaignSvc := campaignsusecase.New(pool, queries, store)
	campaignSvc.SetOutbox(outboxStore)
	pushWeb, pushExpo := notifmodule.PushProviders(cfg, queries)
	campaignSender := campaignsusecase.NewSender(campaignSvc, campaignsusecase.SenderDeps{
		Push:              campaignsusecase.NotificationPush{Web: pushWeb, Expo: pushExpo},
		Email:             campaignsusecase.MailEmail{Mail: mail.NewSMTPSender(cfg.SMTP), Brand: notifmodule.EmailBrandFunc(queries, cfg)},
		WhatsApp:          campaignsusecase.ConversationWhatsApp{Queries: queries, Messaging: waMsgs},
		Media:             store,
		Queue:             queue.CampaignEnqueuer{Client: reviewQueue},
		Limiter:           ratelimit.New(rdb, cfg.App.Env),
		Settings:          sysconfig.New(queries, sysconfig.NoCache{}),
		UnsubscribeSecret: []byte(cfg.JWT.AccessSecret),
		FrontendURL:       cfg.Auth.FrontendURL,
		Log:               log,
	})
	worker.WithCampaigns(campaignSender.Tick, campaignSender.ProcessRecipient)

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
