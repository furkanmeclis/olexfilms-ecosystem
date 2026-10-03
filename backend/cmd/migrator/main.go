// Command migrator imports the legacy Olex hub and warehouse databases into
// this application (design §7, K26/K27, TEC-252). It reads the legacy
// MariaDB databases strictly read-only and writes only to the new Postgres.
//
// Usage:
//
//	migrator run --profile=olex [--mode=full|delta] [--overlap=10m] [--steps=a,b] [--dry-run]
//	migrator report --profile=olex [--json] [--strict]
//	migrator runs [--limit=20] [--json]
//
// run --mode=delta reads, per step, the rows changed since the step's last
// watermark (migration_runs) minus --overlap (TEC-264). report compares the
// legacy tables with what was migrated (expected differences are classified,
// see migrator.BuildReport) and exits non-zero when a row is unaccounted
// for. runs lists the run history and the migration_map counts.
//
// Environment: the usual DB_* settings for the new database, plus
//
//	LEGACY_HUB_DSN          legacy hub, go-sql-driver DSN
//	                        (user:pass@tcp(host:3306)/dbname); a postgres://
//	                        DSN reads the legacy_hub fixture schema instead
//	LEGACY_WH_DSN           legacy warehouse, same format (legacy_wh schema)
//	LEGACY_HUB_STORAGE_DIR  legacy hub storage directory (media copy steps:
//	                        brand logos go to the STORAGE_* object store)
//
// The legacy DSNs are only needed when a run has steps, and by report.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

