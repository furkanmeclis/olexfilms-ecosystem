package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	searchregistry "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/registry"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	if !cfg.Search.Enabled {
		log.Info("search_disabled")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := database.NewQueries(pool)
	client := searchengine.NewClient(cfg.Search, log)
	if client == nil || !client.Enabled() {
		log.Error("search_client_unavailable")
		os.Exit(1)
	}
	reg := searchregistry.New(queries)
	indexer := searchengine.NewIndexer(client, reg, nil, log)
	if err := indexer.ProcessReindex(ctx, ""); err != nil {
		log.Error("search_reindex_failed", "error", err)
		os.Exit(1)
	}
	log.Info("search_reindex_completed")
}
