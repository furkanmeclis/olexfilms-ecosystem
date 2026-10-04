// Package repository wraps contract-template persistence.
package repository

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the contracts repository.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New creates a repository.
func New(pool *pgxpool.Pool, q *db.Queries) *Store {
	return &Store{pool: pool, q: q}
}

// Pool returns the backing pool for short transactional use cases.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Queries returns the generated query set.
func (s *Store) Queries() *db.Queries { return s.q }

// Tx starts a transaction and returns a query set bound to it.
func (s *Store) Tx(ctx context.Context) (pgx.Tx, *db.Queries, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return tx, s.q.WithTx(tx), nil
}

// CountTemplateInstances reports how many contract instances freeze templateID.
func (s *Store) CountTemplateInstances(ctx context.Context, templateID int64) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM contract_instances WHERE template_id = $1`, templateID).Scan(&n)
	return n, err
}

// CountTemplateInstancesTx is the transactional variant.
func CountTemplateInstancesTx(ctx context.Context, tx pgx.Tx, templateID int64) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM contract_instances WHERE template_id = $1`, templateID).Scan(&n)
	return n, err
}

// TemplateIDByUUID returns the internal ID for tests and follow-up modules.
func (s *Store) TemplateIDByUUID(ctx context.Context, brandID int64, id uuid.UUID) (int64, error) {
	t, err := s.q.GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: brandID})
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}
