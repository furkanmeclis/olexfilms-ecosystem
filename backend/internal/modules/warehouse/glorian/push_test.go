package glorian_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-270: listener and outbound helpers without a database.

type fakePushQuerier struct {
	links map[int64]db.GetProductPushLinkRow
	conns map[int64]bool // brand id -> has a glorian connection
}

func (f fakePushQuerier) GetProductPushLink(_ context.Context, id int64) (db.GetProductPushLinkRow, error) {
	l, ok := f.links[id]
	if !ok {
		return l, pgx.ErrNoRows
	}
	return l, nil
}

func (f fakePushQuerier) GetIntegrationConnectionByKey(_ context.Context, arg db.GetIntegrationConnectionByKeyParams) (db.IntegrationConnection, error) {
	if f.conns[arg.BrandID] {
		return db.IntegrationConnection{ID: 7, BrandID: arg.BrandID, Key: arg.Key}, nil
	}
	return db.IntegrationConnection{}, pgx.ErrNoRows
}

const (
	glorianBrand   = 2
	olexBrand      = 1
	linkedProduct  = 100
	localProduct   = 101
	olexProduct    = 200
	pushConnection = 7
)

func newFakeListener(q *recordingQueue, now time.Time) (*events.MemoryBus, *glorian.Listener) {
	fq := fakePushQuerier{
		links: map[int64]db.GetProductPushLinkRow{
			linkedProduct: {ID: linkedProduct, BrandID: glorianBrand, ConnectionID: pgtype.Int8{Int64: pushConnection, Valid: true}, ExternalID: pgtype.Text{String: "10", Valid: true}},
			localProduct:  {ID: localProduct, BrandID: glorianBrand},
			olexProduct:   {ID: olexProduct, BrandID: olexBrand},
		},
		conns: map[int64]bool{glorianBrand: true},
	}
	bus := events.NewBus(nil)
	l := glorian.NewListener(fq, q, nil).WithClock(func() time.Time { return now })
	l.Register(bus)
	return bus, l
}

// Outbox payloads arrive JSON-decoded (numbers as float64).
func stockEvent(name string, movementID, productID int64) events.Event {
	return events.New(name).WithPayload(map[string]any{
		"movement_id": float64(movementID), "product_id": float64(productID), "barcode": fmt.Sprintf("B%d", movementID),
	})
}

func TestListenerDebouncesPlacementsIntoOnePush(t *testing.T) {
	q := &recordingQueue{}
	now := time.Date(2026, 10, 3, 12, 0, 1, 0, time.UTC)
	bus, _ := newFakeListener(q, now)
	ctx := context.Background()
	for i, name := range []string{events.StockEntry, events.StockEntry, events.StockPlacement} {
		if err := bus.Publish(ctx, stockEvent(name, int64(i+1), linkedProduct)); err != nil {
			t.Fatal(err)
		}
	}
	if len(q.tasks) != 1 || q.tasks[0].Type() != queue.TaskGlorianPushBarcodes {
		t.Fatalf("tasks = %d; want one push task", len(q.tasks))
	}
	if want := queue.GlorianPushBarcodesTaskID(pushConnection, now); !q.ids[want] {
		t.Fatalf("task ids = %v; want %s", q.ids, want)
	}
}

