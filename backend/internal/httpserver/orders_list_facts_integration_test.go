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

type orderListFacts struct {
	UUID            string  `json:"uuid"`
	Currency        string  `json:"currency"`
	Total           string  `json:"total"`
	TotalQuantity   int64   `json:"total_quantity"`
	TotalMeters     *string `json:"total_meters"`
	LineCount       int64   `json:"line_count"`
	LiveTryRate     *string `json:"live_try_rate"`
	LiveTotalTry    *string `json:"live_total_try"`
	SellerWarehouse *struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"seller_warehouse"`
	LastActivity *struct {
		Action string `json:"action"`
		User   *struct {
			UUID string `json:"uuid"`
			Name string `json:"name"`
		} `json:"user"`
	} `json:"last_activity"`
}

// TEC-1054 acceptance: list rows carry the seller warehouse (seller side
// only), the ordered amount, today's TRY rate and total, and the user who
// last handled the order; seller_warehouse (CSV uuids) filters the list.
func TestIntegrationOrdersListFacts(t *testing.T) {
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
	code := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, strings.ToUpper(it.suffix))
	if len(code) > 18 {
		code = code[:18]
	}
	w1, err := it.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: center.ID, Code: "LF1" + code, Name: "Liste Depo 1 " + it.suffix, Active: true,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	w2, err := it.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: center.ID, Code: "LF2" + code, Name: "Liste Depo 2 " + it.suffix, Active: true,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}

	dealer := it.org("t1054-dealer", "dealer", center)
	actor, apw := it.user("t1054-actor")
	it.member(center, actor, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	dealerOwner, dpw := it.user("t1054-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	actorTok := it.loginOrg(actor, apw, center)
	dealerTok := it.loginOrg(dealerOwner, dpw, dealer)

	p := it.product(center, "T1054P")
	it.setListPrice(p, cur, "40")
	items := []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 3}}
	a := it.orderCall("POST", "/v1/orders", actorTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "seller_warehouse_uuid": w1.Uuid.String(), "items": items,
	}, http.StatusCreated)
	b := it.orderCall("POST", "/v1/orders", actorTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "seller_warehouse_uuid": w2.Uuid.String(), "items": items,
	}, http.StatusCreated)

	list := func(tok, query string) []orderListFacts {
		t.Helper()
		code, env := it.do("GET", "/v1/orders?"+query, hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("list %s = %d %s", query, code, errCode(env))
		}
		var page struct {
			Items []orderListFacts `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &page); err != nil {
			t.Fatal(err)
		}
		return page.Items
	}

	got := list(actorTok, "side=seller&limit=100&seller_warehouse="+w1.Uuid.String())
	if len(got) != 1 || got[0].UUID != a.UUID {
		t.Fatalf("seller_warehouse filter = %+v, want only %s", got, a.UUID)
	}
	r := got[0]
	if r.SellerWarehouse == nil || r.SellerWarehouse.UUID != w1.Uuid.String() {
		t.Fatalf("seller warehouse = %+v", r.SellerWarehouse)
	}
	if r.TotalQuantity != 3 || r.TotalMeters != nil || r.LineCount != 1 {
		t.Fatalf("amounts = %d %v %d", r.TotalQuantity, r.TotalMeters, r.LineCount)
	}
	wantTry := "120.00"
	if cur != "TRY" {
		wantTry = "4200.00"
	}
	if r.LiveTryRate == nil || r.LiveTotalTry == nil || *r.LiveTotalTry != wantTry {
		t.Fatalf("live try = %v %v, want %s (total %s %s)", r.LiveTryRate, r.LiveTotalTry, wantTry, r.Total, r.Currency)
	}
	if r.LastActivity == nil || r.LastActivity.User == nil || r.LastActivity.User.UUID != actor.Uuid.String() ||
		r.LastActivity.Action != "created" {
		t.Fatalf("last activity = %+v", r.LastActivity)
	}
	both := list(actorTok, "side=seller&limit=100&seller_warehouse="+w1.Uuid.String()+","+w2.Uuid.String())
	if len(both) != 2 {
		t.Fatalf("two warehouses = %d rows", len(both))
	}
	if code, env := it.do("GET", "/v1/orders?seller_warehouse=nope", hostOlex, actorTok, nil); code != http.StatusBadRequest {
		t.Fatalf("bad seller_warehouse = %d %s", code, errCode(env))
	}

	// A status move by another user makes it the last activity.
	mover, mpw := it.user("t1054-mover")
	it.member(center, mover, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	moverTok := it.loginOrg(mover, mpw, center)
	if code, ec := it.transition(moverTok, b.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d %s", code, ec)
	}
	got = list(actorTok, "side=seller&limit=100&seller_warehouse="+w2.Uuid.String())
	if len(got) != 1 || got[0].LastActivity == nil || got[0].LastActivity.Action != "preparing" ||
		got[0].LastActivity.User == nil || got[0].LastActivity.User.UUID != mover.Uuid.String() ||
		!strings.HasPrefix(got[0].LastActivity.User.Name, "t1054-mover") {
		t.Fatalf("last activity after move = %+v", got)
	}

	// The dealer never sees the seller warehouse, in the list either.
	for _, row := range list(dealerTok, "side=buyer&limit=100") {
		if row.SellerWarehouse != nil {
			t.Fatalf("dealer sees warehouse on %s", row.UUID)
		}
	}
}
