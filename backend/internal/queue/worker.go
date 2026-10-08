package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/hibiken/asynq"
)

// DeliverNotificationFunc delivers a queued notification by id.
type DeliverNotificationFunc func(ctx context.Context, notificationID int64) error

// AnnouncementDispatchFunc dispatches one announcement notification batch.
type AnnouncementDispatchFunc func(ctx context.Context, payload AnnouncementDispatchPayload) error

// ProcessExportFunc processes an export job by id.
type ProcessExportFunc func(ctx context.Context, exportJobID int64) error

// ProcessImportFunc processes an import job by id.
type ProcessImportFunc func(ctx context.Context, importJobID int64) error

// ProcessBulkFunc processes a bulk job by id.
type ProcessBulkFunc func(ctx context.Context, bulkJobID int64) error

// ProcessSearchUpsertFunc upserts one search document.
type ProcessSearchUpsertFunc func(ctx context.Context, spec, id string) error

// ProcessSearchDeleteFunc deletes one search document.
type ProcessSearchDeleteFunc func(ctx context.Context, spec, id string) error

// ProcessSearchReindexFunc rebuilds one or all search indexes.
type ProcessSearchReindexFunc func(ctx context.Context, spec string) error

// ProcessDocsRenderFunc renders one document_renders row to PDF.
type ProcessDocsRenderFunc func(ctx context.Context, renderID int64) error

// PurgeLogsFunc applies due log retention rules.
type PurgeLogsFunc func(ctx context.Context) error

// FetchRatesFunc fetches and stores the daily exchange rates.
type FetchRatesFunc func(ctx context.Context) error

// Worker processes Asynq tasks.
type Worker struct {
	server               *asynq.Server
	mux                  *asynq.ServeMux
	log                  *slog.Logger
	deliver              DeliverNotificationFunc
	announcementDispatch AnnouncementDispatchFunc
	processExport        ProcessExportFunc
	processImport        ProcessImportFunc
	processBulk          ProcessBulkFunc
	processSearchUpsert  ProcessSearchUpsertFunc
	processSearchDelete  ProcessSearchDeleteFunc
	processSearchReindex ProcessSearchReindexFunc
	purgeLogs            PurgeLogsFunc
	processDocsRender    ProcessDocsRenderFunc
	fetchRates           FetchRatesFunc
	purgeNotifications   NotificationPurgeFunc
	pollWhatsApp         WhatsAppPollFunc
	warrantyExpire       WarrantyTaskFunc
	warrantyExpiringScan WarrantyTaskFunc
	warrantyRepairScan   WarrantyTaskFunc
	inventoryRebuild     InventoryRebuildFunc
	// TEC-190: pending vehicle transfers past expires_at.
	vehicleTransferExpire VehicleTransferTaskFunc
	// TEC-192: delayed Google review request of a completed service.
	serviceReviewRequest ServiceReviewRequestFunc
	// TEC-325: delayed appointment WhatsApp reminders and no-show scan.
	appointmentReminder   AppointmentReminderFunc
	appointmentNoShowScan AppointmentNoShowScanFunc
	// TEC-221: center task due date reminders.
	tasksDueScan TasksDueScanFunc
	// TEC-314: daily quote expiry.
	quoteExpire QuoteExpireFunc
	// TEC-315: delayed quote WhatsApp reminder.
	quoteReminder QuoteReminderFunc
	// TEC-400: hourly MCP OAuth cleanup.
	oauthCleanup OAuthCleanupFunc
	// TEC-207: end-of-day warehouse reports.
	warehouseEOD WarehouseEODFunc
	// TEC-268: Glorian catalog and dealer pull.
	glorianPull GlorianPullFunc
	// TEC-270: Glorian barcode push (bulk upsert) and outbound PATCH.
	glorianPush  GlorianPushBarcodesFunc
	glorianPatch GlorianPatchStockItemFunc
	// TEC-271: Glorian order outbound and held replay.
	glorianOrder  GlorianOrderOutboundFunc
	glorianReplay GlorianOrderReplayFunc
	// TEC-273: admin-triggered reconcile run and single outbound replay.
	glorianReconcile GlorianReconcileFunc
	glorianReplayOne GlorianOutboundReplayOneFunc
	// TEC-288: executed contract PDF (docs queue).
	contractPDF ContractPDFFunc
	// TEC-298: measurement PDF (docs queue).
	measurementPDF MeasurementPDFFunc
	// TEC-381: planned staff payments booked on their paid_on.
	staffPaymentsPostDue StaffPaymentsPostDueFunc
	// TEC-393: 90-day retention of conversation AI runs.
	purgeConversationAIRuns ConversationAIRunPurgeFunc
	// TEC-395: WhatsApp outgoing send, inbound media storage, queue sweep.
	whatsAppSend  WhatsAppMessageFunc
	whatsAppMedia WhatsAppMessageFunc
	whatsAppSweep WhatsAppQueueSweepFunc
	// TEC-396: WhatsApp AI pipeline (debounced per conversation).
	whatsAppAIReply WhatsAppAIReplyFunc
	// TEC-392: AI first triage of warranty claims.
	warrantyClaimTriage WarrantyClaimTriageFunc
	// TEC-387: AI confirmation card expiry and stale run cleanup.
	aiActionSweep AIActionSweepFunc
	// TEC-407: campaign scheduler tick and recipient sends.
	campaignTick CampaignTickFunc
	campaignSend CampaignRecipientFunc
	// TEC-481: certificate expiry notices and expiry policy refresh.
	certificateExpiryScan CertificateExpiryScanFunc
	// TEC-484: stock forecast daily snapshots and low-stock transitions.
	stockForecastDaily StockForecastDailyFunc
	// TEC-488: weekly efficiency network medians.
	efficiencyNetworkRefresh EfficiencyNetworkRefreshFunc
	// TEC-476: periodic fleet report schedule and generation.
	fleetReportsSchedule FleetReportsScheduleFunc
	fleetReportGenerate  FleetReportGenerateFunc
	// TEC-469: daily dealer showcase Google rating refresh (Places).
	showcaseGoogleRating ShowcaseGoogleRatingFunc
}

