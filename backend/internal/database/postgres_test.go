package database_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func TestQuerierInterfaceCompiles(t *testing.T) {
	t.Parallel()

	var _ db.Querier = (*db.Queries)(nil)
}
