package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1047 regression (RV-7.5): a fixed barcode product kept only as a
// count (a squeegee: one fixed barcode, a quantity on the organization, no
// location) is prepared on an order like the serial lines. The suggestion
// proposes it without include_unlocated, a plain scan (barcode only, as the
// web order scan bar sends it) takes the rest of the line, the same
// barcode scanned again grows the assignment instead of 409, and shipping
// debits the quantity once.
func TestIntegrationOrdersFixedBarcodeCount(t *testing.T) {
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
	dist := it.org("t1047-dist", "distributor", center)
	staff, spw := it.user("t1047-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1047-dist-owner")
	it.member(dist, distOwner, "owner")

	ragle := it.product(center, "T1047F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, ragle.ID); err != nil {
		t.Fatal(err)
	}
	it.setListPrice(ragle, cur, "10")

	chain := it.stockChain()
	unlocated := ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID}
	fu := chain.fixedUnit(center, ragle, 1047, unlocated, 6)

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	o := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": ragle.Uuid.String(), "quantity": 3},
	})
	line := o.Items[0].UUID

	// 1. Suggestion: the count without a location is proposed by default.
	s := it.suggest(staffTok, o.UUID, line, false)
	if len(s.Suggestions) != 1 || s.Suggestions[0].Barcode != fu.Barcode || *s.Suggestions[0].Quantity != 3 || !s.Complete {
		t.Fatalf("fixed count suggestion = %+v", s)
	}

	// 2. One typed piece, then a plain scan takes the rest (2).
	if code, ec, v := it.assign(staffTok, o.UUID, line, map[string]any{"barcode": fu.Barcode, "quantity": 1}); code != http.StatusOK || v.Items[0].Assigned != "1" {
		t.Fatalf("fixed assign 1 = %d %s %+v", code, ec, v.Items)
	}
	code, ec, v := it.assign(staffTok, o.UUID, line, map[string]any{"barcode": fu.Barcode})
	if code != http.StatusOK || v.Items[0].Assigned != "3" || len(v.Items[0].Units) != 1 {
		t.Fatalf("fixed scan again = %d %s %+v", code, ec, v.Items)
	}
	// The line is full: one more scan is an over-assignment.
	if code, ec, _ := it.assign(staffTok, o.UUID, line, map[string]any{"barcode": fu.Barcode}); code != http.StatusConflict || ec != "ORDER_ITEM_OVER_ASSIGNED" {
		t.Fatalf("scan on a full line = %d %s", code, ec)
	}
	var active, released int
	var activeQty int64
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE r.status = 'active'), COUNT(*) FILTER (WHERE r.status = 'released'),
		       COALESCE(SUM(r.quantity) FILTER (WHERE r.status = 'active'), 0)
		FROM stock_reservations r JOIN order_items i ON i.id = r.order_item_id JOIN orders o ON o.id = i.order_id
		WHERE o.uuid = $1`, o.UUID).Scan(&active, &released, &activeQty); err != nil {
		t.Fatal(err)
	}
	if active != 1 || released != 1 || activeQty != 3 {
		t.Fatalf("reservations active=%d released=%d qty=%d", active, released, activeQty)
	}

	// 3. A second order: 3 left, the plain scan reserves what is free.
	o2 := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": ragle.Uuid.String(), "quantity": 5},
	})
	if code, ec, v := it.assign(staffTok, o2.UUID, o2.Items[0].UUID, map[string]any{"barcode": fu.Barcode}); code != http.StatusOK || v.Items[0].Assigned != "3" {
		t.Fatalf("plain scan within free stock = %d %s %+v", code, ec, v.Items)
	}
	if code, ec, _ := it.assign(staffTok, o2.UUID, o2.Items[0].UUID, map[string]any{"barcode": fu.Barcode}); code != http.StatusConflict || ec != "ORDER_INSUFFICIENT_STOCK" {
		t.Fatalf("plain scan without stock = %d %s", code, ec)
	}

	// 4. Ready and shipped: the quantity leaves the organization once.
	for _, status := range []string{"ready", "shipped"} {
		if code, ec := it.transition(staffTok, o.UUID, status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", status, code, ec)
		}
	}
	h, err := it.q.LockFixedBarcodeHolding(ctx, db.LockFixedBarcodeHoldingParams{UnitID: fu.ID, OwnerType: string(unlocated.Type), OwnerID: unlocated.ID})
	if err != nil || h.QuantityOnHand != 3 {
		t.Fatalf("fixed holding after ship = %+v %v", h, err)
	}
}
