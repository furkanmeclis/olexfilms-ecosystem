package usecase

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// WithTx returns a copy of the service bound to the caller's transaction
// (TEC-316 lead conversion). RegisterOrganization then commits only with
// that transaction; its own transaction becomes a savepoint.
func (s *Service) WithTx(tx pgx.Tx) *Service {
	cp := *s
	cp.tx = tx
	cp.q = s.q.WithTx(tx)
	return &cp
}

func (s *Service) begin(ctx context.Context) (pgx.Tx, error) {
	if s.tx != nil {
		return s.tx.Begin(ctx)
	}
	return s.pool.Begin(ctx)
}
