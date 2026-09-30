// Package dbtx stores an ambient pgx.Tx on context so repositories and
// platform helpers (e.g. outbox) share the same transaction.
package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

// WithTx returns a child context that carries tx.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// Tx returns the ambient transaction, if any.
func Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(ctxKey{}).(pgx.Tx)
	return tx, ok
}
