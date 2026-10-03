package glorian_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
)

// TEC-271: order outbound planning and listener without a database.

func TestPlanOrderActions(t *testing.T) {
	ship, receive, cancel := glorian.OrderActionShip, glorian.OrderActionReceive, glorian.OrderActionCancel
	ok := []struct {
		from, to string
		want     []glorian.OrderAction
	}{
		{glorian.RemoteProcessing, glorian.RemoteProcessing, nil},
		{glorian.RemoteProcessing, glorian.RemoteShipped, []glorian.OrderAction{ship}},
		{glorian.RemoteProcessing, glorian.RemoteDelivered, []glorian.OrderAction{ship, receive}},
		{glorian.RemoteShipped, glorian.RemoteDelivered, []glorian.OrderAction{receive}},
		{glorian.RemoteShipped, glorian.RemoteShipped, nil},
		// The hub is already ahead: nothing to send.
		{glorian.RemoteDelivered, glorian.RemoteShipped, nil},
		{glorian.RemoteProcessing, glorian.RemoteCancelled, []glorian.OrderAction{cancel}},
		{glorian.RemoteShipped, glorian.RemoteCancelled, []glorian.OrderAction{cancel}},
		{glorian.RemoteCancelled, glorian.RemoteCancelled, nil},
	}
	for _, c := range ok {
		got, err := glorian.PlanOrderActions(c.from, c.to)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s -> %s = %v, %v; want %v", c.from, c.to, got, err, c.want)
		}
	}
	// Ship after cancel, cancel after delivery, an unprepared hub order.
	for _, c := range [][2]string{
		{glorian.RemoteCancelled, glorian.RemoteShipped},
		{glorian.RemoteCancelled, glorian.RemoteDelivered},
		{glorian.RemoteCancelled, glorian.RemoteProcessing},
		{glorian.RemoteDelivered, glorian.RemoteCancelled},
		{glorian.RemotePending, glorian.RemoteShipped},
	} {
		_, err := glorian.PlanOrderActions(c[0], c[1])
		if !errors.Is(err, glorian.ErrOrderTransition) || !glorian.Permanent(err) {
			t.Errorf("%s -> %s err = %v; want permanent ErrOrderTransition", c[0], c[1], err)
		}
	}
}

func orderEvent(name string, orderID, brandID int64, status string) events.Event {
	id, uid := orderID, uuid.New()
	return events.New(name).WithEntity("order", &id, &uid).
		WithPayload(map[string]any{"brand_id": float64(brandID), "status": status})
}

func TestOrderListenerEnqueuesGlorianTransitionsOnly(t *testing.T) {
	q := &recordingQueue{}
	bus := events.NewBus(nil)
	glorian.NewOrderListener(fakePushQuerier{conns: map[int64]bool{glorianBrand: true}}, q, nil).Register(bus)
	ctx := context.Background()
	for _, ev := range []events.Event{
		orderEvent(events.OrdersReady, 1, glorianBrand, "ready"),
		orderEvent(events.OrdersShipped, 1, glorianBrand, "shipped"),
		orderEvent(events.OrdersShipped, 1, glorianBrand, "shipped"), // redelivered
		orderEvent(events.OrdersReceived, 1, glorianBrand, "received"),
		orderEvent(events.OrdersCancelled, 2, glorianBrand, "cancelled"),
		// Olex: no glorian connection, nothing to send.
		orderEvent(events.OrdersReady, 3, olexBrand, "ready"),
		// Transitions the hub does not follow.
		orderEvent(events.OrdersApproved, 1, glorianBrand, "approved"),
		orderEvent(events.OrdersCancelRequested, 1, glorianBrand, "cancelling"),
	} {
		if err := bus.Publish(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if len(q.tasks) != 4 {
		t.Fatalf("tasks = %d; want 4", len(q.tasks))
	}
	for _, id := range []string{
		queue.GlorianOrderOutboundTaskID(1, "ready"), queue.GlorianOrderOutboundTaskID(1, "shipped"),
		queue.GlorianOrderOutboundTaskID(1, "received"), queue.GlorianOrderOutboundTaskID(2, "cancelled"),
	} {
		if !q.ids[id] {
			t.Errorf("missing task %s in %v", id, q.ids)
		}
	}
	for _, task := range q.tasks {
		if task.Type() != queue.TaskGlorianOrderOutbound {
			t.Errorf("task type = %s", task.Type())
		}
	}
}
