package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
	searchregistry "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/registry"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
)

var testNow = time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)

func clock() time.Time { return testNow }

// --- fakes -------------------------------------------------------------------

type fakeRuns struct {
	last  migrator.RunSummary // any mode
	delta migrator.RunSummary
	none  bool
	err   error
}

func (f fakeRuns) lastRun(_ context.Context, mode migrator.Mode) (migrator.RunSummary, bool, error) {
	if f.err != nil {
		return migrator.RunSummary{}, false, f.err
	}
	if f.none {
		return migrator.RunSummary{}, false, nil
	}
	if mode == migrator.ModeDelta {
		return f.delta, f.delta.ID != 0, nil
	}
	return f.last, true, nil
}

func goodRuns() fakeRuns {
	delta := migrator.RunSummary{ID: 42, Mode: "delta", Status: migrator.StatusSucceeded,
		StartedAt: testNow.Add(-12 * time.Minute), FinishedAt: testNow.Add(-10 * time.Minute),
		Watermark: testNow.Add(-2 * time.Hour)}
	return fakeRuns{last: delta, delta: delta}
}

func cleanReport(context.Context) (*migrator.Report, error) {
	return &migrator.Report{Tables: make([]migrator.TableReport, 3), Expected: []migrator.ReportDiff{{}},
		Mismatches: []migrator.ReportDiff{}}, nil
}

type fakeGlorian struct {
	conns []db.IntegrationConnection
	runs  map[int64][]db.IntegrationSyncRun
}

func (f fakeGlorian) ListIntegrationConnectionsByKey(_ context.Context, key string) ([]db.IntegrationConnection, error) {
	if key != glorian.ConnectionKey {
		return nil, errors.New("unexpected key " + key)
	}
	return f.conns, nil
}

func (f fakeGlorian) ListGlorianSyncRuns(_ context.Context, arg db.ListGlorianSyncRunsParams) ([]db.IntegrationSyncRun, error) {
	if len(arg.Kinds) != 1 || arg.Kinds[0] != glorian.KindReconcile || arg.RowLimit != 1 {
		return nil, errors.New("unexpected sync run filter")
	}
	return f.runs[arg.ConnectionID], nil
}

func reconcileRun(t *testing.T, status string, finished time.Time, s glorian.ReconcileSummary) db.IntegrationSyncRun {
	t.Helper()
	counts, err := json.Marshal(glorian.ReconcileCounts{ReconcileSummary: s, Remote: 10, Local: 10})
	if err != nil {
		t.Fatal(err)
	}
	return db.IntegrationSyncRun{Uuid: uuid.New(), Kind: glorian.KindReconcile, Status: status, Counts: counts,
		FinishedAt: pgtype.Timestamptz{Time: finished, Valid: !finished.IsZero()}}
}

func activeGlorian(t *testing.T, run db.IntegrationSyncRun) fakeGlorian {
	return fakeGlorian{
		conns: []db.IntegrationConnection{{ID: 7, Uuid: uuid.New(), Key: glorian.ConnectionKey, Active: true}},
		runs:  map[int64][]db.IntegrationSyncRun{7: {run}},
	}
}

func inactiveGlorian() fakeGlorian {
	return fakeGlorian{conns: []db.IntegrationConnection{{ID: 7, Uuid: uuid.New(), Key: glorian.ConnectionKey, Active: false}}}
}

func ok(context.Context) error { return nil }

// suite is the full check list with fakes; tweak one field per test.
type suite struct {
	runs    fakeRuns
	report  ReportFunc
	glorian GlorianQuerier
	pings   map[string]func(context.Context) error
	version VersionFunc
	index   fakeIndex
}

// fakeIndex holds the database and index document counts per spec.
type fakeIndex struct {
	db, docs map[string]int64
	err      error
}

func consistentIndex() fakeIndex {
	return fakeIndex{
		db:   map[string]int64{"users": 57, "products": 120, "services": 0},
		docs: map[string]int64{"users": 57, "products": 120, "services": 0},
	}
}

