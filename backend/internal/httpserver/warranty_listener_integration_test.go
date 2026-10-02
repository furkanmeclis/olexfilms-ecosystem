package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// setWarrantyMonths sets the product's warranty period (NULL when months < 0).
func (it *itest) setWarrantyMonths(p db.Product, months int) {
	it.t.Helper()
	var v any
	if months >= 0 {
		v = months
	}
	if _, err := it.pool.Exec(context.Background(), `UPDATE products SET warranty_duration_months = $2 WHERE id = $1`, p.ID, v); err != nil {
		it.t.Fatalf("warranty months: %v", err)
	}
}

// directService writes a service with items and completes it straight in
// the database (the listener rules do not depend on the stock flow; the
// end-to-end test below goes through the API).
func (it *itest) directService(org db.Organization, cust db.User, veh db.Vehicle, seq int,
	items ...db.CreateServiceItemParams) db.Service {
	it.t.Helper()
	ctx := context.Background()
	svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T186-%s-%d", it.suffix[len(it.suffix)-10:], seq),
		OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
		CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
	})
	if err != nil {
		it.t.Fatalf("service: %v", err)
	}
	for _, arg := range items {
		arg.ServiceID = svc.ID
		if arg.AppliedParts == nil {
			arg.AppliedParts = []byte(`[]`)
		}
		if _, err := it.q.CreateServiceItem(ctx, arg); err != nil {
			it.t.Fatalf("service item: %v", err)
		}
	}
	svc, err = it.q.CompleteService(ctx, db.CompleteServiceParams{ID: svc.ID})
	if err != nil {
		it.t.Fatalf("complete: %v", err)
	}
	return svc
}

func completedEvent(svc db.Service) events.Event {
	id, u := svc.ID, svc.Uuid
	return events.New(events.ServiceCompleted).WithTenant(svc.OrganizationID).
		WithEntity("service", &id, &u).
		WithPayload(map[string]any{"service_id": svc.ID, "service_uuid": svc.Uuid.String()})
}

func (it *itest) serviceWarranties(serviceID, brandID int64) []db.Warranty {
	it.t.Helper()
	ws, err := it.q.ListWarrantiesByService(context.Background(), db.ListWarrantiesByServiceParams{
		ServiceID: serviceID, BrandID: brandID,
	})
	if err != nil {
		it.t.Fatal(err)
	}
	return ws
}

