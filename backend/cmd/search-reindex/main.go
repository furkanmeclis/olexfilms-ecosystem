package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
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
	reg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		catalogusecase.NewSearchAdapter(queries),
		customersusecase.NewSearchAdapter(queries), // TEC-164
		// TEC-209: services, warranties, vehicles (plate / VIN).
		servicesusecase.NewSearchAdapter(queries),
		warrantyusecase.NewSearchAdapter(queries),
		customersusecase.NewVehicleSearchAdapter(queries),
		// TEC-210: organizations (dealer code), orders, stock units (barcode).
		orgusecase.NewSearchAdapter(queries),
		ordersusecase.NewSearchAdapter(queries),
		stockusecase.NewSearchAdapter(queries),
	)
	indexer := searchengine.NewIndexer(client, reg, nil, log)
	if err := indexer.ProcessReindex(ctx, ""); err != nil {
		log.Error("search_reindex_failed", "error", err)
		os.Exit(1)
	}
	log.Info("search_reindex_completed")
}
