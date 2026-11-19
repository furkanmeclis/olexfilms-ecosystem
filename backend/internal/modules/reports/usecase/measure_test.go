package usecase

import (
	"fmt"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1244 acceptance (raporlarda ölçü ayrımı): a product sold as a box and
// by the meter shows one plotter row per measure; its pieces and meters
// are never summed into one figure.
func TestPlotterCutReportMeasures(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	center := f.caller(t, f.center, rbac.RoleCenterStaff)
	// A 3.5 m plotter-cut meter line next to the box line of dealer1's order.
	f.exec(t, `INSERT INTO order_items (order_id, organization_id, brand_id, product_id, meters, unit_price, price_source,
 line_total, plotter_cut, line_credits)
SELECT o.id, o.organization_id, o.brand_id, $2, 3.5, 30, 'list', 105, TRUE, 5 FROM orders o WHERE o.buyer_org_id = $1`,
		f.dealer1.ID, f.product)
	env, err := s.Report(f.ctx, center, ReportOrdersPlotterCut, in30)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Rows) != 2 {
		t.Fatalf("rows = %+v", env.Rows)
	}
	byMeasure := map[string]map[string]any{}
	for _, r := range env.Rows {
		byMeasure[fmt.Sprint(r["measure"])] = r
	}
	box, meter := byMeasure["box"], byMeasure["meter"]
	if box == nil || box["line_count"] != int64(6) || box["quantity"] != int64(12) || box["meters"] != float64(0) ||
		box["sale_measure"] != "Box" {
		t.Fatalf("box row = %+v", box)
	}
	if meter == nil || meter["line_count"] != int64(1) || meter["quantity"] != int64(0) || meter["meters"] != 3.5 ||
		meter["sale_measure"] != "Meter" {
		t.Fatalf("meter row = %+v", meter)
	}
	if got := sumSeries(env, "plotter_quantity"); got != 12 {
		t.Fatalf("pieces = %v, want 12 (meters left out)", got)
	}
	if got := sumSeries(env, "plotter_meters"); got != 3.5 {
		t.Fatalf("meters = %v, want 3.5", got)
	}
}

// TEC-1244: the stock aging rows count closed boxes in quantity and add
// opened boxes / cut pieces of a piece product only as meters.
func TestStockAgingMeasures(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	in := in30
	in.Limit = "50"
	wh := f.caller(t, f.center, rbac.RoleCenterWarehouse)
	before, err := s.Report(f.ctx, wh, ReportStockAging, in)
	if err != nil {
		t.Fatal(err)
	}
	row := func(env Envelope) map[string]any {
		t.Helper()
		for _, r := range env.Rows {
			if r["organization_uuid"] == f.dealer1.Uuid {
				return r
			}
		}
		t.Fatalf("no dealer1 row in %+v", env.Rows)
		return nil
	}
	b := row(before)
	for i, m := range []string{"3.50", "12.25"} {
		u := f.id(t, `INSERT INTO units (organization_id, brand_id, product_id, barcode, status, initial_meters, remaining_meters)
VALUES ($1, $2, $3, $4, 'available', $5::numeric, $5::numeric) RETURNING id`, f.center.ID, f.brandID, f.product,
			fmt.Sprintf("CUT-%s-%d", f.prefix, i), m)
		f.exec(t, `INSERT INTO unit_current_state (unit_id, brand_id, owner_type, owner_id, holder_org_id, status)
VALUES ($1, $2, 'organization', $3, $3, 'available')`, u, f.brandID, f.dealer1.ID)
	}
	after, err := s.Report(f.ctx, wh, ReportStockAging, in)
	if err != nil {
		t.Fatal(err)
	}
	a := row(after)
	if a["quantity"] != b["quantity"] {
		t.Fatalf("closed boxes %v -> %v: a cut piece is not a box", b["quantity"], a["quantity"])
	}
	if a["meters"].(float64)-b["meters"].(float64) != 15.75 {
		t.Fatalf("meters %v -> %v, want +15.75", b["meters"], a["meters"])
	}
}
