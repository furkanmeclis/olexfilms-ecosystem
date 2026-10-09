package main

import (
	"context"
	"sort"

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

// searchAdapters is the spec list of the API server's search registry
// (internal/httpserver/server.go).
func searchAdapters(q *db.Queries) []searchengine.Adapter {
	return []searchengine.Adapter{
		searchadapters.NewUsers(q),
		searchadapters.NewRoles(q),
		catalogusecase.NewSearchAdapter(q),
		customersusecase.NewSearchAdapter(q),
		servicesusecase.NewSearchAdapter(q),
		warrantyusecase.NewSearchAdapter(q),
		customersusecase.NewVehicleSearchAdapter(q),
		orgusecase.NewSearchAdapter(q),
		ordersusecase.NewSearchAdapter(q),
		stockusecase.NewSearchAdapter(q),
		leadsusecase.NewSearchAdapter(q),
	}
}

// indexSources turns adapters into index count sources sorted by spec. The
// expected count is what a reindex writes: the adapter's ListAll, so rows
// the index never holds (e.g. anonymized customers) are not counted.
func indexSources(adapters []searchengine.Adapter) []IndexSource {
	out := make([]IndexSource, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, IndexSource{
			Spec: a.Spec().ID,
			Expected: func(ctx context.Context) (int64, error) {
				docs, err := a.ListAll(ctx)
				return int64(len(docs)), err
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec < out[j].Spec })
	return out
}
