package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx                             context.Context
	tx                              pgx.Tx
	q                               *db.Queries
	svc                             *Service
	out                             *outbox.Memory
	brandID                         int64
	seq                             int64
	center, dist, otherDist         db.Organization
	dealer, otherDealer             db.Organization
	centerUser, dealerUser          db.User
	accountingUser, otherDealerUser db.User
}

type fakeTaskEnqueuer struct {
	payloads []queue.AnnouncementDispatchPayload
}

func (f *fakeTaskEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	payload, err := queue.ParseAnnouncementDispatchPayload(task.Payload())
	if err != nil {
		return nil, err
	}
	f.payloads = append(f.payloads, payload)
	return &asynq.TaskInfo{}, nil
}

type fakeDispatcher struct {
	calls []notifmodel.DispatchInput
}

func (f *fakeDispatcher) Dispatch(_ context.Context, in notifmodel.DispatchInput) (notifmodel.DispatchResult, error) {
	f.calls = append(f.calls, in)
	return notifmodel.DispatchResult{}, nil
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool(t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	out := outbox.NewMemory()
	f := &fixture{ctx: ctx, tx: tx, q: q, svc: New(tx, q, out), out: out, brandID: brand.ID, center: center}
	f.dist = f.org(t, "dist", "distributor", center.ID)
	f.otherDist = f.org(t, "dist2", "distributor", center.ID)
	f.dealer = f.org(t, "dealer", "dealer", f.dist.ID)
	f.otherDealer = f.org(t, "dealer2", "dealer", f.otherDist.ID)
	f.centerUser = f.user(t, "center")
	f.dealerUser = f.member(t, f.dealer, "dealer_staff")
	f.accountingUser = f.member(t, f.dealer, "dealer_accounting")
	f.otherDealerUser = f.member(t, f.otherDealer, "dealer_staff")
	return f
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t330-%s-%d", name, time.Now().UnixNano()), Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) user(t *testing.T, name string) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("t330-%s-%d-%d@example.test", name, time.Now().UnixNano(), f.seq), Valid: true},
		PasswordHash: "x", Name: name, Surname: "User", Status: "active",
	})
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	return u
}

func (f *fixture) member(t *testing.T, org db.Organization, role string) db.User {
	t.Helper()
	u := f.user(t, role)
	m, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: org.ID, UserID: u.ID, Role: "staff"})
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{MemberID: m.ID, Slug: role}); err != nil {
		t.Fatalf("member role: %v", err)
	}
	return u
}

func (f *fixture) caller(org db.Organization, user db.User, roles ...string) Caller {
	return Caller{UserID: user.ID, Roles: roles, Org: orgctx.Scope{
		InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID,
	}}
}

func (f *fixture) create(t *testing.T, c Caller, title string, audiences []AudienceInput) Announcement {
	t.Helper()
	a, err := f.svc.Create(f.ctx, c, Input{
		DefaultLocale: "tr", Title: title, Body: "Gövde", BodyFormat: BodyMarkdown, Notify: true,
		Audiences: audiences,
	})
	if err != nil {
		t.Fatalf("create %s: %v", title, err)
	}
	return a
}

func (f *fixture) publish(t *testing.T, c Caller, id uuid.UUID) Announcement {
	t.Helper()
	a, err := f.svc.Publish(f.ctx, c, id)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return a
}

func has(items []Announcement, id uuid.UUID) bool {
	for _, it := range items {
		if it.UUID == id {
			return true
		}
	}
	return false
}

