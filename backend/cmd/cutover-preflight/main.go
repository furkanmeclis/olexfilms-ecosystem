// Command cutover-preflight runs the go/no-go checks of the final cutover
// (TEC-275, TEC-111) and prints them as a PASS/WARN/FAIL/SKIP table. Any
// FAIL exits 1; WARN does not.
//
// Checks:
//
//  1. migrator_report: the last migrator run of the profile exists and
//     succeeded, and the validation report (migrator report, TEC-264) has
//     zero mismatches. It reads the legacy databases (LEGACY_HUB_DSN,
//     LEGACY_WH_DSN) strictly read-only.
//  2. migrator_delta_age: the last delta run succeeded and finished at most
//     --delta-max-age ago.
//  3. glorian_reconcile: for each active glorian connection, the last
//     reconcile sync run succeeded with zero drift and finished at most
//     --reconcile-max-age ago (run inventory-reconcile first; the preflight
//     never starts one). No active connection: SKIP.
//  4. health_*: Postgres, Redis, Meilisearch, S3, Centrifugo, Gotenberg
//     (a dependency switched off in config is SKIP).
//  5. search_index_counts: every Meilisearch index holds as many documents
//     as the database has indexable rows (TEC-525). A difference is a WARN:
//     run search-reindex (docs/runbooks/migrator-cutover.md). Search
//     switched off: SKIP.
//  6. schema_version: schema_migrations is the version embedded in this
//     binary and not dirty.
//
// Usage:
//
//	cutover-preflight [--profile=olex] [--delta-max-age=30m] [--reconcile-max-age=1h] [--strict] [--timeout=10s]
//
// It uses the usual server environment (DB_*, REDIS_*, S3_*, MEILI_*,
// CENTRIFUGO_*, GOTENBERG_URL) plus the legacy DSNs; e.g. in the migrator
// service: docker compose ... --profile migrator run --rm --entrypoint
// /app/cutover-preflight migrator. It writes nothing anywhere.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/cache"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/migrations"
)

const usage = "usage: cutover-preflight [--profile=olex] [--delta-max-age=30m] [--reconcile-max-age=1h] [--strict] [--timeout=10s]\n"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// options is the parsed command line.
type options struct {
	profile         migrator.Profile
	deltaMaxAge     time.Duration
	reconcileMaxAge time.Duration
	strict          bool
	timeout         time.Duration
}

func parseArgs(args []string, errOut io.Writer) (options, error) {
	fs := flag.NewFlagSet("cutover-preflight", flag.ContinueOnError)
	fs.SetOutput(errOut)
	profile := fs.String("profile", envOr("MIGRATOR_PROFILE", "olex"), "migrator profile")
	deltaMaxAge := fs.Duration("delta-max-age", 30*time.Minute, "the last delta run must have finished within this")
	reconcileMaxAge := fs.Duration("reconcile-max-age", time.Hour, "the last glorian reconcile run must have finished within this")
	strict := fs.Bool("strict", false, "migrator report: count rows a step skipped as mismatches")
	timeout := fs.Duration("timeout", 10*time.Second, "timeout of each health ping")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *deltaMaxAge <= 0 || *reconcileMaxAge <= 0 || *timeout <= 0 {
		return options{}, errors.New("--delta-max-age, --reconcile-max-age and --timeout must be positive")
	}
	p, err := migrator.Lookup(migrator.Profiles(), *profile)
	if err != nil {
		return options{}, err
	}
	return options{profile: p, deltaMaxAge: *deltaMaxAge, reconcileMaxAge: *reconcileMaxAge,
		strict: *strict, timeout: *timeout}, nil
}

// run returns the exit code: 0 all PASS/SKIP, 1 any FAIL, 2 bad usage.
func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	opts, err := parseArgs(args, errOut)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "cutover-preflight: %v\n%s", err, usage)
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "cutover-preflight: config: %v\n", err)
		return 1
	}
	checks, closeAll := buildChecks(ctx, cfg, opts)
	defer closeAll()
	return runChecks(ctx, checks, out)
}

