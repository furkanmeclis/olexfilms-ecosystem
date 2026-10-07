package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-466 acceptance. Every test runs in one rolled-back transaction; the
// organizations of a test share a unique slug/name prefix.

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	store    *Store
	prefix   string
	seq      int
	brandID  int64
	centerID int64
	dist     db.Organization
	user     db.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, tx: tx, q: db.New(tx), store: New(tx), prefix: fmt.Sprintf("t466-%d", time.Now().UnixNano())}
	brand, err := f.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	center, err := f.q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	f.brandID, f.centerID = brand.ID, center.ID
	f.dist = f.org(t, "dist", "distributor", center.ID, "Ankara")
	f.user, err = f.q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Vitrin", Surname: "T466", Status: "active",
		Email: pgtype.Text{String: f.prefix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return f
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64, city string) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, name, f.seq), Name: f.prefix + " " + name, City: city,
		Status:         "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) dealer(t *testing.T, name, city string) db.Organization {
	t.Helper()
	return f.org(t, name, "dealer", f.dist.ID, city)
}

func upsertParams(o db.Organization) db.UpsertDealerShowcaseParams {
	return db.UpsertDealerShowcaseParams{
		OrganizationID: o.ID, BrandID: o.BrandID,
		Content:      []byte(`{"tr":{"headline":"PPF uzmanı","about":"Hakkımızda"},"en":{"headline":"PPF expert"}}`),
		WorkingHours: []byte(`{"monday":[{"start":"09:00","end":"18:00"}]}`),
		SocialLinks:  []byte(`{"instagram":"https://instagram.com/olex","website":"https://olex.example"}`),
		SeoKeywords:  []string{"ankara ppf", "boya koruma filmi"},
	}
}

