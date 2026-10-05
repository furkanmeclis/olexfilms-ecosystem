package reminder

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testFixture struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	tx     pgx.Tx
	q      *db.Queries
	brand  int64
	center db.Organization
	dealer db.Organization
	user   db.User
	out    *outbox.Memory
	now    time.Time
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	f := &testFixture{t: t, ctx: ctx, pool: pool, tx: tx, q: q, out: outbox.NewMemory(), now: time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)}
	if err := tx.QueryRow(ctx, `SELECT id FROM brands WHERE slug = 'olex'`).Scan(&f.brand); err != nil {
		t.Fatalf("brand: %v", err)
	}
	f.center = f.centerOrg()
	f.dealer = f.org("tec325-dealer", "dealer", f.center.ID)
	f.user = f.userRow("tec325-customer")
	f.link(f.user, f.dealer)
	return f
}

func (f *testFixture) centerOrg() db.Organization {
	f.t.Helper()
	var row db.Organization
	err := f.tx.QueryRow(f.ctx, `SELECT id, uuid, slug, name, city, district, phone, address, logo_object_key, status, plan_code,
		access_starts_at, access_ends_at, created_at, updated_at, deleted_at, email, website, tagline, footer_text,
		paper_size, primary_color, type, parent_id, brand_id, currency, locale, timezone, country_id, contract_pdf_key,
		contract_valid_until, settings, province_id, district_id, phone_raw, google_business_url, latitude, longitude
		FROM organizations WHERE brand_id = $1 AND type = 'center' ORDER BY id LIMIT 1`, f.brand).Scan(
		&row.ID, &row.Uuid, &row.Slug, &row.Name, &row.City, &row.District, &row.Phone,
		&row.Address, &row.LogoObjectKey, &row.Status, &row.PlanCode, &row.AccessStartsAt,
		&row.AccessEndsAt, &row.CreatedAt, &row.UpdatedAt, &row.DeletedAt, &row.Email,
		&row.Website, &row.Tagline, &row.FooterText, &row.PaperSize, &row.PrimaryColor,
		&row.Type, &row.ParentID, &row.BrandID, &row.Currency, &row.Locale, &row.Timezone,
		&row.CountryID, &row.ContractPdfKey, &row.ContractValidUntil, &row.Settings, &row.ProvinceID,
		&row.DistrictID, &row.PhoneRaw, &row.GoogleBusinessUrl, &row.Latitude, &row.Longitude)
	if err != nil {
		f.t.Fatalf("center: %v", err)
	}
	return row
}

func (f *testFixture) org(prefix, typ string, parent int64) db.Organization {
	f.t.Helper()
	slug := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	params := db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: typ, BrandID: f.brand,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: f.now, Valid: true},
	}
	if parent != 0 {
		params.ParentID = pgtype.Int8{Int64: parent, Valid: true}
	}
	o, err := f.q.CreateOrganization(f.ctx, params)
	if err != nil {
		f.t.Fatalf("org: %v", err)
	}
	return o
}

func (f *testFixture) userRow(prefix string) db.User {
	f.t.Helper()
	n := time.Now().UnixNano() % 10_000_000
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("%s-%d@example.test", prefix, n), Valid: true},
		PhoneE164:    pgtype.Text{String: fmt.Sprintf("+90555%07d", n), Valid: true},
		PasswordHash: "x", Name: prefix, Surname: "User", Status: "active",
	})
	if err != nil {
		f.t.Fatalf("user: %v", err)
	}
	return u
}

func (f *testFixture) link(user db.User, org db.Organization) {
	f.t.Helper()
	if _, err := f.q.LinkCustomerOrganization(f.ctx, db.LinkCustomerOrganizationParams{
		UserID: user.ID, OrganizationID: org.ID, BrandID: org.BrandID,
	}); err != nil {
		f.t.Fatalf("link customer: %v", err)
	}
}

func (f *testFixture) appointment(starts time.Time) db.Appointment {
	f.t.Helper()
	a, err := f.q.CreateAppointment(f.ctx, db.CreateAppointmentParams{
		OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID, CustomerUserID: f.user.ID,
		StartsAt: tsArg(starts), EndsAt: tsArg(starts.Add(time.Hour)),
		EstimatedMinutes: 60, Source: "panel", Status: "scheduled",
	})
	if err != nil {
		f.t.Fatalf("appointment: %v", err)
	}
	return a
}

type fakeQueue struct {
	tasks []*asynq.Task
}

func (f *fakeQueue) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func TestSchedulerPlansTwoReminderJobs(t *testing.T) {
	q := &fakeQueue{}
	s := NewScheduler(q, nil)
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	starts := now.Add(48 * time.Hour)
	ev := events.New(events.AppointmentCreated).WithEntity("appointment", intPtr(42), nil).WithPayload(map[string]any{
		"starts_at": starts.Format(time.RFC3339),
	})
	if err := s.HandleAppointmentChanged(context.Background(), ev); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if len(q.tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(q.tasks))
	}
	got := map[string]bool{}
	for _, task := range q.tasks {
		payload, err := queue.ParseAppointmentReminderPayload(task.Payload())
		if err != nil {
			t.Fatalf("payload: %v", err)
		}
		got[payload.Kind] = payload.AppointmentID == 42 && payload.StartsAt.Equal(starts)
	}
	if !got[Kind24h] || !got[Kind2h] {
		t.Fatalf("scheduled kinds = %v", got)
	}
}