func (f fakeIndex) check() searchIndexCountsCheck {
	specs := make([]string, 0, len(f.db))
	for spec := range f.db {
		specs = append(specs, spec)
	}
	sort.Strings(specs)
	sources := make([]IndexSource, 0, len(specs))
	for _, spec := range specs {
		n := f.db[spec]
		sources = append(sources, IndexSource{Spec: spec,
			Expected: func(context.Context) (int64, error) { return n, nil }})
	}
	return searchIndexCountsCheck{sources: sources, count: func(_ context.Context, spec string) (int64, error) {
		if f.err != nil {
			return 0, f.err
		}
		return f.docs[spec], nil
	}}
}

func newSuite(t *testing.T) *suite {
	return &suite{
		runs:    goodRuns(),
		report:  cleanReport,
		glorian: activeGlorian(t, reconcileRun(t, glorian.RunSucceeded, testNow.Add(-20*time.Minute), glorian.ReconcileSummary{})),
		pings:   map[string]func(context.Context) error{},
		version: func(context.Context) (uint64, bool, bool, error) { return 86, false, true, nil },
		index:   consistentIndex(),
	}
}

func (s *suite) checks() []Check {
	ping := func(name string) Check {
		p := s.pings[name]
		if p == nil {
			p = ok
		}
		return pingCheck{name: name, ping: p, timeout: time.Second}
	}
	return []Check{
		migratorReportCheck{lastRun: s.runs.lastRun, report: s.report},
		deltaAgeCheck{lastRun: s.runs.lastRun, maxAge: 30 * time.Minute, now: clock},
		glorianReconcileCheck{q: s.glorian, maxAge: time.Hour, now: clock},
		ping("health_postgres"), ping("health_redis"), ping("health_meilisearch"),
		ping("health_s3"), ping("health_centrifugo"), ping("health_gotenberg"),
		s.index.check(),
		schemaVersionCheck{expected: func() (uint64, error) { return 86, nil }, current: s.version},
	}
}

func (s *suite) run(t *testing.T) (int, map[string]string) {
	t.Helper()
	var out bytes.Buffer
	code := runChecks(context.Background(), s.checks(), &out)
	return code, parseTable(t, out.String())
}

// parseTable maps check name -> "STATUS detail".
func parseTable(t *testing.T, out string) map[string]string {
	t.Helper()
	rows := map[string]string{}
	lines := strings.Split(out, "\n")
	if !strings.HasPrefix(lines[0], "CHECK") {
		t.Fatalf("no header: %q", out)
	}
	for _, l := range lines[1:] {
		if l == "" {
			break // the summary line follows the blank line
		}
		f := strings.Fields(l)
		rows[f[0]] = strings.Join(f[1:], " ")
	}
	return rows
}

func expectOnlyFail(t *testing.T, code int, rows map[string]string, name, reason string) {
	t.Helper()
	if code != 1 {
		t.Fatalf("exit = %d, want 1; rows %v", code, rows)
	}
	for n, r := range rows {
		if n == name {
			if !strings.HasPrefix(r, "FAIL") || !strings.Contains(r, reason) {
				t.Fatalf("%s = %q, want FAIL with %q", n, r, reason)
			}
			continue
		}
		if strings.HasPrefix(r, "FAIL") {
			t.Fatalf("%s also failed: %q", n, r)
		}
	}
	if _, seen := rows[name]; !seen {
		t.Fatalf("no %s row in %v", name, rows)
	}
}

// --- acceptance --------------------------------------------------------------

func TestAllPassExitsZero(t *testing.T) {
	code, rows := newSuite(t).run(t)
	if code != 0 {
		t.Fatalf("exit = %d; rows %v", code, rows)
	}
	if len(rows) != 11 {
		t.Fatalf("rows = %d: %v", len(rows), rows)
	}
	for n, r := range rows {
		if !strings.HasPrefix(r, "PASS") {
			t.Errorf("%s = %q", n, r)
		}
	}
}