// NewWorker builds a worker that handles known task types on every queue.
func NewWorker(cfg config.Config, log *slog.Logger, deliver DeliverNotificationFunc) *Worker {
	return NewWorkerWithQueues(cfg, log, deliver, DefaultQueues())
}

// DefaultQueues lists every queue with its priority weight (single worker
// process: local dev, in-process worker).
func DefaultQueues() map[string]int {
	return map[string]int{
		"default":          1,
		QueueNotifications: 2,
		QueueExports:       2,
		QueueImports:       2,
		QueueBulk:          2,
		QueueSearch:        2,
		QueueMaintenance:   1,
		QueueLow:           1,
		QueueDocs:          2,
		QueueWhatsApp:      2,
		QueueCampaigns:     1,
	}
}

// Every task type is registered on the mux exactly once, here. The With*
// setters only store the processor, so calling one twice (e.g. main and
// httpserver.New both wiring the in-process worker) replaces it instead of
// panicking with "asynq: multiple registrations" (TEC-142).

// NewWorkerWithQueues builds a worker that consumes only the given queues
// (queue name → priority weight). Production splits queues across
// worker-core and worker-docs (WORKER_QUEUES).
func NewWorkerWithQueues(cfg config.Config, log *slog.Logger, deliver DeliverNotificationFunc, queues map[string]int) *Worker {
	if len(queues) == 0 {
		queues = DefaultQueues()
	}
	if log == nil {
		log = slog.Default()
	}
	concurrency := cfg.Queue.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	server := asynq.NewServer(RedisOpt(cfg.Redis), asynq.Config{
		Concurrency:    concurrency,
		Queues:         queues,
		ErrorHandler:   TaskErrorHandler(log),
		IsFailure:      IsTaskFailure,
		RetryDelayFunc: TaskRetryDelay,
	})
	mux := asynq.NewServeMux()
	w := &Worker{server: server, mux: mux, log: log, deliver: deliver}
	mux.HandleFunc(TaskPing, handlePing(log))
	mux.HandleFunc(TaskNotificationDeliver, w.handleNotificationDeliver)
	mux.HandleFunc(TaskAnnouncementDispatch, w.handleAnnouncementDispatch)
	mux.HandleFunc(TaskExportProcess, w.handleExportProcess)
	mux.HandleFunc(TaskImportProcess, w.handleImportProcess)
	mux.HandleFunc(TaskBulkProcess, w.handleBulkProcess)
	mux.HandleFunc(TaskLogPurgeSweep, w.handleLogPurgeSweep)
	mux.HandleFunc(TaskSearchUpsert, w.handleSearchUpsert)
	mux.HandleFunc(TaskSearchDelete, w.handleSearchDelete)
	mux.HandleFunc(TaskSearchReindex, w.handleSearchReindex)
	mux.HandleFunc(TaskDocsRender, w.handleDocsRender)
	mux.HandleFunc(TaskContractPDF, w.handleContractPDF)
	mux.HandleFunc(TaskRatesFetch, w.handleRatesFetch)
	mux.HandleFunc(TaskNotificationPurge, w.handleNotificationPurge)
	mux.HandleFunc(TaskWhatsAppStatusPoll, w.handleWhatsAppPoll)
	mux.HandleFunc(TaskWarrantyExpire, w.handleWarrantyExpire)
	mux.HandleFunc(TaskWarrantyExpiringScan, w.handleWarrantyExpiringScan)
	mux.HandleFunc(TaskWarrantyRepairScan, w.handleWarrantyRepairScan)
	mux.HandleFunc(TaskInventoryRebuild, w.handleInventoryRebuild)
	mux.HandleFunc(TaskVehicleTransferExpire, w.handleVehicleTransferExpire)
	mux.HandleFunc(TaskServiceReviewRequest, w.handleServiceReviewRequest)
	mux.HandleFunc(TaskAppointmentReminder, w.handleAppointmentReminder)
	mux.HandleFunc(TaskAppointmentNoShowScan, w.handleAppointmentNoShowScan)
	mux.HandleFunc(TaskTasksDueScan, w.handleTasksDueScan)
	mux.HandleFunc(TaskQuoteExpire, w.handleQuoteExpire)
	mux.HandleFunc(TaskQuoteReminder, w.handleQuoteReminder)
	mux.HandleFunc(TaskOAuthCleanup, w.handleOAuthCleanup)
	mux.HandleFunc(TaskAIActionSweep, w.handleAIActionSweep)
	mux.HandleFunc(TaskWarehouseEODReports, w.handleWarehouseEOD)
	mux.HandleFunc(TaskGlorianPullCatalog, w.handleGlorianPull)
	mux.HandleFunc(TaskGlorianPushBarcodes, w.handleGlorianPush)
	mux.HandleFunc(TaskGlorianPatchStockItem, w.handleGlorianPatch)
	mux.HandleFunc(TaskGlorianOrderOutbound, w.handleGlorianOrderOutbound)
	mux.HandleFunc(TaskGlorianOrderReplay, w.handleGlorianOrderReplay)
	mux.HandleFunc(TaskGlorianReconcile, w.handleGlorianReconcile)
	mux.HandleFunc(TaskGlorianOutboundReplayOne, w.handleGlorianOutboundReplayOne)
	mux.HandleFunc(TaskMeasurementPDF, w.handleMeasurementPDF)
	mux.HandleFunc(TaskStaffPaymentsPostDue, w.handleStaffPaymentsPostDue)
	mux.HandleFunc(TaskConversationAIRunPurge, w.handleConversationAIRunPurge)
	mux.HandleFunc(TaskWhatsAppSend, w.handleWhatsAppSend)
	mux.HandleFunc(TaskWhatsAppMediaStore, w.handleWhatsAppMediaStore)
	mux.HandleFunc(TaskWhatsAppQueueSweep, w.handleWhatsAppQueueSweep)
	mux.HandleFunc(TaskWhatsAppAIReply, w.handleWhatsAppAIReply)
	mux.HandleFunc(TaskWarrantyClaimTriage, w.handleWarrantyClaimTriage)
	mux.HandleFunc(TaskCampaignTick, w.handleCampaignTick)
	mux.HandleFunc(TaskCampaignSendRecipient, w.handleCampaignSendRecipient)
	mux.HandleFunc(TaskCertificateExpiryScan, w.handleCertificateExpiryScan)
	mux.HandleFunc(TaskStockForecastDaily, w.handleStockForecastDaily)
	mux.HandleFunc(TaskEfficiencyNetworkRefresh, w.handleEfficiencyNetworkRefresh)
	mux.HandleFunc(TaskFleetReportsSchedule, w.handleFleetReportsSchedule)
	mux.HandleFunc(TaskFleetReportGenerate, w.handleFleetReportGenerate)
	mux.HandleFunc(TaskShowcaseGoogleRating, w.handleShowcaseGoogleRating)
	return w
}