func TestDistributorSubtreeAnnouncementVisibility(t *testing.T) {
	f := newFixture(t)
	distCaller := f.caller(f.dist, f.centerUser, "distributor_owner")
	distUUID := f.dist.Uuid
	a := f.create(t, distCaller, "Subtree", []AudienceInput{{TargetType: TargetSubtree, TargetOrganizationUUID: &distUUID}})
	f.publish(t, distCaller, a.UUID)

	mine, _, err := f.svc.List(f.ctx, f.caller(f.dealer, f.dealerUser, "dealer_staff"), "tr", false, 20, 0)
	if err != nil || !has(mine, a.UUID) {
		t.Fatalf("dealer below distributor sees = %v, %v", has(mine, a.UUID), err)
	}
	other, _, err := f.svc.List(f.ctx, f.caller(f.otherDealer, f.otherDealerUser, "dealer_staff"), "tr", false, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if has(other, a.UUID) {
		t.Fatal("other distributor dealer saw subtree announcement")
	}
}

func TestPublishedAnnouncementFanoutBatchesTargets(t *testing.T) {
	f := newFixture(t)
	centerCaller := f.caller(f.center, f.centerUser, "center_staff")
	dealerUUID := f.dealer.Uuid
	a := f.create(t, centerCaller, "Toplu", []AudienceInput{{TargetType: TargetSubtree, TargetOrganizationUUID: &dealerUUID}})
	row, err := f.q.GetAnnouncementByUUID(f.ctx, a.UUID)
	if err != nil {
		t.Fatalf("announcement row: %v", err)
	}
	for range 1199 {
		f.member(t, f.dealer, "dealer_staff")
	}
	enq := &fakeTaskEnqueuer{}
	dispatcher := &fakeDispatcher{}
	ev := events.New(EventPublished).
		WithTenant(f.center.ID).
		WithEntity("announcement", &row.ID, &row.Uuid).
		WithPayload(map[string]any{
			"announcement_id":   row.ID,
			"announcement_uuid": row.Uuid.String(),
			"brand_id":          row.BrandID,
		})
	if err := EnqueuePublishedBatches(f.ctx, f.q, enq, dispatcher, ev); err != nil {
		t.Fatalf("enqueue batches: %v", err)
	}
	if len(dispatcher.calls) != 0 {
		t.Fatalf("dispatcher calls = %d", len(dispatcher.calls))
	}
	if len(enq.payloads) != 3 {
		t.Fatalf("batch count = %d", len(enq.payloads))
	}
	wantSizes := []int{500, 500, 201}
	for i, want := range wantSizes {
		got := len(enq.payloads[i].UserIDs)
		if got != want {
			t.Fatalf("batch %d size = %d, want %d", i, got, want)
		}
		if enq.payloads[i].Batch != i {
			t.Fatalf("batch %d index = %d", i, enq.payloads[i].Batch)
		}
	}
}

func TestRoleTargetExpiryReadCountLocaleAndPublishConflict(t *testing.T) {
	f := newFixture(t)
	centerCaller := f.caller(f.center, f.centerUser, "center_staff")
	role := "dealer_accounting"
	a := f.create(t, centerCaller, "Muhasebe", []AudienceInput{{TargetType: TargetRole, RoleSlug: &role}})
	if _, err := f.svc.UpsertLocale(f.ctx, centerCaller, a.UUID, "de", LocaleInput{Title: "Deutsch", Body: "Inhalt"}); err != nil {
		t.Fatalf("locale: %v", err)
	}
	f.publish(t, centerCaller, a.UUID)
	if len(f.out.All()) != 1 {
		t.Fatalf("outbox events after publish = %d", len(f.out.All()))
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(f.out.All()[0].Payload, &env); err != nil {
		t.Fatalf("outbox payload: %v", err)
	}
	if _, ok := env.Data["announcement_id"]; !ok {
		t.Fatal("outbox payload missing announcement_id")
	}
	for _, key := range []string{"notify_user_ids", "title", "body"} {
		if _, ok := env.Data[key]; ok {
			t.Fatalf("outbox payload includes %s", key)
		}
	}
	if _, err := f.svc.Publish(f.ctx, centerCaller, a.UUID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second publish err = %v", err)
	}
	if len(f.out.All()) != 1 {
		t.Fatalf("outbox events after second publish = %d", len(f.out.All()))
	}

	accounting := f.caller(f.dealer, f.accountingUser, "dealer_accounting")
	staff := f.caller(f.dealer, f.dealerUser, "dealer_staff")
	rows, _, err := f.svc.List(f.ctx, accounting, "de", false, 20, 0)
	if err != nil || len(rows) != 1 || rows[0].Title != "Deutsch" {
		t.Fatalf("de accounting rows = %+v, %v", rows, err)
	}
	rows, _, err = f.svc.List(f.ctx, staff, "de", false, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if has(rows, a.UUID) {
		t.Fatal("dealer_staff saw dealer_accounting announcement")
	}
	rows, _, err = f.svc.List(f.ctx, accounting, "az", false, 20, 0)
	if err != nil || len(rows) != 1 || rows[0].Title != "Muhasebe" {
		t.Fatalf("fallback rows = %+v, %v", rows, err)
	}
	before, err := f.svc.UnreadCount(f.ctx, accounting, "tr")
	if err != nil || before != 1 {
		t.Fatalf("unread before = %d, %v", before, err)
	}
	first, err := f.svc.GetVisible(f.ctx, accounting, a.UUID, "tr")
	if err != nil || first.ReadAt == nil {
		t.Fatalf("read first = %+v, %v", first, err)
	}
	after, err := f.svc.UnreadCount(f.ctx, accounting, "tr")
	if err != nil || after != 0 {
		t.Fatalf("unread after = %d, %v", after, err)
	}
	second, err := f.svc.GetVisible(f.ctx, accounting, a.UUID, "tr")
	if err != nil || second.ReadAt == nil || !second.ReadAt.Equal(*first.ReadAt) {
		t.Fatalf("read second = %+v, %v; first=%v", second, err, first.ReadAt)
	}

	expired, err := f.q.CreateAnnouncement(f.ctx, db.CreateAnnouncementParams{
		OrganizationID: f.center.ID, BrandID: f.brandID, DefaultLocale: "tr",
		Title: "Eski", Body: "Bitti", BodyFormat: BodyMarkdown, Status: StatusPublished,
		PublishAt: pgtype.Timestamptz{Time: time.Now().Add(-2 * time.Hour), Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("expired create: %v", err)
	}
	if _, err := f.q.AddAnnouncementAudience(f.ctx, db.AddAnnouncementAudienceParams{AnnouncementID: expired.ID, TargetType: TargetAllNetwork}); err != nil {
		t.Fatalf("expired audience: %v", err)
	}
	rows, _, err = f.svc.List(f.ctx, accounting, "tr", false, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if has(rows, expired.Uuid) {
		t.Fatal("expired announcement is visible")
	}
}
