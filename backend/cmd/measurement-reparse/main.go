// Command measurement-reparse (measurement:reparse, TEC-294) normalizes every
// measurement_results row whose parsed_at is NULL: mobile uploads stored
// before the NexPTG parser and the migrator's legacy_import rows (TEC-262).
// Each row runs in its own transaction; a record that cannot be parsed keeps
// parsed_at NULL and is counted as unparseable (logged). It is idempotent: a
// second run writes nothing.
//
// Usage (from backend/):
//
//	make measurement-reparse
//	make measurement-reparse ARGS=-batch=500
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
)

func main() {
	batch := flag.Int("batch", 200, "rows read per page")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall timeout")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	rep, err := usecase.Reparse(ctx, pool, db.New(pool), int32(*batch), log)
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("measurement-reparse: scanned %d, parsed %d, unparseable %d, values %d, tires %d, devices created %d\n",
		rep.Scanned, rep.Parsed, rep.Unparseable, rep.Values, rep.Tires, rep.DevicesCreated)
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "measurement-reparse: "+format+"\n", args...)
	os.Exit(1)
}
