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

// TEC-1024 acceptance: center staff opens an order for a dealer directly
// under the center. It starts approved (rate frozen, created_by_seller),
// products without a price are accepted at 0 (price_missing) and a typed
// unit price wins; the dealer's members get orders.created_by_seller. The
// warehouse flow (preparing, assign, ready, shipped) and the dealer's
// receipt work unchanged. A dealer of a distributor is refused, a dealer
// cannot use buyer_org_uuid, and the dealer's own flow is untouched.
func TestIntegrationOrdersCenterCreatesForDealer(t *testing.T) {
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
	dealer := it.org("t1024-dealer", "dealer", center)
	dist := it.org("t1024-dist", "distributor", center)
	farDealer := it.org("t1024-far-dealer", "dealer", dist)

	staff, spw := it.user("t1024-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	dealerOwner, rpw := it.user("t1024-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	staffTok := it.loginOrg(staff, spw, center)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)

	priced := it.product(center, "T1024P")
	noPrice := it.product(center, "T1024N")
	it.setListPrice(priced, cur, "50")
	chain := it.stockChain()
	loc := chain.location(center, "T1024")
	u1 := chain.unit(center, priced, 10241)
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), loc)
	u2 := chain.unit(center, noPrice, 10242)
	chain.post(ledger.TypeEntry, u2, chain.nextRef(), loc)

	items := []any{
		map[string]any{"product_uuid": priced.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": noPrice.Uuid.String(), "quantity": 1},
	}

	// A dealer of a distributor is not a child of the center.
	code, env := it.do("POST", "/v1/orders", hostOlex, staffTok,
		map[string]any{"buyer_org_uuid": farDealer.Uuid.String(), "items": items})
	if code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("far dealer = %d %s", code, errCode(env))
	}
	// A dealer cannot order on behalf of another organization.
	code, env = it.do("POST", "/v1/orders", hostOlex, dealerTok,
		map[string]any{"buyer_org_uuid": dealer.Uuid.String(), "items": items})
	if code != http.StatusForbidden {
		t.Fatalf("dealer with buyer_org_uuid = %d %s", code, errCode(env))
	}
	// A negative typed price is refused.
	bad := []any{map[string]any{"product_uuid": noPrice.Uuid.String(), "quantity": 1, "unit_price": -1}}
	code, env = it.do("POST", "/v1/orders", hostOlex, staffTok,
		map[string]any{"buyer_org_uuid": dealer.Uuid.String(), "items": bad})
	if code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("negative price = %d %s", code, errCode(env))
	}

	// 1. Happy path: approved at once, missing price accepted at 0.
	code, env = it.do("POST", "/v1/orders", hostOlex, staffTok,
		map[string]any{"buyer_org_uuid": dealer.Uuid.String(), "note": "phone order", "items": items})
	if code != http.StatusCreated {
		t.Fatalf("center create = %d %s", code, errCode(env))
	}
	var o orderView
	var extra struct {
		CreatedBySeller bool `json:"created_by_seller"`
		CreatedBy       *struct {
			UUID string `json:"uuid"`
			Name string `json:"name"`
		} `json:"created_by"`
		Lines []struct {
			PriceMissing bool `json:"price_missing"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &o); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(env.Data, &extra); err != nil {
		t.Fatal(err)
	}
	if o.Status != "approved" || !extra.CreatedBySeller || o.Role != "seller" || o.Seller.UUID != center.Uuid.String() ||
		o.Buyer.UUID != dealer.Uuid.String() || o.TryRate == nil || len(o.Items) != 2 {
		t.Fatalf("created = %+v", o)
	}
	if o.Items[0].PriceSource == "missing" || extra.Lines[0].PriceMissing || sameAmount(&o.Items[0].UnitPrice, "0") {
		t.Fatalf("priced line = %+v", o.Items[0])
	}
	if o.Items[1].PriceSource != "missing" || !extra.Lines[1].PriceMissing || !sameAmount(&o.Items[1].UnitPrice, "0") {
		t.Fatalf("missing line = %+v", o.Items[1])
	}
	if len(o.History) != 1 || o.History[0].ToStatus != "approved" {
		t.Fatalf("history = %+v", o.History)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.created_by_seller'
		AND payload->'data'->>'order_uuid' = $1::text AND payload->'data'->'notify_user_ids' @> to_jsonb($2::bigint)`,
		o.UUID, dealerOwner.ID); n != 1 {
		t.Fatalf("created_by_seller events for dealer owner = %d", n)
	}
	if row := it.orderRow(o.UUID, center.BrandID); !row.CreatedBySeller || !row.CreatedByUserID.Valid ||
		row.CreatedByUserID.Int64 != staff.ID {
		t.Fatalf("row = created_by_seller %v created_by %v", row.CreatedBySeller, row.CreatedByUserID)
	}
	// TEC-1090: the detail and the list name the creator from the same column.
	if extra.CreatedBy == nil || extra.CreatedBy.UUID != staff.Uuid.String() || extra.CreatedBy.Name == "" {
		t.Fatalf("detail created_by = %+v", extra.CreatedBy)
	}
	code, env = it.do("GET", "/v1/orders?side=seller&limit=100", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, errCode(env))
	}
	var page struct {
		Items []struct {
			UUID      string `json:"uuid"`
			CreatedBy *struct {
				UUID string `json:"uuid"`
			} `json:"created_by"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, item := range page.Items {
		if item.UUID == o.UUID {
			listed = item.CreatedBy != nil && item.CreatedBy.UUID == staff.Uuid.String()
		}
	}
	if !listed {
		t.Fatalf("list created_by missing for %s", o.UUID)
	}

	// 2. A typed unit price wins.
	manual := []any{map[string]any{"product_uuid": noPrice.Uuid.String(), "quantity": 2, "unit_price": "12.5"}}
	m := it.orderCall("POST", "/v1/orders", staffTok,
		map[string]any{"buyer_org_uuid": dealer.Uuid.String(), "items": manual}, http.StatusCreated)
	if m.Items[0].PriceSource != "manual" || !sameAmount(&m.Items[0].UnitPrice, "12.5") || !sameAmount(&m.Total, "25") {
		t.Fatalf("manual = %+v", m)
	}

	// 3. The warehouse flow runs unchanged and the dealer receives it.
	if code, ec := it.transition(staffTok, o.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d %s", code, ec)
	}
	prep := it.orderCall("GET", "/v1/orders/"+o.UUID, staffTok, nil, http.StatusOK)
	it.shipOrder(staffTok, prep, [][]map[string]any{{{"barcode": u1.Barcode}}, {{"barcode": u2.Barcode}}})
	// The seller marks it delivered (status only, with a note); only the
	// buyer can, and the dealer then receives it.
	if code, ec := it.transition(dealerTok, o.UUID, "delivered"); code != http.StatusForbidden {
		t.Fatalf("dealer delivered = %d %s", code, ec)
	}
	code, env = it.do("POST", "/v1/orders/"+o.UUID+"/transitions", hostOlex, staffTok,
		map[string]string{"status": "delivered", "reason": "handed over at the depot"})
	if code != http.StatusOK {
		t.Fatalf("delivered = %d %s", code, errCode(env))
	}
	var del struct {
		Status      string  `json:"status"`
		DeliveredAt *string `json:"delivered_at"`
	}
	if err := json.Unmarshal(env.Data, &del); err != nil || del.Status != "delivered" || del.DeliveredAt == nil {
		t.Fatalf("delivered view = %+v %v", del, err)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1 AND type = 'received'`, u1.ID); n != 0 {
		t.Fatalf("received movements before receipt = %d", n)
	}
	if code, ec := it.transition(dealerTok, o.UUID, "received"); code != http.StatusOK {
		t.Fatalf("received = %d %s", code, ec)
	}
	if v := it.orderCall("GET", "/v1/orders/"+o.UUID, dealerTok, nil, http.StatusOK); v.Status != "received" {
		t.Fatalf("final = %s", v.Status)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1 AND type = 'received'`, u1.ID); n != 1 {
		t.Fatalf("received movements after receipt = %d", n)
	}

	// 4. The dealer's own order still starts as a draft to its supplier.
	own := it.orderCall("POST", "/v1/orders", dealerTok,
		map[string]any{"items": []any{map[string]any{"product_uuid": priced.Uuid.String(), "quantity": 1}}}, http.StatusCreated)
	if own.Status != "draft" || own.Role != "buyer" {
		t.Fatalf("dealer own = %+v", own)
	}
}
