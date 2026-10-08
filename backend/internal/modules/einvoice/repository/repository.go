package repository

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// Repository is the persistence boundary for the e-invoice module. F5-08a
// keeps it intentionally thin: mapper/API behavior lands in the next issues.
type Repository struct {
	q *db.Queries
}

func New(d db.DBTX) *Repository {
	return &Repository{q: db.New(d)}
}

func FromQueries(q *db.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) IncrementCounter(ctx context.Context, arg db.IncrementEinvoiceCounterParams) (db.IncrementEinvoiceCounterRow, error) {
	return r.q.IncrementEinvoiceCounter(ctx, arg)
}

func (r *Repository) Create(ctx context.Context, arg db.CreateEinvoiceParams) (db.Einvoice, error) {
	return r.q.CreateEinvoice(ctx, arg)
}

func (r *Repository) List(ctx context.Context, arg db.ListEinvoicesParams) ([]db.ListEinvoicesRow, error) {
	return r.q.ListEinvoices(ctx, arg)
}
