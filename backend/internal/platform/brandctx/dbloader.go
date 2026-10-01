package brandctx

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// BrandQuerier is the subset of db.Queries the loader needs.
type BrandQuerier interface {
	ListBrands(ctx context.Context) ([]db.Brand, error)
	ListBrandDomains(ctx context.Context) ([]db.ListBrandDomainsRow, error)
}

// DBLoader loads the brand catalog from Postgres.
func DBLoader(q BrandQuerier) Loader {
	return func(ctx context.Context) (Catalog, error) {
		brands, err := q.ListBrands(ctx)
		if err != nil {
			return Catalog{}, err
		}
		domains, err := q.ListBrandDomains(ctx)
		if err != nil {
			return Catalog{}, err
		}
		c := Catalog{BySlug: make(map[string]Brand, len(brands)), ByHost: make(map[string]Brand, len(domains))}
		byID := make(map[int64]Brand, len(brands))
		for _, row := range brands {
			b := FromRow(row)
			c.BySlug[b.Slug] = b
			byID[b.ID] = b
		}
		for _, d := range domains {
			if b, ok := byID[d.BrandID]; ok {
				c.ByHost[NormalizeHost(d.Host)] = b
			}
		}
		return c, nil
	}
}

// FromRow maps a brands row.
func FromRow(row db.Brand) Brand {
	return Brand{ID: row.ID, UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Status: row.Status}
}
