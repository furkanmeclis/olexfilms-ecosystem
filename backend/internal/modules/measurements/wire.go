package measurements

import (
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewLinker wires the before/after matching (TEC-296) with the DB outbox.
func NewLinker(pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) *usecase.Linker {
	return usecase.NewLinker(pool, q, outbox.NewStore(pool, q), log)
}

// matchEvents are the service events after which the before/after
// suggestions are computed: the VIN may have been set (created / updated)
// or the after phase opened (processing / ready / completed).
var matchEvents = []string{
	events.ServiceCreated, events.ServiceUpdated, events.ServiceProcessing,
	events.ServiceReady, events.ServiceCompleted,
}

// RegisterEventHandlers subscribes the matching to the platform bus
// (outbox -> bus). Every process that drains the outbox registers it; the
// matching is idempotent and serialized per service.
func RegisterEventHandlers(bus events.Bus, pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) {
	if bus == nil || pool == nil || q == nil {
		return
	}
	l := NewLinker(pool, q, log)
	for _, name := range matchEvents {
		bus.Subscribe(name, l.HandleServiceEvent)
	}
}
