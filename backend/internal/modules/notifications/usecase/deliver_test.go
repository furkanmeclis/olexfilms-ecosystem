package usecase

import (
	"context"
	"log/slog"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/jackc/pgx/v5"
)

type deliverTestQuerier struct {
	pushTestQuerier
	row      db.Notification
	claimErr error
	sent     int
	stuck    []int64
}

func (d *deliverTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.claimErr != nil {
		return db.Notification{}, d.claimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	d.sent++
	d.row.Status = model.StatusDelivered
	return d.row, nil
}

func (d *deliverTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.stuck, nil
}

func TestDeliverResumesProcessingAfterClaimMiss(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
			Title:   "Hello",
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

func TestDeliverSkipsTerminalStatus(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusDelivered,
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestReclaimStuckDeliversProcessingRows(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      3,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
		},
		stuck: []int64{3},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed = %d, want 1", n)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}
