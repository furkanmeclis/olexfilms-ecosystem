package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// itemCategories loads the category names of the products applied on the
// given services (ServiceView.ItemCategories) with one query for the whole
// page. ids must already be inside the caller's scope; the query is also
// brand-scoped. A service without counted items is absent from the map.
func itemCategories(ctx context.Context, q *db.Queries, brandID int64, ids []int64) (map[int64][]string, error) {
	if len(ids) == 0 {
		return map[int64][]string{}, nil
	}
	rows, err := q.ListServiceItemCategories(ctx, db.ListServiceItemCategoriesParams{BrandID: brandID, ServiceIds: ids})
	if err != nil {
		return nil, fmt.Errorf("services: item categories: %w", err)
	}
	return groupItemCategories(rows), nil
}

// groupItemCategories splits the query rows per service, keeping the row
// order (category sort, name, id). Names are trimmed and de-duplicated
// case-insensitively, the first spelling wins; blank names are skipped.
// Category names are stored in a single language (product_categories.name
// has no translations), so they are returned as stored, like the portal
// detail does.
func groupItemCategories(rows []db.ListServiceItemCategoriesRow) map[int64][]string {
	out := make(map[int64][]string)
	seen := make(map[int64]map[string]struct{})
	for _, r := range rows {
		name := strings.TrimSpace(r.CategoryName)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		s := seen[r.ServiceID]
		if s == nil {
			s = make(map[string]struct{})
			seen[r.ServiceID] = s
		}
		if _, dup := s[key]; dup {
			continue
		}
		s[key] = struct{}{}
		out[r.ServiceID] = append(out[r.ServiceID], name)
	}
	return out
}

// categoriesOf is the ItemCategories value of one service: never nil, so
// the API returns [] for a service without items.
func categoriesOf(byService map[int64][]string, serviceID int64) []string {
	if names := byService[serviceID]; names != nil {
		return names
	}
	return []string{}
}
