// Package reminder schedules appointment reminders and marks stale bookings as no-show.
package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	Kind24h = "24h"
	Kind2h  = "2h"

	Delay24h       = 24 * time.Hour
	Delay2h        = 2 * time.Hour
	NoShowAfter    = 2 * time.Hour
	noShowLeadNote = "randevuya gelmedi"
)

type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

type Scheduler struct {
	queue Enqueuer
	now   func() time.Time
	log   *slog.Logger
}

func NewScheduler(q Enqueuer, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{queue: q, now: time.Now, log: log}
}

func (s *Scheduler) SetClock(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}

func (s *Scheduler) HandleAppointmentChanged(ctx context.Context, ev events.Event) error {
	appointmentID, ok := payloadInt64(ev.Payload, "appointment_id")
	if !ok && ev.EntityID != nil {
		appointmentID, ok = *ev.EntityID, true
	}
	startsAt, okTime := payloadTime(ev.Payload, "starts_at")
	if !ok || appointmentID <= 0 || !okTime {
		s.log.Warn("appointment_reminder_missing_payload", "event_id", ev.EventID.String())
		return nil
	}
	if s.queue == nil {
		s.log.Warn("appointment_reminder_queue_missing", "appointment_id", appointmentID)
		return nil
	}
	for _, spec := range []struct {
		kind   string
		before time.Duration
	}{
		{Kind24h, Delay24h},
		{Kind2h, Delay2h},
	} {
		runAt := startsAt.Add(-spec.before)
		if !runAt.After(s.now()) {
			continue
		}
		task, err := queue.NewAppointmentReminderTask(appointmentID, startsAt, spec.kind)
		if err != nil {
			return err
		}
		if _, err := s.queue.Enqueue(task, queue.AppointmentReminderOpts(appointmentID, startsAt, spec.kind, runAt.Sub(s.now()))...); err != nil {
			if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
				continue
			}
			return fmt.Errorf("appointment reminder: enqueue %d %s: %w", appointmentID, spec.kind, err)
		}
	}
	return nil
}

type Sender struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	log  *slog.Logger
	now  func() time.Time
}

func NewSender(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, log *slog.Logger) *Sender {
	if log == nil {
		log = slog.Default()
	}
	return &Sender{pool: pool, q: q, out: out, log: log, now: time.Now}
}

func (s *Sender) SetClock(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}

func (s *Sender) Task(ctx context.Context, appointmentID int64, startsAt time.Time, kind string) error {
	_, err := s.Send(ctx, appointmentID, startsAt, kind)
	return err
}

