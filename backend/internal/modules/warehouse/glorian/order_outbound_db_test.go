package glorian_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-271: the order outbound against the fake hub and the migrated
// database (CI sets TEST_DATABASE_URL), in the pull fixture's rolled-back
// transaction.

// orderCase is a center -> distributor order with assigned serial units.
type orderCase struct {
	buyer  db.Organization
	order  db.Order
	units  pushUnits
	phone  string
	dealer string
}

// distributor creates a child distributor of org with phone.
func (f *pullFixture) distributor(t *testing.T, org db.Organization, phone string) db.Organization {
	t.Helper()
	brand, err := f.q.GetBrandByID(f.ctx, org.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t271-dist-%s-%d", f.suffix, time.Now().UnixNano()), Name: f.name("T271 Dist"), Status: "active",
		Phone:          phone,
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: org.ID, Valid: true},
		BrandID: org.BrandID, Currency: brand.Currency, Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	return o
}

// party links phone to hub dealer remoteID on the fixture connection.
func (f *pullFixture) party(t *testing.T, remoteID, phone string) {
	t.Helper()
	if _, err := f.q.UpsertIntegrationExternalParty(f.ctx, db.UpsertIntegrationExternalPartyParams{
		OrganizationID: f.conn.OrganizationID, BrandID: f.conn.BrandID, ConnectionID: f.conn.ID,
		RemoteID: remoteID, Name: f.name("T271 Bayi " + remoteID),
		PhoneE164: pgtype.Text{String: phone, Valid: true}, Active: true,
	}); err != nil {
		t.Fatalf("party: %v", err)
	}
}

func (f *pullFixture) phone(n int) string {
	return fmt.Sprintf("+90555%07d", (time.Now().UnixNano()/1000+int64(n))%10_000_000)
}

// newOrderCase builds a ready order of seller with n assigned units of a
// product (linked: synced from the fixture connection). linkParty adds the
// buyer's hub dealer; the hub knows the barcodes as available at center.
func (f *pullFixture) newOrderCase(t *testing.T, seller db.Organization, n int, linked, linkParty bool) orderCase {
	t.Helper()
	c := orderCase{phone: f.phone(n), dealer: "1"}
	c.buyer = f.distributor(t, seller, c.phone)
	if linkParty {
		f.party(t, c.dealer, c.phone)
	}
	c.units = f.pushUnits(t, seller, n, linked)
	brand, err := f.q.GetBrandByID(f.ctx, seller.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	c.order, err = f.q.CreateOrder(f.ctx, db.CreateOrderParams{
		SellerOrgID: seller.ID, BrandID: seller.BrandID, BuyerOrgID: c.buyer.ID, Currency: brand.Currency,
		Note: pgtype.Text{String: "T271 sipariş", Valid: true},
	})
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	var price, total pgtype.Numeric
	_ = price.Scan("10.0000")
	_ = total.Scan(fmt.Sprintf("%d.00", 10*n))
	item, err := f.q.CreateOrderItem(f.ctx, db.CreateOrderItemParams{
		OrderID: c.order.ID, ProductID: c.units.product.ID, Quantity: pgtype.Int4{Int32: int32(n), Valid: true},
		UnitPrice: price, PriceSource: "list", LineTotal: total,
	})
	if err != nil {
		t.Fatalf("order item: %v", err)
	}
	var stock []fake.Row
	for i, u := range c.units.units {
		if _, err := f.q.CreateOrderItemUnit(f.ctx, db.CreateOrderItemUnitParams{
			OrderItemID: item.ID, UnitID: u.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
		}); err != nil {
			t.Fatalf("assign unit: %v", err)
		}
		stock = append(stock, fake.Row{
			"id": fmt.Sprintf("%d", 5000+i), "barcode": u.Barcode, "product_id": pushRemoteProduct, "dealer_id": nil,
			"status": glorian.StockStatusAvailable, "location": glorian.StockLocationCenter, "updated_at": "2026-09-01T12:00:00+00:00",
		})
	}
	f.srv.SetFixture(fake.FixtureStockItems, stock)
	c.order = f.setStatus(t, c.order, "ready")
	return c
}

func (f *pullFixture) setStatus(t *testing.T, o db.Order, status string) db.Order {
	t.Helper()
	o, err := f.q.UpdateOrderStatus(f.ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: status})
	if err != nil {
		t.Fatalf("status %s: %v", status, err)
	}
	return o
}