// One FAIL anywhere exits 1 and its row carries the reason.
func TestSingleFailExitsOneWithReason(t *testing.T) {
	cases := []struct {
		name, check, reason string
		mut                 func(t *testing.T, s *suite)
	}{
		{"report mismatch", "migrator_report", "2 mismatches", func(_ *testing.T, s *suite) {
			s.report = func(context.Context) (*migrator.Report, error) {
				return &migrator.Report{Mismatches: []migrator.ReportDiff{
					{Source: "hub", Table: "users", ID: "5", Class: migrator.MismatchNotMigrated}, {}}}, nil
			}
		}},
		{"report error", "migrator_report", "LEGACY_HUB_DSN is not set", func(_ *testing.T, s *suite) {
			s.report = func(context.Context) (*migrator.Report, error) {
				return nil, errors.New("open source hub: LEGACY_HUB_DSN is not set")
			}
		}},
		{"stale delta", "migrator_delta_age", "finished 45m0s ago", func(_ *testing.T, s *suite) {
			s.runs.delta.FinishedAt = testNow.Add(-45 * time.Minute)
		}},
		{"no delta", "migrator_delta_age", "no delta run", func(_ *testing.T, s *suite) {
			s.runs.delta = migrator.RunSummary{}
		}},
		{"failed delta", "migrator_delta_age", "is failed", func(_ *testing.T, s *suite) {
			s.runs.delta.Status = migrator.StatusFailed
		}},
		{"reconcile drift", "glorian_reconcile", "3 drift", func(t *testing.T, s *suite) {
			s.glorian = activeGlorian(t, reconcileRun(t, glorian.RunSucceeded, testNow.Add(-time.Minute),
				glorian.ReconcileSummary{OnlyRemote: 1, OwnerDrift: 2}))
		}},
		{"reconcile still running", "glorian_reconcile", "is running", func(t *testing.T, s *suite) {
			s.glorian = activeGlorian(t, reconcileRun(t, glorian.RunRunning, time.Time{}, glorian.ReconcileSummary{}))
		}},
		{"reconcile stale", "glorian_reconcile", "run inventory-reconcile first", func(t *testing.T, s *suite) {
			s.glorian = activeGlorian(t, reconcileRun(t, glorian.RunSucceeded, testNow.Add(-3*time.Hour), glorian.ReconcileSummary{}))
		}},
		{"no reconcile run", "glorian_reconcile", "no reconcile run", func(_ *testing.T, s *suite) {
			g := inactiveGlorian()
			g.conns[0].Active = true
			s.glorian = g
		}},
		{"redis down", "health_redis", "connection refused", func(_ *testing.T, s *suite) {
			s.pings["health_redis"] = func(context.Context) error { return errors.New("dial tcp: connection refused") }
		}},
		{"gotenberg timeout", "health_gotenberg", "deadline", func(_ *testing.T, s *suite) {
			s.pings["health_gotenberg"] = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
		}},
		{"schema behind", "schema_version", "at version 85, binary expects 86", func(_ *testing.T, s *suite) {
			s.version = func(context.Context) (uint64, bool, bool, error) { return 85, false, true, nil }
		}},
		{"schema dirty", "schema_version", "dirty", func(_ *testing.T, s *suite) {
			s.version = func(context.Context) (uint64, bool, bool, error) { return 86, true, true, nil }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuite(t)
			tc.mut(t, s)
			code, rows := s.run(t)
			expectOnlyFail(t, code, rows, tc.check, tc.reason)
		})
	}
}

func TestNoMigratorRunFails(t *testing.T) {
	s := newSuite(t)
	s.runs = fakeRuns{none: true}
	reported := false
	s.report = func(ctx context.Context) (*migrator.Report, error) { reported = true; return cleanReport(ctx) }
	code, rows := s.run(t)
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if r := rows["migrator_report"]; !strings.HasPrefix(r, "FAIL") || !strings.Contains(r, "no migrator run recorded") {
		t.Fatalf("migrator_report = %q", r)
	}
	if reported {
		t.Fatal("report built without a run")
	}

	// A failed last run fails too, before the report.
	s = newSuite(t)
	s.runs.last = migrator.RunSummary{ID: 9, Mode: "full", Status: migrator.StatusFailed, Error: "step users: boom"}
	code, rows = s.run(t)
	expectOnlyFail(t, code, rows, "migrator_report", "step users: boom")
}

