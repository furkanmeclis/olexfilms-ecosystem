package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// TEC-270: the stock.* outbox listener of the barcode push. It only
// enqueues: an entry or placement of a synced product schedules the
// connection's debounced bulk push, an exit (external_outbound) schedules
// the PATCH of that movement. Products without a sync link (Olex, or a
// local product of the glorian brand) enqueue nothing.

// PushQuerier is what the listener reads.
type PushQuerier interface {
	GetProductPushLink(ctx context.Context, id int64) (db.GetProductPushLinkRow, error)
	GetIntegrationConnectionByKey(ctx context.Context, arg db.GetIntegrationConnectionByKeyParams) (db.IntegrationConnection, error)
}

// Listener turns stock.* events into push tasks.
type Listener struct {
	q     PushQuerier
	queue Enqueuer
	log   *slog.Logger
	now   func() time.Time
}

// NewListener wires a listener; a nil queue disables it.
func NewListener(q PushQuerier, queue Enqueuer, log *slog.Logger) *Listener {
	if log == nil {
		log = slog.Default()
	}
	return &Listener{q: q, queue: queue, log: log, now: time.Now}
}

// RegisterEventHandlers subscribes the push listener to the stock.* events
// and the order outbound listener to the orders.* events
// it acts on. Every process that drains the outbox registers it; task ids
// keep a redelivered event from enqueuing twice.
func RegisterEventHandlers(bus events.Bus, q PushQuerier, queue Enqueuer, log *slog.Logger) {
	if bus == nil {
		return
	}
	NewListener(q, queue, log).Register(bus)
	// TEC-271: orders.* transitions schedule the order outbound.
	NewOrderListener(q, queue, log).Register(bus)
}

// WithClock replaces the clock that picks the debounce window (tests).
func (l *Listener) WithClock(now func() time.Time) *Listener {
	l.now = now
	return l
}

// Register subscribes the listener on bus.
func (l *Listener) Register(bus events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(events.StockEntry, l.HandlePlaced)
	bus.Subscribe(events.StockPlacement, l.HandlePlaced)
	for _, t := range PatchMovementTypes() {
		bus.Subscribe("stock."+t, l.HandleMoved)
	}
}

// HandlePlaced schedules the bulk push of the product's connection.
func (l *Listener) HandlePlaced(ctx context.Context, ev events.Event) error {
	connID, ok, err := l.linkedConnection(ctx, ev)
	if err != nil || !ok {
		return err
	}
	if l.queue == nil {
		l.log.Warn("glorian_push_queue_missing", "connection_id", connID)
		return nil
	}
	task, err := queue.NewGlorianPushBarcodesTask(connID)
	if err != nil {
		return err
	}
	return l.enqueue(task, queue.GlorianPushBarcodesOpts(connID, l.now()), "connection_id", connID)
}

// HandleMoved schedules the PATCH of the movement.
func (l *Listener) HandleMoved(ctx context.Context, ev events.Event) error {
	movementID, ok := payloadInt64(ev.Payload, "movement_id")
	if !ok && ev.EntityID != nil {
		movementID, ok = *ev.EntityID, true
	}
	if !ok || movementID <= 0 {
		return nil
	}
	connID, linked, err := l.linkedConnection(ctx, ev)
	if err != nil || !linked {
		return err
	}
	if l.queue == nil {
		l.log.Warn("glorian_patch_queue_missing", "movement_id", movementID)
		return nil
	}
	task, err := queue.NewGlorianPatchStockItemTask(movementID)
	if err != nil {
		return err
	}
	return l.enqueue(task, queue.GlorianPatchStockItemOpts(movementID), "connection_id", connID, "movement_id", movementID)
}

// linkedConnection returns the connection of the event's product. A
// product without a link enqueues nothing; in a brand that has a glorian
// connection that is a held barcode and is logged.
func (l *Listener) linkedConnection(ctx context.Context, ev events.Event) (int64, bool, error) {
	productID, ok := payloadInt64(ev.Payload, "product_id")
	if !ok || productID <= 0 {
		return 0, false, nil
	}
	link, err := l.q.GetProductPushLink(ctx, productID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("glorian push: product %d: %w", productID, err)
	}
	if link.ConnectionID.Valid && link.ExternalID.Valid {
		return link.ConnectionID.Int64, true, nil
	}
	if _, err := l.q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{
		BrandID: link.BrandID, Key: ConnectionKey,
	}); err == nil {
		barcode, _ := ev.Payload["barcode"].(string)
		l.log.Warn("glorian_push_held", "product_id", productID, "barcode", barcode, "event", ev.Name, "reason", "product_not_linked")
	}
	return 0, false, nil
}

func (l *Listener) enqueue(task *asynq.Task, opts []asynq.Option, logKV ...any) error {
	if _, err := l.queue.Enqueue(task, opts...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("glorian push: enqueue %s: %w", task.Type(), err)
	}
	l.log.Debug("glorian_push_enqueued", append([]any{"type", task.Type()}, logKV...)...)
	return nil
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	switch v := payload[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}
