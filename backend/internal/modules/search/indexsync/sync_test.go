package indexsync

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

type rec struct{ upserts []string }

func (r *rec) EnqueueUpsert(_ context.Context, spec, id string) {
	r.upserts = append(r.upserts, spec+"/"+id)
}

type store struct {
	row  db.ListSearchUuidsByUserIDRow
	user int64
}

func (s *store) ListSearchUuidsByUserID(_ context.Context, id int64) (db.ListSearchUuidsByUserIDRow, error) {
	s.user = id
	return s.row, nil
}

func ev(name, entity string, id uuid.UUID, payload map[string]any) events.Event {
	n := int64(1)
	return events.New(name).WithEntity(entity, &n, &id).WithPayload(payload)
}

// TEC-209: every service / warranty / vehicle event the outbox publishes
// refreshes the touched document.
func TestSyncRoutesEvents(t *testing.T) {
	r := &rec{}
	veh, svc, war := uuid.New(), uuid.New(), uuid.New()
	st := &store{row: db.ListSearchUuidsByUserIDRow{
		ServiceUuids: []uuid.UUID{svc}, VehicleUuids: []uuid.UUID{veh}, WarrantyUuids: []uuid.UUID{war},
	}}
	bus := events.NewBus(nil)
	Register(bus, st, r, nil)
	ctx := context.Background()

	s1, w1, v1, t1 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_ = bus.Publish(ctx, ev(events.ServiceCompleted, "service", s1, map[string]any{"customer_id": int64(3)}))
	_ = bus.Publish(ctx, ev(events.WarrantyVoided, "warranty", w1, nil))
	_ = bus.Publish(ctx, ev(events.VehicleUpdated, "vehicle", v1, nil))
	_ = bus.Publish(ctx, ev(events.VehicleTransferCompleted, "vehicle_transfer", t1, map[string]any{"vehicle_uuid": veh.String()}))
	want := []string{"services/" + s1.String(), "warranties/" + w1.String(), "vehicles/" + v1.String(), "vehicles/" + veh.String()}
	if !slices.Equal(r.upserts, want) {
		t.Fatalf("upserts = %v, want %v", r.upserts, want)
	}

	// service.created also refreshes the customer's vehicles (their
	// organization_ids follow the customer links); float64 after JSON.
	r.upserts = nil
	_ = bus.Publish(ctx, ev(events.ServiceCreated, "service", s1, map[string]any{"customer_id": float64(3)}))
	if st.user != 3 || !slices.Equal(r.upserts, []string{"services/" + s1.String(), "vehicles/" + veh.String()}) {
		t.Fatalf("created: user %d upserts %v", st.user, r.upserts)
	}

	// customer.merged refreshes everything the target now owns.
	r.upserts = nil
	_ = bus.Publish(ctx, ev(events.CustomerMerged, "user", uuid.New(), map[string]any{"target_user_id": int64(8)}))
	if st.user != 8 || len(r.upserts) != 3 {
		t.Fatalf("merged: user %d upserts %v", st.user, r.upserts)
	}
}

type disabled struct{ rec }

func (disabled) Enabled() bool { return false }

func TestRegisterSkipsDisabledIndexer(t *testing.T) {
	bus := events.NewBus(nil)
	Register(bus, nil, &disabled{}, nil)
	if bus.HandlerCount(events.ServiceCreated) != 0 {
		t.Fatal("disabled indexer must not subscribe")
	}
	Register(bus, nil, nil, nil)
	if bus.HandlerCount(events.WarrantyCreated) != 0 {
		t.Fatal("nil indexer must not subscribe")
	}
}
