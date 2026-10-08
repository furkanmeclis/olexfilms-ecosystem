package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-476 (F5-02e): periodic fleet reports. The hourly schedule runs on
// worker-core (maintenance queue): it creates the due report of each fleet
// (idempotent, fleet_reports UNIQUE per period) and enqueues its generation
// on the docs queue (worker-docs: Gotenberg PDF, storage, e-mail).
const (
	TaskFleetReportsSchedule = "fleet:reports_schedule"
	TaskFleetReportGenerate  = "fleet:report_generate"
)

// fleetReportsScheduleCron: hourly; the use case checks 07:00 on the first
// day of the month / quarter in each fleet's timezone.
const fleetReportsScheduleCron = "7 * * * *"

// FleetReportRetries is the retry count of a report generation: after the
// last one the report is marked failed.
const FleetReportRetries = 3

// FleetReportsScheduleFunc runs one schedule tick.
type FleetReportsScheduleFunc func(ctx context.Context) error

// FleetReportGenerateFunc generates one report; final is set on the last
// attempt (the report is marked failed if it errors).
type FleetReportGenerateFunc func(ctx context.Context, reportID int64, final bool) error

// FleetReportPayload names the report.
type FleetReportPayload struct {
	ReportID int64 `json:"report_id"`
}

// NewFleetReportsScheduleTask builds the schedule tick.
func NewFleetReportsScheduleTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskFleetReportsSchedule, []byte("{}")), nil
}

func fleetReportsScheduleOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

// NewFleetReportTask builds the generation task of a report.
func NewFleetReportTask(reportID int64) (*asynq.Task, error) {
	body, err := json.Marshal(FleetReportPayload{ReportID: reportID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal %s: %w", TaskFleetReportGenerate, err)
	}
	return asynq.NewTask(TaskFleetReportGenerate, body), nil
}

// FleetReportOpts: docs queue, task id per report (one queued generation),
// three retries.
func FleetReportOpts(reportID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.TaskID(fmt.Sprintf("fleet-report-%d", reportID)),
		asynq.MaxRetry(FleetReportRetries),
		asynq.Timeout(5 * time.Minute),
	}
}

// FleetReportEnqueuer implements the fleet use case's ReportQueue; a task
// already queued for the report is not an error.
type FleetReportEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueueFleetReport enqueues fleet:report_generate for a report.
func (e FleetReportEnqueuer) EnqueueFleetReport(_ context.Context, reportID int64) error {
	if e.Client == nil {
		return errors.New("queue: client is nil")
	}
	task, err := NewFleetReportTask(reportID)
	if err != nil {
		return err
	}
	if _, err := e.Client.Enqueue(task, FleetReportOpts(reportID)...); err != nil &&
		!errors.Is(err, asynq.ErrDuplicateTask) && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithFleetReports sets the schedule tick and the generation processors.
func (w *Worker) WithFleetReports(schedule FleetReportsScheduleFunc, generate FleetReportGenerateFunc) *Worker {
	w.fleetReportsSchedule = schedule
	w.fleetReportGenerate = generate
	return w
}

func (w *Worker) handleFleetReportsSchedule(ctx context.Context, _ *asynq.Task) error {
	if w.fleetReportsSchedule == nil {
		w.log.Warn("fleet_reports_schedule_handler_missing")
		return nil
	}
	return w.fleetReportsSchedule(ctx)
}

func (w *Worker) handleFleetReportGenerate(ctx context.Context, task *asynq.Task) error {
	var p FleetReportPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("queue: unmarshal %s: %w: %w", TaskFleetReportGenerate, err, asynq.SkipRetry)
	}
	if w.fleetReportGenerate == nil {
		w.log.Warn("fleet_report_generate_handler_missing", "report_id", p.ReportID)
		return nil
	}
	return w.fleetReportGenerate(ctx, p.ReportID, finalAttempt(ctx))
}
