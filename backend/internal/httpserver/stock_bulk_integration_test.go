package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type bulkJobView struct {
	UUID       string `json:"uuid"`
	Status     string `json:"status"`
	Eligible   int    `json:"eligible"`
	Excluded   int    `json:"excluded"`
	Succeeded  int    `json:"succeeded"`
	Reverted   int    `json:"reverted"`
	Reversible bool   `json:"reversible"`
	Selection  struct {
		Missing []string `json:"missing"`
	} `json:"selection"`
	Breakdown []struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
		Count  int    `json:"count"`
	} `json:"breakdown"`
}

// TEC-1008: a super admin converts piece boxes to rolls in bulk. Units at
// a dealer and labels not in stock are excluded with their reason, an
// unknown barcode is reported; the run needs a step-up, writes one
// measure_conversion per unit (the bin stock gains the meters) and the
// undo turns the untouched rolls back into pieces. A void cannot be undone.
func TestIntegrationStockBulkConvertAndUndo(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t1008-dealer", "dealer", center)
	admin, apw := it.user("t1008-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	tok := it.loginOrg(admin, apw, center)

	p := it.product(center, "T1008")
	code, env := it.do("PUT", "/v1/catalog/products/"+p.Uuid.String()+"/roll", hostOlex, tok,
		map[string]any{"default_meters": 30, "width_cm": 152, "min_cut_meters": 0.5, "waste_pct": 3})
	if code != http.StatusOK {
		t.Fatalf("roll settings = %d %s", code, errCode(env))
	}

	c := it.stockChain()
	bin := c.location(center, "B1008")
	units := make([]string, 0, 4)
	for i := range 4 {
		u := c.unit(center, p, 1008*10+i)
		units = append(units, u.Barcode)
		if i == 3 {
			continue // a printed label, not in stock
		}
		c.post(ledger.TypeEntry, u, c.nextRef(), ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID})
		if i == 2 {
			c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealer,
				ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID})
			continue
		}
		c.post(ledger.TypePlacement, u, c.nextRef(), bin)
	}

	job := func(code int, env envelope) bulkJobView {
		t.Helper()
		var v bulkJobView
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatalf("job %d: %v", code, err)
		}
		return v
	}
	code, env = it.do("POST", "/v1/platform/stock-bulk", hostOlex, tok, map[string]any{
		"action": "convert_to_roll",
		"selection": map[string]any{
			"barcodes": []string{units[0], units[1] + ";12,5", units[2], units[3], "T1008-NOPE"},
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("preview = %d %s", code, errCode(env))
	}
	pv := job(code, env)
	reasons := map[string]int{}
	for _, b := range pv.Breakdown {
		reasons[b.Status+"/"+b.Reason] = b.Count
	}
	if pv.Status != "preview" || pv.Eligible != 2 || pv.Excluded != 2 || len(pv.Selection.Missing) != 1 ||
		reasons["excluded/at_other_organization"] != 1 || reasons["excluded/not_in_stock"] != 1 {
		t.Fatalf("preview = %+v", pv)
	}

	// Run needs a recent step-up.
	if code, env := it.do("POST", "/v1/platform/stock-bulk/"+pv.UUID+"/run", hostOlex, tok, nil); code == http.StatusAccepted {
		t.Fatalf("run without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(admin.Uuid)
	code, env = it.do("POST", "/v1/platform/stock-bulk/"+pv.UUID+"/run", hostOlex, tok, nil)
	if code != http.StatusAccepted {
		t.Fatalf("run = %d %s", code, errCode(env))
	}
	ran := job(code, env)
	if ran.Status != "completed" || ran.Succeeded != 2 {
		t.Fatalf("run = %+v", ran)
	}
	meters := func(barcode string) string {
		t.Helper()
		var m *string
		if err := it.pool.QueryRow(ctx, `SELECT initial_meters::text FROM units WHERE barcode = $1`, barcode).Scan(&m); err != nil {
			t.Fatal(err)
		}
		if m == nil {
			return ""
		}
		return *m
	}
	if meters(units[0]) != "30.00" || meters(units[1]) != "12.50" || meters(units[2]) != "" {
		t.Fatalf("meters = %q %q %q", meters(units[0]), meters(units[1]), meters(units[2]))
	}
	var binMeters string
	if err := it.pool.QueryRow(ctx, `SELECT meters::text FROM bin_product_stocks WHERE location_id = $1 AND product_id = $2`,
		bin.ID, p.ID).Scan(&binMeters); err != nil {
		t.Fatal(err)
	}
	if binMeters != "42.50" {
		t.Fatalf("bin meters = %s, want 42.50", binMeters)
	}
	var conversions int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements m JOIN units u ON u.id = m.unit_id
		WHERE u.product_id = $1 AND m.type = 'measure_conversion'`, p.ID).Scan(&conversions); err != nil {
		t.Fatal(err)
	}
	if conversions != 2 {
		t.Fatalf("conversion movements = %d", conversions)
	}

	// Undo: the untouched rolls go back to pieces.
	code, env = it.do("POST", "/v1/platform/stock-bulk/"+pv.UUID+"/undo", hostOlex, tok, nil)
	if code != http.StatusAccepted {
		t.Fatalf("undo = %d %s", code, errCode(env))
	}
	if u := job(code, env); u.Status != "undone" || u.Reverted != 2 {
		t.Fatalf("undo = %+v", u)
	}
	if meters(units[0]) != "" || meters(units[1]) != "" {
		t.Fatalf("meters after undo = %q %q", meters(units[0]), meters(units[1]))
	}

	// A void is not reversible.
	code, env = it.do("POST", "/v1/platform/stock-bulk", hostOlex, tok, map[string]any{
		"action": "void", "selection": map[string]any{"barcodes": []string{units[0]}},
	})
	if code != http.StatusCreated {
		t.Fatalf("void preview = %d %s", code, errCode(env))
	}
	vj := job(code, env)
	if vj.Reversible || vj.Eligible != 1 {
		t.Fatalf("void preview = %+v", vj)
	}
	if code, env := it.do("POST", "/v1/platform/stock-bulk/"+vj.UUID+"/run", hostOlex, tok, nil); code != http.StatusAccepted {
		t.Fatalf("void run = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/platform/stock-bulk/"+vj.UUID+"/undo", hostOlex, tok, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("void undo = %d %s", code, errCode(env))
	}

	// The history lists both jobs.
	code, env = it.do("GET", "/v1/platform/stock-bulk?sort=-created_at", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, errCode(env))
	}
}
