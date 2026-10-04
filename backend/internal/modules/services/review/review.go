// Package review sends the Google review request of a completed service
// (TEC-192, F1-06h; TEC-98 decision 7).
//
// service.completed is consumed by Scheduler, which enqueues one delayed
// service:review_request task per service (default 24 hours, Asynq
// ProcessIn, task id deduplicated). When the task runs, Sender re-checks
// every condition in one conditional UPDATE (service still completed,
// dealer google_business_url set, review_request_sent_at empty, customer
// not anonymized and with a phone), stamps review_request_sent_at and
// writes a service.review_requested outbox event in the same transaction.
// The notification module turns that event into the SERVICE_REVIEW_REQUEST
// WhatsApp message, so nothing here talks to a messaging provider and a
// second run of the task writes nothing.
package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	serviceuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	shorturls "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultDelay is the wait between completion and the review request.
const DefaultDelay = 24 * time.Hour

// Enqueuer schedules an Asynq task (*queue.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// TxBeginner opens a transaction (*pgxpool.Pool, or a pgx.Tx for a savepoint).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Scheduler enqueues the delayed review request on service.completed.
type Scheduler struct {
	queue Enqueuer
	delay time.Duration
	log   *slog.Logger
}

// NewScheduler builds the service.completed consumer. A nil queue disables
// it (no worker, no delayed task); delay <= 0 falls back to DefaultDelay.
func NewScheduler(q Enqueuer, delay time.Duration, log *slog.Logger) *Scheduler {
	if delay <= 0 {
		delay = DefaultDelay
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{queue: q, delay: delay, log: log}
}

// HandleServiceCompleted is the bus handler of service.completed. An
// enqueue failure is returned so the outbox redelivers the event; a task
// that is already pending (same task id) counts as done.
func (s *Scheduler) HandleServiceCompleted(ctx context.Context, ev events.Event) error {
	serviceID, ok := payloadInt64(ev.Payload, "service_id")
	if !ok && ev.EntityID != nil {
		serviceID, ok = *ev.EntityID, true
	}
	if !ok || serviceID <= 0 {
		s.log.Warn("service_review_no_service", "event_id", ev.EventID.String())
		return nil
	}
	if s.queue == nil {
		s.log.Warn("service_review_queue_missing", "service_id", serviceID)
		return nil
	}
	task, err := queue.NewServiceReviewRequestTask(serviceID)
	if err != nil {
		return err
	}
	if _, err := s.queue.Enqueue(task, queue.ServiceReviewRequestOpts(serviceID, s.delay)...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("service review: enqueue service %d: %w", serviceID, err)
	}
	s.log.Info("service_review_scheduled", "service_id", serviceID, "delay", s.delay.String())
	return nil
}

// Sender runs the service:review_request task.
type Sender struct {
	pool  TxBeginner
	q     *db.Queries
	out   outbox.Enqueuer
	links *shorturls.Linker
	log   *slog.Logger
	now   func() time.Time
}

// NewSender builds the task runner; log may be nil.
func NewSender(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, links *shorturls.Linker, log *slog.Logger) *Sender {
	if log == nil {
		log = slog.Default()
	}
	return &Sender{pool: pool, q: q, out: out, links: links, log: log, now: time.Now}
}

// Task is the service:review_request handler.
func (s *Sender) Task(ctx context.Context, serviceID int64) error {
	_, err := s.Send(ctx, serviceID)
	return err
}

// Send stamps review_request_sent_at and writes the service.review_requested
// event in one transaction when every condition holds. It reports whether
// a request was written; false means not eligible or already sent.
func (s *Sender) Send(ctx context.Context, serviceID int64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("service review: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	row, err := q.ClaimServiceReviewRequest(ctx, db.ClaimServiceReviewRequestParams{
		Now:       pgtype.Timestamptz{Time: s.now(), Valid: true},
		ServiceID: serviceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		s.log.Info("service_review_skipped", "service_id", serviceID)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("service review: claim: %w", err)
	}
	formURL := serviceuc.ReviewFormTarget(row.Uuid)
	if s.links != nil {
		link, err := s.links.LinkWithStore(ctx, q, shorturls.CreateInput{
			BrandID: row.BrandID, OrganizationID: &row.OrganizationID, Target: formURL,
		})
		if err != nil {
			return false, fmt.Errorf("service review: short url: %w", err)
		}
		formURL = link
	}
	if err := s.out.Enqueue(ctx, tx, Event(row, formURL)); err != nil {
		return false, fmt.Errorf("service review: outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("service review: commit: %w", err)
	}
	s.log.Info("service_review_requested", "service_id", serviceID)
	return true, nil
}

// Event builds the service.review_requested outbox event; the payload
// carries the recipient, brand and template variables.
func Event(row db.ClaimServiceReviewRequestRow, formURL string) events.Event {
	id, u := row.ID, row.Uuid
	return events.New(events.ServiceReviewRequested).
		WithTenant(row.OrganizationID).
		WithEntity("service", &id, &u).
		WithPayload(map[string]any{
			"service_id":        row.ID,
			"service_uuid":      row.Uuid.String(),
			"service_no":        row.ServiceNo,
			"organization_id":   row.OrganizationID,
			"brand_id":          row.BrandID,
			"customer_user_id":  row.CustomerUserID,
			"plate":             row.Plate.String,
			"organization_name": row.OrganizationName,
			"review_url":        row.ReviewUrl,
			"form_url":          formURL,
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
