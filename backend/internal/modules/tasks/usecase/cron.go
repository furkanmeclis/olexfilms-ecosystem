package usecase

// TEC-221 (F1-11b): due date reminders. tasks:due_scan (hourly) writes
// tasks.overdue for open tasks past due_at and tasks.due_soon for open tasks
// due within DueSoonWindow; the notification module turns them into
// TASK_OVERDUE / TASK_DUE_SOON for the assignee (or the creator of an
// unassigned task). due_soon_notified_at / overdue_notified_at are stamped
// in the event's transaction (000066), so a second run writes nothing for
// the same task and threshold; changing due_at clears the stamps.

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DueSoonWindow is how far ahead a task counts as "due soon".
const DueSoonWindow = 24 * time.Hour

// DueNoticeBatch is how many tasks one reminder transaction handles.
const DueNoticeBatch = 200

// DueDateLayout is the due date shown in notifications (center's zone).
const DueDateLayout = "2006-01-02 15:04"

// CronService runs tasks:due_scan.
type CronService struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	now  func() time.Time
}

// NewCron builds the due date reminder service.
func NewCron(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *CronService {
	return &CronService{pool: pool, q: q, out: out, now: time.Now}
}

// DueScanTask is the tasks:due_scan handler.
func (s *CronService) DueScanTask(ctx context.Context) error {
	_, err := s.NotifyDue(ctx, s.now())
	return err
}

// DueScanResult counts the events one scan wrote.
type DueScanResult struct {
	Overdue int
	DueSoon int
}

// NotifyDue writes the overdue reminders first (they also stamp due_soon,
// so a task that crossed its deadline between runs gets one overdue
// reminder, not a stale due soon one), then the due soon reminders.
func (s *CronService) NotifyDue(ctx context.Context, now time.Time) (DueScanResult, error) {
	var res DueScanResult
	for _, overdue := range []bool{true, false} {
		for {
			n, err := s.notifyBatch(ctx, now, overdue)
			if overdue {
				res.Overdue += n
			} else {
				res.DueSoon += n
			}
			if err != nil {
				return res, err
			}
			if n < DueNoticeBatch {
				break
			}
		}
	}
	return res, nil
}

func (s *CronService) notifyBatch(ctx context.Context, now time.Time, overdue bool) (int, error) {
	var n int
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var rows []db.ClaimTasksOverdueRow
		name := events.TasksOverdue
		at := pgtype.Timestamptz{Time: now, Valid: true}
		if overdue {
			list, err := q.ClaimTasksOverdue(ctx, db.ClaimTasksOverdueParams{Now: at, RowLimit: DueNoticeBatch})
			if err != nil {
				return fmt.Errorf("tasks: claim overdue: %w", err)
			}
			rows = list
		} else {
			name = events.TasksDueSoon
			list, err := q.ClaimTasksDueSoon(ctx, db.ClaimTasksDueSoonParams{
				Now: at, Horizon: pgtype.Timestamptz{Time: now.Add(DueSoonWindow), Valid: true}, RowLimit: DueNoticeBatch,
			})
			if err != nil {
				return fmt.Errorf("tasks: claim due soon: %w", err)
			}
			for _, r := range list {
				rows = append(rows, db.ClaimTasksOverdueRow(r))
			}
		}
		for _, r := range rows {
			if err := s.out.Enqueue(ctx, tx, DueEvent(name, r)); err != nil {
				return fmt.Errorf("tasks: %s event: %w", name, err)
			}
		}
		n = len(rows)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// DueEvent builds a tasks.due_soon / tasks.overdue outbox event; the
// payload carries the recipients, the brand and the template variables.
func DueEvent(name string, r db.ClaimTasksOverdueRow) events.Event {
	notify := []int64{}
	switch {
	case r.AssigneeUserID.Valid:
		notify = append(notify, r.AssigneeUserID.Int64)
	case r.CreatedByUserID.Valid:
		notify = append(notify, r.CreatedByUserID.Int64)
	}
	id, u := r.ID, r.Uuid
	payload := map[string]any{
		"task_id":          r.ID,
		"task_uuid":        r.Uuid.String(),
		"organization_id":  r.OrganizationID,
		"brand_id":         r.BrandID,
		"title":            r.Title,
		"priority":         r.Priority,
		"subject_org_name": r.SubjectName,
		"notify_user_ids":  notify,
	}
	if r.DueAt.Valid {
		payload["due_at"] = r.DueAt.Time.UTC().Format(time.RFC3339)
		payload["due_date"] = DueDate(r.DueAt.Time, r.Timezone)
	}
	return events.New(name).
		WithTenant(r.OrganizationID).
		WithEntity(taskResource, &id, &u).
		WithPayload(payload)
}

// DueDate formats a due date in the center's zone; an unknown zone falls
// back to UTC.
func DueDate(t time.Time, zone string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	return t.In(loc).Format(DueDateLayout)
}

func (s *CronService) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tasks: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tasks: commit: %w", err)
	}
	return nil
}