func (f *fixture) showcase(t *testing.T, o db.Organization) db.DealerShowcase {
	t.Helper()
	s, err := f.q.UpsertDealerShowcase(f.ctx, upsertParams(o))
	if err != nil {
		t.Fatalf("showcase %s: %v", o.Name, err)
	}
	return s
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// expectConstraint runs fn in a savepoint and wants a pg error with code and
// (when given) constraint name.
func (f *fixture) expectConstraint(t *testing.T, name, code, constraint string, fn func(q *db.Queries) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(db.New(sp))
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code || (constraint != "" && pgErr.ConstraintName != constraint) {
		t.Fatalf("%s: want %s on %s, got %v", name, code, constraint, err)
	}
}

func photoParams(showcaseID int64, n int) db.InsertDealerShowcasePhotoParams {
	return db.InsertDealerShowcasePhotoParams{
		ShowcaseID: showcaseID, StorageKey: fmt.Sprintf("showcase/%d/%s.jpg", showcaseID, uuid.NewString()),
		Mime: "image/jpeg", SizeBytes: 1024, Sha256: fmt.Sprintf("%064x", n+1), Caption: []byte(`{}`),
	}
}

func i64(v int64) *int64 { return &v }

// Acceptance: the status CAS updates no row on the second call.
func TestShowcaseStatusCAS(t *testing.T) {
	f := newFixture(t)
	d := f.dealer(t, "cas", "Ankara")
	s := f.showcase(t, d)
	if s.Status != model.StatusDraft || s.PublishedContent != nil {
		t.Fatalf("new showcase = %+v", s)
	}

	got, err := f.store.Submit(f.ctx, s.ID, model.StatusDraft, i64(f.user.ID))
	if err != nil || got.Status != model.StatusPendingReview || !got.SubmittedAt.Valid {
		t.Fatalf("submit = %+v, %v", got, err)
	}
	// Second call: the raw query matches no row, the store reports a conflict.
	if _, err := f.q.SubmitDealerShowcase(f.ctx, db.SubmitDealerShowcaseParams{ID: s.ID, FromStatus: model.StatusDraft}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second raw submit err = %v, want no rows", err)
	}
	if _, err := f.store.Submit(f.ctx, s.ID, model.StatusDraft, nil); !errors.Is(err, ErrStatusConflict) {
		t.Fatalf("second submit err = %v, want ErrStatusConflict", err)
	}
	// The graph refuses moves it does not know before touching the row.
	if _, err := f.store.Submit(f.ctx, s.ID, model.StatusPendingReview, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pending → pending err = %v", err)
	}

	snapshot := []byte(`{"content":{"tr":{"headline":"PPF uzmanı"}},"services":[],"photos":[]}`)
	pub, err := f.store.Publish(f.ctx, s.ID, model.StatusPendingReview, snapshot, i64(f.user.ID), nil)
	if err != nil || pub.Status != model.StatusPublished || !pub.PublishedAt.Valid || pub.ReviewedBy.Int64 != f.user.ID || !pub.ReviewedAt.Valid {
		t.Fatalf("publish = %+v, %v", pub, err)
	}
	if _, err := f.store.Publish(f.ctx, s.ID, model.StatusPendingReview, snapshot, i64(f.user.ID), nil); !errors.Is(err, ErrStatusConflict) {
		t.Fatalf("second publish err = %v, want ErrStatusConflict", err)
	}

	// A draft edit after publishing keeps the status and the snapshot.
	edit := upsertParams(d)
	edit.Content = []byte(`{"tr":{"headline":"Yeni başlık"}}`)
	edited, err := f.q.UpsertDealerShowcase(f.ctx, edit)
	if err != nil || edited.ID != s.ID || edited.Status != model.StatusPublished || string(edited.PublishedContent) != string(pub.PublishedContent) {
		t.Fatalf("edit after publish = %+v, %v", edited, err)
	}

	// Re-submitted and rejected: the reject CAS also moves once, and the old
	// snapshot stays live on the public read.
	if _, err := f.store.Submit(f.ctx, s.ID, model.StatusPublished, nil); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	rej, err := f.store.Reject(f.ctx, s.ID, f.user.ID, "Fotoğraflar eksik")
	if err != nil || rej.Status != model.StatusRejected || rej.ReviewNote.String != "Fotoğraflar eksik" {
		t.Fatalf("reject = %+v, %v", rej, err)
	}
	if _, err := f.store.Reject(f.ctx, s.ID, f.user.ID, "again"); !errors.Is(err, ErrStatusConflict) {
		t.Fatalf("second reject err = %v, want ErrStatusConflict", err)
	}
	live, err := f.q.GetPublishedDealerShowcase(f.ctx, db.GetPublishedDealerShowcaseParams{Slug: d.Slug, BrandID: f.brandID})
	if err != nil || string(live.PublishedContent) != string(pub.PublishedContent) || live.OrganizationLocale != "tr" {
		t.Fatalf("public read after reject = %+v, %v", live, err)
	}

	// Owner publishes directly (approval off): no reviewer is recorded.
	direct, err := f.store.Publish(f.ctx, s.ID, model.StatusRejected, snapshot, nil, i64(f.user.ID))
	if err != nil || direct.ReviewedBy.Valid || direct.ReviewedAt.Valid || direct.ReviewNote.Valid {
		t.Fatalf("direct publish = %+v, %v", direct, err)
	}
}

// Acceptance: the 13th photo hits the usecase limit; the count query
// reports the gallery size.
func TestShowcasePhotoLimit(t *testing.T) {
	f := newFixture(t)
	s := f.showcase(t, f.dealer(t, "photos", "İzmir"))
	const max = 12
	var uuids []uuid.UUID
	for i := 0; i < max; i++ {
		p, err := f.store.AddPhoto(f.ctx, max, photoParams(s.ID, i))
		if err != nil {
			t.Fatalf("photo %d: %v", i+1, err)
		}
		if p.SortOrder != int32((i+1)*10) || p.OrganizationID != s.OrganizationID || p.BrandID != s.BrandID {
			t.Fatalf("photo %d = %+v", i+1, p)
		}
		uuids = append(uuids, p.Uuid)
	}
	_, err := f.store.AddPhoto(f.ctx, max, photoParams(s.ID, max))
	var limit *PhotoLimitError
	if !errors.As(err, &limit) || limit.Count != max || limit.Max != max {
		t.Fatalf("13th photo err = %v, want PhotoLimitError{12, 12}", err)
	}
	if n, err := f.q.CountDealerShowcasePhotos(f.ctx, s.ID); err != nil || n != max {
		t.Fatalf("count = %d, %v", n, err)
	}

	// Reorder: the full list reverses the gallery; a partial list or a
	// duplicate is refused.
	rev := make([]uuid.UUID, len(uuids))
	for i, u := range uuids {
		rev[len(uuids)-1-i] = u
	}
	if err := f.store.ReorderPhotos(f.ctx, s.ID, rev); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	list, err := f.q.ListDealerShowcasePhotos(f.ctx, s.ID)
	if err != nil || list[0].Uuid != uuids[max-1] || list[max-1].Uuid != uuids[0] {
		t.Fatalf("reordered first = %v, %v", list[0].Uuid, err)
	}
	if err := f.store.ReorderPhotos(f.ctx, s.ID, rev[1:]); !errors.Is(err, ErrReorderMismatch) {
		t.Fatalf("partial reorder err = %v", err)
	}
	dup := append([]uuid.UUID{rev[0]}, rev[:max-1]...)
	if err := f.store.ReorderPhotos(f.ctx, s.ID, dup); !errors.Is(err, ErrReorderMismatch) {
		t.Fatalf("duplicate reorder err = %v", err)
	}

	// Deleting frees a slot and returns the storage key.
	key, err := f.q.DeleteDealerShowcasePhoto(f.ctx, db.DeleteDealerShowcasePhotoParams{Uuid: uuids[0], ShowcaseID: s.ID})
	if err != nil || !strings.HasPrefix(key, fmt.Sprintf("showcase/%d/", s.ID)) {
		t.Fatalf("delete = %q, %v", key, err)
	}
	if _, err := f.store.AddPhoto(f.ctx, max, photoParams(s.ID, 100)); err != nil {
		t.Fatalf("photo after delete: %v", err)
	}
}

// Acceptance: the review queue refuses a sort field outside the whitelist
// and breaks ties by id in a stable way.
func TestShowcaseReviewQueue(t *testing.T) {
	f := newFixture(t)
	a := f.dealer(t, "alfa", "Ankara")
	b := f.dealer(t, "beta", "Bursa")
	c := f.dealer(t, "gama", "Ankara")
	sa, sb, sc := f.showcase(t, a), f.showcase(t, b), f.showcase(t, c)
	if _, err := f.store.Submit(f.ctx, sb.ID, model.StatusDraft, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Submit(f.ctx, sc.ID, model.StatusDraft, nil); err != nil {
		t.Fatal(err)
	}
	// a and b share updated_at; c is newer.
	same := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f.exec(t, `ALTER TABLE dealer_showcases DISABLE TRIGGER trg_dealer_showcases_set_updated_at`)
	f.exec(t, `UPDATE dealer_showcases SET updated_at = $2 WHERE id = ANY($1)`, []int64{sa.ID, sb.ID}, same)
	f.exec(t, `UPDATE dealer_showcases SET updated_at = $2 WHERE id = $1`, sc.ID, same.Add(time.Hour))
	f.exec(t, `ALTER TABLE dealer_showcases ENABLE TRIGGER trg_dealer_showcases_set_updated_at`)
	mine := []int64{a.ID, b.ID, c.ID}

	list := func(t *testing.T, flt ReviewFilter) []int64 {
		t.Helper()
		if flt.OrganizationIDs == nil {
			flt.OrganizationIDs = mine
		}
		paged := flt.Limit != 0
		if !paged {
			flt.Limit = 50
		}
		rows, total, err := f.store.ListReviews(f.ctx, f.brandID, flt)
		if err != nil {
			t.Fatalf("list %+v: %v", flt, err)
		}
		if (!paged && total != int64(len(rows))) || (paged && total != 3) {
			t.Fatalf("total %d != rows %d", total, len(rows))
		}
		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		return ids
	}
	eq := func(t *testing.T, got, want []int64) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}

	t.Run("unknown sort field is a validation error", func(t *testing.T) {
		for _, field := range []string{"id", "organization_id", "published_at", "created_at"} {
			_, _, err := f.store.ListReviews(f.ctx, f.brandID, ReviewFilter{Sort: []apiquery.SortField{{Field: field}}, Limit: 10})
			var ve *apiquery.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("sort=%s err = %v, want *apiquery.ValidationError", field, err)
			}
		}
	})

	t.Run("default -updated_at with id tiebreak", func(t *testing.T) {
		// c newest; a/b tie → id in the sort direction (desc).
		eq(t, list(t, ReviewFilter{}), []int64{sc.ID, sb.ID, sa.ID})
		eq(t, list(t, ReviewFilter{Sort: []apiquery.SortField{{Field: "updated_at"}}}), []int64{sa.ID, sb.ID, sc.ID})
		// Stable across pages.
		eq(t, append(list(t, ReviewFilter{Limit: 2}), list(t, ReviewFilter{Limit: 2, Offset: 2})...), []int64{sc.ID, sb.ID, sa.ID})
	})

	t.Run("name and status", func(t *testing.T) {
		eq(t, list(t, ReviewFilter{Sort: []apiquery.SortField{{Field: "name"}}}), []int64{sa.ID, sb.ID, sc.ID})
		eq(t, list(t, ReviewFilter{Sort: []apiquery.SortField{{Field: "name", Desc: true}}}), []int64{sc.ID, sb.ID, sa.ID})
		// draft (a) before pending (b, c by id); desc reverses incl. tiebreak.
		eq(t, list(t, ReviewFilter{Sort: []apiquery.SortField{{Field: "status"}}}), []int64{sa.ID, sb.ID, sc.ID})
		eq(t, list(t, ReviewFilter{Sort: []apiquery.SortField{{Field: "status", Desc: true}}}), []int64{sc.ID, sb.ID, sa.ID})
	})

	t.Run("filters", func(t *testing.T) {
		eq(t, list(t, ReviewFilter{Statuses: []string{model.StatusPendingReview}}), []int64{sc.ID, sb.ID})
		eq(t, list(t, ReviewFilter{Statuses: []string{model.StatusDraft, model.StatusPendingReview}}), []int64{sc.ID, sb.ID, sa.ID})
		// q: organization city or name (brand wide, no scope list).
		eq(t, list(t, ReviewFilter{Q: f.prefix + " beta", OrganizationIDs: []int64{}}), []int64{sb.ID})
		eq(t, list(t, ReviewFilter{Q: "ankara"}), []int64{sc.ID, sa.ID})
		eq(t, list(t, ReviewFilter{Q: "%"}), []int64{})
		after := same.Add(time.Minute)
		eq(t, list(t, ReviewFilter{Updated: apiquery.TimeRange{From: &after}}), []int64{sc.ID})
		eq(t, list(t, ReviewFilter{Updated: apiquery.TimeRange{Before: &after}}), []int64{sb.ID, sa.ID})
		eq(t, list(t, ReviewFilter{OrganizationIDs: []int64{b.ID}}), []int64{sb.ID})
		// Another brand's queue does not see them.
		glorian, err := f.q.GetBrandBySlug(f.ctx, "glorian")
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := f.store.ListReviews(f.ctx, glorian.ID, ReviewFilter{OrganizationIDs: mine, Limit: 10})
		if err != nil || len(rows) != 0 || total != 0 {
			t.Fatalf("glorian queue = %d/%d, %v", len(rows), total, err)
		}
	})
}

