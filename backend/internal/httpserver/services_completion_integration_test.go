package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5/pgtype"
)

type stockUnitPage struct {
	Items []struct {
		UUID            string  `json:"uuid"`
		Barcode         string  `json:"barcode"`
		UnitKind        string  `json:"unit_kind"`
		QuantityOnHand  int32   `json:"quantity_on_hand"`
		RemainingMeters *string `json:"remaining_meters"`
		Product         struct {
			UUID string `json:"uuid"`
		} `json:"product"`
	} `json:"items"`
}

func (p stockUnitPage) barcodes() map[string]bool {
	out := map[string]bool{}
	for _, u := range p.Items {
		out[u.Barcode] = true
	}
	return out
}

func (it *itest) stockUnits(token, svcUUID string, q url.Values, want int) stockUnitPage {
	it.t.Helper()
	path := "/v1/services/" + svcUUID + "/stock-units"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	code, env := it.do("GET", path, hostOlex, token, nil)
	if code != want {
		it.t.Fatalf("GET %s = %d %s, want %d", path, code, errCode(env), want)
	}
	var p stockUnitPage
	if code == http.StatusOK {
		if err := json.Unmarshal(env.Data, &p); err != nil {
			it.t.Fatal(err)
		}
	}
	return p
}

// rollUnit creates a printed roll label of p with meters.
func (c *stockChain) rollUnit(center db.Organization, p db.Product, n int, meters string) db.Unit {
	c.it.t.Helper()
	var m pgtype.Numeric
	if err := m.Scan(meters); err != nil {
		c.it.t.Fatal(err)
	}
	u, err := c.it.q.CreateUnit(context.Background(), db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: center.BrandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T180R-%s-%d", c.it.suffix, n),
		UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		InitialMeters: m, RemainingMeters: m,
	})
	if err != nil {
		c.it.t.Fatalf("roll unit: %v", err)
	}
	return u
}

