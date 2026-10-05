package reminder

import (
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterEventHandlers(bus events.Bus, q Enqueuer, log *slog.Logger) {
	if bus == nil {
		return
	}
	scheduler := NewScheduler(q, log)
	bus.Subscribe(events.AppointmentCreated, scheduler.HandleAppointmentChanged)
	bus.Subscribe(events.AppointmentRescheduled, scheduler.HandleAppointmentChanged)
}

func NewTaskSender(pool *pgxpool.Pool, q *db.Queries, log *slog.Logger) *Sender {
	return NewSender(pool, q, outbox.NewStore(pool, q), log)
}

func NewTaskNoShowScanner(pool *pgxpool.Pool, q *db.Queries, features FeatureChecker, log *slog.Logger) *NoShowScanner {
	return NewNoShowScanner(pool, q, outbox.NewStore(pool, q), features, log)
}