func TestShowcaseServices(t *testing.T) {
	f := newFixture(t)
	s := f.showcase(t, f.dealer(t, "services", "Bursa"))
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: f.prefix + "-cat", AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	catSvc, err := f.q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
		ShowcaseID: s.ID, Kind: model.ServiceKindProductCategory, CategoryID: pgtype.Int8{Int64: cat.ID, Valid: true},
		Title: []byte(`{}`), Description: []byte(`{"tr":"Tüm gövde PPF"}`), Visible: true,
	})
	if err != nil || catSvc.SortOrder != 10 || catSvc.OrganizationID != s.OrganizationID {
		t.Fatalf("category service = %+v, %v", catSvc, err)
	}
	custom, err := f.q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
		ShowcaseID: s.ID, Kind: model.ServiceKindCustom,
		Title: []byte(`{"tr":"Seramik kaplama","en":"Ceramic coating"}`), Description: []byte(`{}`), Visible: true,
	})
	if err != nil || custom.SortOrder != 20 {
		t.Fatalf("custom service = %+v, %v", custom, err)
	}

	f.expectConstraint(t, "same category twice", "23505", "uq_dealer_showcase_services_category", func(q *db.Queries) error {
		_, err := q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
			ShowcaseID: s.ID, Kind: model.ServiceKindProductCategory, CategoryID: pgtype.Int8{Int64: cat.ID, Valid: true},
			Title: []byte(`{}`), Description: []byte(`{}`),
		})
		return err
	})
	f.expectConstraint(t, "custom without title", "23514", "chk_dealer_showcase_services_category", func(q *db.Queries) error {
		_, err := q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
			ShowcaseID: s.ID, Kind: model.ServiceKindCustom, Title: []byte(`{}`), Description: []byte(`{}`),
		})
		return err
	})
	f.expectConstraint(t, "unknown title locale", "23514", "chk_dealer_showcase_services_title", func(q *db.Queries) error {
		_, err := q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
			ShowcaseID: s.ID, Kind: model.ServiceKindCustom, Title: []byte(`{"xx":"?"}`), Description: []byte(`{}`),
		})
		return err
	})
	// A category of another brand is refused by the brand-consistent FK.
	glorian, err := f.q.GetBrandBySlug(f.ctx, "glorian")
	if err != nil {
		t.Fatal(err)
	}
	gCenter, err := f.q.GetBrandCenter(f.ctx, glorian.ID)
	if err != nil {
		t.Fatal(err)
	}
	gCat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: gCenter.ID, BrandID: glorian.ID, Name: f.prefix + "-gcat", AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.expectConstraint(t, "other brand category", "23503", "fk_dealer_showcase_services_category", func(q *db.Queries) error {
		_, err := q.InsertDealerShowcaseService(f.ctx, db.InsertDealerShowcaseServiceParams{
			ShowcaseID: s.ID, Kind: model.ServiceKindProductCategory, CategoryID: pgtype.Int8{Int64: gCat.ID, Valid: true},
			Title: []byte(`{}`), Description: []byte(`{}`),
		})
		return err
	})

	upd, err := f.q.UpdateDealerShowcaseService(f.ctx, db.UpdateDealerShowcaseServiceParams{
		Uuid: custom.Uuid, ShowcaseID: s.ID, Kind: model.ServiceKindCustom,
		Title: custom.Title, Description: []byte(`{"tr":"9H"}`), Visible: false,
	})
	if err != nil || upd.Visible {
		t.Fatalf("update = %+v, %v", upd, err)
	}
	if err := f.store.ReorderServices(f.ctx, s.ID, []uuid.UUID{custom.Uuid, catSvc.Uuid}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	rows, err := f.q.ListDealerShowcaseServices(f.ctx, s.ID)
	if err != nil || len(rows) != 2 || rows[0].Uuid != custom.Uuid {
		t.Fatalf("services after reorder = %+v, %v", rows, err)
	}
	if err := f.store.ReorderServices(f.ctx, s.ID, []uuid.UUID{custom.Uuid, uuid.New()}); !errors.Is(err, ErrReorderMismatch) {
		t.Fatalf("foreign uuid reorder err = %v", err)
	}
	if n, err := f.q.DeleteDealerShowcaseService(f.ctx, db.DeleteDealerShowcaseServiceParams{Uuid: custom.Uuid, ShowcaseID: s.ID}); err != nil || n != 1 {
		t.Fatalf("delete = %d, %v", n, err)
	}
}

