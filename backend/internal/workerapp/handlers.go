// Package workerapp binds every Asynq task processor on a queue.Worker. It is
// the single source for cmd/worker and the API's in-process worker
// (QUEUE_WORKER_INPROCESS), so both handle exactly the same task types with
// the same services (TEC-527). Event bus listeners, the outbox publisher, the
// scheduler and the search bootstrap stay with the caller.
package workerapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
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
	contractsrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	contractsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	showcaseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/usecase"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	efficiencymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency"
	einvoiceusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	featurehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/features/handler"
	fleetusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	libraryusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	measurementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	oauthmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	performanceusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	photostandardusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/usecase"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	searchregistry "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/registry"
	servicecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	servicereview "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	shorturlsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	stockforecastusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/usecase"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	warehouseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	warrantyclaimsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	whatsappmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp"
	whatsapprepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/anthropic"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/places"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Deps are the process resources the task processors run on.
type Deps struct {
	Config   config.Config
	Pool     *pgxpool.Pool
	Queries  *db.Queries
	Storage  storage.Driver
	Realtime realtime.Publisher
	Redis    *redis.Client
	// Queue enqueues follow-up tasks (PDFs, reminders, campaign sends).
	Queue *queue.Client
	// Notifications and WhatsApp come from notifmodule.NewWithWhatsApp
	// (TEC-143): the delivery side of the worker.
	Notifications *notifusecase.Service
	WhatsApp      *whatsappusecase.Service
	Log           *slog.Logger
}

// Handlers are the services the caller needs after Register.
type Handlers struct {
	// SearchIndexer processes the search tasks; cmd/worker also bootstraps
	// the indexes and feeds it from the outbox (indexsync).
	SearchIndexer *searchengine.Indexer
}