func TestReminderSenderSkipsChangedOrCancelledAndIsIdempotent(t *testing.T) {
	f := newTestFixture(t)
	sender := NewSender(f.tx, f.q, f.out, nil)
	sender.SetClock(func() time.Time { return f.now })
	oldStart := f.now.Add(30 * time.Hour)
	a := f.appointment(oldStart)
	newStart := oldStart.Add(24 * time.Hour)
	if _, err := f.q.UpdateAppointment(f.ctx, db.UpdateAppointmentParams{
		ID: a.ID, OrganizationID: a.OrganizationID, CustomerUserID: a.CustomerUserID,
		StartsAt: tsArg(newStart), EndsAt: tsArg(newStart.Add(time.Hour)),
		EstimatedMinutes: a.EstimatedMinutes, Source: a.Source, Note: a.Note,
	}); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	if sent, err := sender.Send(f.ctx, a.ID, oldStart, Kind24h); err != nil || sent {
		t.Fatalf("old reminder sent=%v err=%v, want skipped", sent, err)
	}
	if sent, err := sender.Send(f.ctx, a.ID, newStart, Kind24h); err != nil || !sent {
		t.Fatalf("new reminder sent=%v err=%v, want sent", sent, err)
	}
	if sent, err := sender.Send(f.ctx, a.ID, newStart, Kind24h); err != nil || sent {
		t.Fatalf("duplicate reminder sent=%v err=%v, want skipped", sent, err)
	}
	if got := len(f.out.All()); got != 1 {
		t.Fatalf("outbox events = %d, want 1", got)
	}

	cancelled := f.appointment(f.now.Add(4 * time.Hour))
	reason := pgtype.Text{String: "iptal", Valid: true}
	if _, err := f.q.SetAppointmentStatus(f.ctx, db.SetAppointmentStatusParams{
		ID: cancelled.ID, OrganizationID: cancelled.OrganizationID, Status: "cancelled", CancelReason: reason,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if sent, err := sender.Send(f.ctx, cancelled.ID, cancelled.StartsAt.Time, Kind2h); err != nil || sent {
		t.Fatalf("cancelled reminder sent=%v err=%v, want skipped", sent, err)
	}
}

type fakeFeatures struct {
	enabled bool
}

func (f fakeFeatures) Enabled(context.Context, int64, string) (bool, error) {
	return f.enabled, nil
}

func TestNoShowScanCreatesOneLeadWhenLeadsEnabled(t *testing.T) {
	f := newTestFixture(t)
	f.appointment(f.now.Add(-3 * time.Hour))
	scanner := NewNoShowScanner(f.tx, f.q, f.out, fakeFeatures{enabled: true}, nil)
	scanner.SetClock(func() time.Time { return f.now })
	if n, err := scanner.Scan(f.ctx); err != nil || n != 1 {
		t.Fatalf("scan n=%d err=%v, want 1", n, err)
	}
	if n, err := scanner.Scan(f.ctx); err != nil || n != 0 {
		t.Fatalf("second scan n=%d err=%v, want 0", n, err)
	}
	count := 0
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM leads WHERE organization_id = $1 AND customer_user_id = $2`, f.dealer.ID, f.user.ID).Scan(&count); err != nil {
		t.Fatalf("count leads: %v", err)
	}
	if count != 1 {
		t.Fatalf("leads = %d, want 1", count)
	}
	var status string
	if err := f.tx.QueryRow(f.ctx, `SELECT status FROM appointments WHERE organization_id = $1 AND customer_user_id = $2`, f.dealer.ID, f.user.ID).Scan(&status); err != nil {
		t.Fatalf("appointment status: %v", err)
	}
	if status != "no_show" {
		t.Fatalf("status = %s, want no_show", status)
	}
}

func TestNoShowScanSkipsLeadWhenLeadsDisabled(t *testing.T) {
	f := newTestFixture(t)
	f.appointment(f.now.Add(-3 * time.Hour))
	scanner := NewNoShowScanner(f.tx, f.q, f.out, fakeFeatures{enabled: false}, nil)
	scanner.SetClock(func() time.Time { return f.now })
	if n, err := scanner.Scan(f.ctx); err != nil || n != 1 {
		t.Fatalf("scan n=%d err=%v, want 1", n, err)
	}
	count := 0
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM leads WHERE organization_id = $1 AND customer_user_id = $2`, f.dealer.ID, f.user.ID).Scan(&count); err != nil {
		t.Fatalf("count leads: %v", err)
	}
	if count != 0 {
		t.Fatalf("leads = %d, want 0", count)
	}
}

func intPtr(v int64) *int64 { return &v }

func TestPayloadTimeRejectsBadValues(t *testing.T) {
	if _, ok := payloadTime(map[string]any{"starts_at": "bad"}, "starts_at"); ok {
		t.Fatal("bad timestamp accepted")
	}
	if _, ok := payloadTime(map[string]any{"starts_at": failableStringer{}}, "starts_at"); ok {
		t.Fatal("non-string timestamp accepted")
	}
}

type failableStringer struct{}

func (failableStringer) String() string { return "" }
