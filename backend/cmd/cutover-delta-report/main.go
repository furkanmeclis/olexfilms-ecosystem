// Command cutover-delta-report lists the records born in this application
// since the cutover (TEC-276, rollback support of TEC-111): rows of the
// business tables with created_at >= --since that are not migration_map
// targets. There is no reverse import; the list is worked through by hand.
//
// Usage:
//
//	cutover-delta-report --since=<RFC3339> [--format=csv|json]
//
// The report goes to stdout unmasked (it carries personal data) and is meant
// for the authorized operator only. The command only reads: it runs one
// query in a READ ONLY transaction against the DB_* database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
)

const usage = "usage: cutover-delta-report --since=<RFC3339> [--format=csv|json]\n"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "cutover-delta-report: %v\n", err)
		os.Exit(1)
	}
}

// options is the parsed command line.
type options struct {
	since  time.Time
	format string
}

func parseArgs(args []string, errOut io.Writer) (options, error) {
	fs := flag.NewFlagSet("cutover-delta-report", flag.ContinueOnError)
	fs.SetOutput(errOut)
	since := fs.String("since", "", "cutover instant, RFC3339 (e.g. 2026-11-01T03:00:00+03:00)")
	format := fs.String("format", "csv", "csv | json")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *since == "" {
		return options{}, errors.New("--since is required")
	}
	t, err := time.Parse(time.RFC3339, *since)
	if err != nil {
		return options{}, fmt.Errorf("--since must be RFC3339: %w", err)
	}
	if *format != "csv" && *format != "json" {
		return options{}, fmt.Errorf("--format must be csv or json, not %q", *format)
	}
	return options{since: t, format: *format}, nil
}

func run(ctx context.Context, args []string, out, errOut io.Writer) error {
	opts, err := parseArgs(args, errOut)
	if err != nil {
		_, _ = fmt.Fprint(errOut, usage)
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return fmt.Errorf("begin read only: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := migrator.DeltaReport(ctx, tx, opts.since)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(errOut, "cutover-delta-report: %d records since %s\n", len(rows), opts.since.UTC().Format(time.RFC3339))
	return write(out, opts.format, rows)
}

func write(out io.Writer, format string, rows []migrator.DeltaRow) error {
	if format == "json" {
		return migrator.WriteDeltaJSON(out, rows)
	}
	return migrator.WriteDeltaCSV(out, rows)
}
