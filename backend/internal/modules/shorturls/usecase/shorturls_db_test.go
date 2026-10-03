package usecase

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-249: the service against the migrated database (CI sets
// TEST_DATABASE_URL). Everything runs in one rolled-back transaction.

func dbFixture(t *testing.T) (context.Context, pgx.Tx, *db.Queries, int64) {
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
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	return ctx, tx, q, brand.ID
}

func TestDBCreateResolveCountsHits(t *testing.T) {
	ctx, _, q, brandID := dbFixture(t)
	svc := New(q)
	out, err := svc.Create(ctx, CreateInput{BrandID: brandID, Target: "/garanti/AbCdEfGh?lang=tr"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		res, err := svc.Resolve(ctx, brandID, out.Token)
		if err != nil {
			t.Fatal(err)
		}
		if res.TargetPath != "/garanti/AbCdEfGh?lang=tr" {
			t.Fatalf("target %q", res.TargetPath)
		}
	}
	stats, err := q.GetShortURLStats(ctx, out.Token)
	if err != nil {
		t.Fatal(err)
	}
	if stats.HitCount != 2 || !stats.LastHitAt.Valid {
		t.Fatalf("stats %+v", stats)
	}
	if _, err := svc.Resolve(ctx, brandID+1_000_000, out.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other brand: %v", err)
	}
}

func TestDBExpiredAndUnknown(t *testing.T) {
	ctx, _, q, brandID := dbFixture(t)
	now := time.Now()
	svc := New(q).WithClock(func() time.Time { return now })
	out, err := svc.Create(ctx, CreateInput{BrandID: brandID, Target: "/portal", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := svc.Resolve(ctx, brandID, out.Token); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
	if _, err := svc.Resolve(ctx, brandID, "NoSuchTok1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestDBLegacyTokenResolves(t *testing.T) {
	ctx, tx, q, brandID := dbFixture(t)
	// The migrator (TEC-263) keeps the old hub's 8 character token.
	if _, err := tx.Exec(ctx, `INSERT INTO short_urls (brand_id, token, target_path, legacy_target_url)
		VALUES ($1, 'aZ3kP9qX', '/portal', 'https://olexfilms.app/_/aZ3kP9qX')`, brandID); err != nil {
		t.Fatal(err)
	}
	res, err := New(q).Resolve(ctx, brandID, "aZ3kP9qX")
	if err != nil || res.TargetPath != "/portal" {
		t.Fatalf("res %+v err %v", res, err)
	}
	// Tokens are case-sensitive.
	if _, err := New(q).Resolve(ctx, brandID, "AZ3KP9QX"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("case: %v", err)
	}
}

func TestDBCheckRejectsExternalTarget(t *testing.T) {
	ctx, tx, _, brandID := dbFixture(t)
	for _, target := range []string{"//evil.example/portal", "https://evil.example", "/\\evil.example"} {
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = sp.Exec(ctx, `INSERT INTO short_urls (brand_id, token, target_path) VALUES ($1, 'Chk12345', $2)`, brandID, target)
		_ = sp.Rollback(ctx)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23514" {
			t.Fatalf("%q: %v", target, err)
		}
	}
}
