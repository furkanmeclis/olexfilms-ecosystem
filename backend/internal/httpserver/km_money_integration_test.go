package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// kmMoney is the TEC-1244 (kutu/metraj 7/7) fixture: the center sells the
// film product (box list 400, meter list 30, box 15 m, sold by the meter)
// and a second one without a meter list price (computed: 400 / 15 =
// 26.6667 per meter) to a dealer, on one order with a box line and a
// meter line each; the order is approved so its prices are frozen.
type kmMoney struct {
	it                  *itest
	center, dealer      db.Organization
	cur                 string
	film, computed      db.Product
	salesTok, dealerTok string
	order               boxMeterOrder
	chain               *stockChain
	atDealer, centerLoc ledger.Owner
	nextUnit            int
}

func newKMMoney(t *testing.T, it *itest, tag string) *kmMoney {
	t.Helper()
	ctx := context.Background()
	f := &kmMoney{it: it, center: it.brandCenter("olex")}
	brand, err := it.q.GetBrandByID(ctx, f.center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	f.cur = strings.TrimSpace(brand.Currency)
	if f.cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: f.cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	f.dealer = it.org(tag+"-dealer", "dealer", f.center)
	sales, spw := it.user(tag + "-center-sales")
	it.member(f.center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	owner, dpw := it.user(tag + "-dealer-owner")
	it.member(f.dealer, owner, "owner")
	f.salesTok = it.loginOrg(sales, spw, f.center)
	f.dealerTok = it.loginOrg(owner, dpw, f.dealer)

	f.film = it.product(f.center, strings.ToUpper(tag)+"F")
	f.computed = it.product(f.center, strings.ToUpper(tag)+"C")
	for _, p := range []db.Product{f.film, f.computed} {
		if _, err := it.pool.Exec(ctx, `UPDATE products SET default_roll_meters = 15, min_cut_meters = 2,
			sellable_by_meter = true WHERE id = $1`, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: f.film.ID, BrandID: f.film.BrandID, Currency: f.cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	it.setListPrice(f.computed, f.cur, "400")

	f.order = it.boxMeterOrder("POST", "/v1/orders", f.dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": f.film.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": f.film.Uuid.String(), "meters": "3.5"},
		map[string]any{"product_uuid": f.computed.Uuid.String(), "meters": "3.5"},
	}}, http.StatusCreated)
	if code, ec := it.transition(f.dealerTok, f.order.UUID, "submitted"); code != http.StatusOK {
		t.Fatalf("submit = %d %s", code, ec)
	}
	if code, ec := it.transition(f.salesTok, f.order.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, ec)
	}
	f.chain = it.stockChain()
	f.centerLoc = f.chain.location(f.center, strings.ToUpper(tag))
	f.atDealer = ledger.Owner{Type: ledger.OwnerOrganization, ID: f.dealer.ID, OrgID: f.dealer.ID}
	return f
}

// line is the order line of p and measure.
func (f *kmMoney) line(t *testing.T, p db.Product, measure string) db.OrderItem {
	t.Helper()
	var it db.OrderItem
	if err := f.it.pool.QueryRow(context.Background(), `
		SELECT oi.id, oi.organization_id, oi.brand_id FROM order_items oi JOIN orders o ON o.id = oi.order_id
		WHERE o.uuid = $1 AND oi.product_id = $2 AND oi.sale_measure = $3`, f.order.UUID, p.ID, measure).
		Scan(&it.ID, &it.OrganizationID, &it.BrandID); err != nil {
		t.Fatalf("order line %s/%s: %v", p.Sku, measure, err)
	}
	return it
}

// shipped creates a unit of p (meters "" = a closed box, else a cut
// piece), enters it at the center, assigns it to the order line of measure
// (none: "") and ships it to the dealer.
func (f *kmMoney) shipped(t *testing.T, p db.Product, meters, measure string) db.Unit {
	t.Helper()
	f.nextUnit++
	var u db.Unit
	if meters == "" {
		u = f.chain.unit(f.center, p, 12440+f.nextUnit)
	} else {
		u = f.chain.rollUnit(f.center, p, 12440+f.nextUnit, meters)
	}
	f.chain.post(ledger.TypeEntry, u, f.chain.nextRef(), f.centerLoc)
	if measure != "" {
		l := f.line(t, p, measure)
		var q, m any
		if meters == "" {
			q = 1
		} else {
			m = meters
		}
		if _, err := f.it.pool.Exec(context.Background(), `
			INSERT INTO order_item_units (order_item_id, organization_id, brand_id, unit_id, quantity, meters)
			VALUES ($1, $2, $3, $4, $5, $6::numeric)`, l.ID, l.OrganizationID, l.BrandID, u.ID, q, m); err != nil {
			t.Fatalf("assign: %v", err)
		}
	}
	f.chain.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, f.dealer, f.atDealer)
	return u
}