// postFixed posts a fixed barcode movement (From for debits, To for credits).
func (c *stockChain) postFixed(typ ledger.MovementType, u db.Unit, ref int64, from, to *ledger.Owner, qty int32) {
	c.it.t.Helper()
	ctx := context.Background()
	tx, err := c.it.pool.Begin(ctx)
	if err != nil {
		c.it.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := c.l.Post(ctx, tx, ledger.Movement{
		Type: typ, UnitID: u.ID, From: from, To: to, Quantity: qty, Source: "test", RefType: "t180", RefID: ref,
	}); err != nil {
		c.it.t.Fatalf("post fixed %s: %v", typ, err)
	}
	if err := tx.Commit(ctx); err != nil {
		c.it.t.Fatal(err)
	}
}

// TEC-180 acceptance: the stock picker lists the dealer's units (barcode,
// product, meters filters); completing a service consumes them in one
// transaction (roll meters drop, a whole unit is used, fixed pieces are
// debited), writes stock_movement_id on every item and one
// service.completed outbox row; completing again is a no-op; a cut larger
// than what is left rolls the whole completion back (409).
func TestIntegrationServiceCompletion(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t180-dist", "distributor", center)
	dealerA := it.org("t180-dealer-a", "dealer", dist)
	dealerB := it.org("t180-dealer-b", "dealer", dist)

	ownerA, apw := it.user("t180-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, bpw := it.user("t180-owner-b")
	it.member(dealerB, ownerB, "owner")
	cust, veh := it.svcCustomer(dealerA, "t180-cust", "34T180"+it.suffix[len(it.suffix)-4:])

	piece := it.product(center, "T180P")
	roll := it.product(center, "T180R")
	fixed := it.product(center, "T180F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	// units_check_integrity: a fixed barcode needs a uses_fixed_barcode product.
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}

	c := it.stockChain()
	cLoc := c.location(center, "C180")
	dLoc := c.location(dist, "D180")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID}
	up := c.unit(center, piece, 1800)
	ur := c.rollUnit(center, roll, 1, "50.00")
	for _, u := range []db.Unit{up, ur} {
		c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
		c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
		c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealerA, dealerOrg)
	}
	uf := c.fixedUnit(center, fixed, 1801, cLoc, 10)
	ref := c.nextRef()
	c.postFixed(ledger.TypeOrderOut, uf, ref, &cLoc, nil, 4)
	c.postFixed(ledger.TypeReceived, uf, ref, nil, &dealerOrg, 4)

	aTok := it.loginOrg(ownerA, apw, dealerA)
	bTok := it.loginOrg(ownerB, bpw, dealerB)
	body := map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}
	s1, _ := it.svcCall("POST", "/v1/services", aTok, body, http.StatusCreated)
	s2, _ := it.svcCall("POST", "/v1/services", aTok, body, http.StatusCreated)

	// 1. Stock picker: every unit of dealer A; filters; other dealer 404.
	all := it.stockUnits(aTok, s1.UUID, nil, http.StatusOK).barcodes()
	if !all[up.Barcode] || !all[ur.Barcode] || !all[uf.Barcode] {
		t.Fatalf("picker = %v", all)
	}
	byBarcode := it.stockUnits(aTok, s1.UUID, url.Values{"barcode": {ur.Barcode}}, http.StatusOK)
	if len(byBarcode.Items) != 1 || byBarcode.Items[0].RemainingMeters == nil || *byBarcode.Items[0].RemainingMeters != "50.00" {
		t.Fatalf("picker by barcode = %+v", byBarcode.Items)
	}
	byProduct := it.stockUnits(aTok, s1.UUID, url.Values{"product_uuid": {piece.Uuid.String()}}, http.StatusOK).barcodes()
	if len(byProduct) != 1 || !byProduct[up.Barcode] {
		t.Fatalf("picker by product = %v", byProduct)
	}
	if m := it.stockUnits(aTok, s1.UUID, url.Values{"min_meters": {"10"}}, http.StatusOK).barcodes(); len(m) != 1 || !m[ur.Barcode] {
		t.Fatalf("picker min 10 m = %v", m)
	}
	if m := it.stockUnits(aTok, s1.UUID, url.Values{"min_meters": {"60"}}, http.StatusOK); len(m.Items) != 0 {
		t.Fatalf("picker min 60 m = %+v", m.Items)
	}
	for _, u := range it.stockUnits(aTok, s1.UUID, url.Values{"barcode": {uf.Barcode}}, http.StatusOK).Items {
		if u.QuantityOnHand != 4 {
			t.Fatalf("fixed on hand = %d", u.QuantityOnHand)
		}
	}
	it.stockUnits(bTok, s1.UUID, nil, http.StatusNotFound)

	// 2. Items: whole piece, 12.5 m cut, 3 fixed pieces; s2 cuts 40 m of the
	// same roll (fits now, not after s1).
	add := func(svc string, item map[string]any) serviceView {
		v, _ := it.svcCall("POST", "/v1/services/"+svc+"/items", aTok, item, http.StatusCreated)
		return v
	}
	add(s1.UUID, map[string]any{"barcode": up.Barcode, "kind": "full"})
	add(s1.UUID, map[string]any{"barcode": ur.Barcode, "kind": "partial", "meters": "12.5"})
	add(s1.UUID, map[string]any{"barcode": uf.Barcode, "kind": "full", "quantity": 3})
	add(s2.UUID, map[string]any{"barcode": ur.Barcode, "kind": "partial", "meters": "40"})
	if got := it.stockUnits(aTok, s1.UUID, nil, http.StatusOK).barcodes(); got[up.Barcode] || !got[ur.Barcode] {
		t.Fatalf("picker after items = %v (piece taken, roll still cut)", got)
	}

	// 3. Complete s1 (draft -> completed).
	tr := func(id string, want int) (serviceView, string) {
		return it.svcCall("POST", "/v1/services/"+id+"/transitions", aTok, map[string]string{"status": "completed"}, want)
	}
	done, _ := tr(s1.UUID, http.StatusOK)
	if done.Status != "completed" {
		t.Fatalf("completed = %+v", done)
	}
	var svcID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, s1.UUID).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	var remaining, rollStatus, pieceStatus, pieceOwner string
	var pieceOwnerID int64
	if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text, status FROM units WHERE id = $1`, ur.ID).Scan(&remaining, &rollStatus); err != nil {
		t.Fatal(err)
	}
	if remaining != "37.50" || rollStatus != "available" {
		t.Fatalf("roll = %s m %s, want 37.50 m available", remaining, rollStatus)
	}
	if err := it.pool.QueryRow(ctx, `SELECT u.status, s.owner_type, s.owner_id FROM units u
		JOIN unit_current_state s ON s.unit_id = u.id WHERE u.id = $1`, up.ID).Scan(&pieceStatus, &pieceOwner, &pieceOwnerID); err != nil {
		t.Fatal(err)
	}
	if pieceStatus != "used" || pieceOwner != "service" || pieceOwnerID != svcID {
		t.Fatalf("piece = %s %s %d, want used service %d", pieceStatus, pieceOwner, pieceOwnerID, svcID)
	}
	var fixedLeft int32
	if err := it.pool.QueryRow(ctx, `SELECT quantity_on_hand FROM fixed_barcode_holdings
		WHERE unit_id = $1 AND owner_type = 'organization' AND owner_id = $2`, uf.ID, dealerA.ID).Scan(&fixedLeft); err != nil {
		t.Fatal(err)
	}
	if fixedLeft != 1 {
		t.Fatalf("fixed left = %d, want 1", fixedLeft)
	}
	counts := func() (movements, linked, events int) {
		t.Helper()
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_movements m
			JOIN service_items i ON i.id = m.reference_id
			WHERE m.reference_type = 'service_item' AND i.service_id = $1
			  AND m.idempotency_key LIKE 'service:service_item:' || i.id || ':%'`, svcID).Scan(&movements); err != nil {
			t.Fatal(err)
		}
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM service_items i
			JOIN stock_movements m ON m.id = i.stock_movement_id
			WHERE i.service_id = $1 AND m.reference_id = i.id`, svcID).Scan(&linked); err != nil {
			t.Fatal(err)
		}
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events
			WHERE event_name = 'service.completed' AND payload::text LIKE '%' || $1 || '%'
			  AND payload::text LIKE '%item_ids%'`, s1.UUID).Scan(&events); err != nil {
			t.Fatal(err)
		}
		return
	}
	if m, l, e := counts(); m != 3 || l != 3 || e != 1 {
		t.Fatalf("after completion: movements %d linked %d events %d, want 3 3 1", m, l, e)
	}
	var types string
	if err := it.pool.QueryRow(ctx, `SELECT string_agg(m.type, ',' ORDER BY i.id) FROM service_items i
		JOIN stock_movements m ON m.id = i.stock_movement_id WHERE i.service_id = $1`, svcID).Scan(&types); err != nil {
		t.Fatal(err)
	}
	if types != "consumption,partial_consumption,consumption" {
		t.Fatalf("movement types = %s", types)
	}

	// 4. Completing again is a no-op: no second consumption, no second event.
	again, _ := tr(s1.UUID, http.StatusOK)
	if again.Status != "completed" {
		t.Fatalf("again = %s", again.Status)
	}
	if m, l, e := counts(); m != 3 || l != 3 || e != 1 {
		t.Fatalf("after second completion: movements %d linked %d events %d, want 3 3 1", m, l, e)
	}
	// Items are locked now.
	it.svcCall("POST", "/v1/services/"+s1.UUID+"/items", aTok, map[string]any{"barcode": ur.Barcode, "kind": "partial", "meters": "1"}, http.StatusConflict)

	// 5. s2 wants 40 m, 37.50 m are left: 409, nothing written, still draft.
	if _, ec := tr(s2.UUID, http.StatusConflict); ec != "SERVICE_UNIT_NOT_AVAILABLE" {
		t.Fatalf("s2 completion = %s", ec)
	}
	still, _ := it.svcCall("GET", "/v1/services/"+s2.UUID, aTok, nil, http.StatusOK)
	if still.Status != "draft" {
		t.Fatalf("s2 = %s", still.Status)
	}
	var s2Movements int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM service_items i JOIN services s ON s.id = i.service_id
		WHERE s.uuid = $1 AND i.stock_movement_id IS NOT NULL`, s2.UUID).Scan(&s2Movements); err != nil {
		t.Fatal(err)
	}
	if s2Movements != 0 {
		t.Fatalf("s2 linked movements = %d", s2Movements)
	}
}