// TaskErrorHandler logs a failed task and reports it to error tracking with
// task type, queue, retry and module tags. Handler panics reach it too:
// asynq recovers them into errors.
func TaskErrorHandler(log *slog.Logger) asynq.ErrorHandler {
	if log == nil {
		log = slog.Default()
	}
	return asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
		if !IsTaskFailure(err) {
			log.Debug("queue_task_deferred", "type", task.Type(), "error", err)
			return
		}
		log.Error("queue_task_failed", "type", task.Type(), "error", err)
		info := errtrack.TaskInfo{Type: task.Type()}
		info.Queue, _ = asynq.GetQueueName(ctx)
		info.Retry, _ = asynq.GetRetryCount(ctx)
		info.MaxRetry, _ = asynq.GetMaxRetry(ctx)
		errtrack.CaptureTask(ctx, info, err)
	})
}

// WithExport registers export job processor.
func (w *Worker) WithExport(fn ProcessExportFunc) *Worker {
	w.processExport = fn
	return w
}

// WithImport registers import job processor.
func (w *Worker) WithImport(fn ProcessImportFunc) *Worker {
	w.processImport = fn
	return w
}

// WithBulk registers bulk job processor.
func (w *Worker) WithBulk(fn ProcessBulkFunc) *Worker {
	w.processBulk = fn
	return w
}

