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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5/pgtype"
)

// shipOnMeterLine ties a cut piece to a received meter line of a dist →
// dealer order with line credits (what toplama writes, TEC-1241): the
// piece then carries those credits (unit_credit_default, TEC-1238).
func (it *itest) shipOnMeterLine(seller, buyer db.Organization, p db.Product, u db.Unit, meters string, credits int32) {
	it.t.Helper()
	ctx := context.Background()
	o, err := it.q.CreateOrder(ctx, db.CreateOrderParams{
		SellerOrgID: seller.ID, BrandID: seller.BrandID, BuyerOrgID: buyer.ID, Currency: it.dt5Currency(seller),
	})
	if err != nil {
		it.t.Fatalf("order: %v", err)
	}
	num := func(s string) pgtype.Numeric {
		var n pgtype.Numeric
		if err := n.Scan(s); err != nil {
			it.t.Fatal(err)
		}
		return n
	}
	line, err := it.q.CreateOrderItem(ctx, db.CreateOrderItemParams{
		OrderID: o.ID, ProductID: p.ID, Meters: num(meters), UnitPrice: num("27"), PriceSource: "manual",
		LineTotal: num("81"), LineCredits: pgtype.Int4{Int32: credits, Valid: true},
	})
	if err != nil {
		it.t.Fatalf("meter line: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `INSERT INTO order_item_units (order_item_id, organization_id, brand_id, unit_id, meters)
		VALUES ($1, $2, $3, $4, $5::numeric)`, line.ID, line.OrganizationID, line.BrandID, u.ID, meters); err != nil {
		it.t.Fatalf("assign the piece: %v", err)
	}
}

// TEC-1242 acceptance (kutu/metraj 5/7, K4/K9/K13): a cut piece shipped on
// a 5-credit meter line is used whole in 5 services (one credit each, its
// meters never cut) and refused in the 6th; the picker shows its credits
// and hides it when they are gone, the stock list shows it exhausted; a
// cut (partial) on it is refused while a roll without line credits keeps
// the meter consumption; every service gets its own warranty with the
// piece's barcode and product, a product without a warranty period gets
// none; the piece never enters the waste (efficiency) facts; cancelling a
// completed service gives the credit and the meters back; the dealer's
// quick sale refuses the piece (CUT_PIECE_NOT_SELLABLE) and never offers it
// for its product.
func TestIntegrationCutPieceCredits(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t1242-dist", "distributor", center)
	dealer := it.org("t1242-dealer", "dealer", dist)
	owner, opw := it.user("t1242-owner")
	it.member(dealer, owner, "owner")

	film := it.product(center, "T1242F")
	it.setWarrantyMonths(film, 12)
	noWarranty := it.product(center, "T1242N")
	c := it.stockChain()
	cLoc := c.location(center, "C1242")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	piece := c.rollUnit(center, film, 12420, "3.00")
	roll := c.rollUnit(center, film, 12421, "10.00")
	plain := c.rollUnit(center, noWarranty, 12422, "2.00")
	for _, u := range []db.Unit{piece, roll, plain} {
		c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
		c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	}
	it.shipOnMeterLine(dist, dealer, film, piece, "3", 5)
	it.shipOnMeterLine(dist, dealer, noWarranty, plain, "2", 5)
	tok := it.loginOrg(owner, opw, dealer)

	newSvc := func(i int) serviceView {
		cust, veh := it.svcCustomer(dealer, fmt.Sprintf("t1242-cust-%d", i),
			fmt.Sprintf("34L%d%s", i, it.suffix[len(it.suffix)-4:]))
		s, _ := it.svcCall("POST", "/v1/services", tok,
			map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
		return s
	}
	svcID := func(uuid string) int64 {
		var id int64
		if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, uuid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	remaining := func(u db.Unit) string {
		var m string
		if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text FROM units WHERE id = $1`, u.ID).Scan(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	svcs := make([]serviceView, 0, 6)
	for i := 1; i <= 6; i++ {
		svcs = append(svcs, newSvc(i))
	}

	// 1. The picker shows the piece with its credits; a roll without line
	// credits has none; min_meters (a roll to cut) never lists the piece.
	page := it.stockUnits(tok, svcs[0].UUID, url.Values{"q": {it.suffix}}, http.StatusOK)
	byBarcode := map[string]int{}
	for i, u := range page.Items {
		byBarcode[u.Barcode] = i
	}
	pi, ok := byBarcode[piece.Barcode]
	if !ok || page.Items[pi].Credits == nil || page.Items[pi].Credits.Remaining != 5 || page.Items[pi].Credits.Total != 5 {
		t.Fatalf("picker piece = %+v", page.Items)
	}
	if ri, ok := byBarcode[roll.Barcode]; !ok || page.Items[ri].Credits != nil {
		t.Fatalf("picker roll = %+v", page.Items)
	}
	cut := it.stockUnits(tok, svcs[0].UUID, url.Values{"q": {it.suffix}, "min_meters": {"1"}}, http.StatusOK).barcodes()
	if cut[piece.Barcode] || !cut[roll.Barcode] {
		t.Fatalf("min_meters picker = %v", cut)
	}

	// 2. A cut (partial) on the piece is refused; the roll is still cut.
	if _, ec := it.svcCall("POST", "/v1/services/"+svcs[0].UUID+"/items", tok,
		map[string]any{"barcode": piece.Barcode, "kind": "partial", "meters": "1"}, http.StatusBadRequest); ec == "" {
		t.Fatal("partial on a credit piece: no error code")
	}
	it.svcCall("POST", "/v1/services/"+svcs[0].UUID+"/items", tok,
		map[string]any{"barcode": roll.Barcode, "kind": "partial", "meters": "2"}, http.StatusCreated)

	// 3. Five services take one credit each; the meters stay.
	for i := 0; i < 5; i++ {
		it.svcCall("POST", "/v1/services/"+svcs[i].UUID+"/items", tok, map[string]any{"barcode": piece.Barcode}, http.StatusCreated)
		if total, rem, used := it.unitCreditsOf(piece.ID); total != 5 || rem != int32(4-i) || used != int32(i+1) {
			t.Fatalf("after service %d: credits %d/%d used %d", i+1, rem, total, used)
		}
	}
	if m := remaining(piece); m != "3.00" {
		t.Fatalf("piece meters after 5 uses = %s", m)
	}
	// 4. The 6th is refused and the picker hides the piece.
	if _, ec := it.svcCall("POST", "/v1/services/"+svcs[5].UUID+"/items", tok,
		map[string]any{"barcode": piece.Barcode}, http.StatusConflict); ec != "UNIT_CREDITS_EXHAUSTED" {
		t.Fatalf("6th service = %s", ec)
	}
	if p := it.stockUnits(tok, svcs[5].UUID, url.Values{"barcode": {piece.Barcode}}, http.StatusOK); len(p.Items) != 0 {
		t.Fatalf("picker lists the exhausted piece: %+v", p.Items)
	}
	// The stock list shows the piece exhausted (0 / 5) while it is held.
	code, env := it.do("GET", "/v1/stock/organizations/"+dealer.Uuid.String()+"/units?q="+url.QueryEscape(piece.Barcode), hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("stock units = %d %s", code, errCode(env))
	}
	var stock struct {
		Items []struct {
			Barcode   string `json:"barcode"`
			Total     *int32 `json:"credits_total"`
			Remaining *int32 `json:"credits_remaining"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &stock); err != nil {
		t.Fatal(err)
	}
	if len(stock.Items) != 1 || stock.Items[0].Total == nil || *stock.Items[0].Total != 5 ||
		stock.Items[0].Remaining == nil || *stock.Items[0].Remaining != 0 {
		t.Fatalf("stock list piece = %s", env.Data)
	}

	// 5. Completing: the piece stays in stock until the last service, then
	// it is consumed; every service gets its own warranty showing the
	// piece's barcode and product.
	complete := func(s serviceView) {
		done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", tok, map[string]string{"status": "completed"}, http.StatusOK)
		if done.Status != "completed" {
			t.Fatalf("complete = %+v", done)
		}
		if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, s.UUID)); err != nil {
			t.Fatalf("publish service.completed: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		complete(svcs[i])
		if st, ot, oid := it.unitStateOf(piece.ID); st != "available" || ot != "organization" || oid != dealer.ID {
			t.Fatalf("after completing service %d: %s %s %d", i+1, st, ot, oid)
		}
	}
	complete(svcs[4])
	if st, ot, oid := it.unitStateOf(piece.ID); st != "used" || ot != "service" || oid != svcID(svcs[4].UUID) {
		t.Fatalf("after the last credit: %s %s %d", st, ot, oid)
	}
	for i := 0; i < 5; i++ {
		id := svcID(svcs[i].UUID)
		ws := it.serviceWarranties(id, dealer.BrandID)
		var pieceWarranties int
		for _, w := range ws {
			if w.UnitID == piece.ID && w.Status == "active" {
				pieceWarranties++
			}
		}
		if pieceWarranties != 1 {
			t.Fatalf("service %d piece warranties = %+v", i+1, ws)
		}
		rows, err := it.q.ListWarrantyCertificateItems(ctx, db.ListWarrantyCertificateItemsParams{ServiceID: id, BrandID: dealer.BrandID})
		if err != nil {
			t.Fatal(err)
		}
		var onCert bool
		for _, r := range rows {
			if r.UnitBarcode == piece.Barcode {
				onCert = r.ProductName == film.Name && r.ProductSku == film.Sku && r.ItemKind == "full"
			}
		}
		if !onCert {
			t.Fatalf("service %d certificate items = %+v", i+1, rows)
		}
	}
	// Credit items (kind full) never enter the meter waste facts.
	var pieceFacts int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM efficiency_facts WHERE unit_id = $1`, piece.ID).Scan(&pieceFacts); err != nil {
		t.Fatal(err)
	}
	if pieceFacts != 0 {
		t.Fatalf("piece efficiency facts = %d, want 0", pieceFacts)
	}

	// 6. Cancelling the completed 5th service gives its credit and the
	// piece's meters back.
	it.cancelCompleted(tok, svcs[4].UUID, map[string]any{"reason": "yanlış kayıt"}, http.StatusOK)
	if total, rem, used := it.unitCreditsOf(piece.ID); total != 5 || rem != 1 || used != 4 {
		t.Fatalf("after cancel: credits %d/%d used %d", rem, total, used)
	}
	if st, ot, oid := it.unitStateOf(piece.ID); st != "available" || ot != "organization" || oid != dealer.ID {
		t.Fatalf("after cancel: %s %s %d", st, ot, oid)
	}
	if m := remaining(piece); m != "3.00" {
		t.Fatalf("piece meters after cancel = %s", m)
	}

	// 7. K9: the warranty follows the product like a box — no warranty
	// period, no warranty for the piece.
	s7 := newSvc(7)
	it.svcCall("POST", "/v1/services/"+s7.UUID+"/items", tok, map[string]any{"barcode": plain.Barcode}, http.StatusCreated)
	complete(s7)
	if ws := it.serviceWarranties(svcID(s7.UUID), dealer.BrandID); len(ws) != 0 {
		t.Fatalf("piece without a warranty period got warranties: %+v", ws)
	}

	// 8. K13: the quick sale refuses the piece and never offers it for its
	// product (the roll of the same product is offered).
	if code, env := it.do("GET", "/v1/product-sales/lookup?barcode="+url.QueryEscape(piece.Barcode), hostOlex, tok, nil); code != http.StatusUnprocessableEntity ||
		errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
		t.Fatalf("lookup piece = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/product-sales", hostOlex, tok, map[string]any{
		"payment_method": "cash",
		"lines":          []map[string]any{{"barcode": piece.Barcode, "unit_price": "100"}},
	}); code != http.StatusUnprocessableEntity || errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
		t.Fatalf("sell piece = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/product-sales/lookup?product_uuid="+film.Uuid.String(), hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("lookup product = %d %s", code, errCode(env))
	}
	var look struct {
		Barcode string `json:"barcode"`
	}
	if err := json.Unmarshal(env.Data, &look); err != nil {
		t.Fatal(err)
	}
	if look.Barcode != roll.Barcode {
		t.Fatalf("lookup by product offers %s, want the roll %s", look.Barcode, roll.Barcode)
	}
	if code, env := it.do("GET", "/v1/product-sales/lookup?product_uuid="+noWarranty.Uuid.String(), hostOlex, tok, nil); code != http.StatusUnprocessableEntity ||
		errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
		t.Fatalf("lookup a product with only a cut piece = %d %s", code, errCode(env))
	}
}
