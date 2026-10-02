// Package warranty is the warranty module (TEC-98, F1-06). This skeleton
// carries the periodic tasks (TEC-187); the service.completed listener
// (TEC-186), HTTP routes, PDF and transfers arrive with their own issues.
package warranty

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewCron wires the warranty:expire and warranty:expiring_scan handlers.
// frontendURL is the public origin used for the /garanti/{public_code} link.
func NewCron(pool *pgxpool.Pool, q *db.Queries, frontendURL string) *usecase.CronService {
	return usecase.NewCron(pool, q, outbox.NewStore(pool, q), frontendURL)
}
