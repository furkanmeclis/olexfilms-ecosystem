// Command inventory-reconcile prints the read-only drift report between the
// Glorian hub's stock items and the local units of the connection's brand
// (TEC-272, warehouse inventory:reconcile). It writes nothing but its
// integration_sync_runs row (kind=reconcile).
//
//	inventory-reconcile [--connection=glorian] [--limit=10] [--json]
//
// Exit status: 0 no drift, 3 drift found, 1 error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
)

func main() {
	key := flag.String("connection", glorian.ConnectionKey, "integration connection key")
	limit := flag.Int("limit", 10, "rows to print per drift category")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	flag.Parse()
	if *key != glorian.ConnectionKey {
		fail("--connection must be %q", glorian.ConnectionKey)
	}
	if *limit < 1 {
		*limit = 1
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
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		fail("secret box: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	r := glorian.NewReconciler(database.NewQueries(pool), box,
		glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), log)

	reports, runErr := r.Run(ctx, *key)
	if *asJSON {
		out := make([]glorian.ReconcileReport, 0, len(reports))
		for _, rep := range reports {
			rep.ReconcileDetails = rep.Limit(*limit)
			out = append(out, rep)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fail("encode report: %v", err)
		}
	} else {
		for _, rep := range reports {
			printText(os.Stdout, rep, *limit)
		}
	}
	if runErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "inventory reconcile failed: %v\n", runErr)
	}
	os.Exit(glorian.ReconcileExitCode(reports, runErr))
}

func printText(w io.Writer, rep glorian.ReconcileReport, limit int) {
	s := rep.Summary
	_, _ = fmt.Fprintf(w, "inventory reconcile %s (run %s): %d remote, %d local\n",
		rep.ConnectionUUID, rep.RunUUID, rep.Remote, rep.Local)
	d := rep.Limit(limit)
	for _, c := range []struct {
		name  string
		count int
		rows  []glorian.DriftRow
	}{
		{glorian.DriftOnlyRemote, s.OnlyRemote, d.OnlyRemote},
		{glorian.DriftOnlyLocal, s.OnlyLocal, d.OnlyLocal},
		{glorian.DriftStatus, s.StatusDrift, d.StatusDrift},
		{glorian.DriftProduct, s.ProductDrift, d.ProductDrift},
		{glorian.DriftOwner, s.OwnerDrift, d.OwnerDrift},
	} {
		_, _ = fmt.Fprintf(w, "  %-14s %d\n", c.name, c.count)
		for _, row := range c.rows {
			b, _ := json.Marshal(row)
			_, _ = fmt.Fprintf(w, "    %s\n", b)
		}
	}
	if rep.HasDrift() {
		_, _ = fmt.Fprintln(w, "drift found")
	} else {
		_, _ = fmt.Fprintln(w, "clean: no drift")
	}
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(glorian.ReconcileExitError)
}