// returnPrices has the dealer return units to the center, which approves
// (prices frozen); it returns the unit price and line total per barcode
// and the request total.
func (f *kmMoney) returnPrices(t *testing.T, units ...db.Unit) (map[string][2]string, string) {
	t.Helper()
	items := make([]map[string]any, 0, len(units))
	for _, u := range units {
		items = append(items, map[string]any{"barcode": u.Barcode})
	}
	v, _ := f.it.transferCall("POST", "/v1/stock-transfers", f.dealerTok, map[string]any{"kind": "return", "items": items},
		http.StatusCreated)
	if code, ec := f.it.transferMove(f.salesTok, v.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve return = %d %s", code, ec)
	}
	ctx := context.Background()
	rows, err := f.it.pool.Query(ctx, `
		SELECT u.barcode, COALESCE(i.unit_price::text, ''), COALESCE(i.line_total::text, '')
		FROM stock_transfer_request_items i
		JOIN stock_transfer_requests r ON r.id = i.request_id
		JOIN units u ON u.id = i.unit_id
		WHERE r.uuid = $1`, v.UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][2]string{}
	for rows.Next() {
		var bc, up, lt string
		if err := rows.Scan(&bc, &up, &lt); err != nil {
			t.Fatal(err)
		}
		out[bc] = [2]string{up, lt}
	}
	var total string
	if err := f.it.pool.QueryRow(ctx, `SELECT COALESCE(total::text, '') FROM stock_transfer_requests WHERE uuid = $1`, v.UUID).
		Scan(&total); err != nil {
		t.Fatal(err)
	}
	return out, total
}

// TEC-1244 acceptance (iade tutarı, bulgu 6): a cut piece is credited at
// meters × the meter price of the order line it was sold on, a closed box
// at the box price; without an order line, at the dealer's purchase price
// (meter price for a cut piece, the computed one rounded to 4 decimals
// then the line to cents). Before, a piece product's cut piece was
// credited one meter's price.
func TestIntegrationKMReturnAmounts(t *testing.T) {
	it := newIntegration(t)
	f := newKMMoney(t, it, "t1244r")

	box := f.shipped(t, f.film, "", "box")
	piece := f.shipped(t, f.film, "3.50", "meter")
	cpiece := f.shipped(t, f.computed, "3.50", "meter")
	// Not on any order: the dealer's purchase price.
	loose := f.shipped(t, f.film, "2.50", "")
	cloose := f.shipped(t, f.computed, "3.50", "")
	closed := f.shipped(t, f.computed, "", "")

	got, total := f.returnPrices(t, box, piece, cpiece, loose, cloose, closed)
	want := map[string][2]string{
		box.Barcode:    {"400", "400.00"},    // the box line
		piece.Barcode:  {"30", "105.00"},     // 3.5 m × 30 (meter line)
		cpiece.Barcode: {"26.6667", "93.33"}, // 3.5 m × 400/15, rounded to cents
		loose.Barcode:  {"30", "75.00"},      // 2.5 m × 30 (purchase price)
		cloose.Barcode: {"26.6667", "93.33"},
		closed.Barcode: {"400", "400.00"}, // a closed box at the box price
	}
	for bc, w := range want {
		g := got[bc]
		if !sameAmount(&g[0], w[0]) || !sameAmount(&g[1], w[1]) {
			t.Errorf("%s = %v, want %v", bc, g, w)
		}
	}
	// 400 + 105 + 93.33 + 75 + 93.33 + 400
	if !sameAmount(&total, "1166.66") {
		t.Fatalf("return total = %s", total)
	}
}

