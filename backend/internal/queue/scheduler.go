package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/hibiken/asynq"
)

// SchedulerTimezone is the zone cron expressions are read in.
const SchedulerTimezone = "Europe/Istanbul"

// Periodic is one scheduled task of worker-core.
type Periodic struct {
	Cron  string
	Type  string
	Queue string
	Opts  []asynq.Option
	New   func() (*asynq.Task, error)
}

const logPurgeCron = "@every 5m"
const appointmentNoShowScanCron = "@every 15m"

// Exchange rates (TEC-84): TCMB publishes ~15:30 TR, ECB ~16:00 CET (17:00-18:00
// TR), so the weekday run is after both; the morning run fills a missed day.
const (
	ratesFetchCron        = "30 18 * * 1-5"
	ratesFetchCatchUpCron = "15 10 * * *"
)

// Schedules lists every periodic task. Fetches are idempotent upserts, so a
// duplicate run (catch-up after a successful evening) is harmless.
func Schedules() []Periodic {
	rateOpts := []asynq.Option{asynq.MaxRetry(5), asynq.Timeout(2 * time.Minute)}
	return []Periodic{
		{Cron: logPurgeCron, Type: TaskLogPurgeSweep, Queue: QueueMaintenance, New: NewLogPurgeSweepTask},
		{Cron: ratesFetchCron, Type: TaskRatesFetch, Queue: QueueMaintenance, Opts: rateOpts, New: NewRatesFetchTask},
		{Cron: ratesFetchCatchUpCron, Type: TaskRatesFetch, Queue: QueueMaintenance, Opts: rateOpts, New: NewRatesFetchTask},
		// TEC-187: warranty expiry and 30 / 7 day reminders (idempotent).
		{Cron: warrantyExpireCron, Type: TaskWarrantyExpire, Queue: QueueMaintenance, Opts: warrantyTaskOpts(), New: NewWarrantyExpireTask},
		{Cron: warrantyExpiringScanCron, Type: TaskWarrantyExpiringScan, Queue: QueueMaintenance, Opts: warrantyTaskOpts(), New: NewWarrantyExpiringScanTask},
		// TEC-194: daily repair scan for warranties the listener missed.
		{Cron: warrantyRepairScanCron, Type: TaskWarrantyRepairScan, Queue: QueueMaintenance, Opts: warrantyTaskOpts(), New: NewWarrantyRepairScanTask},
		// TEC-190: pending vehicle transfers past expires_at (idempotent).
		{Cron: vehicleTransferExpireCron, Type: TaskVehicleTransferExpire, Queue: QueueMaintenance, Opts: vehicleTransferTaskOpts(), New: NewVehicleTransferExpireTask},
		// TEC-156: nightly stock projection drift scan (report only).
		{Cron: inventoryRebuildCron, Type: TaskInventoryRebuild, Queue: QueueMaintenance, New: newNightlyInventoryRebuildTask},
		// TEC-221: hourly center task due date reminders (idempotent).
		{Cron: tasksDueScanCron, Type: TaskTasksDueScan, Queue: QueueMaintenance, Opts: tasksDueScanOpts(), New: NewTasksDueScanTask},
		// TEC-325: mark appointments that are still open two hours after start.
		{Cron: appointmentNoShowScanCron, Type: TaskAppointmentNoShowScan, Queue: QueueMaintenance, Opts: appointmentNoShowScanOpts(), New: NewAppointmentNoShowScanTask},
		// TEC-314: daily quote expiry (idempotent).
		{Cron: quoteExpireCron, Type: TaskQuoteExpire, Queue: QueueMaintenance, Opts: quoteExpireOpts(), New: NewQuoteExpireTask},
		// TEC-207: hourly end-of-day warehouse reports (idempotent).
		{Cron: warehouseEODCron, Type: TaskWarehouseEODReports, Queue: QueueMaintenance, Opts: warehouseEODOpts(), New: NewWarehouseEODTask},
		// TEC-268: Glorian catalog and dealer pull every 15 minutes.
		{Cron: glorianPullCron, Type: TaskGlorianPullCatalog, Queue: QueueMaintenance, Opts: glorianPullOpts(), New: NewGlorianPullCatalogTask},
		// TEC-270: Glorian barcode push safety net (every active connection).
		{Cron: glorianPushCron, Type: TaskGlorianPushBarcodes, Queue: QueueMaintenance, Opts: glorianPushCronOpts(), New: NewGlorianPushAllTask},
		// TEC-271: replay of held Glorian order outbounds (every connection).
		{Cron: glorianOrderReplayCron, Type: TaskGlorianOrderReplay, Queue: QueueMaintenance, Opts: glorianOrderReplayCronOpts(), New: NewGlorianOrderReplayAllTask},
		// TEC-381: hourly booking of planned staff payments due (idempotent).
		{Cron: staffPaymentsPostDueCron, Type: TaskStaffPaymentsPostDue, Queue: QueueMaintenance, Opts: staffPaymentsPostDueOpts(), New: NewStaffPaymentsPostDueTask},
		// TEC-393: 90-day retention of WhatsApp conversation AI runs.
		{Cron: conversationAIRunPurgeCron, Type: TaskConversationAIRunPurge, Queue: QueueMaintenance, New: NewConversationAIRunPurgeTask},
		// TEC-400: hourly MCP OAuth cleanup (idempotent deletes).
		{Cron: oauthCleanupCron, Type: TaskOAuthCleanup, Queue: QueueMaintenance, Opts: oauthCleanupOpts(), New: NewOAuthCleanupTask},
		// TEC-395: re-enqueue WhatsApp messages whose send task was lost.
		{Cron: whatsAppQueueSweepCron, Type: TaskWhatsAppQueueSweep, Queue: QueueMaintenance, New: NewWhatsAppQueueSweepTask},
		// TEC-387: AI confirmation card expiry and stale run cleanup (idempotent).
		{Cron: aiActionSweepCron, Type: TaskAIActionSweep, Queue: QueueMaintenance, Opts: aiActionSweepOpts(), New: NewAIActionSweepTask},
		// TEC-407: campaign scheduler (due campaigns, finish, lost tasks).
		{Cron: campaignTickCron, Type: TaskCampaignTick, Queue: QueueMaintenance, Opts: campaignTickOpts(), New: NewCampaignTickTask},
	}
}

