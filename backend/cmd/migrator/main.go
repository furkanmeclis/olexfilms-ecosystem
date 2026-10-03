// Command migrator imports the legacy Olex hub and warehouse databases into
// this application (design §7, K26/K27, TEC-252). It reads the legacy
// MariaDB databases strictly read-only and writes only to the new Postgres.
//
// Usage:
//
//	migrator run --profile=olex [--mode=full|delta] [--steps=a,b] [--dry-run]
//	migrator report [--limit=20] [--json]
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
// The legacy DSNs are only needed when a run has steps.
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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

const usage = `usage:
  migrator run --profile=olex [--mode=full|delta] [--steps=a,b] [--dry-run]
  migrator report [--limit=20] [--json]
`

// dsnEnv maps a source name to its DSN variable.
var dsnEnv = map[string]string{
	migrator.SourceHub: "LEGACY_HUB_DSN",
	migrator.SourceWH:  "LEGACY_WH_DSN",
}

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
	steps := fs.String("steps", "", "comma separated step names (default: all)")
	dryRun := fs.Bool("dry-run", false, "run every step and roll its writes back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := migrator.ParseMode(*mode)
	if err != nil {
		return err
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
	rep, runErr := r.Run(ctx, migrator.Options{Profile: *profile, Mode: m, Steps: only, DryRun: *dryRun})
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if rep.RunID != 0 {
		_ = enc.Encode(rep)
	}
	return runErr
}

func cmdReport(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
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

// openLegacy opens a legacy source from its DSN variable. Only placeholders
// live in .env.example; the real DSNs are set on the server by hand.
func openLegacy(ctx context.Context, name string) (source.LegacySource, error) {
	key, ok := dsnEnv[name]
	if !ok {
		return nil, fmt.Errorf("no DSN variable for source %q", name)
	}
	dsn := strings.TrimSpace(os.Getenv(key))
	if dsn == "" {
		return nil, fmt.Errorf("%s is not set", key)
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return source.OpenPostgres(ctx, name, dsn, source.FixtureSchemas[name])
	}
	return source.OpenMariaDB(ctx, name, dsn)
}

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
