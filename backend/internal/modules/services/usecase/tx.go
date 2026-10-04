package usecase

import "github.com/jackc/pgx/v5"

// WithTx returns a copy of the service bound to the caller's transaction
// (TEC-316 lead conversion): its own transactions become savepoints and
// commit only with the caller's transaction.
func (s *Service) WithTx(tx pgx.Tx) *Service {
	cp := *s
	cp.pool = tx
	cp.q = s.q.WithTx(tx)
	return &cp
}