func (it *itest) createdEvents(serviceID int64) int {
	it.t.Helper()
	var n int
	if err := it.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM outbox_events
		WHERE event_name = 'warranty.created' AND (payload->'data'->>'service_id')::bigint = $1`, serviceID).Scan(&n); err != nil {
		it.t.Fatal(err)
	}
	return n
}

func meters(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var m pgtype.Numeric
	if err := m.Scan(s); err != nil {
		t.Fatal(err)
	}
	return m
}

// TEC-186 acceptance (listener rules): every eligible item of a completed
// service gets one active warranty with end_at = completion + N calendar
// months (month end clamp, end of day in the org zone) and one
// warranty.created event; a replayed event writes nothing; products with no
// period, external_outbound / external units and Glorian get none; a full
// unit that already has an active full warranty on the vehicle gets no
// second one while partial cuts of a roll always do.
func TestIntegrationWarrantyListener(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skipf("tzdata: %v", err)
	}
	center := it.brandCenter("olex")
	dist := it.org("t186-dist", "distributor", center)
	dealer := it.org("t186-dealer", "dealer", dist)
	cust, veh := it.svcCustomer(dealer, "t186-cust", "34W186"+it.suffix[len(it.suffix)-4:])

	p12 := it.product(center, "T186A")
	p1 := it.product(center, "T186M")
	p0 := it.product(center, "T186Z")
	pNull := it.product(center, "T186N")
	roll := it.product(center, "T186R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	it.setWarrantyMonths(p12, 12)
	it.setWarrantyMonths(p1, 1)
	it.setWarrantyMonths(p0, 0)
	it.setWarrantyMonths(roll, 24)

	c := it.stockChain()
	cLoc := c.location(center, "C186")
	u12 := c.unit(center, p12, 18601)
	u1 := c.unit(center, p1, 18602)
	u0 := c.unit(center, p0, 18603)
	uNull := c.unit(center, pNull, 18604)
	uOut := c.unit(center, p12, 18605)
	uRoll := c.rollUnit(center, roll, 18606, "50.00")
	// uOut left the system: entry, then external_outbound (holder's trash).
	c.post(ledger.TypeEntry, uOut, c.nextRef(), cLoc)
	func() {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := c.l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeExternalOutbound, UnitID: uOut.ID, Source: "test", RefType: "t186", RefID: c.nextRef(),
		}); err != nil {
			t.Fatalf("external_outbound: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	// uExt came from an external hub (source external, K2).
	uExt, err := it.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: center.BrandID, ProductID: p12.ID,
		Barcode: "T186X-" + it.suffix, UnitKind: ledger.KindSerial, Source: "external", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		t.Fatalf("external unit: %v", err)
	}

	full := func(p db.Product, u db.Unit) db.CreateServiceItemParams {
		return db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"}
	}
	cut := func(m string) db.CreateServiceItemParams {
		return db.CreateServiceItemParams{ProductID: roll.ID, UnitID: uRoll.ID, Kind: "partial", Meters: meters(t, m)}
	}
	listener := warrantymodule.NewListener(it.pool, it.q, "http://localhost:3000", nil)

	// 1. S1: one eligible piece, two cuts of the roll; no period (0 / NULL),
	// external_outbound and external units are skipped.
	s1 := it.directService(dealer, cust, veh, 1,
		full(p12, u12), full(p0, u0), full(pNull, uNull), full(p12, uOut), full(p12, uExt), cut("10"), cut("5"))
	if err := listener.HandleServiceCompleted(ctx, completedEvent(s1)); err != nil {
		t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(s1.ID, s1.BrandID)
	if len(ws) != 3 {
		t.Fatalf("S1 warranties = %d, want 3: %+v", len(ws), ws)
	}
	start := s1.CompletedAt.Time
	wantEnd := map[int64]time.Time{
		u12.ID:   warrantyusecase.EndAt(start, 12, ist),
		uRoll.ID: warrantyusecase.EndAt(start, 24, ist),
	}
	for _, w := range ws {
		if w.Status != "active" || w.HolderUserID != cust.ID || w.VehicleID != veh.ID ||
			w.OrganizationID != dealer.ID || w.BrandID != dealer.BrandID || !w.StartAt.Time.Equal(start) {
			t.Fatalf("S1 warranty = %+v", w)
		}
		if !w.EndAt.Time.Equal(wantEnd[w.UnitID]) {
			t.Fatalf("S1 warranty unit %d end_at = %s, want %s", w.UnitID, w.EndAt.Time.In(ist), wantEnd[w.UnitID].In(ist))
		}
		if h, m, s := w.EndAt.Time.In(ist).Clock(); h != 23 || m != 59 || s != 59 {
			t.Fatalf("end_at %s is not the end of the day in Istanbul", w.EndAt.Time.In(ist))
		}
		wantKind := "full"
		if w.UnitID == uRoll.ID {
			wantKind = "partial"
		}
		if w.ItemKind != wantKind {
			t.Fatalf("item kind = %s, want %s", w.ItemKind, wantKind)
		}
	}
	if n := it.createdEvents(s1.ID); n != 3 {
		t.Fatalf("S1 warranty.created = %d, want 3", n)
	}
	var withVerify int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events o JOIN warranties w
		ON w.uuid::text = o.payload->'data'->>'warranty_uuid'
		WHERE o.event_name = 'warranty.created' AND w.service_id = $1
		  AND o.payload->'data'->>'verify_url' = 'http://localhost:3000/garanti/' || w.public_code
		  AND (o.payload->'data'->>'holder_user_id')::bigint = $2`, s1.ID, cust.ID).Scan(&withVerify); err != nil {
		t.Fatal(err)
	}
	if withVerify != 3 {
		t.Fatalf("warranty.created payloads with verify_url / holder = %d", withVerify)
	}

	// 2. The same event again: no row, no event.
	res, err := listener.CreateForService(ctx, s1.ID)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(res.Created) != 0 || len(it.serviceWarranties(s1.ID, s1.BrandID)) != 3 || it.createdEvents(s1.ID) != 3 {
		t.Fatalf("replay created %d, rows %d, events %d", len(res.Created), len(it.serviceWarranties(s1.ID, s1.BrandID)), it.createdEvents(s1.ID))
	}
	for unitID, reason := range map[int64]string{
		u0.ID: warrantyusecase.SkipNoPeriod, uNull.ID: warrantyusecase.SkipNoPeriod,
		uOut.ID: warrantyusecase.SkipExternal, uExt.ID: warrantyusecase.SkipExternal,
	} {
		var itemID int64
		if err := it.pool.QueryRow(ctx, `SELECT id FROM service_items WHERE service_id = $1 AND unit_id = $2`, s1.ID, unitID).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		if got := res.Skipped[itemID]; got != reason {
			t.Fatalf("unit %d skip reason = %q, want %q", unitID, got, reason)
		}
	}

	// 3. S2 on the same vehicle: the piece already has an active full
	// warranty (skipped), a new cut of the roll gets its own.
	s2 := it.directService(dealer, cust, veh, 2, full(p12, u12), cut("3"))
	res, err = listener.CreateForService(ctx, s2.ID)
	if err != nil {
		t.Fatalf("S2: %v", err)
	}
	ws2 := it.serviceWarranties(s2.ID, s2.BrandID)
	if len(res.Created) != 1 || len(ws2) != 1 || ws2[0].UnitID != uRoll.ID || ws2[0].ItemKind != "partial" {
		t.Fatalf("S2 created %d, rows %+v", len(res.Created), ws2)
	}
	var activeFull int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM warranties WHERE vehicle_id = $1 AND unit_id = $2
		AND status = 'active' AND item_kind = 'full'`, veh.ID, u12.ID).Scan(&activeFull); err != nil {
		t.Fatal(err)
	}
	if activeFull != 1 || it.createdEvents(s2.ID) != 1 {
		t.Fatalf("active full warranties of the piece = %d, S2 events = %d", activeFull, it.createdEvents(s2.ID))
	}
	var rollWarranties int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM warranties WHERE unit_id = $1 AND status = 'active'`, uRoll.ID).Scan(&rollWarranties); err != nil {
		t.Fatal(err)
	}
	if rollWarranties != 3 {
		t.Fatalf("active partial warranties of the roll = %d, want 3", rollWarranties)
	}

	// 4. Month end clamp: completed 31 January 2028 (Istanbul) + 1 month =
	// 29 February 2028 (leap year), end of that day.
	s3 := it.directService(dealer, cust, veh, 3, full(p1, u1))
	jan31 := time.Date(2028, 1, 31, 12, 0, 0, 0, ist)
	if _, err := it.pool.Exec(ctx, `UPDATE services SET completed_at = $2 WHERE id = $1`, s3.ID, jan31); err != nil {
		t.Fatalf("completed_at: %v", err)
	}
	if _, err := listener.CreateForService(ctx, s3.ID); err != nil {
		t.Fatalf("S3: %v", err)
	}
	ws3 := it.serviceWarranties(s3.ID, s3.BrandID)
	if len(ws3) != 1 || !ws3[0].EndAt.Time.Equal(time.Date(2028, 2, 29, 23, 59, 59, 999999000, ist)) {
		t.Fatalf("S3 warranties = %+v", ws3)
	}

	// 5. Glorian: a completed Glorian service gets no warranty (K2).
	gCenter := it.brandCenter("glorian")
	gDist := it.org("t186-gdist", "distributor", gCenter)
	gDealer := it.org("t186-gdealer", "dealer", gDist)
	gCust, gVeh := it.svcCustomer(gDealer, "t186-gcust", "34G186"+it.suffix[len(it.suffix)-4:])
	gp := it.product(gCenter, "T186G")
	it.setWarrantyMonths(gp, 12)
	gu := c.unit(gCenter, gp, 18607)
	gs := it.directService(gDealer, gCust, gVeh, 4, full(gp, gu))
	res, err = listener.CreateForService(ctx, gs.ID)
	if err != nil {
		t.Fatalf("glorian: %v", err)
	}
	if len(res.Created) != 0 || len(it.serviceWarranties(gs.ID, gs.BrandID)) != 0 || it.createdEvents(gs.ID) != 0 {
		t.Fatalf("glorian created %d", len(res.Created))
	}
	for _, reason := range res.Skipped {
		if reason != warrantyusecase.SkipGlorian {
			t.Fatalf("glorian skip reason = %q", reason)
		}
	}
}

