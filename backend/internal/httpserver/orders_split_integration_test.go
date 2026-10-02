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

type splitResp struct {
	UUID   string `json:"uuid"`
	Meters string `json:"meters"`
	Source struct {
		UUID            string `json:"uuid"`
		Barcode         string `json:"barcode"`
		Status          string `json:"status"`
		RemainingMeters string `json:"remaining_meters"`
	} `json:"source"`
	NewUnit struct {
		UUID            string `json:"uuid"`
		Barcode         string `json:"barcode"`
		Status          string `json:"status"`
		RemainingMeters string `json:"remaining_meters"`
	} `json:"new_unit"`
	Replayed bool `json:"replayed"`
}

type assignSplitResp struct {
	Split *struct {
		UUID                  string `json:"uuid"`
		Meters                string `json:"meters"`
		SourceBarcode         string `json:"source_barcode"`
		SourceRemainingMeters string `json:"source_remaining_meters"`
		NewUnitUUID           string `json:"new_unit_uuid"`
		NewBarcode            string `json:"new_barcode"`
		Replayed              bool   `json:"replayed"`
	} `json:"split"`
}

// TEC-184 acceptance: a roll is split through POST /v1/stock/splits
// (idempotent by key, N > remaining = 422, pieces refused) and through an
// order assignment of fewer meters than the roll: the cut leaves as a new
// unit that is shipped to and received by the buyer while the roll stays
// with the seller; product stock totals are kept.
func TestIntegrationOrdersRollSplit(t *testing.T) {
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
	dist := it.org("t184-dist", "distributor", center)
	staff, spw := it.user("t184-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t184-dist-owner")
	it.member(dist, distOwner, "owner")

	roll := it.product(center, "T184R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	piece := it.product(center, "T184P")
	it.setListPrice(roll, cur, "10")

	chain := it.stockChain()
	loc := chain.location(center, "T184")
	r := chain.rollUnit(center, roll, 1840, "50")
	chain.post(ledger.TypeEntry, r, chain.nextRef(), loc)
	pu := chain.unit(center, piece, 1841)
	chain.post(ledger.TypeEntry, pu, chain.nextRef(), loc)

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)

	orgStock := func(org int64) (int32, string) {
		var qty int32
		var m string
		if err := it.pool.QueryRow(ctx, `SELECT quantity, meters::text FROM organization_product_stocks
			WHERE organization_id = $1 AND product_id = $2`, org, roll.ID).Scan(&qty, &m); err != nil {
			return 0, "none"
		}
		return qty, m
	}
	split := func(body map[string]any) (int, string, splitResp) {
		code, env := it.do("POST", "/v1/stock/splits", hostOlex, staffTok, body)
		var s splitResp
		if code < 300 {
			if err := json.Unmarshal(env.Data, &s); err != nil {
				t.Fatal(err)
			}
		}
		return code, errCode(env), s
	}

	// 1. Standalone split: 50 m -> 38 m + 12 m; the same key writes nothing.
	key := "t184-" + it.suffix
	code, ec, s := split(map[string]any{"barcode": r.Barcode, "meters": "12", "idempotency_key": key})
	if code != http.StatusCreated || s.Source.RemainingMeters != "38.00" || s.NewUnit.RemainingMeters != "12.00" ||
		s.NewUnit.Barcode != r.Barcode+"-S1" || s.NewUnit.Status != "placed" && s.NewUnit.Status != "available" || s.Replayed {
		t.Fatalf("split = %d %s %+v", code, ec, s)
	}
	if q, m := orgStock(center.ID); q != 2 || m != "50.00" {
		t.Fatalf("center roll stock after split = %d %s, want 2 50.00", q, m)
	}
	code, _, again := split(map[string]any{"barcode": r.Barcode, "meters": "12", "idempotency_key": key})
	if code != http.StatusOK || !again.Replayed || again.NewUnit.UUID != s.NewUnit.UUID {
		t.Fatalf("replayed split = %d %+v", code, again)
	}
	var splitMvs, splitUnits int
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM stock_movements m JOIN stock_splits ss ON ss.id = m.reference_id
		  WHERE m.reference_type = 'stock_split' AND ss.uuid = $1),
		(SELECT COUNT(*) FROM units WHERE barcode LIKE $2 || '-S%')`, s.UUID, r.Barcode).Scan(&splitMvs, &splitUnits); err != nil {
		t.Fatal(err)
	}
	if splitMvs != 2 || splitUnits != 1 {
		t.Fatalf("after replay: %d split movements, %d split units; want 2, 1", splitMvs, splitUnits)
	}
	if q, m := orgStock(center.ID); q != 2 || m != "50.00" {
		t.Fatalf("center roll stock after replay = %d %s", q, m)
	}
	// The same key for other meters is a conflict; too many meters is 422;
	// a piece is not split.
	if code, ec, _ := split(map[string]any{"barcode": r.Barcode, "meters": "5", "idempotency_key": key}); code != http.StatusConflict || ec != "STOCK_SPLIT_KEY_CONFLICT" {
		t.Fatalf("key reuse = %d %s", code, ec)
	}
	if code, ec, _ := split(map[string]any{"barcode": r.Barcode, "meters": "39", "idempotency_key": key + "-b"}); code != http.StatusUnprocessableEntity || ec != "STOCK_SPLIT_INSUFFICIENT_METERS" {
		t.Fatalf("split above remaining = %d %s", code, ec)
	}
	if code, ec, _ := split(map[string]any{"barcode": pu.Barcode, "meters": "1", "idempotency_key": key + "-c"}); code != http.StatusUnprocessableEntity || ec != "STOCK_SPLIT_NOT_ROLL" {
		t.Fatalf("piece split = %d %s", code, ec)
	}

	// 2. Order of 12 m from the 38 m roll: the assignment splits it.
	o := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": roll.Uuid.String(), "meters": 12},
	})
	line := o.Items[0].UUID
	if code, ec, _ := it.assign(staffTok, o.UUID, line, map[string]any{"barcode": r.Barcode, "meters": "40"}); code != http.StatusUnprocessableEntity || ec != "ROLL_METERS_EXCEED_REMAINING" {
		t.Fatalf("assign above remaining = %d %s", code, ec)
	}
	assignKey := "a184-" + it.suffix
	assign := func() assignSplitResp {
		code, env := it.do("POST", "/v1/orders/"+o.UUID+"/items/"+line+"/units", hostOlex, staffTok,
			map[string]any{"barcode": r.Barcode, "meters": "12", "idempotency_key": assignKey})
		if code != http.StatusOK {
			t.Fatalf("split assign = %d %s", code, errCode(env))
		}
		var a assignSplitResp
		if err := json.Unmarshal(env.Data, &a); err != nil {
			t.Fatal(err)
		}
		if a.Split == nil || a.Split.Meters != "12.00" || a.Split.SourceRemainingMeters != "26.00" ||
			a.Split.NewBarcode != r.Barcode+"-S2" {
			t.Fatalf("split assign = %+v", a.Split)
		}
		return a
	}
	first := assign()
	// A retry with the same key neither splits again nor assigns twice.
	if second := assign(); !second.Split.Replayed || second.Split.NewUnitUUID != first.Split.NewUnitUUID {
		t.Fatalf("retried split assign = %+v", second.Split)
	}
	v := it.orderCall("GET", "/v1/orders/"+o.UUID, staffTok, nil, http.StatusOK)
	if len(v.Items[0].Units) != 1 || v.Items[0].Units[0].Barcode != first.Split.NewBarcode || v.Items[0].Assigned != "12.00" && v.Items[0].Assigned != "12" {
		t.Fatalf("assigned units = %+v", v.Items[0])
	}
	if q, m := orgStock(center.ID); q != 3 || m != "50.00" {
		t.Fatalf("center roll stock after order split = %d %s, want 3 50.00", q, m)
	}

	// 3. Ship and receive: the new unit goes to the buyer, the roll stays.
	for _, step := range []struct{ tok, status string }{{staffTok, "ready"}, {staffTok, "shipped"}, {distTok, "received"}} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	nu, err := it.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: center.BrandID, Barcode: first.Split.NewBarcode})
	if err != nil {
		t.Fatal(err)
	}
	st, err := it.q.GetUnitCurrentState(ctx, nu.ID)
	if err != nil || st.HolderOrgID != dist.ID || st.Status != "available" {
		t.Fatalf("new unit state = %+v %v", st, err)
	}
	rs, err := it.q.GetUnitCurrentState(ctx, r.ID)
	if err != nil || rs.HolderOrgID != center.ID || rs.OwnerID != loc.ID {
		t.Fatalf("roll state = %+v %v", rs, err)
	}
	if q, m := orgStock(center.ID); q != 2 || m != "38.00" {
		t.Fatalf("center roll stock after shipping = %d %s, want 2 38.00", q, m)
	}
	if q, m := orgStock(dist.ID); q != 1 || m != "12.00" {
		t.Fatalf("distributor roll stock = %d %s, want 1 12.00", q, m)
	}
}
