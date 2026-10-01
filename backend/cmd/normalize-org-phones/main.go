// Command normalize-org-phones converts organization phones to E.164 (K29,
// TEC-159). Migration 000048 moved every non-E.164 organizations.phone into
// phone_raw; this command parses them with the organization's country as the
// default region. Unparseable numbers stay in phone_raw and are listed under
// "unresolved" (nothing is deleted). It is idempotent.
//
// Usage (from backend/):
//
//	make normalize-org-phones ARGS=-dry-run   # report only
//	make normalize-org-phones                 # apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/orgphone"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report conversions without writing")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	rep, err := orgphone.Run(ctx, db.New(pool), *dryRun)
	if err != nil {
		fatal("%v", err)
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	mode := "applied"
	if *dryRun {
		mode = "dry-run"
	}
	fmt.Printf("normalize-org-phones %s: scanned %d, converted %d, superseded %d, unresolved %d\n",
		mode, rep.Scanned, len(rep.Converted), len(rep.Superseded), len(rep.Unresolved))
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "normalize-org-phones: "+format+"\n", args...)
	os.Exit(1)
}