// Register sets every task processor on w. Afterwards w.Unbound() is empty
// (TEC-527 test), so no task the backend enqueues is dropped with a
// "handler missing" warning.
func Register(w *queue.Worker, d Deps) (*Handlers, error) {
	cfg, pool, queries, store, log := d.Config, d.Pool, d.Queries, d.Storage, d.Log
	if log == nil {
		log = slog.Default()
	}
	notifSvc, waSvc, reviewQueue, rdb := d.Notifications, d.WhatsApp, d.Queue, d.Redis
	publisher := d.Realtime
	if publisher == nil {
		publisher = realtime.NoopPublisher{}
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("workerapp: encryption: %w", err)
	}
	outboxStore := outbox.NewStore(pool, queries)

	// TEC-506: recommended prices: the hourly tick applies due versions and
	// writes the price discipline snapshot / weekly digest.
	recommendedSvc := pricingusecase.NewRecommended(pool, queries, outboxStore, sysconfig.New(queries, sysconfig.NoCache{}), log)
	recommendedSvc.SetPriceListQueue(queue.PriceListEnqueuer{Client: reviewQueue})

	activityRec := activity.NewRecorder(queries, log)
	searchReg := searchregistry.New(queries) // TEC-524: same specs as the API server
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
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
	// TEC-473: the fleet import applies in the worker (plate check, outbox).
	workerFleet := fleetusecase.New(pool)
	workerFleet.SetOutbox(outbox.NewStore(pool, queries))
	workerFleet.SetPlates(geo.New(pool, queries))
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
		// TEC-485: center network demand forecast export.
		stockforecastusecase.NewNetworkExportAdapter(stockforecastusecase.New(pool, queries, outboxStore, nil, sysconfig.New(queries, sysconfig.NoCache{}), log)),
		// TEC-377: service and warranty list exports (read only).
		servicesusecase.NewListExportAdapter(servicesusecase.New(pool, queries, nil)),
		warrantyusecase.NewListExportAdapter(warrantyusecase.NewReader(pool, queries, nil, cfg.Auth.FrontendURL)),
		// TEC-389: AI usage report export (read only).
		aiusecase.NewUsageExportAdapter(aiusecase.NewAdmin(airepo.New(pool), llm.ModelsFromConfig(cfg.AI), nil)),
		// TEC-473: fleet statement export and staged fleet vehicle import.
		fleetusecase.NewStatementAdapter(workerFleet),
		fleetusecase.NewListExportAdapter(workerFleet),
		fleetusecase.NewImporter(workerFleet),
		// TEC-506: recommended price import (staged) and discipline export.
		pricingusecase.NewRecommendedImporter(recommendedSvc),
		pricingusecase.NewDisciplineExportAdapter(recommendedSvc, pricingusecase.ParseDisciplineFilter),
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
		return nil, fmt.Errorf("workerapp: documents loader %s: %w", docmodel.KindQuote, err)
	}
	contractsSvc := contractsusecase.New(contractsrepo.New(pool, queries),
		contractsusecase.WithStorage(store),
		contractsusecase.WithPDFRenderer(pdfClient),
		contractsusecase.WithOutbox(outboxStore),
		// TEC-499: intake photo grid of the executed contract PDF.
		contractsusecase.WithIntakePhotos(photostandardusecase.New(pool, queries).WithStorage(store)),
	)
	_ = docSvc.RegisterLoader(docmodel.KindContract, contractsSvc.ContractDocumentLoader())
	// TEC-298: measurement PDF (worker-docs renders, pdf_key caches it).
	measurementPDF := measurementsusecase.NewPDF(pool, queries, store, pdfClient, reviewQueue,
		pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)
	_ = docSvc.RegisterLoader(docmodel.KindMeasurement, measurementPDF.DocumentLoader())
	// TEC-503: e-invoice PDF (worker-docs renders XSLT HTML via Gotenberg).
	einvoicePDF := einvoiceusecase.New(pool, queries, store, outboxStore, log).WithPDF(pdfClient, nil).
		WithSettings(sysconfig.New(queries, sysconfig.NoCache{}))
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
		// TEC-504: e-invoice drafts of the billable center sales.
		einvoiceusecase.NewBulkAdapter(einvoicePDF, queries),
		// TEC-497: dealer bonus accruals (approve).
		performanceusecase.NewBonusBulkAdapter(performanceusecase.New(pool, queries, outboxStore,
			features.New(pool, queries, nil, log), log).WithPanelURL(cfg.Auth.FrontendURL)),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log).
		WithPool(pool).WithUndoWindow(sysconfig.New(queries, sysconfig.NoCache{}).BulkUndoWindowHours)
	logsSvc := logsusecase.New(queries)
	ratesSvc := fxrates.New(queries, fxrates.NewFetcher(cfg.Rates.TCMBURL, cfg.Rates.ECBURL), log)
	warrantyCron := warrantymodule.NewCron(pool, queries, cfg.Auth.FrontendURL)
	// TEC-190: only the transfer expiry of the customers service runs here.
	transferExpirer := customersusecase.New(pool, queries, nil, nil)
	transferExpirer.SetOutbox(outbox.NewStore(pool, queries))

	glorianPusher := glorian.NewPusher(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log)
	glorianOrders := glorian.NewOrderOutbounder(queries, secretBox, glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log)
	featureSvc := features.New(pool, queries, nil, log)
	// TEC-508: decided module requests (auto-approved after a bundle opens a
	// module) notify the requester; expired bundles close their modules.
	featureSvc.WithDecisionNotifier(featurehandler.NewDecisionNotifier(queries, notifSvc, log))
	// TEC-308: the daily job books periods, activates scheduled
	// subscriptions and notifies expiry (outbox), so it gets the full wiring.
	subscriptionExpiry := servicecatalogusecase.New(queries).
		WithLifecycle(pool, outboxStore, fxrates.New(queries, nil, log), featureSvc).
		WithAccounting(accountingposting.New(queries, outboxStore, fxrates.New(queries, nil, log)))
	certificatesCron := certificatesusecase.NewCron(pool, queries, outboxStore, featureSvc, sysconfig.New(queries, sysconfig.NoCache{}), log)
	stockForecastSvc := stockforecastusecase.New(pool, queries, outboxStore, featureSvc, sysconfig.New(queries, sysconfig.NoCache{}), log)
	performanceSvc := performanceusecase.New(pool, queries, outboxStore, featureSvc, log)
	efficiencyNetwork := efficiencymodule.NewNetworkRefresher(queries, sysconfig.New(queries, sysconfig.NoCache{}))
	// TEC-476: periodic fleet reports. The schedule runs on worker-core, the
	// PDF (fleet_report document template) and its e-mail on worker-docs.
	workerFleet.SetModules(featureSvc)
	// TEC-506: price_list document source and its library publication.
	priceListPublisher := pricingusecase.NewPriceListPublisher(queries, docSvc, libraryusecase.New(queries, store), featureSvc, log)
	if err := docSvc.RegisterLoader(docmodel.KindPriceList, priceListPublisher.DocumentLoader()); err != nil {
		return nil, fmt.Errorf("workerapp: documents loader %s: %w", docmodel.KindPriceList, err)
	}
	workerFleet.SetReportFiles(store)
	if err := docSvc.RegisterLoader(docmodel.KindFleetReport, workerFleet.ReportDocumentLoader()); err != nil {
		return nil, fmt.Errorf("workerapp: documents loader %s: %w", docmodel.KindFleetReport, err)
	}
	fleetMailBrand := notifmodule.EmailBrandFunc(queries, cfg)
	workerFleet.SetReports(fleetusecase.ReportConfig{
		Renderer: docSvc, Storage: store, Queue: queue.FleetReportEnqueuer{Client: reviewQueue},
		Mail: mail.NewSMTPSender(cfg.SMTP),
		Brand: func(ctx context.Context, brandID int64) fleetusecase.MailBrand {
			b := fleetMailBrand(ctx, brandID)
			return fleetusecase.MailBrand{Name: b.Name, LogoURL: b.LogoURL, Color: b.Color}
		},
		PortalURL: strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/portal/fleet/reports",
		Log:       log,
	})

	w.WithDeliver(notifSvc.Deliver).
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
		// TEC-503: e-invoice PDF (docs queue).
		WithEinvoicePDF(einvoicePDF.GeneratePDF).
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
		// TEC-314: daily quote expiry (valid_until passed).
		WithQuoteExpire(leadsSvc.ExpireDueQuotesTask).
		WithQuoteReminder(leadsSvc.QuoteReminderTask).
		// TEC-400: hourly MCP OAuth cleanup (expired rows, abandoned clients).
		WithOAuthCleanup(oauthmodule.New(pool, featureSvc, nil, cfg.Auth.FrontendURL, log).Cleanup).
		// TEC-221: hourly center task due date reminders.
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
		WithServiceSubscriptionsExpire(func(ctx context.Context) error {
			loc, err := time.LoadLocation(queue.SchedulerTimezone)
			if err != nil {
				loc = time.UTC
			}
			n, err := subscriptionExpiry.ExpireDue(ctx, time.Now().In(loc))
			if n > 0 {
				log.Info("service_subscriptions_expired", "count", n)
			}
			return err
		}).
		WithServiceSubscriptionsPostPeriods(func(ctx context.Context) error {
			res, err := subscriptionExpiry.RunDaily(ctx)
			if res != (servicecatalogusecase.DailyResult{}) {
				log.Info("service_subscriptions_daily", "activated", res.Activated, "posted", res.Posted,
					"expired", res.Expired, "reminded", res.Reminded)
			}
			return err
		}).
		WithStockForecastDaily(stockForecastSvc.DailyTask).
		WithPerformanceDaily(performanceSvc.DailyTask).
		WithEfficiencyNetworkRefresh(efficiencyNetwork.Task).
		WithFleetReports(workerFleet.ScheduleReportsTask, workerFleet.GenerateReport).
		WithPricing(recommendedSvc.DailyTask, priceListPublisher.Task).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		)

	// TEC-469 (F5-01d): daily showcase Google rating refresh from Places
	// (no-op without GOOGLE_PLACES_API_KEY); backoff kept in Redis.
	w.WithShowcaseGoogleRating(showcaseusecase.NewRatingRefresher(queries, places.New(cfg.Places.APIKey),
		featureSvc, showcaseusecase.NewRedisFailures(rdb, cfg.App.Env), log).Task)

	// TEC-395: WhatsApp outgoing queue, inbound media storage, receipts.
	waMsgs := whatsappmodule.NewMessaging(waSvc, pool, queries, whatsappmodule.MessagingDeps{
		Storage: store, Queue: queue.WhatsAppEnqueuer{Client: reviewQueue},
		Limiter:       ratelimit.New(rdb, cfg.App.Env),
		SendPerMinute: sysconfig.New(queries, sysconfig.NoCache{}).WhatsAppSendPerMinute,
		Publisher:     publisher,
	}, log)
	waMsgs.SetDocuments(whatsappmodule.NewDocumentRenderer(
		servicesusecase.NewPDFAdapter(servicePDF), warrantyusecase.NewCertificateAdapter(warrantyCert), pdfClient))
	w.WithWhatsAppMessaging(waMsgs.ProcessSend, waMsgs.StoreInboundMedia, func(ctx context.Context) (int, error) {
		return waMsgs.RequeueStale(ctx, 2*time.Minute)
	})
	// TEC-392 (F4-01j): AI first triage of warranty claims (default queue).
	var triagePhotos llm.ObjectReader
	if store != nil {
		triagePhotos = store
	}
	w.WithWarrantyClaimTriage(warrantyclaimsusecase.NewTriage(warrantyclaimsusecase.TriageDeps{
		Conn: pool, Provider: anthropic.NewFromConfig(cfg.AI), Models: llm.ModelsFromConfig(cfg.AI),
		Features: featureSvc, Storage: triagePhotos, Log: log,
	}).Auto)
	// TEC-396 (F4-02c): WhatsApp AI pipeline (whatsapp queue).
	w.WithWhatsAppAIReply(newWhatsAppAIPipeline(whatsAppAIDeps{
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
	w.WithCampaigns(campaignSender.Tick, campaignSender.ProcessRecipient)

	return &Handlers{SearchIndexer: searchIndexer}, nil
}