const usage = `usage:
  migrator run --profile=olex [--mode=full|delta] [--overlap=10m] [--steps=a,b] [--dry-run]
  migrator report --profile=olex [--json] [--strict]
  migrator runs [--limit=20] [--json]
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "migrator: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprint(out, usage)
		return errors.New("missing command")
	}
	switch args[0] {
	case "run":
		return cmdRun(ctx, args[1:], out)
	case "report":
		return cmdReport(ctx, args[1:], out)
	case "runs":
		return cmdRuns(ctx, args[1:], out)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(out, usage)
		return nil
	}
	_, _ = fmt.Fprint(out, usage)
	return fmt.Errorf("unknown command %q", args[0])
}

func cmdRun(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	profile := fs.String("profile", envOr("MIGRATOR_PROFILE", "olex"), "profile name")
	mode := fs.String("mode", string(migrator.ModeFull), "full | delta")
	overlap := fs.Duration("overlap", migrator.DefaultDeltaOverlap, "delta mode: read this far before the last watermark")
	steps := fs.String("steps", "", "comma separated step names (default: all)")
	dryRun := fs.Bool("dry-run", false, "run every step and roll its writes back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := migrator.ParseMode(*mode)
	if err != nil {
		return err
	}
	if *overlap < 0 {
		return errors.New("--overlap must not be negative")
	}
	if *overlap == 0 {
		*overlap = -1 // Options: negative means no overlap, zero the default
	}
	// Refuse disabled / unknown profiles before connecting anywhere.
	if _, err := migrator.Lookup(migrator.Profiles(), *profile); err != nil {
		return err
	}

	pool, closePool, err := openTarget(ctx)
	if err != nil {
		return err
	}
	defer closePool()

	files, store, err := openMedia(ctx)
	if err != nil {
		return err
	}

	r := &migrator.Runner{
		Pool:        pool,
		Open:        openLegacy,
		Log:         slog.New(slog.NewJSONHandler(os.Stderr, nil)),
		Storage:     store,
		LegacyFiles: files,
	}
	var only []string
	if *steps != "" {
		only = strings.Split(*steps, ",")
	}
	rep, runErr := r.Run(ctx, migrator.Options{Profile: *profile, Mode: m, Steps: only, DryRun: *dryRun,
		Overlap: *overlap})
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if rep.RunID != 0 {
		_ = enc.Encode(rep)
	}
	return runErr
}

// cmdReport is the validation report (TEC-264): source vs target counts per
// legacy table, expected differences classified, mismatches listed; a
// mismatch is an error, so the command exits non-zero.
func cmdReport(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	profile := fs.String("profile", envOr("MIGRATOR_PROFILE", "olex"), "profile name")
	asJSON := fs.Bool("json", false, "print JSON")
	strict := fs.Bool("strict", false, "count rows a step skipped (with a reported reason) as mismatches")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	p, err := migrator.Lookup(migrator.Profiles(), *profile)
	if err != nil {
		return err
	}

	pool, closePool, err := openTarget(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	rep, err := migrator.BuildReportReadOnly(ctx, pool, p, openLegacy, migrator.ReportOptions{Profile: p.Name, Strict: *strict})
	if err != nil {
		return err
	}
	return writeReport(out, rep, *asJSON)
}

// writeReport prints rep and returns its mismatch error (non-zero exit).
func writeReport(out io.Writer, rep *migrator.Report, asJSON bool) error {
	var err error
	if asJSON {
		err = migrator.WriteReportJSON(out, rep)
	} else {
		err = migrator.WriteReportText(out, rep)
	}
	if err != nil {
		return err
	}
	return rep.Err()
}

func cmdRuns(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "number of runs to list")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit < 1 || *limit > 1000 {
		return errors.New("--limit must be between 1 and 1000")
	}
	pool, closePool, err := openTarget(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	q := db.New(pool)
	runs, err := q.ListMigrationRuns(ctx, int32(*limit))
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}
	maps, err := q.CountMigrationMap(ctx)
	if err != nil {
		return fmt.Errorf("count map: %w", err)
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"runs": runs, "migration_map": maps})
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RUN\tPARENT\tPROFILE\tMODE\tSTEP\tDRY\tSTATUS\tSTARTED\tFINISHED\tCOUNTS\tERROR")
	for _, r := range runs {
		parent, step, finished := "-", "-", "-"
		if r.ParentID.Valid {
			parent = fmt.Sprint(r.ParentID.Int64)
		}
		if r.Step.Valid {
			step = r.Step.String
		}
		if r.FinishedAt.Valid {
			finished = r.FinishedAt.Time.UTC().Format("2006-01-02 15:04:05")
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%v\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, parent, r.Profile, r.Mode, step, r.DryRun, r.Status,
			r.StartedAt.Time.UTC().Format("2006-01-02 15:04:05"), finished, string(r.Counts), r.Error.String)
	}
	_, _ = fmt.Fprintln(tw)
	_, _ = fmt.Fprintln(tw, "SOURCE\tTABLE\tTARGET\tROWS\tLAST")
	for _, m := range maps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", m.SourceSystem, m.SourceTable, m.TargetTable, m.Rows,
			m.LastMigratedAt.Time.UTC().Format("2006-01-02 15:04:05"))
	}
	return tw.Flush()
}

func openTarget(ctx context.Context) (*pgxpool.Pool, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, func() {}, fmt.Errorf("config: %w", err)
	}
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return nil, func() {}, fmt.Errorf("database: %w", err)
	}
	return pool, pool.Close, nil
}

// openLegacy opens a legacy source from its DSN variable
// (migrator.LegacyDSNEnv).
var openLegacy = migrator.OpenLegacyFromEnv

// openMedia returns the legacy hub storage directory and the object store
// the media steps copy into (TEC-256). Both stay nil when
// LEGACY_HUB_STORAGE_DIR is unset; the steps then report the media they
// could not copy.
func openMedia(ctx context.Context) (fs.FS, storage.Driver, error) {
	dir := strings.TrimSpace(os.Getenv("LEGACY_HUB_STORAGE_DIR"))
	if dir == "" {
		return nil, nil, nil
	}
	st, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("LEGACY_HUB_STORAGE_DIR: %w", err)
	}
	if !st.IsDir() {
		return nil, nil, fmt.Errorf("LEGACY_HUB_STORAGE_DIR %q is not a directory", dir)
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		return nil, nil, fmt.Errorf("storage: %w", err)
	}
	return os.DirFS(dir), store, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
