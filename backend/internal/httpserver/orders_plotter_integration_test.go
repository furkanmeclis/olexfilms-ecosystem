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

type plotterDetail struct {
	Items []struct {
		UUID       string `json:"uuid"`
		PlotterCut bool   `json:"plotter_cut"`
		Product    struct {
			UUID string `json:"uuid"`
		} `json:"product"`
	} `json:"items"`
	Timeline []struct {
		Action     string `json:"action"`
		PlotterCut *bool  `json:"plotter_cut"`
		Changes    []struct {
			PlotterCut *bool `json:"plotter_cut"`
		} `json:"changes"`
	} `json:"timeline"`
}

// TEC-1058 acceptance: the "sent plotter cut" mark is off by default, set
// when the order is created, when the lines are edited and on one line
// with PATCH (also after shipping); it changes nothing else, is on the
// timeline and the monthly report counts the shipped marked lines.
func TestIntegrationOrdersPlotterCut(t *testing.T) {
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
	dist := it.org("t1058-dist", "distributor", center)
	staff, spw := it.user("t1058-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1058-dist-owner")
	it.member(dist, distOwner, "owner")
	film := it.product(center, "T1058F")
	kit := it.product(center, "T1058K")
	it.setListPrice(film, cur, "40")
	it.setListPrice(kit, cur, "10")
	chain := it.stockChain()
	loc := chain.location(center, "T1058")
	fu := chain.unit(center, film, 10581)
	chain.post(ledger.TypeEntry, fu, chain.nextRef(), loc)
	ku := chain.unit(center, kit, 10582)
	chain.post(ledger.TypeEntry, ku, chain.nextRef(), loc)
	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	get := func(tok, id string) plotterDetail {
		t.Helper()
		code, env := it.do("GET", "/v1/orders/"+id, hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("get = %d %s", code, errCode(env))
		}
		var d plotterDetail
		if err := json.Unmarshal(env.Data, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	marked := func(d plotterDetail, product string) bool {
		for _, l := range d.Items {
			if l.Product.UUID == product {
				return l.PlotterCut
			}
		}
		t.Fatalf("no line of %s", product)
		return false
	}

	// Created with the film marked; the kit is off by default.
	o := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1, "plotter_cut": true},
		map[string]any{"product_uuid": kit.Uuid.String(), "quantity": 1},
	}}, http.StatusCreated)
	d := get(distTok, o.UUID)
	if !marked(d, film.Uuid.String()) || marked(d, kit.Uuid.String()) {
		t.Fatalf("created marks = %+v", d.Items)
	}
	total := o.Total
	for _, step := range []struct{ tok, status string }{{distTok, "submitted"}, {staffTok, "approved"}, {staffTok, "preparing"}} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	// The seller's edit sets the kit mark (amounts unchanged); the price
	// does not move.
	if code, env := it.do("PUT", "/v1/orders/"+o.UUID+"/items", hostOlex, staffTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": kit.Uuid.String(), "quantity": 1, "plotter_cut": true},
	}}); code != http.StatusOK {
		t.Fatalf("edit = %d %s", code, errCode(env))
	}
	d = get(staffTok, o.UUID)
	if !marked(d, film.Uuid.String()) || !marked(d, kit.Uuid.String()) {
		t.Fatalf("edit marks = %+v", d.Items)
	}
	if v := it.orderCall("GET", "/v1/orders/"+o.UUID, staffTok, nil, http.StatusOK); v.Total != total {
		t.Fatalf("total moved %s -> %s", total, v.Total)
	}
	var kitLine, filmLine string
	for _, l := range d.Items {
		if l.Product.UUID == kit.Uuid.String() {
			kitLine = l.UUID
		} else {
			filmLine = l.UUID
		}
	}
	for line, u := range map[string]db.Unit{filmLine: fu, kitLine: ku} {
		if code, ec, _ := it.assign(staffTok, o.UUID, line, map[string]any{"barcode": u.Barcode}); code != http.StatusOK {
			t.Fatalf("assign = %d %s", code, ec)
		}
	}
	for _, status := range []string{"ready", "shipped"} {
		if code, ec := it.transition(staffTok, o.UUID, status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", status, code, ec)
		}
	}
	// After shipping: the buyer may not, the seller may unmark the kit.
	patch := func(tok, line string, on bool) (int, string) {
		code, env := it.do("PATCH", "/v1/orders/"+o.UUID+"/items/"+line, hostOlex, tok, map[string]any{"plotter_cut": on})
		return code, errCode(env)
	}
	if code, ec := patch(distTok, kitLine, false); code != http.StatusConflict || ec != "ORDER_NOT_EDITABLE" {
		t.Fatalf("buyer patch = %d %s", code, ec)
	}
	if code, ec := patch(staffTok, kitLine, false); code != http.StatusOK {
		t.Fatalf("seller patch = %d %s", code, ec)
	}
	if code, ec := patch(staffTok, kitLine, false); code != http.StatusOK {
		t.Fatalf("same patch = %d %s", code, ec)
	}
	d = get(staffTok, o.UUID)
	if marked(d, kit.Uuid.String()) {
		t.Fatal("kit still marked")
	}
	var steps, editMarks int
	for _, e := range d.Timeline {
		if e.Action == "plotter_marked" {
			steps++
			if e.PlotterCut == nil || *e.PlotterCut {
				t.Fatalf("plotter step = %+v", e)
			}
		}
		if e.Action == "updated" {
			for _, c := range e.Changes {
				if c.PlotterCut != nil && *c.PlotterCut {
					editMarks++
				}
			}
		}
	}
	if steps != 1 || editMarks != 1 {
		t.Fatalf("timeline plotter steps = %d, edit marks = %d", steps, editMarks)
	}

	// The report counts the shipped film line in this month.
	code, env := it.do("GET", "/v1/reports/orders.plotter_cut?period=30d", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("report = %d %s", code, errCode(env))
	}
	var rep struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(env.Data, &rep); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rep.Rows {
		if r["product_uuid"] == film.Uuid.String() {
			found = r["line_count"] == float64(1) && r["quantity"] == float64(1)
		}
		if r["product_uuid"] == kit.Uuid.String() {
			t.Fatalf("unmarked kit in the report: %+v", r)
		}
	}
	if !found {
		t.Fatalf("report rows = %+v", rep.Rows)
	}
}
