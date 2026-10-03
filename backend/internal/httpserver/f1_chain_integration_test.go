package httpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// chainEntry is one GET /v1/accounting/entries row of the chain test.
type chainEntry struct {
	UUID                     string  `json:"uuid"`
	Direction                string  `json:"direction"`
	Category                 string  `json:"category"`
	OrigCurrency             string  `json:"orig_currency"`
	OrigAmount               string  `json:"orig_amount"`
	Currency                 string  `json:"currency"`
	Amount                   string  `json:"amount"`
	Rate                     string  `json:"rate"`
	RateDate                 string  `json:"rate_date"`
	SourceType               *string `json:"source_type"`
	SourceUUID               *string `json:"source_uuid"`
	ReversalOfUUID           *string `json:"reversal_of_uuid"`
	CounterpartyOrganization *struct {
		UUID string `json:"uuid"`
	} `json:"counterparty_organization"`
}

// chainBook reads an organization's book through the accounting API: its
// cari with the counterparty (balance) and the order rows on it.
func (it *itest) chainBook(token string, owner, counterparty db.Organization) (accCari, []chainEntry) {
	it.t.Helper()
	c, err := it.q.GetCariAccountByCounterpartyOrg(context.Background(), db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: counterparty.ID, Valid: true},
	})
	if err != nil {
		it.t.Fatalf("cari %s/%s: %v", owner.Slug, counterparty.Slug, err)
	}
	cari := decodeData[accCari](it.t, it.accDo("GET", "/v1/accounting/cari/"+c.Uuid.String(), token, nil, http.StatusOK))
	page := decodeData[struct {
		Items []chainEntry `json:"items"`
		Total int64        `json:"total"`
	}](it.t, it.accDo("GET", "/v1/accounting/entries?source_type=order&limit=100&cari_uuid="+c.Uuid.String(), token, nil, http.StatusOK))
	return cari, page.Items
}

// ratOfText parses a decimal string; the test fails on garbage.
func ratOfText(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		t.Fatalf("not a decimal: %q", s)
	}
	return r
}

// TEC-217 (F1-13a) acceptance, the F1 gate chain in one scenario through
// the HTTP handlers: the center opens a roll product (catalog API), prices
// it (pricing API), generates 100 roll barcodes of 25 m (TEC-202) and
// places them in a bin of its warehouse by a stock entry (TEC-204), no
// fixture and no import; it sells the first two whole rolls on: a
// distributor keeping its books in EUR orders them, the center ships, the
// distributor receives; a dealer keeping its books in UAH orders them from
// the distributor, ships, receives; the dealer consumes both whole rolls in
// a service and completes it; the service.completed event opens one
// warranty per roll. Every step asserts the ownership of the units, the
// stock ledger, and at the end the three books (center, distributor,
// dealer): income / expense / cari in each organization's currency at the
// rate frozen at approval; a later rate change leaves the booked rows
// alone. Whole rolls only: no cut (TEC-184) is involved.
//
// TestIntegrationF1GateOrderServiceWarranty covers the same chain with one
// fixture unit in the brand currency everywhere; this one adds the API
// product, real barcode generation and placement (TEC-220), three
// currencies and two consumed units.
func TestIntegrationF1ChainEURUAH(t *testing.T) {
	c := runF1Chain(t)

	// 8. Rate freeze: the day's rates change after the receipts; the
	// booked rows and the caris stay as they were.
	for _, r := range []struct{ base, rate string }{{"EUR", "50"}, {"UAH", "0.5"}} {
		if r.base != c.cur {
			c.upsertRate(r.base, c.cur, r.rate)
		}
	}
	c.readBooks("after a rate change")
}

// f1Frozen is the rate snapshot an order froze at approval.
type f1Frozen struct{ snap fxrates.Snapshot }

// f1Book is one expected order row of the chain in one organization's book.
type f1Book struct {
	name      string
	token     string
	owner, cp db.Organization
	order     string
	frozen    f1Frozen
	direction string
	category  string
	total     string
}