// TEC-1244 acceptance (bayi muhasebesi maliyeti): the dealer's purchase
// cost of a unit is meters × the meter cost for a cut piece (the meter
// line it was bought on, else the dealer's meter price) and the box price
// for a closed box. Before, a cut piece cost one meter's price. A piece
// shipped on a meter line is a cut piece and is not sold (TEC-1242, K13).
func TestIntegrationKMDealerCost(t *testing.T) {
	it := newIntegration(t)
	f := newKMMoney(t, it, "t1244c")
	ctx := context.Background()
	var dealerCur string
	if err := it.pool.QueryRow(ctx, `SELECT currency FROM organizations WHERE id = $1`, f.dealer.ID).Scan(&dealerCur); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(dealerCur) != f.cur {
		t.Skipf("dealer currency %s differs from the brand currency %s", dealerCur, f.cur)
	}
	cost := func(u db.Unit) string {
		t.Helper()
		look := decodeData[struct {
			PurchasePrice *string `json:"purchase_price"`
		}](t, mustDo(t, it, "GET", "/v1/product-sales/lookup?barcode="+u.Barcode, f.dealerTok, nil, http.StatusOK))
		if look.PurchasePrice == nil {
			t.Fatalf("%s: no purchase price", u.Barcode)
		}
		return *look.PurchasePrice
	}
	cases := []struct {
		unit db.Unit
		want string
	}{
		{f.shipped(t, f.film, "", "box"), "400"},
		{f.shipped(t, f.film, "2.50", ""), "75"}, // 2.5 m × 30 (price list)
		{f.shipped(t, f.computed, "", ""), "400"},
	}
	for _, c := range cases {
		if got := cost(c.unit); !sameAmount(&got, c.want) {
			t.Errorf("%s cost = %s, want %s", c.unit.Barcode, got, c.want)
		}
	}
	// A piece shipped on a meter line carries the line's credits: it is a
	// cut piece, used only in services and never offered for a quick sale
	// (TEC-1242, K13), so the lookup refuses it instead of pricing it.
	for _, u := range []db.Unit{f.shipped(t, f.film, "3.50", "meter"), f.shipped(t, f.computed, "3.50", "meter")} {
		if code, env := it.do("GET", "/v1/product-sales/lookup?barcode="+u.Barcode, hostOlex, f.dealerTok, nil); code != http.StatusUnprocessableEntity ||
			errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
			t.Errorf("%s lookup = %d %s, want CUT_PIECE_NOT_SELLABLE", u.Barcode, code, errCode(env))
		}
	}
}

// TEC-1244 acceptance (ekstre tutarı): the statement detail of an order
// with a box line and a meter line of one product reads both lines apart,
// each with its measure, amount and total (box 400; 3.5 m × 30 = 105;
// 3.5 m × 26.6667 = 93.33) and the barcodes assigned to each line.
func TestIntegrationKMStatementLines(t *testing.T) {
	it := newIntegration(t)
	f := newKMMoney(t, it, "t1244s")
	box := f.shipped(t, f.film, "", "box")
	piece := f.shipped(t, f.film, "3.50", "meter")
	rows, err := it.q.ListStatementOrderItems(context.Background(), db.ListStatementOrderItemsParams{
		OrderUuids: []uuid.UUID{uuid.MustParse(f.order.UUID)}, OrganizationID: f.dealer.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ sku, measure string }
	got := map[key]db.ListStatementOrderItemsRow{}
	for _, r := range rows {
		got[key{r.ProductSku, r.SaleMeasure}] = r
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	num := func(n pgtype.Numeric) string { v, _ := n.Value(); s, _ := v.(string); return s }
	b, m, c := got[key{f.film.Sku, "box"}], got[key{f.film.Sku, "meter"}], got[key{f.computed.Sku, "meter"}]
	if !b.Quantity.Valid || b.Quantity.Int32 != 1 || b.Meters.Valid || num(b.LineTotal) != "400.00" {
		t.Fatalf("box line = %+v", b)
	}
	if m.Quantity.Valid || num(m.Meters) != "3.50" || num(m.LineTotal) != "105.00" {
		t.Fatalf("meter line = %+v", m)
	}
	if num(c.UnitPrice) != "26.6667" || num(c.LineTotal) != "93.33" {
		t.Fatalf("computed meter line = %+v", c)
	}
	units, err := it.q.ListStatementOrderItemUnits(context.Background(), []int64{b.ItemID, m.ItemID})
	if err != nil {
		t.Fatal(err)
	}
	byLine := map[int64]string{}
	for _, u := range units {
		byLine[u.OrderItemID] = u.Barcode
	}
	if byLine[b.ItemID] != box.Barcode || byLine[m.ItemID] != piece.Barcode {
		t.Fatalf("units = %+v", units)
	}
}
