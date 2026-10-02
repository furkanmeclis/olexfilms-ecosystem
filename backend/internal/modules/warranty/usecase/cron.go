// Package usecase holds the warranty rules (TEC-98, F1-06).
//
// TEC-187 (F1-06c) adds the periodic part: expiry (warranty:expire) and the
// 30 / 7 day reminders (warranty:expiring_scan). Both only write warranty
// rows and outbox events in one transaction; the notification module turns
// the events into customer notifications (WhatsApp over wuzapi, in-app), so
// nothing here talks to a messaging provider.
//
// end_at is stored as the end of the last covered day in the organization's
// time zone (decision 4), so expiry is a plain end_at <= now comparison.
// notified_30_at / notified_7_at are stamped in the event's transaction
// (decision 5): a second run finds nothing and writes nothing.
package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TxBeginner opens a transaction (*pgxpool.Pool, or a pgx.Tx for a savepoint).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Reminder thresholds in days (decision 5).
const (
	Notice30 = 30
	Notice7  = 7
)

// NoticeBatch is how many warranties one reminder transaction handles.
const NoticeBatch = 200

// DateLayout is the end date shown in notifications (ISO, locale neutral).
const DateLayout = "2006-01-02"

// CronService runs the periodic warranty tasks.
type CronService struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	// verifyBaseURL is the public frontend origin; the verify link is
	// {verifyBaseURL}/garanti/{public_code} (decision 1).
	verifyBaseURL string
	now           func() time.Time
}

// NewCron builds the cron service.
func NewCron(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, verifyBaseURL string) *CronService {
	return &CronService{
		pool: pool, q: q, out: out,
		verifyBaseURL: strings.TrimRight(verifyBaseURL, "/"),
		now:           time.Now,
	}
}

// ExpireTask is the warranty:expire handler.
func (s *CronService) ExpireTask(ctx context.Context) error {
	_, err := s.ExpireDue(ctx, s.now())
	return err
}

// ExpiringScanTask is the warranty:expiring_scan handler.
func (s *CronService) ExpiringScanTask(ctx context.Context) error {
	_, err := s.NotifyExpiring(ctx, s.now())
	return err
}

// ExpireDue marks every active warranty with end_at <= now as expired and
// writes one warranty.expired event per warranty, in one transaction. The
// UPDATE only touches active rows, so a second run writes nothing.
func (s *CronService) ExpireDue(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		rows, err := q.ExpireDueWarranties(ctx, ts(now))
		if err != nil {
			return fmt.Errorf("warranty: expire: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		ctxs, err := s.noticeContexts(ctx, q, rows)
		if err != nil {
			return err
		}
		for _, w := range rows {
			if err := s.out.Enqueue(ctx, tx, s.event(events.WarrantyExpired, w, ctxs[w.ID], 0)); err != nil {
				return fmt.Errorf("warranty: expired event: %w", err)
			}
		}
		n = len(rows)
		return nil
	})
	return n, err
}

// NotifyExpiring writes warranty.expiring_soon for warranties that reach
// the 7 and 30 day thresholds. The 7 day pass runs first and also stamps
// notified_30_at, so a warranty that is already inside 7 days (created
// late, or a missed run) gets one 7 day reminder, not a stale 30 day one.
// It returns the number of events written per threshold.
func (s *CronService) NotifyExpiring(ctx context.Context, now time.Time) (map[int]int, error) {
	out := map[int]int{Notice7: 0, Notice30: 0}
	for _, days := range []int{Notice7, Notice30} {
		for {
			written, listed, err := s.notifyBatch(ctx, now, days)
			if err != nil {
				return out, err
			}
			out[days] += written
			if listed < NoticeBatch {
				break
			}
		}
	}
	return out, nil
}

