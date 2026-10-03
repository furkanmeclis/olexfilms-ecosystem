package migrator

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupRun(t *testing.T, pool *pgxpool.Pool, runID int64) {
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM migration_runs WHERE id = $1", runID)
	})
}

// A run with an empty step list still leaves a finished migration_runs row
// and opens no legacy source.
func TestRunEmptyStepsCreatesRun(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	r := &Runner{
		Pool: pool,
		Open: func(context.Context, string) (source.LegacySource, error) {
			return nil, errors.New("no source may be opened without steps")
		},
	}
	rep, err := r.Run(ctx, Options{Profile: "olex", Mode: ModeFull})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	cleanupRun(t, pool, rep.RunID)
	if rep.Status != StatusSucceeded || len(rep.Steps) != 0 {
		t.Fatalf("report = %+v", rep)
	}

	var (
		profile, mode, status string
		stepNull, finished    bool
		counts                string
	)
	err = pool.QueryRow(ctx, `SELECT profile, mode, status, step IS NULL, finished_at IS NOT NULL, counts::text
		FROM migration_runs WHERE id = $1`, rep.RunID).Scan(&profile, &mode, &status, &stepNull, &finished, &counts)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if profile != "olex" || mode != "full" || status != StatusSucceeded || !stepNull || !finished || counts != "{}" {
		t.Errorf("run row = %s %s %s step_null=%v finished=%v counts=%s", profile, mode, status, stepNull, finished, counts)
	}
}

// The mapper hands out one uuid per legacy key: the second call returns it
// with created=false, a new checksum reports changed.
func TestMapperUpsertIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	m := NewMapper(db.New(tx))
	key := Key{System: "test", Table: "customers", ID: uuid.NewString(), TargetTable: "customers"}

	first, err := m.Upsert(ctx, key, "c1")
	if err != nil || !first.Created || first.Changed || first.UUID == uuid.Nil {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := m.Upsert(ctx, key, "c1")
	if err != nil || second.Created || second.Changed || second.UUID != first.UUID {
		t.Fatalf("second = %+v, %v (want created=false, same uuid)", second, err)
	}
	third, err := m.Upsert(ctx, key, "c2")
	if err != nil || third.Created || !third.Changed || third.UUID != first.UUID {
		t.Fatalf("third = %+v, %v (want changed=true)", third, err)
	}
	fourth, err := m.Upsert(ctx, key, "c2")
	if err != nil || fourth.Changed {
		t.Fatalf("fourth = %+v, %v (checksum stored)", fourth, err)
	}
	if got, ok, err := m.Lookup(ctx, key.System, key.Table, key.ID); err != nil || !ok || got != first.UUID {
		t.Fatalf("lookup = %v %v %v", got, ok, err)
	}
	if _, ok, err := m.Lookup(ctx, key.System, key.Table, "missing"); err != nil || ok {
		t.Fatalf("lookup missing = %v %v", ok, err)
	}
	other := key
	other.TargetTable = "users"
	if _, err := m.Upsert(ctx, other, "c2"); err == nil {
		t.Fatal("remapping a key to another target table must fail")
	}
}

// mapCustomersStep maps every fixture hub customer through the mapper.
type mapCustomersStep struct{ system string }

func (s mapCustomersStep) Name() string { return "map_customers" }

func (s mapCustomersStep) Run(ctx context.Context, src Sources, _ *Target, m *Mapper) (StepResult, error) {
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	rows, err := hub.Query(ctx, "SELECT id, name, phone FROM customers WHERE id > ? ORDER BY id", 0)
	if err != nil {
		return StepResult{}, err
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int64{}
	for rows.Next() {
		var id int64
		var name, phone string
		if err := rows.Scan(&id, &name, &phone); err != nil {
			return StepResult{}, err
		}
		res, err := m.Upsert(ctx, Key{System: s.system, Table: "customers", ID: strconv.FormatInt(id, 10), TargetTable: "customers"},
			Checksum(name, phone))
		if err != nil {
			return StepResult{}, err
		}
		counts["read"]++
		if res.Created {
			counts["created"]++
		}
	}
	return StepResult{Counts: counts}, rows.Err()
}

type failingStep struct{}

func (failingStep) Name() string { return "boom" }
func (failingStep) Run(context.Context, Sources, *Target, *Mapper) (StepResult, error) {
	return StepResult{}, errors.New("boom")
}

// End to end over the PG fixture: a real step reads legacy_hub through the
// read-only source; a rerun creates nothing, a dry run persists nothing, a
// failing step marks the run failed.
func TestRunWithFixtureStep(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	legacyfixture.LoadAndHold(t, pool)
	system := "t" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM migration_map WHERE source_system = $1", system)
	})
	want := legacyfixture.ExpectedRowCounts["legacy_hub.customers"]

	profiles := map[string]Profile{
		"fx": {Name: "fx", Enabled: true, Sources: []string{SourceHub, SourceWH},
			Steps: func() []Step { return []Step{mapCustomersStep{system: system}, failingStep{}} }},
	}
	r := &Runner{
		Pool:     pool,
		Profiles: profiles,
		Open: func(_ context.Context, name string) (source.LegacySource, error) {
			return source.NewPostgres(name, pool, source.FixtureSchemas[name])
		},
	}
	run := func(opts Options) RunReport {
		t.Helper()
		opts.Profile = "fx"
		rep, err := r.Run(ctx, opts)
		if rep.RunID != 0 {
			cleanupRun(t, pool, rep.RunID)
		}
		if err != nil {
			t.Fatalf("run %+v: %v", opts, err)
		}
		return rep
	}
	mapped := func() int64 {
		var n int64
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM migration_map WHERE source_system = $1", system).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	dry := run(Options{Steps: []string{"map_customers"}, DryRun: true})
	if got := dry.Steps[0].Counts["created"]; got != want {
		t.Errorf("dry run created = %d, want %d", got, want)
	}
	if n := mapped(); n != 0 {
		t.Fatalf("dry run persisted %d mappings", n)
	}

	first := run(Options{Steps: []string{"map_customers"}})
	if got := first.Steps[0].Counts["created"]; got != want {
		t.Errorf("first run created = %d, want %d", got, want)
	}
	second := run(Options{Steps: []string{"map_customers"}, Mode: ModeDelta})
	if got := second.Steps[0].Counts["created"]; got != 0 {
		t.Errorf("rerun created = %d, want 0", got)
	}
	if n := mapped(); n != want {
		t.Errorf("mappings = %d, want %d", n, want)
	}

	var steps int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM migration_runs WHERE parent_id = $1 AND status = 'succeeded'", first.RunID).Scan(&steps); err != nil {
		t.Fatal(err)
	}
	if steps != 1 {
		t.Errorf("step rows = %d, want 1", steps)
	}

	rep, err := r.Run(ctx, Options{Profile: "fx"})
	cleanupRun(t, pool, rep.RunID)
	if err == nil || rep.Status != StatusFailed {
		t.Fatalf("failing run = %+v, %v", rep, err)
	}
	var status, errText string
	if err := pool.QueryRow(ctx, "SELECT status, COALESCE(error, '') FROM migration_runs WHERE id = $1", rep.RunID).Scan(&status, &errText); err != nil {
		t.Fatal(err)
	}
	if status != StatusFailed || errText == "" {
		t.Errorf("failed run row = %s %q", status, errText)
	}
}
