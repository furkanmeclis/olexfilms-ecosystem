// Command org-merge merges one organization into another (TEC-1003), or
// undoes an earlier merge. Without -apply it is a dry run: the merge runs in
// a transaction that is rolled back and the report shows what would move
// (per table and column, members and their new roles, balances before and
// after). The migrator's organizations step runs the Olex merges itself (the
// Türkiye distributor and the AFM Florya service point); this command is for
// reviewing and undoing them by hand.
//
// Usage (from backend/):
//
//	make org-merge ARGS="-source 4 -target 1 -service-point -keep orders.buyer_org_id"   # dry run
//	make org-merge ARGS="-source 4 -target 1 -service-point -keep orders.buyer_org_id -apply"
//	make org-merge ARGS="-undo <run uuid>"            # dry run of the undo
//	make org-merge ARGS="-undo <run uuid> -apply"
//
// After an applied merge or undo run search-reindex (the search index keeps
// organization ids).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgmerge"
)

type keepFlag []orgmerge.Column

func (k *keepFlag) String() string {
	parts := make([]string, len(*k))
	for i, c := range *k {
		parts[i] = c.String()
	}
	return strings.Join(parts, ",")
}

func (k *keepFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		c, err := orgmerge.ParseColumn(part)
		if err != nil {
			return err
		}
		*k = append(*k, c)
	}
	return nil
}

func main() {
	source := flag.Int64("source", 0, "organization id to merge (it is soft deleted)")
	target := flag.Int64("target", 0, "organization id that takes the records over")
	servicePoint := flag.Bool("service-point", false, "copy the source name and logo to the target's service point")
	undo := flag.String("undo", "", "uuid of a merge run to undo")
	apply := flag.Bool("apply", false, "commit (default: dry run, rolled back)")
	var keep keepFlag
	flag.Var(&keep, "keep", "table.column that stays on the source (repeatable or comma separated)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		fatal("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var rep any
	if *undo != "" {
		rep, err = orgmerge.Undo(ctx, tx, *undo)
	} else {
		rep, err = orgmerge.Merge(ctx, tx, orgmerge.Options{
			SourceID: *source, TargetID: *target, Actor: actor(), Keep: keep, ServicePoint: *servicePoint,
		})
	}
	if out, mErr := json.MarshalIndent(rep, "", "  "); mErr == nil {
		fmt.Println(string(out))
	}
	if err != nil {
		fatal("%v", err)
	}
	if !*apply {
		fmt.Println("org-merge: dry run, rolled back (add -apply to commit)")
		return
	}
	if err := tx.Commit(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		fatal("commit: %v", err)
	}
	fmt.Println("org-merge: applied; run search-reindex next")
}

func actor() string {
	name := "unknown"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	return "cli:" + name
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "org-merge: "+format+"\n", args...)
	os.Exit(1)
}