// WithSearch registers search index processors.
func (w *Worker) WithSearch(upsert ProcessSearchUpsertFunc, del ProcessSearchDeleteFunc, reindex ProcessSearchReindexFunc) *Worker {
	w.processSearchUpsert = upsert
	w.processSearchDelete = del
	w.processSearchReindex = reindex
	return w
}

// WithLogPurge registers log retention rule processor.
func (w *Worker) WithLogPurge(fn PurgeLogsFunc) *Worker {
	w.purgeLogs = fn
	return w
}

// WithDocsRender registers the PDF document render processor.
func (w *Worker) WithDocsRender(fn ProcessDocsRenderFunc) *Worker {
	w.processDocsRender = fn
	return w
}

// WithRatesFetch registers the exchange rate fetcher (TEC-84).
func (w *Worker) WithRatesFetch(fn FetchRatesFunc) *Worker {
	w.fetchRates = fn
	return w
}

// WithAnnouncementDispatch registers announcement notification batch dispatch.
func (w *Worker) WithAnnouncementDispatch(fn AnnouncementDispatchFunc) *Worker {
	w.announcementDispatch = fn
	return w
}

// Start blocks until the worker stops.
func (w *Worker) Start() error {
	if w == nil || w.server == nil {
		return fmt.Errorf("queue: worker is nil")
	}
	w.log.Info("queue_worker_start")
	if err := w.server.Run(w.mux); err != nil {
		return fmt.Errorf("queue: worker run: %w", err)
	}
	return nil
}

