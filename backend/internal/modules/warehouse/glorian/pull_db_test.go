package glorian_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-268: the catalog and dealer pull against the fake hub and the
// migrated database (CI sets TEST_DATABASE_URL). Everything runs in one
// rolled-back transaction. Names and SKUs carry a per-test suffix so rows
// committed by other tests in the glorian brand never collide.

type pullFixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	srv     *fake.Server
	box     *crypto.SecretBox
	glorian db.Organization
	olex    db.Organization
	conn    db.IntegrationConnection
	suffix  string
}

func newPullFixture(t *testing.T, active bool) *pullFixture {
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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	f := &pullFixture{
		ctx: ctx, tx: tx, q: db.New(tx), srv: fake.New(t), box: box,
		suffix: fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000),
	}
	f.glorian = centerOf(t, ctx, f.q, "glorian")
	f.olex = centerOf(t, ctx, f.q, "olex")
	f.conn, err = glorian.NewStore(f.q, box).Create(ctx, glorian.ConnectionInput{
		OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, Key: glorian.ConnectionKey,
		BaseURL: f.srv.URL, APIKey: fake.APIKey, Active: active,
	})
	if err != nil {
		t.Fatalf("connection: %v", err)
	}
	f.srv.SetFixture(fake.FixtureCategories, []fake.Row{
		f.category("1", "PPF", "2026-09-01T10:00:00+00:00", true, nil),
		f.category("2", "Cam Filmi", "2026-09-02T10:00:00+00:00", true, nil),
	})
	f.srv.SetFixture(fake.FixtureProducts, []fake.Row{
		f.product("10", "HYDRA-150", "HYDRA 150", "1", "2026-09-01T11:00:00+00:00"),
		f.product("11", "HYDRA-200", "HYDRA 200", "1", "2026-09-02T11:00:00+00:00"),
		f.product("12", "TINT-35", "Tint 35", "2", "2026-09-04T11:00:00+00:00"),
	})
	f.srv.SetFixture(fake.FixtureDealers, []fake.Row{
		f.dealer("1", "Parazit Dizayn", "+905550000001", true, "2026-09-01T09:00:00+00:00"),
		f.dealer("2", "Kapalı Bayi", "0555 000 00 02", false, "2026-09-05T09:00:00+00:00"),
	})
	return f
}

func centerOf(t *testing.T, ctx context.Context, q *db.Queries, slug string) db.Organization {
	t.Helper()
	brand, err := q.GetBrandBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("%s brand: %v", slug, err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("%s center: %v", slug, err)
	}
	return center
}

func (f *pullFixture) name(s string) string { return s + " " + f.suffix }

func (f *pullFixture) category(id, name, updated string, active bool, deleted any) fake.Row {
	return fake.Row{"id": id, "name": f.name(name), "available_parts": []any{"hood"}, "is_active": active, "deleted_at": deleted, "updated_at": updated}
}

func (f *pullFixture) product(id, sku, name, cat, updated string) fake.Row {
	return fake.Row{
		"id": id, "sku": sku + "-" + f.suffix, "name": f.name(name), "description": nil, "category_id": cat,
		"warranty_duration": 24, "micron_thickness": 150, "is_active": true, "image_url": nil,
		"deleted_at": nil, "updated_at": updated,
	}
}

func (f *pullFixture) dealer(id, name, phone string, active bool, updated string) fake.Row {
	return fake.Row{
		"id": id, "dealer_code": nil, "name": f.name(name), "email": nil, "phone": phone, "country": "TR",
		"is_active": active, "social": fake.Row{}, "created_at": updated, "updated_at": updated,
	}
}

func (f *pullFixture) puller() *glorian.Puller {
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	return glorian.NewPuller(f.q, f.box, factory, nil, nil)
}

