package notifications

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
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

// TEC-1224: the order outbox payload carries the total (and the approval
// carries the rate snapshot); the notifications built from it, which reach
// warehouse staff, carry neither.
func TestOrderDispatchesDropMoney(t *testing.T) {
	payload := map[string]any{
		"order_uuid": "o-1", "order_no": "ORD-1", "status": "approved", "brand_id": int64(3),
		"currency": "EUR", "total": "987654.32", "rate_snapshot": map[string]any{"EUR": "41.12"},
		"notify_user_ids": []any{float64(5)},
	}
	dispatchers := map[string]func(events.Event) (notifmodel.DispatchInput, bool){
		events.OrdersWarehouseNew: orderWarehouseNewDispatch,
		"created_by_seller":       orderCreatedBySellerDispatch,
	}
	for name, code := range OrderStatusEventCodes {
		dispatchers[name] = orderStatusDispatcher(code)
	}
	for name, fn := range dispatchers {
		in, ok := fn(events.New(name).WithPayload(payload))
		if !ok {
			t.Fatalf("%s: dispatch skipped", name)
		}
		for _, k := range []string{"total", "rate_snapshot", "currency"} {
			if _, leak := in.Payload[k]; leak {
				t.Fatalf("%s: payload carries %s: %v", name, k, in.Payload)
			}
			if _, leak := in.Vars[k]; leak {
				t.Fatalf("%s: vars carry %s: %v", name, k, in.Vars)
			}
		}
	}
}
