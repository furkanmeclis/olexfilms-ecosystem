package sysconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// The HTTP integration test writes forecast_min_days and smtp.*; this one
// writes contract_grace_days so the two never race on a shared database.

type dbtest struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *Service
	mr    *miniredis.Miniredis
	cache *RedisCache
}

func newDBTest(t *testing.T) *dbtest {
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
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewRedisCache(rdb, "test", nil)
	d := &dbtest{t: t, pool: pool, svc: New(db.New(pool), cache), mr: mr, cache: cache}
	d.reset(KeyContractGraceDays)
	t.Cleanup(func() { d.reset(KeyContractGraceDays) })
	return d
}

func (d *dbtest) reset(keys ...string) {
	for _, k := range keys {
		_, _ = d.pool.Exec(context.Background(), "DELETE FROM system_settings WHERE key = $1", k)
	}
}

// Acceptance: contract_grace_days defaults to 0 (K23) without a row.
func TestDBContractGraceDefaultZero(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()
	if got := d.svc.ContractGraceDays(ctx); got != 0 {
		t.Fatalf("contract_grace_days default = %d, want 0", got)
	}
	e, err := d.svc.Get(ctx, KeyContractGraceDays)
	if err != nil || !e.IsDefault || string(e.Value) != "0" {
		t.Fatalf("Get = %+v, %v; want default 0", e, err)
	}
}

// Acceptance: write/read round trip, schema rejection, cache invalidation.
func TestDBWriteReadInvalidate(t *testing.T) {
	d := newDBTest(t)
	ctx := context.Background()

	// Warm the cache, then write: the snapshot must be dropped and the next
	// read must see the new value (not the cached 0).
	if d.svc.ContractGraceDays(ctx) != 0 {
		t.Fatal("precondition: default 0")
	}
	if !d.mr.Exists(d.cache.Key()) {
		t.Fatal("snapshot not cached after read")
	}
	e, err := d.svc.Set(ctx, KeyContractGraceDays, json.RawMessage("14"), 0)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if e.IsDefault || string(e.Value) != "14" || e.SchemaVersion != SchemaVersion {
		t.Fatalf("set entry = %+v", e)
	}
	if d.mr.Exists(d.cache.Key()) {
		t.Fatal("cache not invalidated on write")
	}
	if got := d.svc.ContractGraceDays(ctx); got != 14 {
		t.Fatalf("after write = %d, want 14", got)
	}
	if !d.mr.Exists(d.cache.Key()) {
		t.Fatal("snapshot not re-cached")
	}

	// Values outside the schema are rejected and nothing changes.
	for _, raw := range []string{"-1", "1.5", `"7"`, "true", "null"} {
		_, err := d.svc.Set(ctx, KeyContractGraceDays, json.RawMessage(raw), 0)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("set %s: err = %v, want ValidationError", raw, err)
		}
	}
	if got := d.svc.ContractGraceDays(ctx); got != 14 {
		t.Fatalf("after rejected writes = %d, want 14", got)
	}
	if _, err := d.svc.Set(ctx, "no.such.key", json.RawMessage("1"), 0); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key err = %v", err)
	}

	// A stale row that no longer fits the schema falls back to the default.
	if _, err := d.pool.Exec(ctx, "UPDATE system_settings SET value = '\"bad\"'::jsonb WHERE key = $1", KeyContractGraceDays); err != nil {
		t.Fatal(err)
	}
	d.cache.Invalidate(ctx)
	if got := d.svc.ContractGraceDays(ctx); got != 0 {
		t.Fatalf("stale row = %d, want default 0", got)
	}
}

// TEC-317: a write guard refuses a well-formed value before anything is
// stored (the handler answers 422 with the rule code).
func TestSetGuardRefusesBeforeWrite(t *testing.T) {
	s := New(nil, NoCache{})
	s.SetGuard(KeyLeadsDealerApplicationEnabled, func(_ context.Context, v json.RawMessage) error {
		if string(v) == "true" {
			return &RuleError{Key: KeyLeadsDealerApplicationEnabled, Code: "LEADS_MODULE_DISABLED", Message: "closed"}
		}
		return nil
	})
	_, err := s.Set(context.Background(), KeyLeadsDealerApplicationEnabled, json.RawMessage(`true`), 0)
	var re *RuleError
	if !errors.As(err, &re) || re.Code != "LEADS_MODULE_DISABLED" {
		t.Fatalf("err = %v", err)
	}
	if d, ok := Lookup(KeyLeadsDealerApplicationEnabled); !ok || d.Default != false || d.Group != GroupLeads {
		t.Fatalf("definition = %+v", d)
	}
}