func (f *pullFixture) orders() *glorian.OrderOutbounder {
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	return glorian.NewOrderOutbounder(f.q, f.box, factory, nil)
}

func (f *pullFixture) outbounds(t *testing.T, orderID int64) []db.OrderOutbound {
	t.Helper()
	rows, err := f.q.ListOrderOutboundsByOrder(f.ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// orderPosts lists the POST paths sent to the hub's /orders routes.
func (f *pullFixture) orderPosts() []string {
	var out []string
	for _, r := range f.srv.Requests() {
		if r.Method == http.MethodPost && (r.Path == "/orders" || len(r.Path) > len("/orders/") && r.Path[:len("/orders/")] == "/orders/") {
			out = append(out, r.Path)
		}
	}
	return out
}

// dispatchOrders feeds events through the order outbound listener.
func (f *pullFixture) dispatchOrders(t *testing.T, evs ...events.Event) *recordingQueue {
	t.Helper()
	q := &recordingQueue{}
	bus := events.NewBus(nil)
	glorian.NewOrderListener(f.q, q, nil).Register(bus)
	for _, ev := range evs {
		if err := bus.Publish(f.ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	return q
}

func samePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The same external_reference sent twice leaves one order on the hub: a
// second run finds it by reference and creates nothing, and a replayed
// POST /orders returns the existing order.
func TestOrderOutboundOneRemoteOrderPerReference(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 2, true, true)
	o := f.orders()
	if err := o.SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := o.SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	obs := f.outbounds(t, c.order.ID)
	if len(obs) != 1 || obs[0].State != glorian.OutboundSent || obs[0].ExternalReference != c.order.Uuid.String() {
		t.Fatalf("outbounds = %+v", obs)
	}
	posts := f.srv.RequestsTo(http.MethodPost, "/orders")
	if len(posts) != 1 {
		t.Fatalf("POST /orders = %d; want 1", len(posts))
	}
	var body glorian.CreateOrderInput
	if err := posts[0].JSON(&body); err != nil {
		t.Fatal(err)
	}
	if body.DealerID != c.dealer || body.ExternalReference != c.order.Uuid.String() || body.AutoPrepare == nil || !*body.AutoPrepare ||
		len(body.Items) != 1 || body.Items[0].ProductID != pushRemoteProduct || body.Items[0].Quantity != 2 || len(body.Items[0].Barcodes) != 2 ||
		body.Notes == nil || *body.Notes != "T271 sipariş" {
		t.Fatalf("create body = %+v", body)
	}
	// Replaying the create with the same reference returns the same order.
	client, err := glorian.NewHTTPClient(glorian.Options{BaseURL: f.srv.URL, APIKey: fake.APIKey, HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	again, err := client.CreateOrder(f.ctx, body)
	if err != nil || again.ID != "1" {
		t.Fatalf("replayed create = %+v, %v", again, err)
	}
	if _, ok := f.srv.Order("2"); ok {
		t.Fatal("the hub must hold one order for the reference")
	}
	if row, _ := f.srv.Order("1"); row["status"] != glorian.RemoteProcessing {
		t.Fatalf("hub order = %v", row)
	}
	run := latestRun(t, f.runs(t), glorian.KindOutbound)
	if run.Status != glorian.RunSucceeded {
		t.Fatalf("outbound run = %+v", run)
	}
}

// Transitions go out in order: create, ship, receive; an order that jumped
// ahead gets every missing step in one run, in order.
func TestOrderOutboundTransitionsInOrder(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 1, true, true)
	o := f.orders()
	for _, status := range []string{"", "shipped", "received"} {
		if status != "" {
			c.order = f.setStatus(t, c.order, status)
		}
		if err := o.SyncOrder(f.ctx, c.order.ID); err != nil {
			t.Fatalf("sync %s: %v", status, err)
		}
	}
	want := []string{"/orders", "/orders/1/ship", "/orders/1/receive"}
	if got := f.orderPosts(); !samePaths(got, want) {
		t.Fatalf("posts = %v; want %v", got, want)
	}
	if row, _ := f.srv.Order("1"); row["status"] != glorian.RemoteDelivered || row["cargo_company"] != glorian.DefaultCargoCompany {
		t.Fatalf("hub order = %v", row)
	}

	// Late: the order is already received when the first task runs.
	late := f.newOrderCase(t, f.glorian, 1, true, true)
	late.order = f.setStatus(t, late.order, "shipped")
	late.order = f.setStatus(t, late.order, "received")
	before := len(f.orderPosts())
	if err := o.SyncOrder(f.ctx, late.order.ID); err != nil {
		t.Fatalf("late sync: %v", err)
	}
	want = []string{"/orders", "/orders/2/ship", "/orders/2/receive"}
	if got := f.orderPosts()[before:]; !samePaths(got, want) {
		t.Fatalf("late posts = %v; want %v", got, want)
	}
	if obs := f.outbounds(t, late.order.ID); len(obs) != 1 || obs[0].State != glorian.OutboundSent || obs[0].Attempts != 1 {
		t.Fatalf("late outbound = %+v", obs)
	}
}

// Without a dealer link the outbound is held (no request); once the link
// exists the replay sends it.
func TestOrderOutboundHeldThenReplayed(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 1, true, false)
	o := f.orders()
	err := o.SyncOrder(f.ctx, c.order.ID)
	if reason, held := glorian.IsHeld(err); !held || reason != glorian.HeldMissingCustomer {
		t.Fatalf("sync err = %v; want held missing_customer_link", err)
	}
	if glorian.TaskError(err) != nil {
		t.Fatal("a hold is not retried by Asynq")
	}
	obs := f.outbounds(t, c.order.ID)
	if len(obs) != 1 || obs[0].State != glorian.OutboundHeld || obs[0].HeldReason.String != glorian.HeldMissingCustomer {
		t.Fatalf("outbound = %+v", obs)
	}
	if posts := f.orderPosts(); len(posts) != 0 {
		t.Fatalf("held outbound sent %v", posts)
	}
	run := latestRun(t, f.runs(t), glorian.KindOutbound)
	if run.Status != glorian.RunFailed || run.Error.String != "held: "+glorian.HeldMissingCustomer {
		t.Fatalf("held run = %+v", run)
	}

	// Still no link: the replay keeps it held.
	if res, err := o.ReplayHeld(f.ctx, f.conn.ID); err != nil || res.StillHeld != 1 || res.Replayed != 0 {
		t.Fatalf("replay without link = %+v, %v", res, err)
	}
	f.party(t, c.dealer, c.phone)
	res, err := o.ReplayHeld(f.ctx, f.conn.ID)
	if err != nil || res.Replayed != 1 || res.StillHeld != 0 {
		t.Fatalf("replay = %+v, %v", res, err)
	}
	obs = f.outbounds(t, c.order.ID)
	if obs[0].State != glorian.OutboundSent || obs[0].HeldReason.Valid {
		t.Fatalf("replayed outbound = %+v", obs[0])
	}
	if posts := f.orderPosts(); !samePaths(posts, []string{"/orders"}) {
		t.Fatalf("posts = %v", posts)
	}
}

// An inactive connection holds the outbound (inactive_connection).
func TestOrderOutboundHeldOnInactiveConnection(t *testing.T) {
	f := newPullFixture(t, false)
	c := f.newOrderCase(t, f.glorian, 1, true, true)
	err := f.orders().SyncOrder(f.ctx, c.order.ID)
	if reason, held := glorian.IsHeld(err); !held || reason != glorian.HeldInactiveConnection {
		t.Fatalf("sync err = %v; want held inactive_connection", err)
	}
	obs := f.outbounds(t, c.order.ID)
	if len(obs) != 1 || obs[0].State != glorian.OutboundHeld || obs[0].HeldReason.String != glorian.HeldInactiveConnection {
		t.Fatalf("outbound = %+v", obs)
	}
	if len(f.srv.Requests()) != 0 {
		t.Fatal("an inactive connection makes no request")
	}
}

// After a cancel the hub order is cancelled; a later ship is refused
// without a request and recorded as a failure.
func TestOrderOutboundShipAfterCancelRejected(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 1, true, true)
	o := f.orders()
	if err := o.SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	c.order = f.setStatus(t, c.order, "cancelled")
	if err := o.SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("cancel sync: %v", err)
	}
	if row, _ := f.srv.Order("1"); row["status"] != glorian.RemoteCancelled {
		t.Fatalf("hub order = %v", row)
	}
	obs := f.outbounds(t, c.order.ID)
	if obs[0].State != glorian.OutboundCancelled {
		t.Fatalf("outbound = %+v", obs[0])
	}
	// The hub releases the reservation on cancel.
	if row, _ := f.srv.StockItem(c.units.units[0].Barcode); row["status"] != glorian.StockStatusAvailable {
		t.Fatalf("hub stock = %v", row)
	}

	// Force a ship onto the cancelled order (the local state machine
	// refuses it; this checks the outbound guard) and replay the outbound.
	if _, err := f.tx.Exec(f.ctx, `UPDATE orders SET status = 'shipped', cancelled_at = NULL WHERE id = $1`, c.order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.UpdateOrderOutboundState(f.ctx, db.UpdateOrderOutboundStateParams{ID: obs[0].ID, State: glorian.OutboundSent}); err != nil {
		t.Fatal(err)
	}
	err := o.ReplayOutbound(f.ctx, obs[0].ID)
	if !errors.Is(err, glorian.ErrOrderTransition) || !glorian.Permanent(err) {
		t.Fatalf("ship after cancel err = %v; want permanent ErrOrderTransition", err)
	}
	if n := len(f.srv.RequestsTo(http.MethodPost, "/orders/1/ship")); n != 0 {
		t.Fatalf("ship requests = %d; want none", n)
	}
	obs = f.outbounds(t, c.order.ID)
	if obs[0].State != glorian.OutboundFailed || !obs[0].LastError.Valid {
		t.Fatalf("outbound = %+v", obs[0])
	}
}

// A cancel before the order reached the hub sends nothing.
func TestOrderOutboundCancelledBeforeSend(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.glorian, 1, true, true)
	c.order = f.setStatus(t, c.order, "cancelled")
	if err := f.orders().SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if obs := f.outbounds(t, c.order.ID); len(obs) != 1 || obs[0].State != glorian.OutboundCancelled {
		t.Fatalf("outbound = %+v", obs)
	}
	if posts := f.orderPosts(); len(posts) != 0 {
		t.Fatalf("posts = %v", posts)
	}
}

// An Olex order produces no outbound, no task and no request.
func TestOlexOrderProducesNoOutbound(t *testing.T) {
	f := newPullFixture(t, true)
	c := f.newOrderCase(t, f.olex, 1, false, false)
	if err := f.orders().SyncOrder(f.ctx, c.order.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if obs := f.outbounds(t, c.order.ID); len(obs) != 0 {
		t.Fatalf("outbounds = %+v", obs)
	}
	if len(f.srv.Requests()) != 0 {
		t.Fatal("an Olex order makes no request")
	}
	id := c.order.ID
	ev := events.New(events.OrdersReady).WithEntity("order", &id, &c.order.Uuid).
		WithPayload(map[string]any{"brand_id": float64(c.order.BrandID), "status": "ready"})
	if q := f.dispatchOrders(t, ev); len(q.tasks) != 0 {
		t.Fatalf("olex order enqueued %d tasks", len(q.tasks))
	}
	// The glorian order of the same event shape is enqueued.
	g := f.newOrderCase(t, f.glorian, 1, true, true)
	gid := g.order.ID
	gev := events.New(events.OrdersReady).WithEntity("order", &gid, &g.order.Uuid).
		WithPayload(map[string]any{"brand_id": float64(g.order.BrandID), "status": "ready"})
	if q := f.dispatchOrders(t, gev); len(q.tasks) != 1 || q.tasks[0].Type() != queue.TaskGlorianOrderOutbound {
		t.Fatalf("glorian order enqueued %d tasks", len(q.tasks))
	}
}