func TestShowcaseSchemaConstraints(t *testing.T) {
	f := newFixture(t)
	d := f.dealer(t, "schema", "Ankara")

	t.Run("owner is a dealer or distributor of the brand", func(t *testing.T) {
		center := upsertParams(db.Organization{ID: f.centerID, BrandID: f.brandID})
		f.expectConstraint(t, "center showcase", "23514", "", func(q *db.Queries) error {
			_, err := q.UpsertDealerShowcase(f.ctx, center)
			return err
		})
		glorian, err := f.q.GetBrandBySlug(f.ctx, "glorian")
		if err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "brand mismatch", "23514", "", func(q *db.Queries) error {
			_, err := q.UpsertDealerShowcase(f.ctx, upsertParams(db.Organization{ID: d.ID, BrandID: glorian.ID}))
			return err
		})
		// A distributor may have a showcase; one row per organization.
		ds := f.showcase(t, f.dist)
		again := f.showcase(t, f.dist)
		if again.ID != ds.ID {
			t.Fatalf("upsert created a second row: %d != %d", again.ID, ds.ID)
		}
	})

	t.Run("content, links and keywords", func(t *testing.T) {
		cases := []struct {
			name, constraint string
			mut              func(p *db.UpsertDealerShowcaseParams)
		}{
			{"unknown locale", "chk_dealer_showcases_content", func(p *db.UpsertDealerShowcaseParams) { p.Content = []byte(`{"xx":{"headline":"a"}}`) }},
			{"unknown field", "chk_dealer_showcases_content", func(p *db.UpsertDealerShowcaseParams) { p.Content = []byte(`{"tr":{"price":"100"}}`) }},
			{"http link", "chk_dealer_showcases_social_links", func(p *db.UpsertDealerShowcaseParams) {
				p.SocialLinks = []byte(`{"instagram":"http://instagram.com/x"}`)
			}},
			{"javascript link", "chk_dealer_showcases_social_links", func(p *db.UpsertDealerShowcaseParams) {
				p.SocialLinks = []byte(`{"website":"javascript:alert(1)"}`)
			}},
			{"unknown network", "chk_dealer_showcases_social_links", func(p *db.UpsertDealerShowcaseParams) {
				p.SocialLinks = []byte(`{"twitter":"https://x.com/a"}`)
			}},
			{"working hours array", "chk_dealer_showcases_working_hours", func(p *db.UpsertDealerShowcaseParams) { p.WorkingHours = []byte(`[]`) }},
			{"blank keyword", "chk_dealer_showcases_seo_keywords", func(p *db.UpsertDealerShowcaseParams) { p.SeoKeywords = []string{"ppf", " "} }},
		}
		for _, c := range cases {
			p := upsertParams(d)
			c.mut(&p)
			f.expectConstraint(t, c.name, "23514", c.constraint, func(q *db.Queries) error {
				_, err := q.UpsertDealerShowcase(f.ctx, p)
				return err
			})
		}
	})

	s := f.showcase(t, d)

	t.Run("status guards", func(t *testing.T) {
		f.expectConstraint(t, "published without snapshot", "23514", "chk_dealer_showcases_published", func(q *db.Queries) error {
			// Published needs a snapshot: only PublishDealerShowcase sets it.
			_, err := q.SubmitDealerShowcase(f.ctx, db.SubmitDealerShowcaseParams{ID: s.ID, FromStatus: model.StatusDraft})
			if err != nil {
				return err
			}
			_, err = f.tx.Exec(f.ctx, `UPDATE dealer_showcases SET status = 'published' WHERE id = $1`, s.ID)
			return err
		})
		if _, err := f.store.Submit(f.ctx, s.ID, model.StatusDraft, nil); err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "reject without note", "23514", "chk_dealer_showcases_rejected", func(q *db.Queries) error {
			_, err := q.RejectDealerShowcase(f.ctx, db.RejectDealerShowcaseParams{
				ID: s.ID, ReviewerUserID: pgtype.Int8{Int64: f.user.ID, Valid: true}, ReviewNote: pgtype.Text{String: " ", Valid: true},
			})
			return err
		})
	})

	t.Run("google rating", func(t *testing.T) {
		got, err := f.q.SetDealerShowcaseGoogleRating(f.ctx, db.SetDealerShowcaseGoogleRatingParams{
			ID: s.ID, GoogleRating: numeric(t, "4.7"), GoogleReviewCount: pgtype.Int4{Int32: 128, Valid: true},
			GoogleRatingSource: pgtype.Text{String: model.RatingSourceManual, Valid: true},
		})
		if err != nil || !got.GoogleRatingUpdatedAt.Valid || got.GoogleReviewCount.Int32 != 128 {
			t.Fatalf("rating = %+v, %v", got, err)
		}
		f.expectConstraint(t, "rating without source", "23514", "chk_dealer_showcases_rating_set", func(q *db.Queries) error {
			_, err := q.SetDealerShowcaseGoogleRating(f.ctx, db.SetDealerShowcaseGoogleRatingParams{ID: s.ID, GoogleRating: numeric(t, "4.0")})
			return err
		})
		f.expectConstraint(t, "rating above 5", "23514", "chk_dealer_showcases_rating", func(q *db.Queries) error {
			_, err := q.SetDealerShowcaseGoogleRating(f.ctx, db.SetDealerShowcaseGoogleRatingParams{
				ID: s.ID, GoogleRating: numeric(t, "5.5"), GoogleRatingSource: pgtype.Text{String: "places", Valid: true},
			})
			return err
		})
		cleared, err := f.q.SetDealerShowcaseGoogleRating(f.ctx, db.SetDealerShowcaseGoogleRatingParams{ID: s.ID})
		if err != nil || cleared.GoogleRating.Valid || cleared.GoogleRatingUpdatedAt.Valid {
			t.Fatalf("clear = %+v, %v", cleared, err)
		}
	})

	t.Run("photo guards", func(t *testing.T) {
		bad := photoParams(s.ID, 1)
		bad.Mime = "image/gif"
		f.expectConstraint(t, "gif", "23514", "chk_dealer_showcase_photos_mime", func(q *db.Queries) error {
			_, err := q.InsertDealerShowcasePhoto(f.ctx, bad)
			return err
		})
		bad = photoParams(s.ID, 1)
		bad.Sha256 = strings.Repeat("Z", 64)
		f.expectConstraint(t, "sha256", "23514", "chk_dealer_showcase_photos_sha256", func(q *db.Queries) error {
			_, err := q.InsertDealerShowcasePhoto(f.ctx, bad)
			return err
		})
		if _, err := f.q.InsertDealerShowcasePhoto(f.ctx, photoParams(s.ID, 7)); err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "same image twice", "23505", "uq_dealer_showcase_photos_sha256", func(q *db.Queries) error {
			_, err := q.InsertDealerShowcasePhoto(f.ctx, photoParams(s.ID, 7))
			return err
		})
	})

	t.Run("public read hides drafts and inactive organizations", func(t *testing.T) {
		other := f.dealer(t, "public", "Bursa")
		os := f.showcase(t, other)
		read := func() error {
			_, err := f.q.GetPublishedDealerShowcase(f.ctx, db.GetPublishedDealerShowcaseParams{Slug: other.Slug, BrandID: f.brandID})
			return err
		}
		if err := read(); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("draft public read err = %v", err)
		}
		if _, err := f.store.Publish(f.ctx, os.ID, model.StatusDraft, []byte(`{"content":{}}`), nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := read(); err != nil {
			t.Fatalf("published read: %v", err)
		}
		f.exec(t, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, other.ID)
		if err := read(); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("suspended org read err = %v", err)
		}
	})
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatal(err)
	}
	return n
}
