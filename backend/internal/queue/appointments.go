package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TaskAppointmentReminder   = "appointment:reminder"
	TaskAppointmentNoShowScan = "appointment:no_show_scan"
)

type AppointmentReminderPayload struct {
	AppointmentID int64     `json:"appointment_id"`
	StartsAt      time.Time `json:"starts_at"`
	Kind          string    `json:"kind"`
}

type AppointmentReminderFunc func(ctx context.Context, appointmentID int64, startsAt time.Time, kind string) error
type AppointmentNoShowScanFunc func(ctx context.Context) error

func NewAppointmentReminderTask(appointmentID int64, startsAt time.Time, kind string) (*asynq.Task, error) {
	body, err := json.Marshal(AppointmentReminderPayload{
		AppointmentID: appointmentID,
		StartsAt:      startsAt.UTC(),
		Kind:          kind,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal appointment reminder: %w", err)
	}
	return asynq.NewTask(TaskAppointmentReminder, body), nil
}

func ParseAppointmentReminderPayload(data []byte) (AppointmentReminderPayload, error) {
	var payload AppointmentReminderPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return AppointmentReminderPayload{}, fmt.Errorf("queue: unmarshal appointment reminder: %w", err)
	}
	return payload, nil
}

func AppointmentReminderTaskID(appointmentID int64, startsAt time.Time, kind string) string {
	return fmt.Sprintf("appointment-reminder-%s-%d-%d", kind, appointmentID, startsAt.UTC().Unix())
}

func AppointmentReminderOpts(appointmentID int64, startsAt time.Time, kind string, delay time.Duration) []asynq.Option {
	if delay < 0 {
		delay = 0
	}
	return []asynq.Option{
		asynq.Queue(QueueNotifications),
		asynq.ProcessIn(delay),
		asynq.TaskID(AppointmentReminderTaskID(appointmentID, startsAt, kind)),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
	}
}

func NewAppointmentNoShowScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskAppointmentNoShowScan, []byte("{}")), nil
}

func appointmentNoShowScanOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(5 * time.Minute)}
}

func (w *Worker) WithAppointmentReminder(fn AppointmentReminderFunc) *Worker {
	w.appointmentReminder = fn
	return w
}

func (w *Worker) handleAppointmentReminder(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseAppointmentReminderPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.appointmentReminder == nil {
		w.log.Warn("appointment_reminder_handler_missing", "appointment_id", payload.AppointmentID, "kind", payload.Kind)
		return nil
	}
	return w.appointmentReminder(ctx, payload.AppointmentID, payload.StartsAt, payload.Kind)
}

func (w *Worker) WithAppointmentNoShowScan(fn AppointmentNoShowScanFunc) *Worker {
	w.appointmentNoShowScan = fn
	return w
}

func (w *Worker) handleAppointmentNoShowScan(ctx context.Context, _ *asynq.Task) error {
	if w.appointmentNoShowScan == nil {
		w.log.Warn("appointment_no_show_scan_handler_missing")
		return nil
	}
	return w.appointmentNoShowScan(ctx)
}