// buildChecks opens the dependencies; one that cannot be opened turns its
// checks into FAIL rows instead of stopping the preflight.
func buildChecks(ctx context.Context, cfg config.Config, opts options) ([]Check, func()) {
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	now := time.Now

	pool, dbErr := database.NewPostgresPool(ctx, cfg.DB)
	var dataChecks []Check
	indexCounts := searchIndexCountsCheck{}
	if dbErr != nil {
		dbErr = fmt.Errorf("database: %w", dbErr)
		dataChecks = []Check{
			unavailable{"migrator_report", dbErr},
			unavailable{"migrator_delta_age", dbErr},
			unavailable{"glorian_reconcile", dbErr},
			unavailable{"schema_version", dbErr},
			unavailable{"health_postgres", dbErr},
		}
	} else {
		closers = append(closers, pool.Close)
		lastRun := func(ctx context.Context, mode migrator.Mode) (migrator.RunSummary, bool, error) {
			return migrator.LastRun(ctx, pool, opts.profile.Name, mode)
		}
		report := func(ctx context.Context) (*migrator.Report, error) {
			return migrator.BuildReportReadOnly(ctx, pool, opts.profile, migrator.OpenLegacyFromEnv,
				migrator.ReportOptions{Profile: opts.profile.Name, Strict: opts.strict})
		}
		dataChecks = []Check{
			migratorReportCheck{lastRun: lastRun, report: report},
			deltaAgeCheck{lastRun: lastRun, maxAge: opts.deltaMaxAge, now: now},
			glorianReconcileCheck{q: db.New(pool), maxAge: opts.reconcileMaxAge, now: now},
			schemaVersionCheck{expected: migrations.LatestVersion, current: schemaVersion(pool)},
			pingCheck{name: "health_postgres", timeout: opts.timeout,
				ping: func(ctx context.Context) error { return database.Ping(ctx, pool) }},
		}
		indexCounts.sources = indexSources(searchAdapters(database.NewQueries(pool)))
	}

	var redisCheck Check
	if rdb, err := cache.NewRedisClient(ctx, cfg.Redis); err != nil {
		redisCheck = unavailable{"health_redis", err}
	} else {
		closers = append(closers, func() { _ = rdb.Close() })
		redisCheck = pingCheck{name: "health_redis", timeout: opts.timeout,
			ping: func(ctx context.Context) error { return cache.Ping(ctx, rdb) }}
	}

	meili := pingCheck{name: "health_meilisearch", timeout: opts.timeout}
	if search := searchengine.NewClient(cfg.Search, nil); search == nil {
		meili.disabled = "search is disabled (SEARCH_ENABLED / SEARCH_DRIVER)"
		indexCounts.disabled = meili.disabled
	} else {
		meili.ping = search.Ping
		indexCounts.count = search.Stats
	}
	if dbErr != nil && indexCounts.disabled == "" {
		indexCounts.disabled = "database unavailable (see health_postgres)"
	}

	var s3Check Check
	if store, err := storage.NewFromConfig(ctx, cfg.Storage); err != nil {
		s3Check = unavailable{"health_s3", err}
	} else {
		s3Check = pingCheck{name: "health_s3", timeout: opts.timeout, ping: store.Ping}
	}

	centrifugo := pingCheck{name: "health_centrifugo", timeout: opts.timeout}
	if c := realtime.NewCentrifugoClient(cfg.Centrifugo); c == nil {
		centrifugo.disabled = "realtime is disabled (CENTRIFUGO_ENABLED=false)"
	} else {
		centrifugo.ping = c.Ping
	}

	gotenberg := pdfrender.New(cfg.Gotenberg.URL)
	health := []Check{redisCheck, meili, s3Check, centrifugo,
		pingCheck{name: "health_gotenberg", timeout: opts.timeout, ping: gotenberg.Ping}, indexCounts}

	// Order: data checks (1-3), health (4), search index counts (5), schema
	// version (6).
	checks := make([]Check, 0, len(dataChecks)+len(health))
	var schema, pg Check
	for _, c := range dataChecks {
		switch c.Name() {
		case "schema_version":
			schema = c
		case "health_postgres":
			pg = c
		default:
			checks = append(checks, c)
		}
	}
	checks = append(checks, pg)
	checks = append(checks, health...)
	checks = append(checks, schema)
	return checks, closeAll
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