// notifyBatch handles one page of candidates in one transaction. Marked
// rows leave the candidate list, so the next page starts after them.
func (s *CronService) notifyBatch(ctx context.Context, now time.Time, days int) (written, listed int, err error) {
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var rows []db.Warranty
		var err error
		if days == Notice7 {
			rows, err = q.ListWarrantiesDue7DayNotice(ctx, db.ListWarrantiesDue7DayNoticeParams{Now: ts(now), RowLimit: NoticeBatch})
		} else {
			rows, err = q.ListWarrantiesDue30DayNotice(ctx, db.ListWarrantiesDue30DayNoticeParams{Now: ts(now), RowLimit: NoticeBatch})
		}
		if err != nil {
			return fmt.Errorf("warranty: list %d day notice: %w", days, err)
		}
		listed = len(rows)
		if listed == 0 {
			return nil
		}
		ctxs, err := s.noticeContexts(ctx, q, rows)
		if err != nil {
			return err
		}
		for _, w := range rows {
			var marked int64
			if days == Notice7 {
				marked, err = q.MarkWarrantyNotified7(ctx, db.MarkWarrantyNotified7Params{ID: w.ID, Now: ts(now)})
				if err == nil && marked == 1 {
					// The 30 day reminder is superseded (no event).
					_, err = q.MarkWarrantyNotified30(ctx, db.MarkWarrantyNotified30Params{ID: w.ID, Now: ts(now)})
				}
			} else {
				marked, err = q.MarkWarrantyNotified30(ctx, db.MarkWarrantyNotified30Params{ID: w.ID, Now: ts(now)})
			}
			if err != nil {
				return fmt.Errorf("warranty: mark %d day notice: %w", days, err)
			}
			if marked != 1 {
				continue // stamped meanwhile: the other run sent it
			}
			if err := s.out.Enqueue(ctx, tx, s.event(events.WarrantyExpiringSoon, w, ctxs[w.ID], days)); err != nil {
				return fmt.Errorf("warranty: expiring_soon event: %w", err)
			}
			written++
		}
		return nil
	})
	if err != nil {
		written = 0
	}
	return written, listed, err
}

func (s *CronService) noticeContexts(ctx context.Context, q *db.Queries, rows []db.Warranty) (map[int64]db.ListWarrantyNoticeContextsRow, error) {
	ids := make([]int64, len(rows))
	for i, w := range rows {
		ids[i] = w.ID
	}
	list, err := q.ListWarrantyNoticeContexts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("warranty: notice context: %w", err)
	}
	out := make(map[int64]db.ListWarrantyNoticeContextsRow, len(list))
	for _, r := range list {
		out[r.ID] = r
	}
	return out, nil
}

// event builds the outbox event of a cron task.
func (s *CronService) event(name string, w db.Warranty, nc db.ListWarrantyNoticeContextsRow, days int) events.Event {
	return warrantyEvent(name, w, nc, s.VerifyURL(w.PublicCode), days, nil)
}

// warrantyEvent builds a warranty outbox event; the payload carries
// everything the notification handler needs (recipient, brand, template
// variables). extra adds event specific keys.
func warrantyEvent(name string, w db.Warranty, nc db.ListWarrantyNoticeContextsRow, verifyURL string, days int,
	extra map[string]any) events.Event {
	id, u := w.ID, w.Uuid
	payload := map[string]any{
		"warranty_uuid":     w.Uuid.String(),
		"public_code":       w.PublicCode,
		"holder_user_id":    w.HolderUserID,
		"organization_id":   w.OrganizationID,
		"brand_id":          w.BrandID,
		"service_id":        w.ServiceID,
		"vehicle_id":        w.VehicleID,
		"product_id":        w.ProductID,
		"end_at":            w.EndAt.Time.UTC().Format(time.RFC3339),
		"end_date":          EndDate(w.EndAt.Time, nc.Timezone),
		"plate":             nc.Plate.String,
		"product_name":      nc.ProductName,
		"organization_name": nc.OrganizationName,
		"verify_url":        verifyURL,
	}
	if days > 0 {
		payload["days"] = days
	}
	for k, v := range extra {
		payload[k] = v
	}
	return events.New(name).
		WithTenant(w.OrganizationID).
		WithEntity("warranty", &id, &u).
		WithPayload(payload)
}

// VerifyURL is the public warranty page of a public code.
func (s *CronService) VerifyURL(publicCode string) string {
	return verifyURL(s.verifyBaseURL, publicCode)
}

func verifyURL(base, publicCode string) string {
	if base == "" || publicCode == "" {
		return ""
	}
	return base + "/garanti/" + publicCode
}

// EndDate formats the last covered day in the organization's zone. end_at
// is the end of that day there (23:59:59 or the next midnight: both give
// the same day after the one second step back); an unknown zone falls back
// to UTC.
func EndDate(endAt time.Time, zone string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	return endAt.Add(-time.Second).In(loc).Format(DateLayout)
}

func (s *CronService) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("warranty: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("warranty: commit: %w", err)
	}
	return nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