// Registrar is the part of *asynq.Scheduler that registers entries.
type Registrar interface {
	Register(cronspec string, task *asynq.Task, opts ...asynq.Option) (string, error)
}

// RegisterSchedules registers every entry of Schedules on r.
func RegisterSchedules(r Registrar, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	for _, p := range Schedules() {
		task, err := p.New()
		if err != nil {
			return fmt.Errorf("queue: %s task: %w", p.Type, err)
		}
		opts := append([]asynq.Option{asynq.Queue(p.Queue)}, p.Opts...)
		if _, err := r.Register(p.Cron, task, opts...); err != nil {
			return fmt.Errorf("queue: register %s schedule: %w", p.Type, err)
		}
		log.Info("queue_schedule_registered", "type", p.Type, "cron", p.Cron, "queue", p.Queue)
	}
	return nil
}

// StartScheduler builds the worker-core scheduler with every periodic task.
// The caller starts it under the leader lock (one scheduler per cluster).
func StartScheduler(cfg config.Config, log *slog.Logger) (*asynq.Scheduler, error) {
	if log == nil {
		log = slog.Default()
	}
	loc, err := time.LoadLocation(SchedulerTimezone)
	if err != nil {
		return nil, fmt.Errorf("queue: scheduler timezone: %w", err)
	}
	scheduler := asynq.NewScheduler(RedisOpt(cfg.Redis), &asynq.SchedulerOpts{
		Location: loc,
		// PostEnqueueFunc gets a nil TaskInfo on failure (no task type), so
		// the deprecated error handler stays the only hook with task + error.
		EnqueueErrorHandler: SchedulerErrorHandler(log), //nolint:staticcheck // see above
	})
	if err := RegisterSchedules(scheduler, log); err != nil {
		return nil, err
	}
	return scheduler, nil
}

// SchedulerErrorHandler logs and reports periodic task enqueue failures.
func SchedulerErrorHandler(log *slog.Logger) func(task *asynq.Task, opts []asynq.Option, err error) {
	if log == nil {
		log = slog.Default()
	}
	return func(task *asynq.Task, opts []asynq.Option, err error) {
		taskType := ""
		if task != nil {
			taskType = task.Type()
		}
		queueName := "default"
		for _, o := range opts {
			if o.Type() == asynq.QueueOpt {
				if q, ok := o.Value().(string); ok {
					queueName = q
				}
			}
		}
		log.Error("queue_schedule_enqueue_failed", "type", taskType, "error", err)
		errtrack.CaptureTask(context.Background(), errtrack.TaskInfo{
			Type: taskType, Queue: queueName, Scheduler: true,
		}, err)
	}
}
