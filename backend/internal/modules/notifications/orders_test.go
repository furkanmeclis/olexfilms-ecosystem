package notifications

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-1028: orders.warehouse_new (as the outbox redelivers it) becomes a
// dispatch to the warehouse staff with the order link payload; without
// recipients nothing is sent.
func TestOrderWarehouseNewDispatch(t *testing.T) {
	ev := events.New(events.OrdersWarehouseNew).WithPayload(map[string]any{
		"order_uuid": "o-1", "order_no": "ORD-00000042", "status": "approved", "brand_id": int64(3),
		"buyer_name": "Kuzey Oto", "warehouse_uuid": "w-1", "warehouse_name": "Merkez Depo",
		"notify_user_ids": []any{float64(7), float64(9)},
	})
	in, ok := orderWarehouseNewDispatch(ev)
	if !ok {
		t.Fatal("dispatch skipped")
	}
	if in.EventCode != catalog.EventOrderWarehouseNew || len(in.UserIDs) != 2 || in.UserIDs[0] != 7 || in.UserIDs[1] != 9 {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 3 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["order_no"] != "ORD-00000042" || in.Vars["warehouse_name"] != "Merkez Depo" || in.Vars["buyer_name"] != "Kuzey Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if in.Payload["order_uuid"] != "o-1" || in.Payload["warehouse_uuid"] != "w-1" {
		t.Fatalf("payload = %v", in.Payload)
	}
	if _, ok := orderWarehouseNewDispatch(events.New(events.OrdersWarehouseNew).WithPayload(map[string]any{
		"order_uuid": "o-1", "notify_user_ids": []any{},
	})); ok {
		t.Fatal("an event without recipients must be skipped")
	}
}

// orders.shipped / delivered / received with recipients become dispatches
// of their catalog codes with the order link payload; without recipients
// nothing is sent.
func TestOrderStatusDispatch(t *testing.T) {
	for name, code := range OrderStatusEventCodes {
		ev := events.New(name).WithPayload(map[string]any{
			"order_uuid": "o-1", "order_no": "ORD-00000042", "status": "shipped", "brand_id": int64(3),
			"seller_name": "Olex Merkez", "buyer_name": "Kuzey Oto", "tracking_no": "TR1",
			"notify_user_ids": []any{float64(5)},
		})
		in, ok := orderStatusDispatcher(code)(ev)
		if !ok {
			t.Fatalf("%s: dispatch skipped", name)
		}
		if in.EventCode != code || len(in.UserIDs) != 1 || in.UserIDs[0] != 5 {
			t.Fatalf("%s: dispatch = %+v", name, in)
		}
		if in.Vars["seller_name"] != "Olex Merkez" || in.Vars["buyer_name"] != "Kuzey Oto" || in.Payload["order_uuid"] != "o-1" ||
			in.Payload["tracking_no"] != "TR1" {
			t.Fatalf("%s: vars %v payload %v", name, in.Vars, in.Payload)
		}
		if _, ok := orderStatusDispatcher(code)(events.New(name).WithPayload(map[string]any{"order_uuid": "o-1"})); ok {
			t.Fatalf("%s: an event without recipients must be skipped", name)
		}
	}
}