func (f *pullFixture) syncedProducts(t *testing.T) []db.Product {
	t.Helper()
	rows, err := f.tx.Query(f.ctx, `SELECT id FROM products WHERE connection_id = $1 ORDER BY external_id`, f.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	out := make([]db.Product, 0, len(ids))
	for _, id := range ids {
		p, err := f.q.GetProduct(f.ctx, db.GetProductParams{ID: id, BrandID: f.glorian.BrandID})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func (f *pullFixture) runs(t *testing.T) []db.IntegrationSyncRun {
	t.Helper()
	runs, err := f.q.ListIntegrationSyncRuns(f.ctx, db.ListIntegrationSyncRunsParams{ConnectionID: f.conn.ID, RowLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

func runCounts(t *testing.T, run db.IntegrationSyncRun) glorian.PullCounts {
	t.Helper()
	var c glorian.PullCounts
	if err := json.Unmarshal(run.Counts, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// latestRun returns the newest run of kind.
func latestRun(t *testing.T, runs []db.IntegrationSyncRun, kind string) db.IntegrationSyncRun {
	t.Helper()
	for _, r := range runs {
		if r.Kind == kind {
			return r
		}
	}
	t.Fatalf("no %s run in %d runs", kind, len(runs))
	return db.IntegrationSyncRun{}
}

func TestPullCreatesThenIsIdempotent(t *testing.T) {
	f := newPullFixture(t, true)
	p := f.puller()

	// 1. First pull creates every record.
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	products := f.syncedProducts(t)
	if len(products) != 3 {
		t.Fatalf("synced products = %d, want 3", len(products))
	}
	for _, pr := range products {
		if pr.BrandID != f.glorian.BrandID || pr.OrganizationID != f.glorian.ID {
			t.Fatalf("product %s scope = org %d brand %d", pr.Sku, pr.OrganizationID, pr.BrandID)
		}
		if len(pr.LockedFields) == 0 || !pr.ExternalID.Valid {
			t.Fatalf("product %s not marked synced: %+v", pr.Sku, pr)
		}
		if !pr.WarrantyDurationMonths.Valid || pr.WarrantyDurationMonths.Int32 != 24 || !pr.MicronThickness.Valid {
			t.Fatalf("product %s fields = %+v", pr.Sku, pr)
		}
	}
	ppf, err := f.q.GetProductCategoryByName(f.ctx, db.GetProductCategoryByNameParams{BrandID: f.glorian.BrandID, Name: f.name("PPF")})
	if err != nil {
		t.Fatalf("category PPF: %v", err)
	}
	if products[0].CategoryID != ppf.ID {
		t.Fatalf("HYDRA-150 category = %d, want %d", products[0].CategoryID, ppf.ID)
	}
	parties, err := f.q.ListIntegrationExternalParties(f.ctx, f.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parties) != 2 {
		t.Fatalf("parties = %d, want 2", len(parties))
	}
	for _, party := range parties {
		if party.RemoteID == "2" && (party.Active || party.PhoneE164.String != "+905550000002") {
			t.Fatalf("dealer 2 = %+v; want inactive, phone normalized to E.164", party)
		}
	}
	runs := f.runs(t)
	if len(runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(runs))
	}
	for kind, want := range map[string]int{glorian.KindPullCategories: 2, glorian.KindPullProducts: 3, glorian.KindPullDealers: 2} {
		run := latestRun(t, runs, kind)
		c := runCounts(t, run)
		if run.Status != glorian.RunSucceeded || c.Fetched != want || c.Created != want || !run.Watermark.Valid {
			t.Fatalf("%s run = %s %+v watermark %v", kind, run.Status, c, run.Watermark)
		}
	}
	productsWM := latestRun(t, runs, glorian.KindPullProducts).Watermark.Time

	// 2. Second pull: updated_since = watermark - 90 s, so the newest row
	// comes again and is not duplicated.
	before := len(f.srv.RequestsTo("GET", "/products"))
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	reqs := f.srv.RequestsTo("GET", "/products")[before:]
	wantSince := productsWM.Add(-glorian.PullOverlap).UTC().Format(time.RFC3339)
	var sawSince bool
	for _, r := range reqs {
		if r.Query.Get("updated_since") == wantSince {
			sawSince = true
		}
	}
	if !sawSince {
		t.Fatalf("no products request with updated_since=%s: %+v", wantSince, reqs)
	}
	if got := len(f.syncedProducts(t)); got != 3 {
		t.Fatalf("after second run products = %d, want 3", got)
	}
	run := latestRun(t, f.runs(t), glorian.KindPullProducts)
	if c := runCounts(t, run); c.Fetched != 1 || c.Created != 0 || c.Unchanged != 1 {
		t.Fatalf("second products run counts = %+v; want the overlapping row only, unchanged", c)
	}
	if !run.Watermark.Time.Equal(productsWM) {
		t.Fatalf("watermark moved: %v -> %v", productsWM, run.Watermark.Time)
	}
}

func TestPullReflectsRemoteUpdates(t *testing.T) {
	f := newPullFixture(t, true)
	p := f.puller()
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	oldCat, err := f.q.GetProductCategoryByName(f.ctx, db.GetProductCategoryByNameParams{BrandID: f.glorian.BrandID, Name: f.name("Cam Filmi")})
	if err != nil {
		t.Fatal(err)
	}

	// Rename a category, change a product, deactivate a dealer.
	f.srv.SetFixture(fake.FixtureCategories, []fake.Row{
		f.category("1", "PPF", "2026-09-01T10:00:00+00:00", true, nil),
		f.category("2", "Cam Filmi Pro", "2026-09-10T10:00:00+00:00", true, nil),
	})
	updated := f.product("12", "TINT-35", "Tint 35 Yeni", "2", "2026-09-10T11:00:00+00:00")
	updated["is_active"] = false
	f.srv.SetFixture(fake.FixtureProducts, []fake.Row{
		f.product("10", "HYDRA-150", "HYDRA 150", "1", "2026-09-01T11:00:00+00:00"),
		f.product("11", "HYDRA-200", "HYDRA 200", "1", "2026-09-02T11:00:00+00:00"),
		updated,
	})
	f.srv.SetFixture(fake.FixtureDealers, []fake.Row{
		f.dealer("1", "Parazit Dizayn 2", "+905550000001", false, "2026-09-10T09:00:00+00:00"),
		f.dealer("2", "Kapalı Bayi", "0555 000 00 02", false, "2026-09-05T09:00:00+00:00"),
	})
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}

	renamed, err := f.q.GetProductCategory(f.ctx, db.GetProductCategoryParams{ID: oldCat.ID, BrandID: f.glorian.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != f.name("Cam Filmi Pro") {
		t.Fatalf("category rename not applied in place: %q", renamed.Name)
	}
	prod, err := f.q.GetProductByConnectionExternalID(f.ctx, db.GetProductByConnectionExternalIDParams{
		ConnectionID: pgtype.Int8{Int64: f.conn.ID, Valid: true}, ExternalID: pgtype.Text{String: "12", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prod.Name != f.name("Tint 35 Yeni") || prod.Active || prod.CategoryID != oldCat.ID {
		t.Fatalf("product 12 = %+v", prod)
	}
	party, err := f.q.GetIntegrationExternalPartyByRemoteID(f.ctx, db.GetIntegrationExternalPartyByRemoteIDParams{ConnectionID: f.conn.ID, RemoteID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if party.Name != f.name("Parazit Dizayn 2") || party.Active {
		t.Fatalf("dealer 1 = %+v", party)
	}
	runs := f.runs(t)
	if c := runCounts(t, latestRun(t, runs, glorian.KindPullProducts)); c.Updated != 1 {
		t.Fatalf("products counts = %+v, want 1 updated", c)
	}
	if c := runCounts(t, latestRun(t, runs, glorian.KindPullCategories)); c.Updated != 1 || c.Created != 0 {
		t.Fatalf("categories counts = %+v, want 1 updated (rename), none created", c)
	}
}

func TestLockedFieldsRejectPanelEdits(t *testing.T) {
	f := newPullFixture(t, true)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	prod := f.syncedProducts(t)[0]
	svc := catalogusecase.New(f.q, nil)
	center := orgctx.Scope{InternalID: f.glorian.ID, OrgType: "center", BrandID: f.glorian.BrandID}

	_, err := svc.UpdateProduct(f.ctx, center, prod.Uuid, model.ProductInput{Name: ptr("Panelden")})
	var locked *catalogusecase.LockedError
	if !errors.As(err, &locked) || len(locked.Fields) != 1 || locked.Fields[0] != model.FieldName {
		t.Fatalf("update locked name: err = %v, want LockedError{name}", err)
	}
	if err := svc.DeleteProduct(f.ctx, center, prod.Uuid); !errors.As(err, &locked) {
		t.Fatalf("delete synced product: err = %v, want LockedError", err)
	}
	// Local fields stay editable; resending the synced value is no change.
	got, err := svc.UpdateProduct(f.ctx, center, prod.Uuid, model.ProductInput{
		UsesFixedBarcode: ptr(true), Name: ptr(prod.Name),
	})
	if err != nil || !got.UsesFixedBarcode {
		t.Fatalf("update local field: %+v, %v", got, err)
	}
	// Bulk deactivate skips the locked product.
	res, err := svc.SetProductsActive(f.ctx, center, []uuid.UUID{prod.Uuid}, false)
	if err != nil || res.Updated != 0 {
		t.Fatalf("bulk deactivate: %+v, %v", res, err)
	}

	cat, err := f.q.GetProductCategory(f.ctx, db.GetProductCategoryParams{ID: prod.CategoryID, BrandID: f.glorian.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateCategory(f.ctx, center, cat.Uuid, model.CategoryInput{Name: ptr("Panel Adı")}); !errors.As(err, &locked) {
		t.Fatalf("rename synced category: err = %v, want LockedError", err)
	}
	if _, err := svc.UpdateCategory(f.ctx, center, cat.Uuid, model.CategoryInput{Sort: ptr(int32(7))}); err != nil {
		t.Fatalf("category sort is local: %v", err)
	}
}

func TestPullSkipsInactiveConnection(t *testing.T) {
	f := newPullFixture(t, false)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := len(f.srv.Requests()); n != 0 {
		t.Fatalf("inactive connection made %d hub requests", n)
	}
	if runs := f.runs(t); len(runs) != 0 {
		t.Fatalf("inactive connection wrote %d sync runs", len(runs))
	}
}

// K1: an Olex user never sees the pulled glorian products.
func TestOlexCatalogHidesGlorianProducts(t *testing.T) {
	f := newPullFixture(t, true)
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	svc := catalogusecase.New(f.q, nil)
	filter := model.ProductFilter{Q: f.suffix, Limit: 50}
	olex := orgctx.Scope{InternalID: f.olex.ID, OrgType: "center", BrandID: f.olex.BrandID}
	if items, total, err := svc.ListProducts(f.ctx, olex, filter); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("olex list = %d/%d, %v; want none", len(items), total, err)
	}
	glorianScope := orgctx.Scope{InternalID: f.glorian.ID, OrgType: "center", BrandID: f.glorian.BrandID}
	if items, total, err := svc.ListProducts(f.ctx, glorianScope, filter); err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("glorian list = %d/%d, %v; want 3", len(items), total, err)
	}
	prod := f.syncedProducts(t)[0]
	if _, err := svc.GetProduct(f.ctx, olex, prod.Uuid); !errors.Is(err, catalogusecase.ErrNotFound) {
		t.Fatalf("olex get glorian product: err = %v, want ErrNotFound", err)
	}
}

func ptr[T any](v T) *T { return &v }