// Shutdown stops the worker gracefully.
func (w *Worker) Shutdown() {
	if w != nil && w.server != nil {
		w.server.Shutdown()
	}
}

func (w *Worker) handleNotificationDeliver(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseNotificationDeliverPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.deliver == nil {
		w.log.Warn("notification_deliver_handler_missing", "id", payload.NotificationID)
		return nil
	}
	return w.deliver(ctx, payload.NotificationID)
}

func (w *Worker) handleAnnouncementDispatch(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseAnnouncementDispatchPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.announcementDispatch == nil {
		w.log.Warn("announcement_dispatch_handler_missing", "announcement_id", payload.AnnouncementID)
		return nil
	}
	return w.announcementDispatch(ctx, payload)
}

func (w *Worker) handleExportProcess(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseExportProcessPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processExport == nil {
		w.log.Warn("export_process_handler_missing", "id", payload.ExportJobID)
		return nil
	}
	return w.processExport(ctx, payload.ExportJobID)
}

func (w *Worker) handleImportProcess(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseImportProcessPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processImport == nil {
		w.log.Warn("import_process_handler_missing", "id", payload.ImportJobID)
		return nil
	}
	return w.processImport(ctx, payload.ImportJobID)
}

func (w *Worker) handleBulkProcess(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseBulkProcessPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processBulk == nil {
		w.log.Warn("bulk_process_handler_missing", "id", payload.BulkJobID)
		return nil
	}
	return w.processBulk(ctx, payload.BulkJobID)
}

func (w *Worker) handleLogPurgeSweep(ctx context.Context, _ *asynq.Task) error {
	if w.purgeLogs == nil {
		w.log.Warn("log_purge_handler_missing")
		return nil
	}
	return w.purgeLogs(ctx)
}

func (w *Worker) handleRatesFetch(ctx context.Context, _ *asynq.Task) error {
	if w.fetchRates == nil {
		w.log.Warn("rates_fetch_handler_missing")
		return nil
	}
	return w.fetchRates(ctx)
}

func (w *Worker) handleSearchUpsert(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseSearchUpsertPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processSearchUpsert == nil {
		w.log.Warn("search_upsert_handler_missing", "spec", payload.Spec, "id", payload.ID)
		return nil
	}
	return w.processSearchUpsert(ctx, payload.Spec, payload.ID)
}

func (w *Worker) handleSearchDelete(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseSearchDeletePayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processSearchDelete == nil {
		w.log.Warn("search_delete_handler_missing", "spec", payload.Spec, "id", payload.ID)
		return nil
	}
	return w.processSearchDelete(ctx, payload.Spec, payload.ID)
}

func (w *Worker) handleSearchReindex(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseSearchReindexPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processSearchReindex == nil {
		w.log.Warn("search_reindex_handler_missing", "spec", payload.Spec)
		return nil
	}
	return w.processSearchReindex(ctx, payload.Spec)
}

func handlePing(log *slog.Logger) asynq.HandlerFunc {
	return func(_ context.Context, task *asynq.Task) error {
		payload, err := ParsePingPayload(task.Payload())
		if err != nil {
			return err
		}
		log.Info(
			"queue_ping_received",
			"message", payload.Message,
			"enqueued_at", payload.EnqueuedAt,
		)
		return nil
	}
}

func (w *Worker) handleDocsRender(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseDocsRenderPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.processDocsRender == nil {
		w.log.Warn("docs_render_handler_missing", "id", payload.RenderID)
		return nil
	}
	return w.processDocsRender(ctx, payload.RenderID)
}