func TestGlorianInactiveSkipsAndExitsZero(t *testing.T) {
	for name, g := range map[string]fakeGlorian{"inactive": inactiveGlorian(), "none": {}} {
		t.Run(name, func(t *testing.T) {
			s := newSuite(t)
			s.glorian = g
			code, rows := s.run(t)
			if code != 0 {
				t.Fatalf("exit = %d; rows %v", code, rows)
			}
			if r := rows["glorian_reconcile"]; !strings.HasPrefix(r, "SKIP") || !strings.Contains(r, "no active glorian connection") {
				t.Fatalf("glorian_reconcile = %q", r)
			}
		})
	}
}

// TEC-525: an empty or short index (users kept only the super admin after a
// full migrator run) is a WARN with the counts; the exit code stays 0.
func TestSearchIndexCountsWarnOnEmptyOrMissing(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(f *fakeIndex)
		reason []string
	}{
		{"empty index", func(f *fakeIndex) { f.docs["products"] = 0 },
			[]string{"1 of 3 indexes differ", "products: index 0, database 120", "run search-reindex"}},
		{"short index", func(f *fakeIndex) { f.docs["users"] = 1 },
			[]string{"users: index 1, database 57"}},
		{"missing index", func(f *fakeIndex) { delete(f.docs, "users"); delete(f.docs, "products") },
			[]string{"2 of 3 indexes differ", "products: index 0, database 120", "users: index 0, database 57"}},
		{"stale extra documents", func(f *fakeIndex) { f.docs["services"] = 4 },
			[]string{"services: index 4, database 0"}},
		{"index error", func(f *fakeIndex) { f.err = errors.New("index_not_found") },
			[]string{"3 of 3 indexes differ", "users: index: index_not_found"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuite(t)
			tc.mut(&s.index)
			var out bytes.Buffer
			code := runChecks(context.Background(), s.checks(), &out)
			rows := parseTable(t, out.String())
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (WARN does not fail); rows %v", code, rows)
			}
			r := rows["search_index_counts"]
			if !strings.HasPrefix(r, "WARN") {
				t.Fatalf("search_index_counts = %q, want WARN", r)
			}
			for _, want := range tc.reason {
				if !strings.Contains(r, want) {
					t.Errorf("search_index_counts = %q, missing %q", r, want)
				}
			}
			if !strings.Contains(out.String(), "10 PASS, 1 WARN, 0 FAIL, 0 SKIP") {
				t.Fatalf("summary: %q", out.String())
			}
		})
	}
}

func TestSearchIndexCountsPassWhenConsistent(t *testing.T) {
	r := consistentIndex().check().Run(context.Background())
	if r.Status != Pass || r.Detail != "3 indexes match the database (177 documents)" {
		t.Fatalf("result = %+v", r)
	}
}

func TestSearchIndexCountsSkips(t *testing.T) {
	if r := (searchIndexCountsCheck{disabled: "search is disabled"}).Run(context.Background()); r.Status != Skip {
		t.Fatalf("disabled: %+v", r)
	}
	if r := (searchIndexCountsCheck{}).Run(context.Background()); r.Status != Skip {
		t.Fatalf("no specs: %+v", r)
	}
}

type fakeAdapter struct {
	id   string
	docs int
	err  error
}

func (a fakeAdapter) Spec() searchengine.Spec { return searchengine.Spec{ID: a.id} }
func (a fakeAdapter) ListAll(context.Context) ([]searchengine.Document, error) {
	return make([]searchengine.Document, a.docs), a.err
}
func (a fakeAdapter) Document(context.Context, string) (searchengine.Document, error) {
	return searchengine.Document{}, nil
}

