package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
)

// Status is the outcome of one check.
type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
	Skip Status = "SKIP"
)

// Result is one row of the preflight table. Detail says why on FAIL / SKIP
// and what was seen on PASS.
type Result struct {
	Status Status
	Detail string
}

func pass(format string, a ...any) Result { return Result{Pass, fmt.Sprintf(format, a...)} }
func fail(format string, a ...any) Result { return Result{Fail, fmt.Sprintf(format, a...)} }
func skip(format string, a ...any) Result { return Result{Skip, fmt.Sprintf(format, a...)} }

// Check is one preflight check. Every dependency sits behind it, so tests
// run the table with fakes.
type Check interface {
	Name() string
	Run(ctx context.Context) Result
}

// runChecks runs every check in order, prints the PASS/FAIL/SKIP table and
// returns the exit code: 1 when any check failed, else 0. A check that
// panics is a FAIL; it does not stop the others.
func runChecks(ctx context.Context, checks []Check, out io.Writer) int {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	counts := map[Status]int{}
	for _, c := range checks {
		r := runOne(ctx, c)
		counts[r.Status]++
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name(), r.Status, oneLine(r.Detail))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\n%d PASS, %d FAIL, %d SKIP\n", counts[Pass], counts[Fail], counts[Skip])
	if counts[Fail] > 0 {
		return 1
	}
	return 0
}

func runOne(ctx context.Context, c Check) (r Result) {
	defer func() {
		if p := recover(); p != nil {
			r = fail("panic: %v", p)
		}
	}()
	r = c.Run(ctx)
	if r.Status != Pass && r.Status != Skip {
		r.Status = Fail
	}
	return r
}

// oneLine keeps a table row on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// unavailable is the check of a dependency that could not be opened: it
// fails with the open error.
type unavailable struct {
	name string
	err  error
}

func (u unavailable) Name() string               { return u.name }
func (u unavailable) Run(context.Context) Result { return fail("%v", u.err) }

// --- 1. migrator report ----------------------------------------------------

// LastRunFunc returns the newest non-dry migrator run (migrator.LastRun).
type LastRunFunc func(ctx context.Context, mode migrator.Mode) (migrator.RunSummary, bool, error)

// ReportFunc builds the TEC-264 validation report (migrator.BuildReport).
type ReportFunc func(ctx context.Context) (*migrator.Report, error)

// migratorReportCheck: the last migrator run exists and succeeded, and the
// validation report has no mismatch.
type migratorReportCheck struct {
	lastRun LastRunFunc
	report  ReportFunc
}

func (migratorReportCheck) Name() string { return "migrator_report" }

func (c migratorReportCheck) Run(ctx context.Context) Result {
	run, ok, err := c.lastRun(ctx, "")
	if err != nil {
		return fail("%v", err)
	}
	if !ok {
		return fail("no migrator run recorded (migration_runs is empty for the profile)")
	}
	if run.Status != migrator.StatusSucceeded {
		return fail("last run #%d (%s) is %s: %s", run.ID, run.Mode, run.Status, run.Error)
	}
	rep, err := c.report(ctx)
	if err != nil {
		return fail("last run #%d succeeded, report failed: %v", run.ID, err)
	}
	if n := len(rep.Mismatches); n > 0 {
		d := rep.Mismatches[0]
		return fail("last run #%d: report has %d mismatches (first: %s %s.%s id=%s); see `migrator report`",
			run.ID, n, d.Class, d.Source, d.Table, d.ID)
	}
	return pass("last run #%d (%s) succeeded; report: %d tables, 0 mismatches, %d expected differences",
		run.ID, run.Mode, len(rep.Tables), len(rep.Expected))
}

// --- 2. delta age ----------------------------------------------------------

// deltaAgeCheck: the last delta run succeeded and finished within maxAge.
// Its age is measured on finished_at, not on the step watermarks: the
// watermark is the newest legacy source timestamp, which stops moving once
// the legacy hub is read only (design §7 closing order, step 2), so it would
// fail a perfectly fresh delta. The highest watermark is shown for the
// operator.
type deltaAgeCheck struct {
	lastRun LastRunFunc
	maxAge  time.Duration
	now     func() time.Time
}

