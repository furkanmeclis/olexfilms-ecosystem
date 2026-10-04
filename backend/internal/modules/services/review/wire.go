package review

import (
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	shorturlsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterEventHandlers subscribes the scheduler to service.completed. Every
// process that drains the outbox registers it; the task id keeps a
// redelivered event from enqueuing a second task.
func RegisterEventHandlers(bus events.Bus, q Enqueuer, delay time.Duration, log *slog.Logger) {
	if bus == nil {
		return
	}
	bus.Subscribe(events.ServiceCompleted, NewScheduler(q, delay, log).HandleServiceCompleted)
}

// NewTaskSender wires the service:review_request handler.
func NewTaskSender(pool *pgxpool.Pool, q *db.Queries, frontendURL string, log *slog.Logger) *Sender {
	return NewSender(pool, q, outbox.NewStore(pool, q), shorturlsmodule.NewLinker(q, frontendURL), log)
}
