package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency/usecase"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "database init failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	limit := int32(500)
	if raw := os.Getenv("EFFICIENCY_BACKFILL_BATCH"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 32); err == nil && n > 0 {
			limit = int32(n)
		}
	}
	n, err := usecase.New(database.NewQueries(pool)).Backfill(ctx, limit)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "efficiency backfill failed after %d items: %v\n", n, err)
		os.Exit(1)
	}
	fmt.Printf("efficiency backfill completed: %d items\n", n)
}
