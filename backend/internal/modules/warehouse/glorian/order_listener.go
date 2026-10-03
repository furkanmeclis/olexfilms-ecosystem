package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// TEC-271: the orders.* outbox listener of the order outbound. It only
// enqueues glorian:order_outbound for an order of a brand that has a
// glorian connection, on the transitions the hub follows (ready, shipped,
// received, cancelled). An Olex order enqueues nothing.

// OrderQuerier is what the order listener reads.
type OrderQuerier interface {
	GetIntegrationConnectionByKey(ctx context.Context, arg db.GetIntegrationConnectionByKeyParams) (db.IntegrationConnection, error)
}

// OrderListener turns orders.* events into order outbound tasks.
type OrderListener struct {
	q     OrderQuerier
	queue Enqueuer
	log   *slog.Logger
}

// NewOrderListener wires an order listener; a nil queue disables it.
func NewOrderListener(q OrderQuerier, queue Enqueuer, log *slog.Logger) *OrderListener {
	if log == nil {
		log = slog.Default()
	}
	return &OrderListener{q: q, queue: queue, log: log}
}

// OrderOutboundEvents are the orders.* events that move a hub order.
func OrderOutboundEvents() []string {
	return []string{events.OrdersReady, events.OrdersShipped, events.OrdersReceived, events.OrdersCancelled}
}

// Register subscribes the listener on bus.
func (l *OrderListener) Register(bus events.Bus) {
	if bus == nil {
		return
	}
	for _, name := range OrderOutboundEvents() {
		bus.Subscribe(name, l.Handle)
	}
}

// Handle schedules the outbound task of the event's order.
func (l *OrderListener) Handle(ctx context.Context, ev events.Event) error {
	if ev.EntityID == nil || *ev.EntityID <= 0 {
		return nil
	}
	orderID := *ev.EntityID
	brandID, ok := payloadInt64(ev.Payload, "brand_id")
	if !ok || brandID <= 0 {
		return nil
	}
	if _, err := l.q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{
		BrandID: brandID, Key: ConnectionKey,
	}); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("glorian order: connection of brand %d: %w", brandID, err)
	}
	status, _ := ev.Payload["status"].(string)
	if l.queue == nil {
		l.log.Warn("glorian_order_queue_missing", "order_id", orderID)
		return nil
	}
	task, err := queue.NewGlorianOrderOutboundTask(orderID)
	if err != nil {
		return err
	}
	if _, err := l.queue.Enqueue(task, queue.GlorianOrderOutboundOpts(orderID, status)...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("glorian order: enqueue: %w", err)
	}
	l.log.Debug("glorian_order_enqueued", "order_id", orderID, "status", status)
	return nil
}