func (deltaAgeCheck) Name() string { return "migrator_delta_age" }

func (c deltaAgeCheck) Run(ctx context.Context) Result {
	run, ok, err := c.lastRun(ctx, migrator.ModeDelta)
	if err != nil {
		return fail("%v", err)
	}
	if !ok {
		return fail("no delta run recorded")
	}
	if run.Status != migrator.StatusSucceeded {
		return fail("last delta run #%d is %s: %s", run.ID, run.Status, run.Error)
	}
	if run.FinishedAt.IsZero() {
		return fail("last delta run #%d has no finished_at", run.ID)
	}
	wm := "none"
	if !run.Watermark.IsZero() {
		wm = run.Watermark.UTC().Format(time.RFC3339)
	}
	age := c.now().Sub(run.FinishedAt).Truncate(time.Second)
	if age > c.maxAge {
		return fail("last delta run #%d finished %s ago (> %s); watermark %s", run.ID, age, c.maxAge, wm)
	}
	return pass("last delta run #%d finished %s ago (<= %s); watermark %s", run.ID, age, c.maxAge, wm)
}

// --- 3. glorian reconcile ----------------------------------------------------

// GlorianQuerier is the read access of the reconcile check (db.Querier
// subset).
type GlorianQuerier interface {
	ListIntegrationConnectionsByKey(ctx context.Context, key string) ([]db.IntegrationConnection, error)
	ListGlorianSyncRuns(ctx context.Context, arg db.ListGlorianSyncRunsParams) ([]db.IntegrationSyncRun, error)
}

// glorianReconcileCheck: for every active glorian connection the last
// reconcile sync run succeeded with zero drift and finished within maxAge.
// No active connection: SKIP.
//
// Reconcile has no schedule (only the admin endpoint and
// cmd/inventory-reconcile start one), so an old clean run proves nothing
// about now. The preflight only reads: it does not start a run (that would
// write a sync run row and call Glorian); a last run that is still running,
// failed or older than maxAge is a FAIL, and the operator runs
// inventory-reconcile right before the preflight.
type glorianReconcileCheck struct {
	q      GlorianQuerier
	maxAge time.Duration
	now    func() time.Time
}

func (glorianReconcileCheck) Name() string { return "glorian_reconcile" }

func (c glorianReconcileCheck) Run(ctx context.Context) Result {
	conns, err := c.q.ListIntegrationConnectionsByKey(ctx, glorian.ConnectionKey)
	if err != nil {
		return fail("list connections: %v", err)
	}
	var problems, oks []string
	for _, conn := range conns {
		if !conn.Active {
			continue
		}
		msg, ok := c.connection(ctx, conn)
		if ok {
			oks = append(oks, msg)
		} else {
			problems = append(problems, msg)
		}
	}
	switch {
	case len(problems) > 0:
		return fail("%s", strings.Join(problems, "; "))
	case len(oks) == 0:
		return skip("no active glorian connection")
	}
	return pass("%s", strings.Join(oks, "; "))
}