func TestListenerSkipsUnlinkedAndOlexProducts(t *testing.T) {
	q := &recordingQueue{}
	bus, _ := newFakeListener(q, time.Now())
	ctx := context.Background()
	for _, ev := range []events.Event{
		stockEvent(events.StockEntry, 1, olexProduct),
		stockEvent(events.StockExternalOutbound, 2, olexProduct),
		stockEvent(events.StockEntry, 3, localProduct), // glorian, not synced: held log only
		stockEvent(events.StockConsumption, 4, linkedProduct),
	} {
		if err := bus.Publish(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if len(q.tasks) != 0 {
		t.Fatalf("tasks = %d; want none", len(q.tasks))
	}
}

func TestListenerPatchesExitTransferShipment(t *testing.T) {
	q := &recordingQueue{}
	bus, _ := newFakeListener(q, time.Now())
	ctx := context.Background()
	names := []string{events.StockExternalOutbound, events.StockTransferOut, events.StockOrderOut}
	for i, name := range names {
		if err := bus.Publish(ctx, stockEvent(name, int64(10+i), linkedProduct)); err != nil {
			t.Fatal(err)
		}
	}
	// A redelivered event finds its task id and enqueues nothing.
	if err := bus.Publish(ctx, stockEvent(events.StockTransferOut, 11, linkedProduct)); err != nil {
		t.Fatal(err)
	}
	if len(q.tasks) != len(names) {
		t.Fatalf("tasks = %d; want %d", len(q.tasks), len(names))
	}
	for i := range names {
		if q.tasks[i].Type() != queue.TaskGlorianPatchStockItem || !q.ids[queue.GlorianPatchStockItemTaskID(int64(10+i))] {
			t.Fatalf("task %d = %s ids %v", i, q.tasks[i].Type(), q.ids)
		}
	}
}

func TestPatchFor(t *testing.T) {
	cases := map[string]glorian.StockItemPatch{
		"external_outbound":       {Status: glorian.StockStatusExternalOutbound},
		"transfer_out":            {Status: glorian.StockStatusExternalOutbound, Location: glorian.StockLocationDealer},
		"order_out":               {Status: glorian.StockStatusExternalOutbound, Location: glorian.StockLocationDealer},
		"transfer_cancel_restore": {Status: glorian.StockStatusAvailable, Location: glorian.StockLocationCenter},
		"order_cancel_restore":    {Status: glorian.StockStatusAvailable, Location: glorian.StockLocationCenter},
	}
	for typ, want := range cases {
		if got, ok := glorian.PatchFor(typ); !ok || got != want {
			t.Errorf("%s = %+v %v; want %+v", typ, got, ok, want)
		}
	}
	for _, typ := range []string{"entry", "placement", "consumption", "transfer_in"} {
		if _, ok := glorian.PatchFor(typ); ok {
			t.Errorf("%s must not patch", typ)
		}
	}
}

func TestTaskError(t *testing.T) {
	if glorian.TaskError(nil) != nil {
		t.Fatal("nil")
	}
	if err := glorian.TaskError(&glorian.HeldError{Reason: glorian.HeldInactiveConnection}); err != nil {
		t.Fatalf("held = %v; want done", err)
	}
	if !errors.Is(&glorian.HeldError{Reason: glorian.HeldInactiveConnection}, glorian.ErrInactiveConnection) {
		t.Fatal("inactive hold must match ErrInactiveConnection")
	}
	for _, err := range []error{glorian.ErrServer, glorian.ErrTransport, glorian.ErrRateLimited, glorian.ErrNotFound, errors.New("db down")} {
		got := glorian.TaskError(fmt.Errorf("x: %w", err))
		if got == nil || errors.Is(got, asynq.SkipRetry) {
			t.Errorf("%v = %v; want retry", err, got)
		}
	}
	for _, err := range []error{glorian.ErrConflict, glorian.ErrValidation, glorian.ErrUnauthorized, glorian.ErrInvalidInput} {
		got := glorian.TaskError(fmt.Errorf("x: %w", err))
		if !errors.Is(got, asynq.SkipRetry) || !errors.Is(got, err) {
			t.Errorf("%v = %v; want SkipRetry", err, got)
		}
	}
}

func TestPushOptsDebounceWindow(t *testing.T) {
	a := time.Date(2026, 10, 3, 12, 0, 1, 0, time.UTC)
	b := a.Add(8 * time.Second)
	c := a.Add(9 * time.Second)
	if queue.GlorianPushBarcodesTaskID(7, a) != queue.GlorianPushBarcodesTaskID(7, b) {
		t.Fatal("same window must share the task id")
	}
	if queue.GlorianPushBarcodesTaskID(7, a) == queue.GlorianPushBarcodesTaskID(7, c) {
		t.Fatal("next window must get a new task id")
	}
	if queue.GlorianPushBarcodesTaskID(7, a) == queue.GlorianPushBarcodesTaskID(8, a) {
		t.Fatal("connections must not share a task id")
	}
	var retry int
	for _, o := range queue.GlorianPushBarcodesOpts(7, a) {
		if o.Type() == asynq.MaxRetryOpt {
			retry = o.Value().(int)
		}
	}
	if retry == 0 {
		t.Fatal("push task must be retried")
	}
}
