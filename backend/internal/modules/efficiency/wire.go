package efficiency

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5/pgtype"
)

type NetworkRefresher struct {
	q        *db.Queries
	settings usecase.Settings
	now      func() time.Time
}

func NewNetworkRefresher(q *db.Queries, settings usecase.Settings) *NetworkRefresher {
	return &NetworkRefresher{q: q, settings: settings, now: time.Now}
}

func (r *NetworkRefresher) Task(ctx context.Context) error {
	brands, err := r.q.ListBrands(ctx)
	if err != nil {
		return fmt.Errorf("efficiency network brands: %w", err)
	}
	svc := usecase.New(r.q)
	for _, brand := range brands {
		if _, err := svc.RefreshNetwork(ctx, brand.ID, r.now(), r.settings); err != nil {
			return err
		}
	}
	return nil
}

func RegisterEventHandlers(bus events.Bus, q *db.Queries, log *slog.Logger) {
	if bus == nil || q == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	svc := usecase.New(q)
	refresh := func(ctx context.Context, ev events.Event) error {
		serviceID, ok := payloadInt64(ev.Payload, "service_id")
		if !ok && ev.EntityID != nil {
			serviceID, ok = *ev.EntityID, true
		}
		if !ok || serviceID <= 0 {
			log.Warn("efficiency_listener_no_service", "event_id", ev.EventID.String())
			return nil
		}
		return svc.RefreshService(ctx, serviceID)
	}
	bus.Subscribe(events.ServiceCompleted, refresh)
	bus.Subscribe(events.ServiceUpdated, func(ctx context.Context, ev events.Event) error {
		if change, _ := ev.Payload["change"].(string); change != "consumption_corrected" {
			return nil
		}
		if itemID, ok := payloadInt64(ev.Payload, "item_id"); ok && itemID > 0 {
			return svc.RefreshItem(ctx, itemID)
		}
		return refresh(ctx, ev)
	})
}

func payloadInt64(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		return int64(v), true
	case pgtype.Int8:
		return v.Int64, v.Valid
	case interface{ Int64() (int64, error) }:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}
