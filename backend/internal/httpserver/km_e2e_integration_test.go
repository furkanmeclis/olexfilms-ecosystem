package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1236 (kutu/metraj, K1–K14) end to end, over HTTP on one product:
//
//  1. Price (K2): box list 400, meter list 30; the dealer's 10 % discount
//     makes them 360 and 27.
//  2. Order (K3/K10/K12): 1 box + 3 m of the same product on one order,
//     the meter line with 5 credits and the plotter mark; approved.
//  3. Picking (K5/K7): the box line takes a closed box; the meter line opens
//     a closed box on confirmation and cuts 3 m (12 m left, not a remnant).
//  4. Shipping: ready, shipped, received by the dealer; the cut piece
//     carries the line's 5 credits.
//  5. Services (K4/K9): the piece is used whole in 5 services, one credit
//     each, its meters kept; the 6th is refused; every completed service
//     gets its own warranty for the piece.
//  6. No vehicle, no warranty (K9): a service without a vehicle cannot be
//     opened, and the quick sale — the only use outside a vehicle — refuses
//     the piece (K13).
//  7. Return (bulgu 6): the piece is credited at meters × the meter price of
//     its order line (3 × 27 = 81).
//  8. Critical stock (K14): the box threshold and the meter threshold are
//     evaluated apart over the center's warehouse.
func TestIntegrationKutuMetreEndToEnd(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	dealer := it.org("tkm-e2e-dealer", "dealer", center)
	var dealerCur string
	if err := it.pool.QueryRow(ctx, `SELECT currency FROM organizations WHERE id = $1`, dealer.ID).Scan(&dealerCur); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(dealerCur) != cur {
		t.Skipf("dealer currency %s differs from the brand currency %s", dealerCur, cur)
	}
	admin, apw := it.user("tkm-e2e-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	sales, spw := it.user("tkm-e2e-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	wh, wpw := it.user("tkm-e2e-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	owner, opw := it.user("tkm-e2e-dealer-owner")
	it.member(dealer, owner, "owner")
	adminTok := it.loginOrg(admin, apw, center)
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(owner, opw, dealer)

	// --- 1. The product card and the meter price (K1/K2/K6).
	film := it.product(center, "TKME2E")
	it.setWarrantyMonths(film, 12)
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: film.ID, BrandID: film.BrandID, Currency: cur,
		SaleToDistributorPrice: pgtype.Text{String: "400", Valid: true},
		MeterPrice:             pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"sellable_by_meter": true, "default_meters": 15, "min_cut_meters": 2})
	if rec := it.raw("PUT", "/v1/catalog/products/"+film.Uuid.String()+"/roll", adminTok, "application/json", raw, nil); rec.Code != http.StatusOK {
		t.Fatalf("roll settings = %d %s", rec.Code, rec.Body.String())
	}
	it.stepUp(sales.Uuid)
	if code, env := it.do("PUT", "/v1/tenant/pricing/discounts/"+dealer.Uuid.String(), hostOlex, salesTok,
		map[string]any{"standard_discount_pct": 10}); code != http.StatusOK {
		t.Fatalf("discount = %d %s", code, errCode(env))
	}

	// The center's warehouse with three closed boxes.
	tag := it.suffix[len(it.suffix)-9:]
	whs, err := it.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: center.ID, Code: "KM" + tag, Name: "km e2e " + tag, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := it.q.CreateRoom(ctx, db.CreateRoomParams{
		OrganizationID: center.ID, WarehouseID: whs.ID, Code: "R1", Name: "room", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	loc, err := it.q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
		OrganizationID: center.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true},
		Type: pgtype.Text{String: "aisle", Valid: true}, Code: "A", Name: "bin A", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	bin := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: center.ID}
	chain := it.stockChain()
	boxes := make([]db.Unit, 0, 3)
	for i := 1; i <= 3; i++ {
		u := chain.unit(center, film, 12360+i)
		chain.post(ledger.TypeEntry, u, chain.nextRef(), bin)
		boxes = append(boxes, u)
	}

	// --- 2. 1 box + 3 m on one order, the meter line with 5 credits and
	// the plotter mark.
	order := it.boxMeterOrder("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": film.Uuid.String(), "meters": "3", "line_credits": 5, "plotter_cut": true},
	}}, http.StatusCreated)
	boxLine, meterLine := order.line(t, "box"), order.line(t, "meter")
	if !sameAmount(boxLine.UnitPrice, "360") || !sameAmount(meterLine.UnitPrice, "27") || !sameAmount(meterLine.LineTotal, "81") ||
		meterLine.LineCredits == nil || *meterLine.LineCredits != 5 || !sameAmount(order.Total, "441") {
		t.Fatalf("order = %+v", order)
	}
	var plotter bool
	if err := it.pool.QueryRow(ctx, `SELECT plotter_cut FROM order_items WHERE uuid = $1`, meterLine.UUID).Scan(&plotter); err != nil || !plotter {
		t.Fatalf("meter line plotter mark = %v %v", plotter, err)
	}
	for _, step := range []struct{ tok, status string }{
		{dealerTok, "submitted"}, {salesTok, "approved"}, {salesTok, "preparing"},
	} {
		if code, ec := it.transition(step.tok, order.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}

	// --- 3. Picking: a closed box on the box line; the meter line opens
	// the second box on confirmation and cuts 3 m.
	if code, ec, _ := it.assign(whTok, order.UUID, boxLine.UUID, map[string]any{"barcode": boxes[0].Barcode}); code != http.StatusOK {
		t.Fatalf("box line = %d %s", code, ec)
	}
	cutPath := "/v1/orders/" + order.UUID + "/items/" + meterLine.UUID
	code, env := it.do("GET", cutPath+"/cut-preview?"+url.Values{"barcode": {boxes[1].Barcode}}.Encode(), hostOlex, whTok, nil)
	var prev cutPreview
	if code != http.StatusOK || json.Unmarshal(env.Data, &prev) != nil || !prev.ClosedBox || prev.SourceMeters != "15.00" ||
		prev.CutMeters != "3.00" || prev.RemainingMeters != "12.00" || prev.Remnant || prev.Credits != 5 {
		t.Fatalf("cut preview = %d %s", code, env.Data)
	}
	if code, env := it.do("POST", cutPath+"/units", hostOlex, whTok, map[string]any{"barcode": boxes[1].Barcode}); code != http.StatusConflict ||
		errCode(env) != "BOX_OPEN_CONFIRM_REQUIRED" {
		t.Fatalf("box without confirmation = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", cutPath+"/units", hostOlex, whTok, map[string]any{
		"barcode": boxes[1].Barcode, "open_box": true, "idempotency_key": "tkm-e2e-" + it.suffix,
	})
	var cut cutAssign
	if code != http.StatusOK || json.Unmarshal(env.Data, &cut) != nil || cut.Split == nil || !cut.Split.BoxOpened ||
		cut.Split.Meters != "3.00" || cut.Split.SourceRemainingMeters != "12.00" || cut.Split.SourceRemnant {
		t.Fatalf("open and cut = %d %s", code, env.Data)
	}
	var left string
	if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text FROM units WHERE id = $1`, boxes[1].ID).Scan(&left); err != nil || left != "12.00" {
		t.Fatalf("opened box left = %s %v", left, err)
	}
	piece, err := it.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: center.BrandID, Barcode: cut.Split.NewBarcode})
	if err != nil {
		t.Fatalf("cut piece: %v", err)
	}

	// --- 4. Ship and receive.
	for _, step := range []struct{ tok, status string }{{whTok, "ready"}, {whTok, "shipped"}, {dealerTok, "received"}} {
		if code, ec := it.transition(step.tok, order.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	if st, ot, oid := it.unitStateOf(piece.ID); st != "available" || ot != "organization" || oid != dealer.ID {
		t.Fatalf("piece after receipt: %s %s %d", st, ot, oid)
	}
	var credits int
	if err := it.pool.QueryRow(ctx, `SELECT unit_credit_default($1)`, piece.ID).Scan(&credits); err != nil || credits != 5 {
		t.Fatalf("piece credits after receipt = %d %v", credits, err)
	}

	// --- 5. Five services with the piece, the 6th refused.
	newSvc := func(i int) serviceView {
		cust, veh := it.svcCustomer(dealer, fmt.Sprintf("tkm-e2e-cust-%d", i),
			fmt.Sprintf("34K%d%s", i, it.suffix[len(it.suffix)-4:]))
		s, _ := it.svcCall("POST", "/v1/services", dealerTok,
			map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
		return s
	}
	svcs := make([]serviceView, 0, 6)
	for i := 1; i <= 6; i++ {
		svcs = append(svcs, newSvc(i))
	}
	for i := 0; i < 5; i++ {
		it.svcCall("POST", "/v1/services/"+svcs[i].UUID+"/items", dealerTok, map[string]any{"barcode": piece.Barcode}, http.StatusCreated)
	}
	if total, rem, used := it.unitCreditsOf(piece.ID); total != 5 || rem != 0 || used != 5 {
		t.Fatalf("after 5 services: credits %d/%d used %d", rem, total, used)
	}
	if _, ec := it.svcCall("POST", "/v1/services/"+svcs[5].UUID+"/items", dealerTok,
		map[string]any{"barcode": piece.Barcode}, http.StatusConflict); ec != "UNIT_CREDITS_EXHAUSTED" {
		t.Fatalf("6th service = %s", ec)
	}
	var meters string
	if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text FROM units WHERE id = $1`, piece.ID).Scan(&meters); err != nil || meters != "3.00" {
		t.Fatalf("piece meters after 5 uses (meters never cut) = %s %v", meters, err)
	}
	svcID := func(uuid string) int64 {
		var id int64
		if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, uuid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for i := 0; i < 5; i++ {
		done, _ := it.svcCall("POST", "/v1/services/"+svcs[i].UUID+"/transitions", dealerTok, map[string]string{"status": "completed"}, http.StatusOK)
		if done.Status != "completed" {
			t.Fatalf("complete %d = %+v", i+1, done)
		}
		if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, svcs[i].UUID)); err != nil {
			t.Fatalf("publish service.completed: %v", err)
		}
		var n int
		for _, w := range it.serviceWarranties(svcID(svcs[i].UUID), dealer.BrandID) {
			if w.UnitID == piece.ID && w.Status == "active" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("service %d piece warranties = %d", i+1, n)
		}
	}
	// Cancelling the 5th completed service gives a credit and the piece back
	// (so it can be returned below).
	it.cancelCompleted(dealerTok, svcs[4].UUID, map[string]any{"reason": "yanlış kayıt"}, http.StatusOK)
	if st, ot, oid := it.unitStateOf(piece.ID); st != "available" || ot != "organization" || oid != dealer.ID {
		t.Fatalf("piece after the cancel: %s %s %d", st, ot, oid)
	}
	if err := it.pool.QueryRow(ctx, `SELECT remaining_meters::text FROM units WHERE id = $1`, piece.ID).Scan(&meters); err != nil || meters != "3.00" {
		t.Fatalf("piece meters after the cancel = %s %v", meters, err)
	}

	// --- 6. No vehicle, no warranty: a service needs a vehicle, and the
	// quick sale refuses the piece.
	cust, _ := it.svcCustomer(dealer, "tkm-e2e-novehicle", "34KX"+it.suffix[len(it.suffix)-4:])
	if code, env := it.do("POST", "/v1/services", hostOlex, dealerTok, map[string]any{"customer_uuid": cust.Uuid.String()}); code != http.StatusBadRequest {
		t.Fatalf("service without a vehicle = %d %s", code, errCode(env))
	}
	if code, env := it.do("GET", "/v1/product-sales/lookup?barcode="+url.QueryEscape(piece.Barcode), hostOlex, dealerTok, nil); code != http.StatusUnprocessableEntity ||
		errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
		t.Fatalf("quick sale lookup = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/product-sales", hostOlex, dealerTok, map[string]any{
		"payment_method": "cash", "lines": []map[string]any{{"barcode": piece.Barcode, "unit_price": "100"}},
	}); code != http.StatusUnprocessableEntity || errCode(env) != "CUT_PIECE_NOT_SELLABLE" {
		t.Fatalf("quick sale = %d %s", code, errCode(env))
	}

	// --- 7. The return: 3 m × 27 (the meter line's price) = 81.
	ret, _ := it.transferCall("POST", "/v1/stock-transfers", dealerTok, map[string]any{
		"kind": "return", "items": []map[string]any{{"barcode": piece.Barcode}},
	}, http.StatusCreated)
	if code, ec := it.transferMove(salesTok, ret.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve return = %d %s", code, ec)
	}
	var unitPrice, lineTotal string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(i.unit_price::text, ''), COALESCE(i.line_total::text, '')
		FROM stock_transfer_request_items i JOIN stock_transfer_requests r ON r.id = i.request_id
		WHERE r.uuid = $1 AND i.unit_id = $2`, ret.UUID, piece.ID).Scan(&unitPrice, &lineTotal); err != nil {
		t.Fatal(err)
	}
	if !sameAmount(&unitPrice, "27") || !sameAmount(&lineTotal, "81") {
		t.Fatalf("return amount = %s / %s, want 27 / 81", unitPrice, lineTotal)
	}

	// --- 8. Critical stock, two thresholds: 1 closed box and 12 m open
	// are left in the warehouse.
	rule := func(minQty int, minMeters string) (level, meterLevel string) {
		t.Helper()
		code, env := it.do("PUT", "/v1/stock/reorder-rules", hostOlex, adminTok, map[string]any{
			"warehouse_uuid": whs.Uuid.String(), "product_uuid": film.Uuid.String(),
			"min_quantity": minQty, "min_meters": minMeters,
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("reorder rule = %d %s %s", code, errCode(env), env.Data)
		}
		var v struct {
			Level      string `json:"level"`
			MeterLevel string `json:"meter_level"`
		}
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatal(err)
		}
		return v.Level, v.MeterLevel
	}
	if l, ml := rule(2, "10"); l != "below_min" || ml != "ok" {
		t.Fatalf("2 boxes / 10 m = %s / %s, want below_min / ok", l, ml)
	}
	if l, ml := rule(1, "20"); l != "ok" || ml != "below_min" {
		t.Fatalf("1 box / 20 m = %s / %s, want ok / below_min", l, ml)
	}
}
