package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-192 (F1-06h): the delayed review request against the real schema.
// The sender runs in savepoints of the fixture transaction (pgx.Tx is its
// TxBeginner), so everything is rolled back at the end.

// reviewCustomer creates a customer (with a phone when withPhone) and a
// vehicle of theirs.
func (f *serviceFixture) reviewCustomer(t *testing.T, withPhone bool) (db.User, db.Vehicle) {
	t.Helper()
	f.seq++
	nano := time.Now().UnixNano()
	arg := db.CreateUserParams{
		PasswordHash: "x", Name: "Musteri", Surname: "T192", Status: "active",
		Email: text(fmt.Sprintf("t192-%d-%d@example.test", nano, f.seq)),
	}
	if withPhone {
		// +90 5xx: 12 digits after "+", unique per run.
		arg.PhoneE164 = text(fmt.Sprintf("+905%09d", (nano/1000+int64(f.seq))%1_000_000_000))
	}
	u, err := f.q.CreateUser(f.ctx, arg)
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: u.ID, BrandID: f.brandID,
		CarBrandID: pgtype.Int8{Int64: f.carBrand, Valid: true},
		CarModelID: pgtype.Int8{Int64: f.carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	return u, v
}

// reviewService creates a completed dealer service for the customer.
func (f *serviceFixture) reviewService(t *testing.T, u db.User, v db.Vehicle, complete bool) db.Service {
	t.Helper()
	arg := f.serviceParams(f.dealer)
	arg.CustomerUserID, arg.VehicleID = u.ID, v.ID
	svc, err := f.q.CreateService(f.ctx, arg)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if !complete {
		return svc
	}
	svc, err = f.q.CompleteService(f.ctx, db.CompleteServiceParams{ID: svc.ID})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	return svc
}

func (f *serviceFixture) setReviewURL(t *testing.T, url *string) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, `UPDATE organizations SET google_business_url = $2 WHERE id = $1`, f.dealer.ID, url); err != nil {
		t.Fatalf("google_business_url: %v", err)
	}
}

func (f *serviceFixture) reviewState(t *testing.T, svc db.Service) (sentAt bool, n int) {
	t.Helper()
	var sent pgtype.Timestamptz
	if err := f.tx.QueryRow(f.ctx, `SELECT review_request_sent_at FROM services WHERE id = $1`, svc.ID).Scan(&sent); err != nil {
		t.Fatal(err)
	}
	return sent.Valid, f.outboxCount(t, events.ServiceReviewRequested, svc.ID, -1)
}

func TestServiceReviewRequest(t *testing.T) {
	f := newServiceFixture(t)
	sender := review.NewSender(f.tx, f.q, outbox.NewStore(nil, f.q), nil)
	url := "https://g.page/r/t192-dealer/review"

	send := func(t *testing.T, svc db.Service, want bool) {
		t.Helper()
		got, err := sender.Send(f.ctx, svc.ID)
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		if got != want {
			t.Fatalf("send service %d = %v, want %v", svc.ID, got, want)
		}
	}

	t.Run("dealer without google_business_url", func(t *testing.T) {
		f.setReviewURL(t, nil)
		u, v := f.reviewCustomer(t, true)
		svc := f.reviewService(t, u, v, true)
		send(t, svc, false)
		if sent, n := f.reviewState(t, svc); sent || n != 0 {
			t.Fatalf("no url: sent=%v events=%d", sent, n)
		}
	})

	f.setReviewURL(t, &url)

	t.Run("sent once", func(t *testing.T) {
		u, v := f.reviewCustomer(t, true)
		svc := f.reviewService(t, u, v, true)
		send(t, svc, true)
		if sent, n := f.reviewState(t, svc); !sent || n != 1 {
			t.Fatalf("first run: sent=%v events=%d", sent, n)
		}
		var customer, brand int64
		var reviewURL, org string
		err := f.tx.QueryRow(f.ctx, `SELECT (payload->'data'->>'customer_user_id')::bigint,
				(payload->'data'->>'brand_id')::bigint, payload->'data'->>'review_url',
				payload->'data'->>'organization_name'
			FROM outbox_events WHERE event_name = $1 AND (payload->>'entity_id')::bigint = $2`,
			events.ServiceReviewRequested, svc.ID).Scan(&customer, &brand, &reviewURL, &org)
		if err != nil {
			t.Fatal(err)
		}
		if customer != u.ID || brand != svc.BrandID || reviewURL != url || org != f.dealer.Name {
			t.Fatalf("payload customer=%d brand=%d url=%q org=%q", customer, brand, reviewURL, org)
		}
		// The task runs again (retry, duplicate): no second message.
		send(t, svc, false)
		if sent, n := f.reviewState(t, svc); !sent || n != 1 {
			t.Fatalf("second run: sent=%v events=%d", sent, n)
		}
	})

	t.Run("anonymized customer", func(t *testing.T) {
		u, v := f.reviewCustomer(t, true)
		svc := f.reviewService(t, u, v, true)
		if _, err := f.tx.Exec(f.ctx, `UPDATE users SET status = 'anonymized' WHERE id = $1`, u.ID); err != nil {
			t.Fatalf("anonymize: %v", err)
		}
		send(t, svc, false)
		if sent, n := f.reviewState(t, svc); sent || n != 0 {
			t.Fatalf("anonymized: sent=%v events=%d", sent, n)
		}
	})

	t.Run("customer without phone", func(t *testing.T) {
		u, v := f.reviewCustomer(t, false)
		svc := f.reviewService(t, u, v, true)
		send(t, svc, false)
		if sent, n := f.reviewState(t, svc); sent || n != 0 {
			t.Fatalf("no phone: sent=%v events=%d", sent, n)
		}
	})

	t.Run("service not completed", func(t *testing.T) {
		u, v := f.reviewCustomer(t, true)
		svc := f.reviewService(t, u, v, false)
		send(t, svc, false)
		if sent, n := f.reviewState(t, svc); sent || n != 0 {
			t.Fatalf("draft: sent=%v events=%d", sent, n)
		}
	})
}
