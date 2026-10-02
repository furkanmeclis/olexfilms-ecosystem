// Command inventory-rebuild replays stock_movements, reports where the stock
// projections drifted from the ledger and, with -apply, repairs them in one
// locked transaction with an audit row (TEC-156). The default is a dry run.
//
//	inventory-rebuild [-org ID] [-dry-run | -apply] [-json]
//
// Exit status: 0 no drift (or repaired), 3 drift found in a dry run, 1 error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
)

const exitDrift = 3

func main() {
	org := flag.Int64("org", 0, "organization id to scan (0 = every organization)")
	dryRun := flag.Bool("dry-run", false, "report only, write nothing (the default unless -apply)")
	apply := flag.Bool("apply", false, "repair the projections in one locked transaction")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	flag.Parse()
	if *dryRun && *apply {
		fail("-dry-run and -apply are exclusive")
	}

	cfg, err := config.Load()
	if err != nil {
		fail("config load failed: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fail("database init failed: %v", err)
	}
	defer pool.Close()

	svc := rebuild.New(pool, database.NewQueries(pool))
	rep, err := svc.Run(ctx, rebuild.Options{OrganizationID: *org, Apply: *apply, Source: "cli"})
	if err != nil {
		fail("inventory rebuild failed: %v", err)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fail("encode report: %v", err)
		}
	} else {
		printText(os.Stdout, rep, *apply)
	}
	if !*apply && rep.DiffCount > 0 {
		os.Exit(exitDrift)
	}
}

func printText(w io.Writer, rep rebuild.Report, apply bool) {
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	scope := "all organizations"
	if rep.OrganizationID > 0 {
		scope = fmt.Sprintf("organization %d", rep.OrganizationID)
	}
	_, _ = fmt.Fprintf(w, "inventory rebuild (%s, %s): %d units, %d movements, %d differences\n",
		mode, scope, rep.UnitsScanned, rep.MovementsReplayed, rep.DiffCount)
	for _, d := range rep.Diffs {
		unit := ""
		if d.UnitID != 0 {
			unit = fmt.Sprintf(" [%s]", d.Barcode)
		}
		_, _ = fmt.Fprintf(w, "  %s %s%s %s: expected %s, actual %s\n", d.Table, d.Key, unit, d.Field, d.Expected, d.Actual)
	}
	for _, a := range rep.Anomalies {
		_, _ = fmt.Fprintf(w, "  anomaly: %s\n", a)
	}
	if rep.Applied {
		_, _ = fmt.Fprintln(w, "projections repaired; audit row written")
	}
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
