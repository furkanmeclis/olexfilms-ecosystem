// Package warranty is the warranty module (TEC-98, F1-06): the periodic
// tasks (TEC-187) and the service.completed listener (TEC-186). HTTP
// routes, PDF and transfers arrive with their own issues.
package warranty

import (
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewCron wires the warranty:expire and warranty:expiring_scan handlers.
// frontendURL is the public origin used for the /garanti/{public_code} link.
func NewCron(pool *pgxpool.Pool, q *db.Queries, frontendURL string) *usecase.CronService {
	return usecase.NewCron(pool, q, outbox.NewStore(pool, q), frontendURL)
}

// NewListener builds the service.completed consumer.
func NewListener(pool *pgxpool.Pool, q *db.Queries, frontendURL string, log *slog.Logger) *usecase.Listener {
	return usecase.NewListener(pool, q, outbox.NewStore(pool, q), frontendURL, log)
}

// NewRepairScanner wires the warranty:repair_scan handler (TEC-194) over
// the same listener the bus uses; days is the look-back window.
func NewRepairScanner(pool *pgxpool.Pool, q *db.Queries, frontendURL string, days int, log *slog.Logger) *usecase.RepairScanner {
	return usecase.NewRepairScanner(NewListener(pool, q, frontendURL, log), days, log)
}

// RegisterEventHandlers subscribes the warranty listener to the platform
// bus (outbox -> bus, same as the notification handlers). Every process
// that drains the outbox registers it, so whichever claims a
// service.completed row opens the warranties; the listener is idempotent.
func RegisterEventHandlers(bus events.Bus, pool *pgxpool.Pool, q *db.Queries, frontendURL string, log *slog.Logger) {
	if bus == nil || pool == nil || q == nil {
		return
	}
	l := NewListener(pool, q, frontendURL, log)
	bus.Subscribe(events.ServiceCompleted, l.HandleServiceCompleted)
}

// NewCertificate wires the warranty certificate service (TEC-188); store
// loads the dealer logo, frontendURL is the public origin of the QR links.
func NewCertificate(q *db.Queries, store storage.Driver, frontendURL string, log *slog.Logger) *usecase.CertificateService {
	return usecase.NewCertificate(q, store, frontendURL, log)
}
