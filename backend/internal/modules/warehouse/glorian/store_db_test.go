package glorian

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-266: repository tests against the migrated database (CI sets
// TEST_DATABASE_URL). Everything runs in one rolled-back transaction; a
// statement expected to fail runs in its own savepoint.

type storeFixture struct {
	ctx    context.Context
	tx     pgx.Tx
	q      *db.Queries
	center db.Organization
	olex   db.Organization
}

func newStoreFixture(t *testing.T) *storeFixture {
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
	q := db.New(tx)
	return &storeFixture{ctx: ctx, tx: tx, q: q, center: brandCenterOrg(t, ctx, q, "glorian"), olex: brandCenterOrg(t, ctx, q, "olex")}
}

func brandCenterOrg(t *testing.T, ctx context.Context, q *db.Queries, slug string) db.Organization {
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

// savepoint runs fn in a savepoint that is always rolled back.
func (f *storeFixture) savepoint(t *testing.T, fn func(q *db.Queries) error) error {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	return fn(db.New(sp))
}

func pgCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

func TestStoreConnectionCRUD(t *testing.T) {
	f := newStoreFixture(t)
	s := NewStore(f.q, testBox(t))

	wh, err := f.q.CreateWarehouse(f.ctx, db.CreateWarehouseParams{
		OrganizationID: f.center.ID, Code: "GLR266", Name: "Glorian depo", Active: true,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}

	created, err := s.Create(f.ctx, ConnectionInput{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, Key: ConnectionKey,
		BaseURL: "https://hub.glorian.example", APIKey: "tec266-plain-api-key",
		DefaultWarehouseID: &wh.ID, Active: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ApiVersion != APIVersion || !created.DefaultWarehouseID.Valid || created.DefaultWarehouseID.Int64 != wh.ID {
		t.Fatalf("created = %+v", created)
	}

	// The key in the database is not plain text.
	var stored string
	if err := f.tx.QueryRow(f.ctx, `SELECT api_key_enc FROM integration_connections WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || strings.Contains(stored, "tec266-plain-api-key") {
		t.Fatalf("api_key_enc = %q; want ciphertext", stored)
	}
	var plainHits int
	if err := f.tx.QueryRow(f.ctx,
		`SELECT COUNT(*) FROM integration_connections c WHERE position('tec266-plain-api-key' IN row_to_json(c)::text) > 0`,
	).Scan(&plainHits); err != nil {
		t.Fatal(err)
	}
	if plainHits != 0 {
		t.Fatal("plain api key found in integration_connections")
	}

	// Read back through the ConnectionSource.
	conn, err := s.ConnectionByKey(f.ctx, ConnectionKey)
	if err != nil || conn.APIKey != "tec266-plain-api-key" || conn.BaseURL != "https://hub.glorian.example" || !conn.Active {
		t.Fatalf("by key = %+v, %v", conn, err)
	}
	got, err := s.Get(f.ctx, f.center.BrandID, created.Uuid)
	if err != nil || got.ID != created.ID {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := s.Get(f.ctx, f.olex.BrandID, created.Uuid); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("get from another brand = %v; want not found", err)
	}
	list, err := s.List(f.ctx, f.center.BrandID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d, %v", len(list), err)
	}

	// Update and key rotation.
	updated, err := s.Update(f.ctx, created.ID, ConnectionUpdate{BaseURL: "https://hub2.glorian.example", Active: false})
	if err != nil || updated.Active || updated.BaseUrl != "https://hub2.glorian.example" || updated.DefaultWarehouseID.Valid {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	rotated, err := s.SetAPIKey(f.ctx, created.ID, "tec266-rotated-key")
	if err != nil || rotated.ApiKeyEnc == stored || strings.Contains(rotated.ApiKeyEnc, "tec266-rotated-key") {
		t.Fatalf("rotate = %+v, %v", rotated, err)
	}
	if c, err := s.Decrypt(rotated); err != nil || c.APIKey != "tec266-rotated-key" {
		t.Fatalf("rotated decrypt = %+v, %v", c, err)
	}

	// An active connection without a key is rejected by the database.
	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.CreateIntegrationConnection(f.ctx, db.CreateIntegrationConnectionParams{
			OrganizationID: f.olex.ID, BrandID: f.olex.BrandID, Key: "nokey",
			BaseUrl: "https://x.example", Active: true, ApiVersion: "1",
		})
		return err
	})
	if pgCode(err) != "23514" {
		t.Fatalf("active without key = %v; want 23514", err)
	}

	// brand_id must be the organization's brand.
	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.CreateIntegrationConnection(f.ctx, db.CreateIntegrationConnectionParams{
			OrganizationID: f.center.ID, BrandID: f.olex.BrandID, Key: "mismatch",
			BaseUrl: "https://x.example", ApiVersion: "1",
		})
		return err
	})
	if pgCode(err) != "23514" {
		t.Fatalf("brand mismatch = %v; want 23514", err)
	}

	// Delete.
	if err := s.Delete(f.ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(f.ctx, created.ID); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("second delete = %v; want not found", err)
	}
	if _, err := s.ConnectionByKey(f.ctx, ConnectionKey); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("by key after delete = %v", err)
	}
}

func TestStoreConnectionKeyUniquePerBrand(t *testing.T) {
	f := newStoreFixture(t)
	s := NewStore(f.q, testBox(t))
	in := ConnectionInput{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, Key: ConnectionKey,
		BaseURL: "https://hub.glorian.example", APIKey: "k1",
	}
	if _, err := s.Create(f.ctx, in); err != nil {
		t.Fatalf("first: %v", err)
	}

	err := f.savepoint(t, func(q *db.Queries) error {
		_, err := NewStore(q, testBox(t)).Create(f.ctx, in)
		return err
	})
	if !errors.Is(err, ErrConnectionKeyTaken) {
		t.Fatalf("duplicate = %v; want ErrConnectionKeyTaken", err)
	}
	if status, _, _ := HTTPStatus(err); status != http.StatusConflict {
		t.Fatalf("duplicate status = %d; want 409", status)
	}

	// The same key under another brand is allowed (unique per brand).
	other := in
	other.OrganizationID, other.BrandID = f.olex.ID, f.olex.BrandID
	err = f.savepoint(t, func(q *db.Queries) error {
		sp := NewStore(q, testBox(t))
		if _, err := sp.Create(f.ctx, other); err != nil {
			return err
		}
		// Now a key-only lookup is ambiguous instead of picking one.
		if _, err := sp.ConnectionByKey(f.ctx, ConnectionKey); !errors.Is(err, ErrAmbiguousConnection) {
			t.Errorf("ambiguous lookup = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("other brand: %v", err)
	}
}

func TestExternalPartiesRemoteIDUnique(t *testing.T) {
	f := newStoreFixture(t)
	s := NewStore(f.q, testBox(t))
	conn, err := s.Create(f.ctx, ConnectionInput{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, Key: ConnectionKey,
		BaseURL: "https://hub.glorian.example", APIKey: "k1", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	arg := db.InsertIntegrationExternalPartyParams{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, ConnectionID: conn.ID,
		RemoteID: "D-266", Name: "Glorian Bayi", PhoneE164: pgtype.Text{String: "+905551266001", Valid: true}, Active: true,
	}
	first, err := f.q.InsertIntegrationExternalParty(f.ctx, arg)
	if err != nil {
		t.Fatalf("first party: %v", err)
	}

	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.InsertIntegrationExternalParty(f.ctx, arg)
		return err
	})
	if pgCode(err) != "23505" {
		t.Fatalf("duplicate remote_id = %v; want 23505", err)
	}

	// The pull upserts on (connection, remote_id).
	up, err := f.q.UpsertIntegrationExternalParty(f.ctx, db.UpsertIntegrationExternalPartyParams{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, ConnectionID: conn.ID,
		RemoteID: "D-266", Name: "Glorian Bayi 2", Active: false,
	})
	if err != nil || up.ID != first.ID || up.Name != "Glorian Bayi 2" || up.Active || up.PhoneE164.Valid {
		t.Fatalf("upsert = %+v, %v", up, err)
	}

	// The party must sit under its connection's organization and brand.
	err = f.savepoint(t, func(q *db.Queries) error {
		bad := arg
		bad.RemoteID, bad.OrganizationID, bad.BrandID = "D-267", f.olex.ID, f.olex.BrandID
		_, err := q.InsertIntegrationExternalParty(f.ctx, bad)
		return err
	})
	if pgCode(err) != "23503" {
		t.Fatalf("party of another brand = %v; want 23503", err)
	}

	// Phone is E.164.
	err = f.savepoint(t, func(q *db.Queries) error {
		bad := arg
		bad.RemoteID, bad.PhoneE164 = "D-268", pgtype.Text{String: "05551266001", Valid: true}
		_, err := q.InsertIntegrationExternalParty(f.ctx, bad)
		return err
	})
	if pgCode(err) != "23514" {
		t.Fatalf("non E.164 phone = %v; want 23514", err)
	}
}

func TestSyncRunLifecycle(t *testing.T) {
	f := newStoreFixture(t)
	conn, err := NewStore(f.q, testBox(t)).Create(f.ctx, ConnectionInput{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, Key: ConnectionKey,
		BaseURL: "https://hub.glorian.example", APIKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.q.StartIntegrationSyncRun(f.ctx, db.StartIntegrationSyncRunParams{
		OrganizationID: f.center.ID, BrandID: f.center.BrandID, ConnectionID: conn.ID, Kind: "pull_products",
	})
	if err != nil || run.Status != "running" || run.FinishedAt.Valid {
		t.Fatalf("start = %+v, %v", run, err)
	}
	done, err := f.q.FinishIntegrationSyncRun(f.ctx, db.FinishIntegrationSyncRunParams{
		ID: run.ID, Status: "succeeded", Counts: []byte(`{"fetched":3}`),
		Watermark: pgtype.Timestamptz{Time: run.StartedAt.Time, Valid: true},
	})
	if err != nil || done.Status != "succeeded" || !done.FinishedAt.Valid {
		t.Fatalf("finish = %+v, %v", done, err)
	}
	last, err := f.q.LastSucceededIntegrationSyncRun(f.ctx, db.LastSucceededIntegrationSyncRunParams{
		ConnectionID: conn.ID, Kind: "pull_products",
	})
	if err != nil || last.ID != run.ID || !last.Watermark.Valid {
		t.Fatalf("last succeeded = %+v, %v", last, err)
	}

	err = f.savepoint(t, func(q *db.Queries) error {
		_, err := q.StartIntegrationSyncRun(f.ctx, db.StartIntegrationSyncRunParams{
			OrganizationID: f.center.ID, BrandID: f.center.BrandID, ConnectionID: conn.ID, Kind: "pull_everything",
		})
		return err
	})
	if pgCode(err) != "23514" {
		t.Fatalf("unknown kind = %v; want 23514", err)
	}
}