// outboxEvent decodes the outbox row of a service.completed event the way
// the outbox publisher hands it to the bus.
func (it *itest) outboxEvent(name, serviceUUID string) events.Event {
	it.t.Helper()
	var raw []byte
	if err := it.pool.QueryRow(context.Background(), `SELECT payload FROM outbox_events
		WHERE event_name = $1 AND payload->'data'->>'service_uuid' = $2 ORDER BY id DESC LIMIT 1`,
		name, serviceUUID).Scan(&raw); err != nil {
		it.t.Fatalf("outbox %s: %v", name, err)
	}
	var env struct {
		EventID    uuid.UUID      `json:"event_id"`
		EntityType string         `json:"entity_type"`
		EntityID   *int64         `json:"entity_id"`
		EntityUUID *uuid.UUID     `json:"entity_uuid"`
		Data       map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		it.t.Fatal(err)
	}
	return events.Event{
		Name: name, EventID: env.EventID, EntityType: env.EntityType, EntityID: env.EntityID,
		EntityUUID: env.EntityUUID, Payload: env.Data, OccurredAt: time.Now().UTC(),
	}
}

// TEC-186 / F1 gate, end to end: the order chain center -> distributor ->
// dealer (TEC-168/169 fixture), the dealer completes a service with the
// received unit (TEC-180), the service.completed outbox event goes through
// the server's bus to the warranty listener. Three ledgers agree: the
// stock ledger has the consumption, accounting has the two cari rows of
// the dealer's purchase, the warranty is active.
func TestIntegrationF1GateOrderServiceWarranty(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t186", 1860)
	it.setWarrantyMonths(f.piece, 24)
	item := []any{map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 1}}

	// 1. Center -> distributor -> dealer.
	a := it.openPreparing(f.distTok, f.staffTok, item)
	it.shipOrder(f.staffTok, a, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	if code, ec := it.transition(f.distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive A = %d %s", code, ec)
	}
	b := it.openPreparing(f.dealerTok, f.distTok, item)
	it.shipOrder(f.distTok, b, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	if code, ec := it.transition(f.dealerTok, b.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive B = %d %s", code, ec)
	}

	// 2. The dealer completes a service with the unit.
	cust, veh := it.svcCustomer(f.dealer, "t186-e2e-cust", "34E186"+it.suffix[len(it.suffix)-4:])
	s, _ := it.svcCall("POST", "/v1/services", f.dealerTok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+s.UUID+"/items", f.dealerTok,
		map[string]any{"barcode": f.u1.Barcode, "kind": "full"}, http.StatusCreated)
	if done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", f.dealerTok,
		map[string]string{"status": "completed"}, http.StatusOK); done.Status != "completed" {
		t.Fatalf("service = %+v", done)
	}
	var svcID int64
	var completedAt time.Time
	if err := it.pool.QueryRow(ctx, `SELECT id, completed_at FROM services WHERE uuid = $1`, s.UUID).Scan(&svcID, &completedAt); err != nil {
		t.Fatal(err)
	}

	// 3. The outbox row through the server's bus (outbox -> bus -> listener),
	// twice: the second delivery writes nothing.
	ev := it.outboxEvent(events.ServiceCompleted, s.UUID)
	for i := 0; i < 2; i++ {
		if err := it.srv.events.Publish(ctx, ev); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// Ledger 1: stock, entry ... received, consumption into the service.
	var types string
	if err := it.pool.QueryRow(ctx, `SELECT string_agg(type, ',' ORDER BY id) FROM stock_movements WHERE unit_id = $1`,
		f.u1.ID).Scan(&types); err != nil {
		t.Fatal(err)
	}
	if types != "entry,order_out,received,order_out,received,consumption" {
		t.Fatalf("u1 history = %s", types)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, f.u1.ID); err != nil || st.Status != "used" || st.OwnerType != "service" || st.OwnerID != svcID {
		t.Fatalf("u1 state = %+v %v", st, err)
	}

	// Ledger 2: accounting, the dealer's purchase is two cari rows
	// (distributor sale on the dealer, dealer purchase on the distributor).
	rows := it.orderEntries(b.UUID)
	if len(rows) != 2 ||
		rows[0].OrgID != f.dist.ID || rows[0].Counterparty != f.dealer.ID || rows[0].Direction != "income" ||
		rows[1].OrgID != f.dealer.ID || rows[1].Counterparty != f.dist.ID || rows[1].Direction != "expense" {
		t.Fatalf("order B finance rows = %+v", rows)
	}
	if got := it.cariBalance(f.dealer.ID, f.dist.ID); got == "0.00" {
		t.Fatalf("dealer/dist cari balance = %s", got)
	}

	// Ledger 3: warranty, one active warranty for the unit on the vehicle.
	ws := it.serviceWarranties(svcID, f.dealer.BrandID)
	if len(ws) != 1 {
		t.Fatalf("warranties = %+v", ws)
	}
	w := ws[0]
	ist, err := time.LoadLocation(f.dealer.Timezone)
	if err != nil {
		t.Skipf("tzdata: %v", err)
	}
	if w.Status != "active" || w.UnitID != f.u1.ID || w.VehicleID != veh.ID || w.HolderUserID != cust.ID ||
		w.OrganizationID != f.dealer.ID || !w.StartAt.Time.Equal(completedAt) ||
		!w.EndAt.Time.Equal(warrantyusecase.EndAt(completedAt, 24, ist)) {
		t.Fatalf("warranty = %+v", w)
	}
	if n := it.createdEvents(svcID); n != 1 {
		t.Fatalf("warranty.created events = %d, want 1", n)
	}
}
