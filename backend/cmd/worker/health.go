package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// runHealthFile touches path after every successful Redis ping. The
// container health check (compose.prod.yml) tests that the file is fresh,
// so a worker that lost the queue turns unhealthy.
func runHealthFile(ctx context.Context, rdb redis.UniversalClient, path string, interval time.Duration, log *slog.Logger) {
	touch := func() {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := rdb.Ping(pctx).Err(); err != nil {
			if ctx.Err() == nil {
				log.Warn("worker_health_check_failed", "error", err)
			}
			return
		}
		if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
			log.Warn("worker_health_file_failed", "error", err)
		}
	}
	touch()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			touch()
		}
	}
}
