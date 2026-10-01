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

// PurgeLogsFunc applies due log retention rules.
type PurgeLogsFunc func(ctx context.Context) error

// Worker processes Asynq tasks.
type Worker struct {
	server               *asynq.Server
	mux                  *asynq.ServeMux
	log                  *slog.Logger
	deliver              DeliverNotificationFunc
	processExport        ProcessExportFunc
	processImport        ProcessImportFunc
	processBulk          ProcessBulkFunc
	processSearchUpsert  ProcessSearchUpsertFunc
	processSearchDelete  ProcessSearchDeleteFunc
	processSearchReindex ProcessSearchReindexFunc
	purgeLogs            PurgeLogsFunc
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
	}
}

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
		Concurrency:  concurrency,
		Queues:       queues,
		ErrorHandler: TaskErrorHandler(log),
	})
	mux := asynq.NewServeMux()
	w := &Worker{server: server, mux: mux, log: log, deliver: deliver}
	mux.HandleFunc(TaskPing, handlePing(log))
	mux.HandleFunc(TaskNotificationDeliver, w.handleNotificationDeliver)
	mux.HandleFunc(TaskExportProcess, w.handleExportProcess)
	mux.HandleFunc(TaskImportProcess, w.handleImportProcess)
	mux.HandleFunc(TaskBulkProcess, w.handleBulkProcess)
	mux.HandleFunc(TaskLogPurgeSweep, w.handleLogPurgeSweep)
	mux.HandleFunc(TaskSearchUpsert, w.handleSearchUpsert)
	mux.HandleFunc(TaskSearchDelete, w.handleSearchDelete)
	mux.HandleFunc(TaskSearchReindex, w.handleSearchReindex)
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
