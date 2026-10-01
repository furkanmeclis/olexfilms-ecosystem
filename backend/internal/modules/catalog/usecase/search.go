package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IndexStore is what the products search adapter reads.
type IndexStore interface {
	GetProductByUUIDForIndex(ctx context.Context, id uuid.UUID) (db.Product, error)
	ListProductsForIndex(ctx context.Context) ([]db.Product, error)
	GetProductCategory(ctx context.Context, arg db.GetProductCategoryParams) (db.ProductCategory, error)
}

// SearchAdapter indexes products in Meilisearch. Documents carry brand_id;
// the search service filters on the brand of the active organization, so an
// Olex organization never finds a Glorian product (K1/K20).
type SearchAdapter struct{ q IndexStore }

// NewSearchAdapter creates the products search adapter.
func NewSearchAdapter(q IndexStore) *SearchAdapter { return &SearchAdapter{q: q} }

// Spec implements searchengine.Adapter.
func (a *SearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:          SearchSpec,
		LabelKey:    "search.specs_products",
		Permission:  rbac.PermCatalogRead,
		Icon:        "package",
		Searchable:  []string{"title", "subtitle", "keywords"},
		Filterable:  []string{"brand_id"},
		BrandScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *SearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListProductsForIndex(ctx)
	if err != nil {
		return nil, err
	}
	cats := map[int64]db.ProductCategory{}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		cat, ok := cats[r.CategoryID]
		if !ok {
			c, err := a.q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: r.CategoryID, BrandID: r.BrandID})
			if err != nil {
				return nil, err
			}
			cats[r.CategoryID], cat = c, c
		}
		out = append(out, productDocument(r, cat))
	}
	return out, nil
}

// Document implements searchengine.Adapter.
func (a *SearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("products search: invalid uuid")
	}
	row, err := a.q.GetProductByUUIDForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, fmt.Errorf("products search: not found")
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	cat, err := a.q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: row.CategoryID, BrandID: row.BrandID})
	if err != nil {
		return searchengine.Document{}, err
	}
	return productDocument(row, cat), nil
}

func productDocument(r db.Product, cat db.ProductCategory) searchengine.Document {
	keywords := []string{r.Sku, cat.Name, r.UnitType}
	if !r.Active {
		keywords = append(keywords, "inactive")
	}
	return searchengine.Document{
		ID:       r.Uuid.String(),
		Spec:     SearchSpec,
		Title:    r.Name,
		Subtitle: r.Sku + " · " + cat.Name,
		Keywords: keywords,
		Href:     "/catalog/products/" + r.Uuid.String(),
		Icon:     "package",
		BrandID:  r.BrandID,
	}
}

var _ searchengine.Adapter = (*SearchAdapter)(nil)
