package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type orderView struct {
	UUID         string          `json:"uuid"`
	Status       string          `json:"status"`
	StatusLabel  string          `json:"status_label"`
	Role         string          `json:"role"`
	Currency     string          `json:"currency"`
	Subtotal     string          `json:"subtotal"`
	Total        string          `json:"total"`
	TryRate      *string         `json:"try_rate"`
	RateSnapshot json.RawMessage `json:"rate_snapshot"`
	Seller       struct {
		UUID string `json:"uuid"`
	} `json:"seller"`
	Buyer struct {
		UUID string `json:"uuid"`
	} `json:"buyer"`
	AvailableTransitions []string `json:"available_transitions"`
	Items                []struct {
		UnitPrice   string `json:"unit_price"`
		PriceSource string `json:"price_source"`
		LineTotal   string `json:"line_total"`
		Quantity    *int32 `json:"quantity"`
	} `json:"items"`
	History []struct {
		FromStatus *string `json:"from_status"`
		ToStatus   string  `json:"to_status"`
	} `json:"history"`
}

func (it *itest) orderCall(method, path, access string, body any, want int) orderView {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	var v orderView
	if code < 300 {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return v
}

func (it *itest) transition(access, orderUUID, status string) (int, string) {
	it.t.Helper()
	code, env := it.do("POST", "/v1/orders/"+orderUUID+"/transitions", hostOlex, access, map[string]string{"status": status})
	return code, errCode(env)
}

func (it *itest) setListPrice(p db.Product, currency, sale string) {
	it.t.Helper()
	if _, err := it.q.UpsertProductPrice(context.Background(), db.UpsertProductPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, Currency: currency,
		SaleToDistributorPrice: pgtype.Text{String: sale, Valid: true},
	}); err != nil {
		it.t.Fatalf("list price: %v", err)
	}
}

