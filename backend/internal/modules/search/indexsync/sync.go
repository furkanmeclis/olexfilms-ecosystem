// Package indexsync keeps the TEC-209 record indexes (services,
// warranties, vehicles) in step with the outbox: every service, warranty
// and vehicle event the outbox publisher hands to the bus enqueues an
// upsert of the touched document. The adapter rebuilds the document from
// Postgres, so the event payload is only a pointer (no personal data is
// read from it), and a record that must leave the index (deleted vehicle,
// anonymized owner) is removed by the indexer (ErrSkipDocument).
package indexsync

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Indexer schedules document refreshes (*searchengine.Indexer).
type Indexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
}

// Store lists the records of one customer (*db.Queries).
type Store interface {
	ListSearchUuidsByUserID(ctx context.Context, userID int64) (db.ListSearchUuidsByUserIDRow, error)
}

// Sync is the outbox -> search index bridge.
type Sync struct {
	q   Store
	idx Indexer
	log *slog.Logger
}

// New builds the bridge.
func New(q Store, idx Indexer, log *slog.Logger) *Sync {
	if log == nil {
		log = slog.Default()
	}
	return &Sync{q: q, idx: idx, log: log}
}

// Register subscribes the bridge on bus. A nil indexer (search disabled)
// registers nothing: the lists fall back to SQL.
func Register(bus events.Bus, q Store, idx Indexer, log *slog.Logger) {
	if bus == nil || idx == nil {
		return
	}
	if e, ok := idx.(interface{ Enabled() bool }); ok && !e.Enabled() {
		return
	}
	s := New(q, idx, log)
	bus.Subscribe("service.*", s.HandleService)
	bus.Subscribe("warranty.*", s.HandleWarranty)
	bus.Subscribe("vehicle.*", s.HandleVehicle)
	bus.Subscribe(events.CustomerMerged, s.HandleCustomerMerged)
}

// HandleService refreshes the service document; a new service may link
// the customer to the organization, so the customer's vehicles (whose
// organization_ids follow those links) are refreshed too.
func (s *Sync) HandleService(ctx context.Context, ev events.Event) error {
	id := entityUUID(ev, "service", "service_uuid")
	if id == "" {
		return nil
	}
	s.idx.EnqueueUpsert(ctx, searchengine.SpecServices, id)
	if ev.Name == events.ServiceCreated {
		if uid, ok := payloadInt(ev.Payload, "customer_id"); ok {
			s.refreshUser(ctx, uid, false)
		}
	}
	return nil
}

// HandleWarranty refreshes the warranty document (created, expired,
// voided, holder changed ...).
func (s *Sync) HandleWarranty(ctx context.Context, ev events.Event) error {
	if id := entityUUID(ev, "warranty", "warranty_uuid"); id != "" {
		s.idx.EnqueueUpsert(ctx, searchengine.SpecWarranties, id)
	}
	return nil
}

// HandleVehicle refreshes the vehicle document: record events carry the
// vehicle as entity, transfer events carry it in the payload (a completed
// transfer changes the owner).
func (s *Sync) HandleVehicle(ctx context.Context, ev events.Event) error {
	if id := entityUUID(ev, "vehicle", "vehicle_uuid"); id != "" {
		s.idx.EnqueueUpsert(ctx, searchengine.SpecVehicles, id)
	}
	return nil
}

// HandleCustomerMerged refreshes every record that moved to the target
// customer (services, vehicles, warranties).
func (s *Sync) HandleCustomerMerged(ctx context.Context, ev events.Event) error {
	if uid, ok := payloadInt(ev.Payload, "target_user_id"); ok {
		s.refreshUser(ctx, uid, true)
	}
	return nil
}

func (s *Sync) refreshUser(ctx context.Context, userID int64, all bool) {
	if s.q == nil || userID == 0 {
		return
	}
	row, err := s.q.ListSearchUuidsByUserID(ctx, userID)
	if err != nil {
		s.log.Warn("search_sync_user_records_failed", "user_id", userID, "error", err)
		return
	}
	for _, u := range row.VehicleUuids {
		s.idx.EnqueueUpsert(ctx, searchengine.SpecVehicles, u.String())
	}
	if !all {
		return
	}
	for _, u := range row.ServiceUuids {
		s.idx.EnqueueUpsert(ctx, searchengine.SpecServices, u.String())
	}
	for _, u := range row.WarrantyUuids {
		s.idx.EnqueueUpsert(ctx, searchengine.SpecWarranties, u.String())
	}
}

// entityUUID returns the event entity uuid when the entity type matches,
// otherwise the uuid in payload[key].
func entityUUID(ev events.Event, entityType, key string) string {
	if ev.EntityType == entityType && ev.EntityUUID != nil && *ev.EntityUUID != uuid.Nil {
		return ev.EntityUUID.String()
	}
	if raw, ok := ev.Payload[key].(string); ok {
		if u, err := uuid.Parse(raw); err == nil && u != uuid.Nil {
			return u.String()
		}
	}
	return ""
}

// payloadInt reads an integer payload value in any of the shapes it takes
// before and after the outbox JSON round trip.
func payloadInt(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, v != 0
	case int:
		return int64(v), v != 0
	case int32:
		return int64(v), v != 0
	case float64:
		return int64(v), v != 0
	case json.Number:
		n, err := v.Int64()
		return n, err == nil && n != 0
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil && n != 0
	}
	return 0, false
}