func (c glorianReconcileCheck) connection(ctx context.Context, conn db.IntegrationConnection) (string, bool) {
	runs, err := c.q.ListGlorianSyncRuns(ctx, db.ListGlorianSyncRunsParams{
		ConnectionID: conn.ID,
		Kinds:        []string{glorian.KindReconcile},
		SortKey:      "started_at",
		SortDesc:     true,
		RowLimit:     1,
	})
	if err != nil {
		return fmt.Sprintf("connection %s: list sync runs: %v", conn.Uuid, err), false
	}
	if len(runs) == 0 {
		return fmt.Sprintf("connection %s: no reconcile run recorded", conn.Uuid), false
	}
	run := runs[0]
	if run.Status != glorian.RunSucceeded {
		return fmt.Sprintf("connection %s: last reconcile run %s is %s: %s", conn.Uuid, run.Uuid, run.Status,
			run.Error.String), false
	}
	var counts glorian.ReconcileCounts
	if err := json.Unmarshal(run.Counts, &counts); err != nil {
		return fmt.Sprintf("connection %s: reconcile run %s counts: %v", conn.Uuid, run.Uuid, err), false
	}
	s := counts.ReconcileSummary
	if s.Total() > 0 {
		return fmt.Sprintf("connection %s: reconcile run %s has %d drift (%s=%d %s=%d %s=%d %s=%d %s=%d)",
			conn.Uuid, run.Uuid, s.Total(),
			glorian.DriftOnlyRemote, s.OnlyRemote, glorian.DriftOnlyLocal, s.OnlyLocal, glorian.DriftStatus, s.StatusDrift,
			glorian.DriftProduct, s.ProductDrift, glorian.DriftOwner, s.OwnerDrift), false
	}
	if !run.FinishedAt.Valid {
		return fmt.Sprintf("connection %s: reconcile run %s has no finished_at", conn.Uuid, run.Uuid), false
	}
	age := c.now().Sub(run.FinishedAt.Time).Truncate(time.Second)
	if age > c.maxAge {
		return fmt.Sprintf("connection %s: last reconcile run %s finished %s ago (> %s); run inventory-reconcile first",
			conn.Uuid, run.Uuid, age, c.maxAge), false
	}
	return fmt.Sprintf("connection %s: reconcile run %s finished %s ago, 0 drift (remote %d, local %d)",
		conn.Uuid, run.Uuid, age, counts.Remote, counts.Local), true
}

// --- 4. health ---------------------------------------------------------------

// pingCheck pings one dependency with a timeout. A non-empty disabled
// reason makes the check SKIP (the dependency is switched off in config).
type pingCheck struct {
	name     string
	ping     func(ctx context.Context) error
	disabled string
	timeout  time.Duration
}

func (c pingCheck) Name() string { return c.name }

func (c pingCheck) Run(ctx context.Context) Result {
	if c.disabled != "" {
		return skip("%s", c.disabled)
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	if err := c.ping(ctx); err != nil {
		return fail("%v", err)
	}
	return pass("ok (%s)", time.Since(start).Round(time.Millisecond))
}

// --- 5. schema version -------------------------------------------------------

// VersionFunc reads schema_migrations: the applied version and the dirty
// flag. ok is false when the table has no row.
type VersionFunc func(ctx context.Context) (version uint64, dirty, ok bool, err error)

// schemaVersionCheck: the database is at the migration version embedded in
// this binary and is not dirty.
type schemaVersionCheck struct {
	expected func() (uint64, error)
	current  VersionFunc
}

func (schemaVersionCheck) Name() string { return "schema_version" }

func (c schemaVersionCheck) Run(ctx context.Context) Result {
	want, err := c.expected()
	if err != nil {
		return fail("embedded migrations: %v", err)
	}
	got, dirty, ok, err := c.current(ctx)
	if err != nil {
		return fail("schema_migrations: %v", err)
	}
	if !ok {
		return fail("schema_migrations is empty; binary expects %d", want)
	}
	if dirty {
		return fail("database is dirty at version %d; binary expects %d", got, want)
	}
	if got != want {
		return fail("database is at version %d, binary expects %d", got, want)
	}
	return pass("version %d", got)
}

// schemaVersion reads golang-migrate's schema_migrations row.
func schemaVersion(q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) VersionFunc {
	return func(ctx context.Context) (uint64, bool, bool, error) {
		var (
			v     int64
			dirty bool
		)
		err := q.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&v, &dirty)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, false, nil
		}
		if err != nil {
			return 0, false, false, err
		}
		if v < 0 {
			return 0, dirty, true, fmt.Errorf("negative version %d", v)
		}
		return uint64(v), dirty, true, nil
	}
}
