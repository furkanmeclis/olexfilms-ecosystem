package db_test

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
)

// TEC-187 (F1-06c): warranty cron against the real schema. The cron runs
// in savepoints of the fixture transaction (pgx.Tx is its TxBeginner), so
// everything is rolled back at the end.

// cronWarranty opens a full-unit warranty on a completed service ending at
// end (each one gets its own unit: one active full warranty per unit).
func (f *serviceFixture) cronWarranty(t *testing.T, end time.Time) db.Warranty {
	t.Helper()
	piece := f.unit(t, f.piece, "")
	_, items := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"})
	arg := f.warrantyParams(items[0])
	arg.StartAt, arg.EndAt = ts(end.AddDate(-1, 0, 0)), ts(end)
	w, err := f.q.CreateWarrantyForServiceItem(f.ctx, arg)
	if err != nil {
		t.Fatalf("warranty: %v", err)
	}
	return w
}

// outboxCount counts outbox rows of an event for one warranty; days < 0
// matches any days value.
func (f *serviceFixture) outboxCount(t *testing.T, name string, warrantyID int64, days int) int {
	t.Helper()
	var n int
	err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = $1 AND (payload->>'entity_id')::bigint = $2
		  AND ($3::int < 0 OR (payload->'data'->>'days')::int = $3)`,
		name, warrantyID, days).Scan(&n)
	if err != nil {
		t.Fatalf("outbox count: %v", err)
	}
	return n
}

func (f *serviceFixture) reloadWarranty(t *testing.T, w db.Warranty) db.Warranty {
	t.Helper()
	got, err := f.q.GetWarranty(f.ctx, db.GetWarrantyParams{ID: w.ID, BrandID: w.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWarrantyCron(t *testing.T) {
	f := newServiceFixture(t)
	cron := warrantyusecase.NewCron(f.tx, f.q, outbox.NewStore(nil, f.q), "https://olexfilms.app")
	now := time.Now()
	day := 24 * time.Hour

	t.Run("expire", func(t *testing.T) {
		due := f.cronWarranty(t, now.Add(-time.Hour))
		future := f.cronWarranty(t, now.Add(60*day))
		if _, err := cron.ExpireDue(f.ctx, now); err != nil {
			t.Fatalf("expire: %v", err)
		}
		if got := f.reloadWarranty(t, due); got.Status != "expired" || !got.ExpiredAt.Valid {
			t.Fatalf("due warranty = %s (expired_at %v)", got.Status, got.ExpiredAt.Valid)
		}
		if got := f.reloadWarranty(t, future); got.Status != "active" {
			t.Fatalf("future warranty = %s", got.Status)
		}
		if n := f.outboxCount(t, events.WarrantyExpired, due.ID, -1); n != 1 {
			t.Fatalf("warranty.expired rows = %d, want 1", n)
		}
		if n := f.outboxCount(t, events.WarrantyExpired, future.ID, -1); n != 0 {
			t.Fatalf("future warranty.expired rows = %d", n)
		}
		// Second run: nothing new.
		if _, err := cron.ExpireDue(f.ctx, now.Add(time.Minute)); err != nil {
			t.Fatalf("second expire: %v", err)
		}
		if n := f.outboxCount(t, events.WarrantyExpired, due.ID, -1); n != 1 {
			t.Fatalf("after second run warranty.expired rows = %d, want 1", n)
		}
		// The payload carries the holder, brand and template variables.
		var holder, brand int64
		var plate, endDate, verify string
		err := f.tx.QueryRow(f.ctx, `SELECT (payload->'data'->>'holder_user_id')::bigint,
				(payload->'data'->>'brand_id')::bigint, payload->'data'->>'plate',
				payload->'data'->>'end_date', payload->'data'->>'verify_url'
			FROM outbox_events WHERE event_name = $1 AND (payload->>'entity_id')::bigint = $2`,
			events.WarrantyExpired, due.ID).Scan(&holder, &brand, &plate, &endDate, &verify)
		if err != nil {
			t.Fatal(err)
		}
		if holder != f.customer.ID || brand != due.BrandID || endDate == "" ||
			verify != "https://olexfilms.app/garanti/"+due.PublicCode {
			t.Fatalf("payload holder=%d brand=%d end=%q verify=%q", holder, brand, endDate, verify)
		}
	})

	t.Run("30 and 7 day reminders", func(t *testing.T) {
		w30 := f.cronWarranty(t, now.Add(30*day-time.Hour))
		w7 := f.cronWarranty(t, now.Add(7*day-time.Hour))
		far := f.cronWarranty(t, now.Add(45*day))
		voided := f.cronWarranty(t, now.Add(5*day))
		if _, err := f.q.VoidWarranty(f.ctx, db.VoidWarrantyParams{ID: voided.ID, BrandID: voided.BrandID, VoidReason: text("test")}); err != nil {
			t.Fatalf("void: %v", err)
		}

		if _, err := cron.NotifyExpiring(f.ctx, now); err != nil {
			t.Fatalf("notify: %v", err)
		}
		check := func(stage string, w db.Warranty, days, want int) {
			t.Helper()
			if n := f.outboxCount(t, events.WarrantyExpiringSoon, w.ID, days); n != want {
				t.Fatalf("%s: warranty %d days=%d rows = %d, want %d", stage, w.ID, days, n, want)
			}
		}
		check("first run", w30, 30, 1)
		check("first run", w30, 7, 0)
		check("first run", w7, 7, 1)
		check("first run", w7, 30, 0) // superseded by the 7 day reminder
		check("first run", far, -1, 0)
		check("first run", voided, -1, 0)
		if got := f.reloadWarranty(t, w7); !got.Notified7At.Valid || !got.Notified30At.Valid {
			t.Fatalf("w7 stamps: 7=%v 30=%v", got.Notified7At.Valid, got.Notified30At.Valid)
		}
		if got := f.reloadWarranty(t, w30); !got.Notified30At.Valid || got.Notified7At.Valid {
			t.Fatalf("w30 stamps: 30=%v 7=%v", got.Notified30At.Valid, got.Notified7At.Valid)
		}

		// Second run the same day: no new rows.
		if _, err := cron.NotifyExpiring(f.ctx, now.Add(time.Minute)); err != nil {
			t.Fatalf("second notify: %v", err)
		}
		check("second run", w30, -1, 1)
		check("second run", w7, -1, 1)
		check("second run", voided, -1, 0)

		// 24 days later w30 reaches the 7 day threshold: one more reminder.
		later := now.Add(24 * day)
		if _, err := cron.NotifyExpiring(f.ctx, later); err != nil {
			t.Fatalf("later notify: %v", err)
		}
		check("later run", w30, 7, 1)
		check("later run", w30, 30, 1)
		check("later run", w7, -1, 1)
		check("later run", voided, -1, 0)

		// The void warranty is never expired either.
		if _, err := cron.ExpireDue(f.ctx, now.Add(10*day)); err != nil {
			t.Fatalf("expire: %v", err)
		}
		if got := f.reloadWarranty(t, voided); got.Status != "void" {
			t.Fatalf("void warranty = %s", got.Status)
		}
		if n := f.outboxCount(t, events.WarrantyExpired, voided.ID, -1); n != 0 {
			t.Fatalf("void warranty.expired rows = %d", n)
		}
	})
}
