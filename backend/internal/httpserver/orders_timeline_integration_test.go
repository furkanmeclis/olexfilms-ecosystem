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

type timelineEntry struct {
	Action string `json:"action"`
	User   *struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"user"`
	Barcode  *string `json:"barcode"`
	Product  *string `json:"product"`
	Quantity *int64  `json:"quantity"`
	Lines    *int64  `json:"lines"`
}

// TEC-1060 acceptance: the order detail has a timeline of every step with
// its user — created, the draft edit, the status moves, each barcode
// assignment (with its amount, a fixed barcode scanned again counts the
// growth) and an unassignment — and the list's last activity follows the
// latest of them.
func TestIntegrationOrdersTimeline(t *testing.T) {
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
	dist := it.org("t1060-dist", "distributor", center)
	manager, mpw := it.user("t1060-manager")
	it.member(center, manager, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	picker, ppw := it.user("t1060-picker")
	it.member(center, picker, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1060-dist-owner")
	it.member(dist, distOwner, "owner")

	piece := it.product(center, "T1060P")
	fixed := it.product(center, "T1060F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}
	fixed.UsesFixedBarcode = true
	it.setListPrice(piece, cur, "50")
	it.setListPrice(fixed, cur, "5")
	chain := it.stockChain()
	loc := chain.location(center, "T1060")
	su := chain.unit(center, piece, 10601)
	chain.post(ledger.TypeEntry, su, chain.nextRef(), loc)
	fu := chain.fixedUnit(center, fixed, 10602, loc, 5)

	managerTok := it.loginOrg(manager, mpw, center)
	pickerTok := it.loginOrg(picker, ppw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)

	o := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
	}}, http.StatusCreated)
	if code, env := it.do("PUT", "/v1/orders/"+o.UUID+"/items", hostOlex, distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 3},
	}}); code != http.StatusOK {
		t.Fatalf("edit = %d %s", code, errCode(env))
	}
	for _, step := range []struct{ tok, status string }{
		{distTok, "submitted"}, {managerTok, "approved"}, {pickerTok, "preparing"},
	} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	o = it.orderCall("GET", "/v1/orders/"+o.UUID, managerTok, nil, http.StatusOK)
	var pieceLine, fixedLine string
	for _, l := range o.Items {
		if l.Quantity != nil && *l.Quantity == 1 {
			pieceLine = l.UUID
		} else {
			fixedLine = l.UUID
		}
	}
	if code, ec, _ := it.assign(pickerTok, o.UUID, pieceLine, map[string]any{"barcode": su.Barcode}); code != http.StatusOK {
		t.Fatalf("assign serial = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(pickerTok, o.UUID, fixedLine, map[string]any{"barcode": fu.Barcode, "quantity": 2}); code != http.StatusOK {
		t.Fatalf("assign fixed = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(pickerTok, o.UUID, fixedLine, map[string]any{"barcode": fu.Barcode, "quantity": 1}); code != http.StatusOK {
		t.Fatalf("grow fixed = %d %s", code, ec)
	}
	if code, env := it.do("DELETE", "/v1/orders/"+o.UUID+"/items/"+pieceLine+"/units/"+su.Uuid.String(), hostOlex, managerTok, nil); code != http.StatusOK {
		t.Fatalf("unassign = %d %s", code, errCode(env))
	}

	code, env := it.do("GET", "/v1/orders/"+o.UUID, hostOlex, managerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("get = %d %s", code, errCode(env))
	}
	var detail struct {
		Timeline []timelineEntry `json:"timeline"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatal(err)
	}
	type want struct {
		action string
		user   string
		qty    int64
	}
	wants := []want{
		{"created", distOwner.Uuid.String(), 0},
		{"updated", distOwner.Uuid.String(), 0},
		{"submitted", distOwner.Uuid.String(), 0},
		{"approved", manager.Uuid.String(), 0},
		{"preparing", picker.Uuid.String(), 0},
		{"unit_assigned", picker.Uuid.String(), 1},
		{"unit_assigned", picker.Uuid.String(), 2},
		{"unit_assigned", picker.Uuid.String(), 1},
		{"unit_unassigned", manager.Uuid.String(), 1},
	}
	if len(detail.Timeline) != len(wants) {
		raw, _ := json.Marshal(detail.Timeline)
		t.Fatalf("timeline = %s", raw)
	}
	for i, w := range wants {
		e := detail.Timeline[i]
		if e.Action != w.action || e.User == nil || e.User.UUID != w.user {
			t.Fatalf("timeline[%d] = %+v, want %+v", i, e, w)
		}
		if w.qty > 0 && (e.Quantity == nil || *e.Quantity != w.qty || e.Barcode == nil || e.Product == nil) {
			t.Fatalf("timeline[%d] amount = %+v, want %d", i, e, w.qty)
		}
	}
	if l := detail.Timeline[1].Lines; l == nil || *l != 2 {
		t.Fatalf("edit lines = %v", l)
	}
	if b := detail.Timeline[6].Barcode; *b != fu.Barcode {
		t.Fatalf("fixed barcode = %s", *b)
	}

	var row *orderListFacts
	for _, r := range listOrderFacts(t, it, managerTok, "side=seller&limit=100&status=preparing") {
		if r.UUID == o.UUID {
			row = &r
		}
	}
	if row == nil || row.LastActivity == nil || row.LastActivity.Action != "unit_unassigned" ||
		row.LastActivity.User == nil || row.LastActivity.User.UUID != manager.Uuid.String() {
		t.Fatalf("last activity = %+v", row)
	}
	// The buyer sees the same timeline (steps are not secret to the parties).
	code, env = it.do("GET", "/v1/orders/"+o.UUID, hostOlex, distTok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"unit_unassigned"`) {
		t.Fatalf("buyer timeline = %d", code)
	}
}

func listOrderFacts(t *testing.T, it *itest, tok, query string) []orderListFacts {
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