func (s *Sender) Send(ctx context.Context, appointmentID int64, startsAt time.Time, kind string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("appointment reminder: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	var row reminderRow
	switch kind {
	case Kind24h:
		got, err := q.ClaimAppointmentReminder24h(ctx, db.ClaimAppointmentReminder24hParams{
			ID: appointmentID, StartsAt: tsArg(startsAt), SentAt: tsArg(s.now()),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("appointment reminder: claim 24h: %w", err)
		}
		row = reminderRow{Appointment: appointmentFromReminder24h(got), OrganizationName: got.OrganizationName, Plate: got.Plate}
	case Kind2h:
		got, err := q.ClaimAppointmentReminder2h(ctx, db.ClaimAppointmentReminder2hParams{
			ID: appointmentID, StartsAt: tsArg(startsAt), SentAt: tsArg(s.now()),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("appointment reminder: claim 2h: %w", err)
		}
		row = reminderRow{Appointment: appointmentFromReminder2h(got), OrganizationName: got.OrganizationName, Plate: got.Plate}
	default:
		return false, fmt.Errorf("appointment reminder: unknown kind %q", kind)
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, ReminderEvent(row, kind)); err != nil {
			return false, fmt.Errorf("appointment reminder: outbox: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("appointment reminder: commit: %w", err)
	}
	return true, nil
}

type NoShowScanner struct {
	pool     TxBeginner
	q        *db.Queries
	out      outbox.Enqueuer
	features FeatureChecker
	log      *slog.Logger
	now      func() time.Time
}

func NewNoShowScanner(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, features FeatureChecker, log *slog.Logger) *NoShowScanner {
	if log == nil {
		log = slog.Default()
	}
	return &NoShowScanner{pool: pool, q: q, out: out, features: features, log: log, now: time.Now}
}

func (s *NoShowScanner) SetClock(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}

func (s *NoShowScanner) Task(ctx context.Context) error {
	_, err := s.Scan(ctx)
	return err
}

func (s *NoShowScanner) Scan(ctx context.Context) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("appointment no-show: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	rows, err := q.MarkDueNoShowAppointments(ctx, tsArg(s.now().Add(-NoShowAfter)))
	if err != nil {
		return 0, fmt.Errorf("appointment no-show: mark: %w", err)
	}
	for _, a := range rows {
		if err := s.handleLead(ctx, q, a); err != nil {
			return 0, err
		}
		if s.out != nil {
			if err := s.out.Enqueue(ctx, tx, NoShowEvent(a)); err != nil {
				return 0, fmt.Errorf("appointment no-show: outbox: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("appointment no-show: commit: %w", err)
	}
	return len(rows), nil
}

func (s *NoShowScanner) handleLead(ctx context.Context, q *db.Queries, a db.Appointment) error {
	if s.features != nil {
		enabled, err := s.features.Enabled(ctx, a.OrganizationID, features.ModuleLeads)
		if err != nil {
			return fmt.Errorf("appointment no-show: feature: %w", err)
		}
		if !enabled {
			return nil
		}
	}
	lead, err := q.FindOpenCustomerLeadForAppointment(ctx, db.FindOpenCustomerLeadForAppointmentParams{
		OrganizationID: a.OrganizationID, BrandID: a.BrandID,
		CustomerUserID: pgtype.Int8{Int64: a.CustomerUserID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		followUp := s.now().AddDate(0, 0, 1)
		lead, err = q.CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: a.OrganizationID,
			BrandID:        a.BrandID,
			TargetType:     "customer",
			CustomerUserID: pgtype.Int8{Int64: a.CustomerUserID, Valid: true},
			VehicleID:      a.VehicleID,
			Source:         "other",
			Temperature:    "warm",
			Status:         "new",
			FollowUpDate:   tsArg(followUp),
			Notes:          noShowLeadNote,
		})
		if err != nil {
			return fmt.Errorf("appointment no-show: create lead: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("appointment no-show: find lead: %w", err)
	} else {
		_, err = q.AddLeadEvent(ctx, db.AddLeadEventParams{
			LeadID: lead.ID, OrganizationID: lead.OrganizationID, BrandID: lead.BrandID,
			EventType: "note",
			Payload:   []byte(fmt.Sprintf(`{"note":%q,"appointment_uuid":%q}`, noShowLeadNote, a.Uuid.String())),
		})
		if err != nil {
			return fmt.Errorf("appointment no-show: lead event: %w", err)
		}
	}
	_, err = q.LinkAppointmentLead(ctx, db.LinkAppointmentLeadParams{
		ID: a.ID, OrganizationID: a.OrganizationID, LeadID: pgtype.Int8{Int64: lead.ID, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("appointment no-show: link lead: %w", err)
	}
	return nil
}

type reminderRow struct {
	db.Appointment
	OrganizationName string
	Plate            string
}

func ReminderEvent(row reminderRow, kind string) events.Event {
	id, uid := row.ID, row.Uuid
	return events.New(events.AppointmentReminder).
		WithTenant(row.OrganizationID).
		WithEntity("appointment", &id, &uid).
		WithPayload(map[string]any{
			"appointment_id":    row.ID,
			"appointment_uuid":  row.Uuid.String(),
			"organization_id":   row.OrganizationID,
			"brand_id":          row.BrandID,
			"customer_user_id":  row.CustomerUserID,
			"starts_at":         row.StartsAt.Time.UTC().Format(time.RFC3339),
			"reminder_kind":     kind,
			"organization_name": row.OrganizationName,
			"plate":             row.Plate,
		})
}

func NoShowEvent(a db.Appointment) events.Event {
	id, uid := a.ID, a.Uuid
	return events.New(events.AppointmentNoShow).
		WithTenant(a.OrganizationID).
		WithEntity("appointment", &id, &uid).
		WithPayload(map[string]any{
			"appointment_id":   a.ID,
			"appointment_uuid": a.Uuid.String(),
			"organization_id":  a.OrganizationID,
			"brand_id":         a.BrandID,
			"customer_user_id": a.CustomerUserID,
			"starts_at":        a.StartsAt.Time.UTC().Format(time.RFC3339),
		})
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	switch v := payload[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func payloadTime(payload map[string]any, key string) (time.Time, bool) {
	switch v := payload[key].(type) {
	case time.Time:
		return v.UTC(), true
	case string:
		t, err := time.Parse(time.RFC3339, v)
		return t.UTC(), err == nil
	default:
		return time.Time{}, false
	}
}

func tsArg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func appointmentFromReminder24h(r db.ClaimAppointmentReminder24hRow) db.Appointment {
	return db.Appointment{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID,
		CustomerUserID: r.CustomerUserID, VehicleID: r.VehicleID, StartsAt: r.StartsAt, EndsAt: r.EndsAt,
		EstimatedMinutes: r.EstimatedMinutes, Source: r.Source, Status: r.Status, CancelReason: r.CancelReason,
		LeadID: r.LeadID, ServiceID: r.ServiceID, Note: r.Note, CreatedByUserID: r.CreatedByUserID,
		Reminded24hAt: r.Reminded24hAt, Reminded2hAt: r.Reminded2hAt, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
	}
}

func appointmentFromReminder2h(r db.ClaimAppointmentReminder2hRow) db.Appointment {
	return db.Appointment{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID,
		CustomerUserID: r.CustomerUserID, VehicleID: r.VehicleID, StartsAt: r.StartsAt, EndsAt: r.EndsAt,
		EstimatedMinutes: r.EstimatedMinutes, Source: r.Source, Status: r.Status, CancelReason: r.CancelReason,
		LeadID: r.LeadID, ServiceID: r.ServiceID, Note: r.Note, CreatedByUserID: r.CreatedByUserID,
		Reminded24hAt: r.Reminded24hAt, Reminded2hAt: r.Reminded2hAt, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
	}
}
