package main

import (
	"context"
	"sort"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	searchregistry "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/registry"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
)

// searchSources counts every spec of the shared search registry (TEC-524),
// the list the server, worker and search-reindex use, so the check covers
// exactly the indexes a reindex writes.
func searchSources(q *db.Queries) []IndexSource {
	return indexSources(searchregistry.Adapters(q))
}

// indexSources turns adapters into index count sources sorted by spec. The
// expected count is the adapter's ListAll, so rows the index
// never holds (e.g. anonymized customers) are not counted.
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
