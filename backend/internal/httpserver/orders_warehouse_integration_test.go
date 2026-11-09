package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1028 acceptance: an order the center opens for a dealer falls into
// the center's primary warehouse, or the warehouse picked on creation; a
// dealer order the center approves falls into the primary warehouse.
// Suggestions and pick locations are limited to the order's warehouse, and
// orders.warehouse_new names that warehouse's staff (members holding
// orders.ship, holders without any membership) but not the actor or the
// staff of another warehouse. The dealer never sees the warehouse.
// TEC-1052: a self-prepared order tells no warehouse staff at all.
func TestIntegrationOrdersSellerWarehouse(t *testing.T) {
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

	// Two warehouses of the center, the first one primary (the previous
	// primary comes back after the test).
	var prevPrimary pgtype.Int8
	_ = it.pool.QueryRow(ctx, `SELECT id FROM warehouses WHERE organization_id = $1 AND is_primary`, center.ID).Scan(&prevPrimary)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `UPDATE warehouses SET is_primary = FALSE WHERE organization_id = $1`, center.ID)
		if prevPrimary.Valid {
			_, _ = it.pool.Exec(context.Background(), `UPDATE warehouses SET is_primary = TRUE WHERE id = $1`, prevPrimary.Int64)
		}
	})
	whCode := strings.ToUpper(it.suffix)
	if len(whCode) > 20 {
		whCode = whCode[:20]
	}
	whCode = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, whCode)
	type wh struct {
		w   db.Warehouse
		loc ledger.Owner
	}
	newWarehouse := func(prefix string) wh {
		t.Helper()
		w, err := it.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
			OrganizationID: center.ID, Code: prefix + whCode, Name: "Depo " + prefix + " " + it.suffix, Active: true,
		})
		if err != nil {
			t.Fatalf("warehouse: %v", err)
		}
		r, err := it.q.CreateRoom(ctx, db.CreateRoomParams{OrganizationID: center.ID, WarehouseID: w.ID, Code: "R1", Name: "Oda", Active: true})
		if err != nil {
			t.Fatalf("room: %v", err)
		}
		l, err := it.q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
			OrganizationID: center.ID, RoomID: pgtype.Int8{Int64: r.ID, Valid: true}, Type: pgtype.Text{String: "aisle", Valid: true}, Code: "A1", Name: "Koridor", Active: true,
		})
		if err != nil {
			t.Fatalf("location: %v", err)
		}
		return wh{w: w, loc: ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: l.ID, OrgID: center.ID}}
	}
	w1, w2 := newWarehouse("WA"), newWarehouse("WB")
	if _, err := it.pool.Exec(ctx, `UPDATE warehouses SET is_primary = FALSE WHERE organization_id = $1`, center.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE warehouses SET is_primary = TRUE WHERE id = $1`, w1.w.ID); err != nil {
		t.Fatal(err)
	}

	dealer := it.org("t1028-dealer", "dealer", center)
	actor, apw := it.user("t1028-actor")
	it.member(center, actor, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	staff1, _ := it.user("t1028-w1")
	it.member(center, staff1, "staff", rbac.RoleCenterWarehouse)
	staff2, _ := it.user("t1028-w2")
	it.member(center, staff2, "staff", rbac.RoleCenterWarehouse)
	anyStaff, _ := it.user("t1028-any")
	it.member(center, anyStaff, "staff", rbac.RoleCenterWarehouse)
	for _, m := range []struct {
		u db.User
		w db.Warehouse
	}{{staff1, w1.w}, {staff2, w2.w}} {
		if _, err := it.q.InsertWarehouseMember(ctx, db.InsertWarehouseMemberParams{
			OrganizationID: center.ID, WarehouseID: m.w.ID, UserID: m.u.ID,
		}); err != nil {
			t.Fatalf("warehouse member: %v", err)
		}
	}
	dealerOwner, rpw := it.user("t1028-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	actorTok := it.loginOrg(actor, apw, center)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)

	p := it.product(center, "T1028P")
	it.setListPrice(p, cur, "40")
	chain := it.stockChain()
	u1 := chain.unit(center, p, 10281)
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), w1.loc)
	u2 := chain.unit(center, p, 10282)
	chain.post(ledger.TypeEntry, u2, chain.nextRef(), w2.loc)
	items := []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}}

	type sellerWarehouse struct {
		SellerWarehouse *struct {
			UUID      string `json:"uuid"`
			Name      string `json:"name"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"seller_warehouse"`
	}
	create := func(body map[string]any) (orderView, sellerWarehouse) {
		t.Helper()
		code, env := it.do("POST", "/v1/orders", hostOlex, actorTok, body)
		if code != http.StatusCreated {
			t.Fatalf("create = %d %s", code, errCode(env))
		}
		var o orderView
		var sw sellerWarehouse
		if err := json.Unmarshal(env.Data, &o); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(env.Data, &sw); err != nil {
			t.Fatal(err)
		}
		return o, sw
	}
	notified := func(orderUUID string, u db.User) int {
		return it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.warehouse_new'
			AND payload->'data'->>'order_uuid' = $1::text AND payload->'data'->'notify_user_ids' @> to_jsonb($2::bigint)`,
			orderUUID, u.ID)
	}

	// An unknown warehouse is refused; a dealer cannot pick one.
	code, env := it.do("POST", "/v1/orders", hostOlex, actorTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "seller_warehouse_uuid": dealer.Uuid.String(), "items": items,
	})
	if code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("foreign warehouse = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/orders", hostOlex, dealerTok, map[string]any{
		"seller_warehouse_uuid": w1.w.Uuid.String(), "items": items,
	})
	if code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("dealer warehouse = %d %s", code, errCode(env))
	}

	// 1. Default: the primary warehouse; its staff and the unrestricted
	// staff are notified, not the other warehouse or the actor.
	a, aw := create(map[string]any{"buyer_org_uuid": dealer.Uuid.String(), "items": items})
	if aw.SellerWarehouse == nil || aw.SellerWarehouse.UUID != w1.w.Uuid.String() || !aw.SellerWarehouse.IsPrimary {
		t.Fatalf("default warehouse = %+v", aw.SellerWarehouse)
	}
	if notified(a.UUID, staff1) != 1 || notified(a.UUID, anyStaff) != 1 || notified(a.UUID, staff2) != 0 ||
		notified(a.UUID, actor) != 0 {
		t.Fatalf("recipients of %s: w1 %d any %d w2 %d actor %d", a.UUID,
			notified(a.UUID, staff1), notified(a.UUID, anyStaff), notified(a.UUID, staff2), notified(a.UUID, actor))
	}
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.warehouse_new'
		AND payload->'data'->>'order_uuid' = $1::text AND payload->'data'->>'warehouse_name' = $2::text`,
		a.UUID, w1.w.Name); n != 1 {
		t.Fatalf("warehouse name in payload = %d", n)
	}

	// TEC-1052: the creator prepares it himself: no warehouse staff (of the
	// picked warehouse, another one or unrestricted) is told; the history
	// keeps the choice. A dealer's own order cannot carry the flag.
	sp, _ := create(map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "seller_warehouse_uuid": w2.w.Uuid.String(),
		"self_prepare": true, "items": items,
	})
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.warehouse_new'
		AND payload->'data'->>'order_uuid' = $1::text`, sp.UUID); n != 0 {
		t.Fatalf("self-prepared order warehouse_new events = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM order_status_history h JOIN orders o ON o.id = h.order_id
		WHERE o.uuid = $1::uuid AND h.metadata->>'self_prepare' = 'true'`, sp.UUID); n != 1 {
		t.Fatalf("self_prepare in history = %d", n)
	}
	code, env = it.do("POST", "/v1/orders", hostOlex, dealerTok, map[string]any{"self_prepare": true, "items": items})
	if code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("dealer self_prepare = %d %s", code, errCode(env))
	}

	// 2. A picked warehouse: suggestions and pick locations stay in it.
	b, bw := create(map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "seller_warehouse_uuid": w2.w.Uuid.String(), "items": items,
	})
	if bw.SellerWarehouse == nil || bw.SellerWarehouse.UUID != w2.w.Uuid.String() {
		t.Fatalf("picked warehouse = %+v", bw.SellerWarehouse)
	}
	if notified(b.UUID, staff2) != 1 || notified(b.UUID, anyStaff) != 1 || notified(b.UUID, staff1) != 0 {
		t.Fatalf("recipients of %s: w2 %d any %d w1 %d", b.UUID,
			notified(b.UUID, staff2), notified(b.UUID, anyStaff), notified(b.UUID, staff1))
	}
	if code, ec := it.transition(actorTok, b.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d %s", code, ec)
	}
	code, env = it.do("GET", "/v1/orders/"+b.UUID+"/items/"+b.Items[0].UUID+"/suggestions", hostOlex, actorTok, nil)
	if code != http.StatusOK {
		t.Fatalf("suggestions = %d %s", code, errCode(env))
	}
	var sug struct {
		Suggestions []struct {
			Barcode string `json:"barcode"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(env.Data, &sug); err != nil {
		t.Fatal(err)
	}
	if len(sug.Suggestions) != 1 || sug.Suggestions[0].Barcode != u2.Barcode {
		t.Fatalf("suggestions = %+v, want only %s", sug.Suggestions, u2.Barcode)
	}
	code, env = it.do("GET", "/v1/orders/"+b.UUID+"/items/"+b.Items[0].UUID+"/pick-locations", hostOlex, actorTok, nil)
	if code != http.StatusOK {
		t.Fatalf("pick locations = %d %s", code, errCode(env))
	}
	var pl struct {
		Items []struct {
			LocationCode *string `json:"location_code"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &pl); err != nil {
		t.Fatal(err)
	}
	locs := pl.Items
	for _, l := range locs {
		if l.LocationCode != nil && !strings.HasPrefix(*l.LocationCode, w2.w.Code+"-") {
			t.Fatalf("pick location outside the order's warehouse: %s", *l.LocationCode)
		}
	}
	if len(locs) == 0 {
		t.Fatal("no pick location in the order's warehouse")
	}

	// 3. The dealer does not see the warehouse.
	code, env = it.do("GET", "/v1/orders/"+b.UUID, hostOlex, dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("dealer get = %d %s", code, errCode(env))
	}
	var dv sellerWarehouse
	if err := json.Unmarshal(env.Data, &dv); err != nil || dv.SellerWarehouse != nil {
		t.Fatalf("dealer sees warehouse %+v %v", dv.SellerWarehouse, err)
	}

	// 4. A dealer order the center approves falls into the primary warehouse.
	own := it.orderCall("POST", "/v1/orders", dealerTok, map[string]any{"items": items}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{{dealerTok, "submitted"}, {actorTok, "approved"}} {
		if code, ec := it.transition(step.tok, own.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	if row := it.orderRow(own.UUID, center.BrandID); !row.SellerWarehouseID.Valid || row.SellerWarehouseID.Int64 != w1.w.ID {
		t.Fatalf("approved dealer order warehouse = %v", row.SellerWarehouseID)
	}
	if notified(own.UUID, staff1) != 1 || notified(own.UUID, staff2) != 0 {
		t.Fatalf("approval recipients: w1 %d w2 %d", notified(own.UUID, staff1), notified(own.UUID, staff2))
	}
}
