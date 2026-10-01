// Command roles-sync reconciles the database with the Go permission catalog
// and system role packages (internal/platform/rbac/catalog.go). It is
// idempotent: running it twice reports no changes the second time.
//
// Usage (from backend/):
//
//	make roles-sync            # apply
//	make roles-sync ARGS=-dry-run
//	go run ./cmd/roles-sync -dry-run -prune
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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac/rbacsync"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report changes without applying them")
	prune := flag.Bool("prune", false, "delete permissions that are not in the catalog")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	rep, err := rbacsync.Sync(ctx, pool, rbacsync.Options{DryRun: *dryRun, Prune: *prune})
	if err != nil {
		fatal("%v", err)
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	mode := "applied"
	if *dryRun {
		mode = "dry-run"
	}
	fmt.Printf("roles-sync %s: %d change(s)\n", mode, rep.Changes())
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "roles-sync: "+format+"\n", args...)
	os.Exit(1)
}