// TEC-166 acceptance: center -> distributor order takes its price from the
// effective purchase price (client prices are ignored); approval freezes the
// rate and re-locks the prices; later price list changes leave the order
// alone; organizations outside the order get 404; invalid and stock-bound
// transitions answer 409. A distributor -> dealer order is invisible to the
// distributor's other dealers.
func TestIntegrationOrdersCenterToDistributor(t *testing.T) {
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
	dist := it.org("t166-dist", "distributor", center)
	otherDist := it.org("t166-dist2", "distributor", center)
	dealer := it.org("t166-dealer", "dealer", dist)
	otherDealer := it.org("t166-dealer2", "dealer", dist)

	staff, spw := it.user("t166-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	distOwner, dpw := it.user("t166-dist-owner")
	it.member(dist, distOwner, "owner")
	otherOwner, opw := it.user("t166-dist2-owner")
	it.member(otherDist, otherOwner, "owner")
	dealerOwner, rpw := it.user("t166-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	otherDealerOwner, xpw := it.user("t166-dealer2-owner")
	it.member(otherDealer, otherDealerOwner, "owner")

	p := it.product(center, "T166")
	noPrice := it.product(center, "T166N")
	it.setListPrice(p, cur, "80")

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	otherTok := it.loginOrg(otherOwner, opw, otherDist)

	// 1. The distributor opens a draft; the client's price is ignored.
	item := map[string]any{"product_uuid": p.Uuid.String(), "quantity": 3, "unit_price": "1", "line_total": "3"}
	o := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{item}, "total": "3"}, http.StatusCreated)
	if o.Status != "draft" || o.Role != "buyer" || o.Seller.UUID != center.Uuid.String() || o.Currency != cur ||
		len(o.Items) != 1 || !sameAmount(&o.Items[0].UnitPrice, "80") || o.Items[0].PriceSource != "list" ||
		!sameAmount(&o.Total, "240") || string(o.RateSnapshot) != "null" || o.TryRate != nil || o.StatusLabel == "" {
		t.Fatalf("draft = %+v", o)
	}
	// A product without a price in the order currency is refused.
	code, env := it.do("POST", "/v1/orders", hostOlex, distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": noPrice.Uuid.String(), "quantity": 1},
	}})
	if code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("no price = %d %s", code, errCode(env))
	}
	// The center cannot buy products (no supplier).
	if code, env = it.do("POST", "/v1/orders", hostOlex, staffTok, map[string]any{"items": []any{item}}); code != http.StatusUnprocessableEntity ||
		errCode(env) != "ORDER_NO_SUPPLIER" {
		t.Fatalf("center order = %d %s", code, errCode(env))
	}

	// 2. Organizations outside the order get 404.
	it.orderCall("GET", "/v1/orders/"+o.UUID, otherTok, nil, http.StatusNotFound)
	if code, _ = it.transition(otherTok, o.UUID, "cancelled"); code != http.StatusNotFound {
		t.Fatalf("other distributor cancel = %d", code)
	}
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	it.orderCall("GET", "/v1/orders/"+o.UUID, dealerTok, nil, http.StatusNotFound)
	if v := it.orderCall("GET", "/v1/orders/"+o.UUID, staffTok, nil, http.StatusOK); v.Role != "seller" {
		t.Fatalf("center role = %s", v.Role)
	}

	// 3. Invalid transitions answer 409; the seller cannot submit, the
	// buyer cannot approve.
	if code, ec := it.transition(staffTok, o.UUID, "approved"); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
		t.Fatalf("approve draft = %d %s", code, ec)
	}
	if code, _ := it.transition(staffTok, o.UUID, "submitted"); code != http.StatusForbidden {
		t.Fatalf("seller submit = %d", code)
	}
	if code, ec := it.transition(distTok, o.UUID, "bogus"); code != http.StatusBadRequest {
		t.Fatalf("unknown status = %d %s", code, ec)
	}
	if code, _ := it.transition(distTok, o.UUID, "submitted"); code != http.StatusOK {
		t.Fatalf("submit = %d", code)
	}
	if code, _ := it.transition(distTok, o.UUID, "submitted"); code != http.StatusOK {
		t.Fatalf("repeated submit must be a no-op, got %d", code)
	}
	if code, _ := it.transition(distTok, o.UUID, "approved"); code != http.StatusForbidden {
		t.Fatalf("buyer approve = %d", code)
	}
	if code, _ := it.do("PUT", "/v1/orders/"+o.UUID+"/items", hostOlex, distTok, map[string]any{"items": []any{item}}); code != http.StatusConflict {
		t.Fatalf("edit submitted = %d", code)
	}

	// 4. The list price changes before approval: approval takes the current
	// price and freezes it together with the rate.
	it.setListPrice(p, cur, "90")
	if code, ec := it.transition(staffTok, o.UUID, "approved"); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, ec)
	}
	a := it.orderCall("GET", "/v1/orders/"+o.UUID, staffTok, nil, http.StatusOK)
	var snap struct {
		Base, Quote, Rate, Source string
	}
	if err := json.Unmarshal(a.RateSnapshot, &snap); err != nil {
		t.Fatalf("snapshot %s: %v", a.RateSnapshot, err)
	}
	wantRate := "1"
	if cur != "TRY" {
		wantRate = "35"
	}
	if a.Status != "approved" || !sameAmount(&a.Items[0].UnitPrice, "90") || !sameAmount(&a.Total, "270") ||
		snap.Base != cur || snap.Quote != "TRY" || !sameAmount(&snap.Rate, wantRate) || !sameAmount(a.TryRate, wantRate) {
		t.Fatalf("approved = %+v snapshot %+v", a, snap)
	}

	// 5. After approval the price list moves; the order does not.
	it.setListPrice(p, cur, "100")
	if _, err := it.q.UpsertDistributorPriceOverride(ctx, db.UpsertDistributorPriceOverrideParams{
		ProductID: p.ID, BrandID: p.BrandID, DistributorOrgID: dist.ID, Currency: cur, Price: "60",
	}); err != nil {
		t.Fatal(err)
	}
	b := it.orderCall("GET", "/v1/orders/"+o.UUID, distTok, nil, http.StatusOK)
	if !sameAmount(&b.Items[0].UnitPrice, "90") || !sameAmount(&b.Total, "270") || !sameAmount(b.TryRate, wantRate) {
		t.Fatalf("frozen order moved: %+v", b)
	}

	// 6. Stock-bound transitions are not available yet (TEC-167/168).
	for _, st := range []string{"shipped", "received", "cancelling", "ready", "delivered"} {
		if code, ec := it.transition(staffTok, o.UUID, st); code != http.StatusConflict || ec != "ORDER_TRANSITION_UNAVAILABLE" {
			t.Fatalf("%s = %d %s", st, code, ec)
		}
	}
	if code, _ := it.transition(staffTok, o.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d", code)
	}
	// Preparing cannot jump to processing; the buyer may still cancel.
	if code, ec := it.transition(staffTok, o.UUID, "processing"); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
		t.Fatalf("preparing -> processing = %d %s", code, ec)
	}
	if code, _ := it.do("POST", "/v1/orders/"+o.UUID+"/transitions", hostOlex, distTok,
		map[string]string{"status": "cancelled", "reason": "changed plans"}); code != http.StatusOK {
		t.Fatalf("cancel = %d", code)
	}
	c := it.orderCall("GET", "/v1/orders/"+o.UUID, distTok, nil, http.StatusOK)
	wantHist := []string{"draft", "submitted", "approved", "preparing", "cancelled"}
	if c.Status != "cancelled" || len(c.History) != len(wantHist) {
		t.Fatalf("history = %+v", c.History)
	}
	for i, h := range c.History {
		if h.ToStatus != wantHist[i] {
			t.Fatalf("history[%d] = %s, want %s", i, h.ToStatus, wantHist[i])
		}
	}
	if code, ec := it.transition(distTok, o.UUID, "submitted"); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
		t.Fatalf("cancelled -> submitted = %d %s", code, ec)
	}
	var events []string
	rows, err := it.pool.Query(ctx, `SELECT event_name FROM outbox_events WHERE payload->'data'->>'order_uuid' = $1 ORDER BY id`, o.UUID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		events = append(events, n)
	}
	rows.Close()
	wantEv := []string{"orders.created", "orders.submitted", "orders.approved", "orders.preparing", "orders.cancelled"}
	if strings.Join(events, ",") != strings.Join(wantEv, ",") {
		t.Fatalf("outbox = %v, want %v", events, wantEv)
	}

	// 7. Distributor -> dealer: the dealer pays the distributor's dealer
	// price; the distributor's other dealer cannot see the order.
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, DistributorOrgID: dist.ID, Currency: cur, Price: "95.5",
	}); err != nil {
		t.Fatal(err)
	}
	d := it.orderCall("POST", "/v1/orders", dealerTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": p.Uuid.String(), "quantity": 2, "unit_price": "0"},
	}}, http.StatusCreated)
	if d.Seller.UUID != dist.Uuid.String() || !sameAmount(&d.Items[0].UnitPrice, "95.5") ||
		d.Items[0].PriceSource != "distributor_dealer" || !sameAmount(&d.Total, "191") {
		t.Fatalf("dealer order = %+v", d)
	}
	xTok := it.loginOrg(otherDealerOwner, xpw, otherDealer)
	it.orderCall("GET", "/v1/orders/"+d.UUID, xTok, nil, http.StatusNotFound)
	code, env = it.do("GET", "/v1/orders", hostOlex, xTok, nil)
	if code != http.StatusOK || strings.Contains(string(env.Data), d.UUID) {
		t.Fatalf("other dealer list = %d %s", code, env.Data)
	}
	code, env = it.do("GET", "/v1/orders?side=seller", hostOlex, distTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), d.UUID) || strings.Contains(string(env.Data), o.UUID) {
		t.Fatalf("distributor sales = %d %s", code, env.Data)
	}
	code, env = it.do("GET", "/v1/orders?side=buyer", hostOlex, distTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), o.UUID) {
		t.Fatalf("distributor purchases = %d %s", code, env.Data)
	}
}
