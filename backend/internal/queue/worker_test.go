package queue

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/hibiken/asynq"
)

// wireAll calls every With* setter once, as cmd/worker and httpserver.New do.
func wireAll(w *Worker, hits map[string]int, tag string) *Worker {
	hit := func(k string) { hits[k+":"+tag]++ }
	return w.
		WithExport(func(context.Context, int64) error { hit(TaskExportProcess); return nil }).
		WithImport(func(context.Context, int64) error { hit(TaskImportProcess); return nil }).
		WithBulk(func(context.Context, int64) error { hit(TaskBulkProcess); return nil }).
		WithLogPurge(func(context.Context) error { hit(TaskLogPurgeSweep); return nil }).
		WithRatesFetch(func(context.Context) error { hit(TaskRatesFetch); return nil }).
		WithNotificationPurge(func(context.Context) (int64, error) { hit(TaskNotificationPurge); return 0, nil }).
		WithConversationAIRunPurge(func(context.Context) (int64, error) { hit(TaskConversationAIRunPurge); return 0, nil }).
		WithWhatsAppPoll(func(context.Context) error { hit(TaskWhatsAppStatusPoll); return nil }).
		WithDocsRender(func(context.Context, int64) error { hit(TaskDocsRender); return nil }).
		WithContractPDF(func(context.Context, int64) error { hit(TaskContractPDF); return nil }).
		WithMeasurementPDF(func(context.Context, int64) error { hit(TaskMeasurementPDF); return nil }).
		WithWarrantyCron(
			func(context.Context) error { hit(TaskWarrantyExpire); return nil },
			func(context.Context) error { hit(TaskWarrantyExpiringScan); return nil },
		).
		WithWarrantyRepairScan(func(context.Context) error { hit(TaskWarrantyRepairScan); return nil }).
		WithVehicleTransferExpire(func(context.Context) error { hit(TaskVehicleTransferExpire); return nil }).
		WithAppointmentReminder(func(context.Context, int64, time.Time, string) error { hit(TaskAppointmentReminder); return nil }).
		WithAppointmentNoShowScan(func(context.Context) error { hit(TaskAppointmentNoShowScan); return nil }).
		WithWarehouseEOD(func(context.Context) error { hit(TaskWarehouseEODReports); return nil }).
		WithStaffPaymentsPostDue(func(context.Context) error { hit(TaskStaffPaymentsPostDue); return nil }).
		WithGlorianPull(func(context.Context) error { hit(TaskGlorianPullCatalog); return nil }).
		WithGlorianPush(
			func(context.Context, int64) error { hit(TaskGlorianPushBarcodes); return nil },
			func(context.Context, int64) error { hit(TaskGlorianPatchStockItem); return nil },
		).
		WithGlorianOrderOutbound(
			func(context.Context, int64) error { hit(TaskGlorianOrderOutbound); return nil },
			func(context.Context, int64) error { hit(TaskGlorianOrderReplay); return nil },
		).
		WithGlorianAdmin(
			func(context.Context, int64) error { hit(TaskGlorianReconcile); return nil },
			func(context.Context, int64) error { hit(TaskGlorianOutboundReplayOne); return nil },
		).
		WithSearch(
			func(context.Context, string, string) error { hit(TaskSearchUpsert); return nil },
			func(context.Context, string, string) error { hit(TaskSearchDelete); return nil },
			func(context.Context, string) error { hit(TaskSearchReindex); return nil },
		).
		WithCertificateExpiryScan(func(context.Context) error { hit(TaskCertificateExpiryScan); return nil }).
		WithStockForecastDaily(func(context.Context, int64, time.Time) error { hit(TaskStockForecastDaily); return nil })
}

// TEC-142: wiring the same processor twice (main + httpserver.New with
// QUEUE_WORKER_INPROCESS=true) must not panic with "asynq: multiple
// registrations"; the last setter wins.
func TestWorkerSettersAreIdempotent(t *testing.T) {
	cfg := config.Config{Redis: config.RedisConfig{Addr: "127.0.0.1:0"}}
	w := NewWorker(cfg, nil, nil)
	hits := map[string]int{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("second wiring panicked: %v", r)
			}
		}()
		wireAll(w, hits, "first")
		wireAll(w, hits, "second")
	}()

	for _, typ := range []string{TaskNotificationPurge, TaskWhatsAppStatusPoll, TaskLogPurgeSweep, TaskRatesFetch, TaskWarrantyExpire, TaskWarrantyExpiringScan, TaskWarrantyRepairScan, TaskVehicleTransferExpire, TaskAppointmentNoShowScan, TaskWarehouseEODReports, TaskGlorianPullCatalog, TaskGlorianPushBarcodes, TaskGlorianPatchStockItem, TaskGlorianOrderOutbound, TaskGlorianOrderReplay, TaskGlorianReconcile, TaskGlorianOutboundReplayOne, TaskContractPDF, TaskMeasurementPDF, TaskStaffPaymentsPostDue, TaskConversationAIRunPurge, TaskCertificateExpiryScan, TaskStockForecastDaily} {
		if err := w.mux.ProcessTask(context.Background(), asynq.NewTask(typ, []byte("{}"))); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if hits[typ+":second"] != 1 || hits[typ+":first"] != 0 {
			t.Fatalf("%s: want only the last processor called, hits=%v", typ, hits)
		}
	}
}

// Unwired periodic tasks are acknowledged, not failed.
func TestWorkerMissingPeriodicHandlersAreNoops(t *testing.T) {
	w := NewWorker(config.Config{Redis: config.RedisConfig{Addr: "127.0.0.1:0"}}, nil, nil)
	for _, typ := range []string{TaskNotificationPurge, TaskWhatsAppStatusPoll, TaskWarrantyExpire, TaskWarrantyExpiringScan, TaskAppointmentNoShowScan, TaskWarehouseEODReports, TaskGlorianPullCatalog, TaskGlorianPushBarcodes, TaskGlorianPatchStockItem, TaskGlorianOrderOutbound, TaskGlorianOrderReplay, TaskGlorianReconcile, TaskGlorianOutboundReplayOne, TaskStaffPaymentsPostDue, TaskConversationAIRunPurge, TaskOAuthCleanup, TaskAIActionSweep, TaskCertificateExpiryScan, TaskStockForecastDaily} {
		if err := w.mux.ProcessTask(context.Background(), asynq.NewTask(typ, []byte("{}"))); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}
}