// f1Chain is the state runF1Chain leaves behind for the scenarios that
// continue from it (TEC-219).
type f1Chain struct {
	it                           *itest
	cur                          string
	toOrg                        map[string]*big.Rat
	center, dist, dealer         db.Organization
	staff                        db.User
	staffTok, distTok, dealerTok string
	item                         catalogItem
	rolls                        [2]db.Unit
	a, b                         orderView
	svcID                        int64
	svcUUID                      string
	centerStockBefore            int32
	books                        []f1Book

	upsertRate func(base, quote, rate string)
	owned      func(step string, status, ownerType string, ownerID, holder int64, moves string)
	stockOf    func(tok string, org db.Organization) int32
	readBooks  func(step string)
}

// runF1Chain runs steps 0-7 of the F1 gate chain (see
// TestIntegrationF1ChainEURUAH) and returns its state.
func runF1Chain(t *testing.T) *f1Chain {
	t.Helper()
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	it.cleanupCatalog()
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	day := today.Format(time.DateOnly)

	// 0. Parties: the brand center, a EUR distributor, a UAH dealer.
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if strings.TrimSpace(center.Currency) != cur {
		t.Fatalf("fixture assumption: center currency %s = brand currency %s", center.Currency, cur)
	}
	withCurrency := func(o db.Organization, currency string) db.Organization {
		t.Helper()
		if _, err := it.pool.Exec(ctx, `UPDATE organizations SET currency = $2 WHERE id = $1`, o.ID, currency); err != nil {
			t.Fatalf("org currency: %v", err)
		}
		o, err := it.q.GetOrganizationByID(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	dist := withCurrency(it.org("t217-dist", "distributor", center), "EUR")
	dealer := withCurrency(it.org("t217-dealer", "dealer", dist), "UAH")

	// Rates of the day (manual): the approval freezes cur -> TRY; the
	// books convert cur -> EUR (1/40) and cur -> UAH (1/0.8).
	upsertRate := func(base, quote, rate string) {
		t.Helper()
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: base, Quote: quote, Rate: rate, Source: "manual",
			Note: pgtype.Text{String: "TEC-217 fixture", Valid: true},
		}); err != nil {
			t.Fatalf("rate %s/%s: %v", base, quote, err)
		}
	}
	if cur != "TRY" {
		upsertRate(cur, "TRY", "35")
	}
	toOrg := map[string]*big.Rat{cur: big.NewRat(1, 1)}
	for _, r := range []struct {
		base, rate string
		inv        *big.Rat
	}{{"EUR", "40", big.NewRat(1, 40)}, {"UAH", "0.8", big.NewRat(5, 4)}} {
		if r.base == cur {
			continue
		}
		upsertRate(r.base, cur, r.rate)
		toOrg[r.base] = r.inv
	}
	t.Cleanup(func() {
		for _, base := range []string{"EUR", "UAH"} {
			if base != cur {
				_, _ = it.q.DeleteManualExchangeRate(context.Background(), db.DeleteManualExchangeRateParams{
					RateDate: pgtype.Date{Time: today, Valid: true}, Base: base, Quote: cur,
				})
			}
		}
	})

	staff, spw := it.user("t217-center")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse, rbac.RoleCenterAccounting)
	distOwner, dpw := it.user("t217-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rpw := it.user("t217-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)

	// 1. The center opens the product (roll_meter, 120 months warranty)
	// and prices it: 4 / m to distributors; the distributor sells at 6 / m.
	item := it.createCatalogProduct(staffTok, hostOlex, "F1C-"+it.suffix)
	product, err := it.q.GetProductByUUIDAnyBrand(ctx, uuid.MustParse(item.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if product.OrganizationID != center.ID || product.UnitType != "roll_meter" {
		t.Fatalf("product = %+v", product)
	}
	it.stepUp(staff.Uuid)
	it.accDo("PUT", "/v1/tenant/pricing/products/"+item.UUID+"/prices/"+cur, staffTok, map[string]any{
		"purchase_price": "2", "sale_to_distributor_price": "4", "recommended_sale_price": "10",
	}, http.StatusOK)
	it.stepUp(distOwner.Uuid)
	it.accDo("PUT", "/v1/tenant/pricing/products/"+item.UUID+"/dealer-prices/"+cur, distTok,
		map[string]string{"price": "6"}, http.StatusOK)

	// 2. Barcodes (K14: only the center brings units in): the center builds
	// a bin (warehouse -> room -> aisle -> shelf -> bin), generates 100
	// roll labels of 25 m (TEC-202, POST /v1/stock/barcodes) and places
	// them in the bin through a stock entry (TEC-204): draft, link the
	// printed barcodes, place by the bin's scanned QR, confirm. No fixture
	// and no import: every unit comes from the barcode generator. The
	// chain then sells the first two rolls.
	const generated = 100
	wh := func(method, path string, body, out any) { it.whDo(staffTok, method, path, body, http.StatusOK, out) }
	whNew := func(path string, body, out any) { it.whDo(staffTok, "POST", path, body, http.StatusCreated, out) }
	sfx := it.suffix[len(it.suffix)-9:]
	var whs, room, aisle, shelf, bin whItem
	whNew("/v1/warehouse/warehouses", map[string]any{"code": "F" + sfx, "name": "F1 chain"}, &whs)
	whNew("/v1/warehouse/warehouses/"+whs.UUID+"/rooms", map[string]any{"code": "R1"}, &room)
	whNew("/v1/warehouse/locations", map[string]any{"room_uuid": room.UUID, "type": "aisle", "code": "A"}, &aisle)
	whNew("/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": aisle.UUID, "type": "shelf", "code": "01"}, &shelf)
	whNew("/v1/warehouse/locations",
		map[string]any{"room_uuid": room.UUID, "parent_uuid": shelf.UUID, "type": "bin", "code": "01"}, &bin)
	var binID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM warehouse_locations WHERE uuid = $1`, bin.UUID).Scan(&binID); err != nil {
		t.Fatalf("bin id: %v", err)
	}

	var batch struct {
		UUID     string `json:"uuid"`
		Quantity int    `json:"quantity"`
		Meters   string `json:"meters"`
		Units    []struct {
			Barcode string `json:"barcode"`
			Status  string `json:"status"`
		} `json:"units"`
	}
	whNew("/v1/stock/barcodes", map[string]any{"product_uuid": item.UUID, "quantity": generated, "meters": "25"}, &batch)
	if batch.UUID == "" || batch.Quantity != generated || len(batch.Units) != generated ||
		ratOfText(t, batch.Meters).Cmp(big.NewRat(25, 1)) != 0 {
		t.Fatalf("barcode batch = %s q%d m%s units %d", batch.UUID, batch.Quantity, batch.Meters, len(batch.Units))
	}
	barcodes := make([]string, 0, generated)
	for _, u := range batch.Units {
		if u.Status != "printed" {
			t.Fatalf("generated unit %s status = %s, want printed", u.Barcode, u.Status)
		}
		barcodes = append(barcodes, u.Barcode)
	}

	type entryLine struct {
		Barcode  string `json:"barcode"`
		Location *struct {
			UUID string `json:"uuid"`
		} `json:"location"`
		EntryMovementUUID     *string `json:"entry_movement_uuid"`
		PlacementMovementUUID *string `json:"placement_movement_uuid"`
	}
	type entryView struct {
		UUID      string      `json:"uuid"`
		Mode      string      `json:"mode"`
		Status    string      `json:"status"`
		LineCount int64       `json:"line_count"`
		Lines     []entryLine `json:"lines"`
	}
	var entry entryView
	whNew("/v1/warehouse/stock-entries", map[string]any{"warehouse_uuid": whs.UUID, "mode": "with_existing"}, &entry)
	if entry.Status != "draft" || entry.Mode != "with_existing" {
		t.Fatalf("stock entry = %+v", entry)
	}
	ebase := "/v1/warehouse/stock-entries/" + entry.UUID
	wh("POST", ebase+"/lines", map[string]any{"barcodes": barcodes}, &entry)
	if entry.LineCount != generated {
		t.Fatalf("stock entry lines = %d, want %d", entry.LineCount, generated)
	}
	wh("POST", ebase+"/place", map[string]any{"location_code": "OFW:LOC:" + bin.FullCode}, &entry)
	wh("POST", ebase+"/confirm", nil, &entry)
	if entry.Status != "confirmed" || entry.LineCount != generated || len(entry.Lines) != generated {
		t.Fatalf("confirmed entry = %s lines %d/%d", entry.Status, entry.LineCount, len(entry.Lines))
	}
	for _, l := range entry.Lines {
		if l.Location == nil || l.Location.UUID != bin.UUID || l.EntryMovementUUID == nil || l.PlacementMovementUUID == nil {
			t.Fatalf("confirmed entry line = %+v", l)
		}
	}
	var placed int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM unit_current_state s JOIN units u ON u.id = s.unit_id
		WHERE u.batch_id = (SELECT id FROM barcode_batches WHERE uuid = $1)
		  AND s.status = 'placed' AND s.owner_type = 'warehouse_location' AND s.owner_id = $2`,
		batch.UUID, binID).Scan(&placed); err != nil {
		t.Fatal(err)
	}
	if placed != generated {
		t.Fatalf("units placed in the bin = %d, want %d", placed, generated)
	}
	var rolls [2]db.Unit
	for i := range rolls {
		if rolls[i], err = it.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: center.BrandID, Barcode: barcodes[i]}); err != nil {
			t.Fatalf("roll %d: %v", i+1, err)
		}
	}

	history := func(u db.Unit) string {
		t.Helper()
		var types string
		if err := it.pool.QueryRow(ctx, `SELECT COALESCE(string_agg(type, ',' ORDER BY id), '') FROM stock_movements WHERE unit_id = $1`,
			u.ID).Scan(&types); err != nil {
			t.Fatal(err)
		}
		return types
	}
	owned := func(step string, status, ownerType string, ownerID, holder int64, moves string) {
		t.Helper()
		for _, u := range rolls {
			st, err := it.q.GetUnitCurrentState(ctx, u.ID)
			if err != nil {
				t.Fatalf("%s: state %s: %v", step, u.Barcode, err)
			}
			if st.Status != status || st.OwnerType != ownerType || st.OwnerID != ownerID || st.HolderOrgID != holder {
				t.Fatalf("%s: %s state = %+v, want %s %s/%d holder %d", step, u.Barcode, st, status, ownerType, ownerID, holder)
			}
			if h := history(u); h != moves {
				t.Fatalf("%s: %s history = %s, want %s", step, u.Barcode, h, moves)
			}
		}
	}
	for _, u := range rolls {
		if u.Source != "generated" || !u.BatchID.Valid || u.OrganizationID != center.ID || !u.RemainingMeters.Valid ||
			ratOfText(t, posting.FormatNumeric(u.RemainingMeters)).Cmp(big.NewRat(25, 1)) != 0 {
			t.Fatalf("generated roll = %+v", u)
		}
	}
	owned("placed", "placed", "warehouse_location", binID, center.ID, "entry,placement")
	stockOf := func(tok string, org db.Organization) int32 {
		t.Helper()
		code, page := it.productStock(tok, "/v1/stock/organizations/"+org.Uuid.String()+"/products?product_uuid="+item.UUID)
		if code != http.StatusOK {
			t.Fatalf("stock of %s = %d", org.Slug, code)
		}
		q, _ := page.qty(item.UUID)
		return q
	}
	centerStockBefore := stockOf(staffTok, center)
	if centerStockBefore != generated {
		t.Fatalf("center stock after the entry = %d, want %d", centerStockBefore, generated)
	}

	line := []any{map[string]any{"product_uuid": item.UUID, "meters": "50"}}
	whole := [][]map[string]any{{{"barcode": rolls[0].Barcode}, {"barcode": rolls[1].Barcode}}}
	freeze := func(name string, o orderView) f1Frozen {
		t.Helper()
		var f f1Frozen
		if err := json.Unmarshal(o.RateSnapshot, &f.snap); err != nil || f.snap.RateDate == "" ||
			f.snap.Base != cur || f.snap.Quote != "TRY" || o.TryRate == nil {
			t.Fatalf("%s rate snapshot = %s try_rate %v (%v)", name, o.RateSnapshot, o.TryRate, err)
		}
		return f
	}

	// 3. Distributor (EUR books) orders 50 m from the center: price 4 / m,
	// total 200 in the order (brand) currency; the rate is frozen at
	// approval; the center ships both whole rolls, the distributor receives.
	a := it.openPreparing(distTok, staffTok, line)
	if a.Currency != cur || !sameAmount(&a.Total, "200") || len(a.Items) != 1 || !sameAmount(&a.Items[0].UnitPrice, "4") {
		t.Fatalf("order A = %+v", a)
	}
	fa := freeze("A", a)
	it.shipOrder(staffTok, a, whole)
	owned("A shipped", "in_transit", "organization", dist.ID, dist.ID, "entry,placement,order_out")
	if code, ec := it.transition(distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive A = %d %s", code, ec)
	}
	owned("A received", "available", "organization", dist.ID, dist.ID, "entry,placement,order_out,received")
	if v := it.orderCall("GET", "/v1/orders/"+a.UUID, distTok, nil, http.StatusOK); v.Status != "received" ||
		string(v.RateSnapshot) != string(a.RateSnapshot) {
		t.Fatalf("A after receipt = %s %s", v.Status, v.RateSnapshot)
	}

	// 4. Dealer (UAH books) orders 50 m from the distributor: dealer price
	// 6 / m, total 300; the distributor ships, the dealer receives.
	b := it.openPreparing(dealerTok, distTok, line)
	if b.Currency != cur || !sameAmount(&b.Total, "300") || !sameAmount(&b.Items[0].UnitPrice, "6") {
		t.Fatalf("order B = %+v", b)
	}
	fb := freeze("B", b)
	it.shipOrder(distTok, b, whole)
	owned("B shipped", "in_transit", "organization", dealer.ID, dealer.ID, "entry,placement,order_out,received,order_out")
	if code, ec := it.transition(dealerTok, b.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive B = %d %s", code, ec)
	}
	owned("B received", "available", "organization", dealer.ID, dealer.ID, "entry,placement,order_out,received,order_out,received")
	if got := stockOf(dealerTok, dealer); got != 2 {
		t.Fatalf("dealer stock after B = %d, want 2", got)
	}
	if got := stockOf(distTok, dist); got != 0 {
		t.Fatalf("distributor stock after B = %d, want 0", got)
	}

	// 5. The dealer consumes both whole rolls in a service and completes it.
	cust, veh := it.svcCustomer(dealer, "t217-cust", "34F217"+it.suffix[len(it.suffix)-4:])
	s, _ := it.svcCall("POST", "/v1/services", dealerTok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	for _, u := range rolls {
		it.svcCall("POST", "/v1/services/"+s.UUID+"/items", dealerTok,
			map[string]any{"barcode": u.Barcode, "kind": "full"}, http.StatusCreated)
	}
	if done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", dealerTok,
		map[string]string{"status": "completed"}, http.StatusOK); done.Status != "completed" {
		t.Fatalf("service = %+v", done)
	}
	var svcID int64
	var completedAt time.Time
	if err := it.pool.QueryRow(ctx, `SELECT id, completed_at FROM services WHERE uuid = $1`, s.UUID).Scan(&svcID, &completedAt); err != nil {
		t.Fatal(err)
	}
	owned("consumed", "used", "service", svcID, dealer.ID, "entry,placement,order_out,received,order_out,received,consumption")
	if got := stockOf(dealerTok, dealer); got != 0 {
		t.Fatalf("dealer stock after the service = %d, want 0", got)
	}
	if code, h := it.unitHistory(dealerTok, rolls[0].Barcode, ""); code != http.StatusOK || h.Movements.Total != 7 {
		t.Fatalf("dealer unit history = %d %+v", code, h.Movements)
	}

	// 6. Warranty: the service.completed outbox row through the bus; one
	// active warranty per roll for the customer's vehicle (120 months).
	ev := it.outboxEvent(events.ServiceCompleted, s.UUID)
	if err := it.srv.events.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}
	ws := it.serviceWarranties(svcID, dealer.BrandID)
	if len(ws) != 2 {
		t.Fatalf("warranties = %+v", ws)
	}
	ist, err := time.LoadLocation(dealer.Timezone)
	if err != nil {
		t.Skipf("tzdata: %v", err)
	}
	covered := map[int64]bool{}
	for _, w := range ws {
		if w.Status != "active" || w.VehicleID != veh.ID || w.HolderUserID != cust.ID || w.OrganizationID != dealer.ID ||
			!w.StartAt.Time.Equal(completedAt) || !w.EndAt.Time.Equal(warrantyusecase.EndAt(completedAt, 120, ist)) {
			t.Fatalf("warranty = %+v", w)
		}
		covered[w.UnitID] = true
	}
	if !covered[rolls[0].ID] || !covered[rolls[1].ID] {
		t.Fatalf("warranty units = %v", covered)
	}
	if n := it.createdEvents(svcID); n != 2 {
		t.Fatalf("warranty.created events = %d, want 2", n)
	}
	if page, _ := it.listWarranties("/v1/warranties?limit=100", dealerTok); page.Total != 2 {
		t.Fatalf("dealer warranty list = %+v", page)
	}

	// 7. The three books. Each received order is one income row at the
	// seller (on the buyer's cari) and one purchase expense at the buyer
	// (on the seller's cari), each in its organization's currency.
	checks := []f1Book{
		{"center sells A", staffTok, center, dist, a.UUID, fa, "income", "sale", "200"},
		{"distributor buys A", distTok, dist, center, a.UUID, fa, "expense", "purchase", "200"},
		{"distributor sells B", distTok, dist, dealer, b.UUID, fb, "income", "sale", "300"},
		{"dealer buys B", dealerTok, dealer, dist, b.UUID, fb, "expense", "purchase", "300"},
	}
	type booked struct{ balance, amount, rate string }
	snapshot := map[string]booked{}
	readBooks := func(step string) {
		t.Helper()
		for _, w := range checks {
			oc := strings.TrimSpace(w.owner.Currency)
			rate := toOrg[oc]
			if rate == nil {
				t.Fatalf("%s: no expected rate for %s", w.name, oc)
			}
			amount := new(big.Rat).Mul(ratOfText(t, w.total), rate).FloatString(2)
			cari, rows := it.chainBook(w.token, w.owner, w.cp)
			var mine []chainEntry
			for _, r := range rows {
				if r.SourceUUID != nil && *r.SourceUUID == w.order {
					mine = append(mine, r)
				}
			}
			if len(mine) != 1 {
				t.Fatalf("%s %s: %d rows of the order, want 1: %+v", step, w.name, len(mine), rows)
			}
			r := mine[0]
			if r.Direction != w.direction || r.Category != w.category || r.ReversalOfUUID != nil ||
				strings.TrimSpace(r.OrigCurrency) != cur || !sameAmount(&r.OrigAmount, w.total) ||
				strings.TrimSpace(r.Currency) != oc || !sameAmount(&r.Amount, amount) ||
				ratOfText(t, r.Rate).Cmp(rate) != 0 || r.RateDate != w.frozen.snap.RateDate ||
				r.CounterpartyOrganization == nil || r.CounterpartyOrganization.UUID != w.cp.Uuid.String() {
				t.Fatalf("%s %s row = %+v, want %s %s %s %s at %s on %s", step, w.name, r, w.direction,
					w.category, amount, oc, rate.FloatString(8), w.frozen.snap.RateDate)
			}
			// Only this order touches each cari (fresh organizations).
			balance := amount
			if w.direction == "expense" {
				balance = "-" + amount
			}
			if !sameAmount(&cari.Balance, balance) {
				t.Fatalf("%s %s cari balance = %s, want %s", step, w.name, cari.Balance, balance)
			}
			key := w.name
			if prev, ok := snapshot[key]; ok && (prev.balance != cari.Balance || prev.amount != r.Amount || prev.rate != r.Rate) {
				t.Fatalf("%s %s moved: %+v -> %s %s %s", step, w.name, prev, cari.Balance, r.Amount, r.Rate)
			}
			snapshot[key] = booked{cari.Balance, r.Amount, r.Rate}
		}
	}
	readBooks("after the chain")
	if fa.snap.RateDate != day || fb.snap.RateDate != day {
		t.Fatalf("frozen rate days = %s %s, want %s", fa.snap.RateDate, fb.snap.RateDate, day)
	}
	return &f1Chain{
		it: it, cur: cur, toOrg: toOrg, center: center, dist: dist, dealer: dealer, staff: staff,
		staffTok: staffTok, distTok: distTok, dealerTok: dealerTok, item: item, rolls: rolls,
		a: a, b: b, svcID: svcID, svcUUID: s.UUID, centerStockBefore: centerStockBefore, books: checks,
		upsertRate: upsertRate, owned: owned, stockOf: stockOf, readBooks: readBooks,
	}
}
