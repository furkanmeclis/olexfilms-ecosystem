// Package repository stores stock forecast snapshots (TEC-483, F5-04a).
package repository

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// Repository is the persistence boundary used by the stock forecast worker
// and API. It intentionally stays thin; sqlc owns the SQL contract.
type Repository struct {
	q *db.Queries
}

func New(q *db.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) UpsertSnapshot(ctx context.Context, arg db.UpsertStockForecastSnapshotParams) (db.StockForecast, error) {
	return r.q.UpsertStockForecastSnapshot(ctx, arg)
}

func (r *Repository) List(ctx context.Context, arg db.ListStockForecastsParams) ([]db.ListStockForecastsRow, error) {
	return r.q.ListStockForecasts(ctx, arg)
}

func (r *Repository) Count(ctx context.Context, arg db.CountStockForecastsParams) (int64, error) {
	return r.q.CountStockForecasts(ctx, arg)
}

func (r *Repository) UpsertThreshold(ctx context.Context, arg db.UpsertStockForecastThresholdParams) (db.StockForecastThreshold, error) {
	return r.q.UpsertStockForecastThreshold(ctx, arg)
}