// The expected count is what a reindex would write (ListAll), per spec.
func TestIndexSourcesCountListAll(t *testing.T) {
	src := indexSources([]searchengine.Adapter{fakeAdapter{id: "users", docs: 3},
		fakeAdapter{id: "roles", err: errors.New("db down")}})
	if len(src) != 2 || src[0].Spec != "roles" || src[1].Spec != "users" {
		t.Fatalf("sources = %+v", src)
	}
	if n, err := src[1].Expected(context.Background()); n != 3 || err != nil {
		t.Fatalf("users = %d, %v", n, err)
	}
	if _, err := src[0].Expected(context.Background()); err == nil {
		t.Fatal("roles: want error")
	}
	r := searchIndexCountsCheck{sources: src, count: func(context.Context, string) (int64, error) { return 3, nil }}.
		Run(context.Background())
	if r.Status != Warn || !strings.Contains(r.Detail, "roles: database: db down") {
		t.Fatalf("result = %+v", r)
	}
}

// The check counts exactly the specs of the shared search registry: a spec
// the server indexes but the preflight leaves out (or a stale extra one)
// fails here.
func TestSearchSourcesMatchRegistry(t *testing.T) {
	var got []string
	for _, s := range searchSources(nil) {
		got = append(got, s.Spec)
	}
	want := searchregistry.New(nil).SpecIDs()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("preflight specs = %v, registry specs = %v", got, want)
	}
	if !slices.Contains(got, "leads") {
		t.Errorf("leads missing from %v", got)
	}
}

// --- runner and helpers ------------------------------------------------------

type panicky struct{}

func (panicky) Name() string               { return "boom" }
func (panicky) Run(context.Context) Result { panic("kaboom") }

type badStatus struct{}

func (badStatus) Name() string               { return "odd" }
func (badStatus) Run(context.Context) Result { return Result{Status: "MAYBE", Detail: "x"} }

func TestRunnerPanicAndUnknownStatusFail(t *testing.T) {
	var out bytes.Buffer
	code := runChecks(context.Background(), []Check{panicky{}, badStatus{}, unavailable{"db", errors.New("down")}}, &out)
	rows := parseTable(t, out.String())
	if code != 1 || !strings.Contains(rows["boom"], "FAIL panic: kaboom") || !strings.HasPrefix(rows["odd"], "FAIL") ||
		rows["db"] != "FAIL down" {
		t.Fatalf("code=%d rows=%v", code, rows)
	}
	if !strings.Contains(out.String(), "0 PASS, 0 WARN, 3 FAIL, 0 SKIP") {
		t.Fatalf("summary missing: %q", out.String())
	}
}

func TestDisabledPingSkips(t *testing.T) {
	r := pingCheck{name: "health_centrifugo", disabled: "realtime is disabled"}.Run(context.Background())
	if r.Status != Skip {
		t.Fatalf("result = %+v", r)
	}
}

func TestParseArgs(t *testing.T) {
	o, err := parseArgs(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.profile.Name != "olex" || o.deltaMaxAge != 30*time.Minute || o.reconcileMaxAge != time.Hour || o.strict {
		t.Fatalf("defaults = %+v", o)
	}
	if o, err = parseArgs([]string{"--delta-max-age=5m", "--strict"}, io.Discard); err != nil || o.deltaMaxAge != 5*time.Minute || !o.strict {
		t.Fatalf("options = %+v, %v", o, err)
	}
	for _, args := range [][]string{
		{"--delta-max-age=0"}, {"--reconcile-max-age=-1h"}, {"--profile=nope"}, {"extra"}, {"--bogus"},
	} {
		if _, err := parseArgs(args, io.Discard); err == nil {
			t.Errorf("parseArgs(%v) succeeded", args)
		}
	}
}

// Bad usage exits 2 before loading config or connecting.
func TestRunBadArgsExitTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"--delta-max-age=zz"}, &out, &errOut); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
}
