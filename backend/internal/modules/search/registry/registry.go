// Package registry is the single list of search index adapters (TEC-524).
// The API server (bootstrap, index refresh), the worker (upsert / delete /
// reindex tasks) and cmd/search-reindex all build their registry here, so
// every spec the server indexes is also known to the worker.
package registry

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
)

// Adapters returns every search index adapter. A new searchable resource is
// added here only; TestAdaptersCoverEverySearchAdapter fails otherwise.
func Adapters(q *db.Queries) []searchengine.Adapter {
	return []searchengine.Adapter{
		searchadapters.NewUsers(q),
		searchadapters.NewRoles(q),
		catalogusecase.NewSearchAdapter(q),
		customersusecase.NewSearchAdapter(q), // TEC-164
		// TEC-209: services, warranties, vehicles (plate / VIN).
		servicesusecase.NewSearchAdapter(q),
		warrantyusecase.NewSearchAdapter(q),
		customersusecase.NewVehicleSearchAdapter(q),
		// TEC-210: organizations (dealer code), orders, stock units (barcode).
		orgusecase.NewSearchAdapter(q),
		ordersusecase.NewSearchAdapter(q),
		stockusecase.NewSearchAdapter(q),
		leadsusecase.NewSearchAdapter(q),
	}
}

// New builds the search registry shared by the server, worker and CLI.
func New(q *db.Queries) *searchengine.Registry {
	return searchengine.NewRegistry(Adapters(q)...)
}
