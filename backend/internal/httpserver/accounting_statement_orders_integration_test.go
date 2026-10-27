package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type stmtOrderUnit struct {
	Barcode string `json:"barcode"`
}

type stmtOrderItem struct {
	ProductSKU  string          `json:"product_sku"`
	ProductName string          `json:"product_name"`
	Quantity    *int32          `json:"quantity"`
	UnitPrice   string          `json:"unit_price"`
	LineTotal   string          `json:"line_total"`
	Units       []stmtOrderUnit `json:"units"`
}

type stmtOrder struct {
	UUID    string          `json:"uuid"`
	OrderNo string          `json:"order_no"`
	Items   []stmtOrderItem `json:"items"`
}

type stmtOrderLine struct {
	SourceType *string    `json:"source_type"`
	SourceUUID *string    `json:"source_uuid"`
	Order      *stmtOrder `json:"order"`
}

type stmtWithOrders struct {
	Lines []stmtOrderLine `json:"lines"`
}

// TEC-1035 acceptance: the statement row of a received order carries the
// order number, its lines and the barcodes that went out with it, on the
// seller's statement of the buyer cari and on the buyer's own statement.
func TestIntegrationStatementOrderBarcodes(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t1035", 10350)
	accUser, apw := it.user("t1035-acc")
	it.member(f.center, accUser, "staff", rbac.RoleCenterAccounting)
	accTok := it.loginOrg(accUser, apw, f.center)

	item := []any{map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 2}}
	a := it.openPreparing(f.distTok, f.staffTok, item)
	it.shipOrder(f.staffTok, a, [][]map[string]any{{{"barcode": f.u1.Barcode}, {"barcode": f.u2.Barcode}}})
	if code, ec := it.transition(f.distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive = %d %s", code, ec)
	}
	order := it.orderRow(a.UUID, f.center.BrandID)

	cariOf := func(owner, counterparty db.Organization) string {
		t.Helper()
		c, err := it.q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
			OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: counterparty.ID, Valid: true},
		})
		if err != nil {
			t.Fatalf("cari %s/%s: %v", owner.Name, counterparty.Name, err)
		}
		return c.Uuid.String()
	}
	check := func(name, tok, cari string) {
		t.Helper()
		st := decodeData[stmtWithOrders](t, it.accDo("GET", "/v1/accounting/cari/"+cari+"/statement", tok, nil, http.StatusOK))
		var got *stmtOrder
		for _, l := range st.Lines {
			if l.SourceType != nil && *l.SourceType == "order" && l.SourceUUID != nil && *l.SourceUUID == a.UUID {
				got = l.Order
			} else if l.Order != nil {
				t.Fatalf("%s: non-order line carries an order: %+v", name, l)
			}
		}
		if got == nil {
			t.Fatalf("%s: no order detail on the order line: %+v", name, st.Lines)
		}
		if got.UUID != a.UUID || got.OrderNo != order.OrderNo || len(got.Items) != 1 {
			t.Fatalf("%s: order = %+v", name, got)
		}
		li := got.Items[0]
		if li.ProductSKU != f.piece.Sku || li.ProductName != f.piece.Name || li.Quantity == nil || *li.Quantity != 2 ||
			li.UnitPrice != "50.0000" || li.LineTotal != "100.00" {
			t.Fatalf("%s: item = %+v", name, li)
		}
		if len(li.Units) != 2 {
			t.Fatalf("%s: units = %+v", name, li.Units)
		}
		want := map[string]bool{f.u1.Barcode: true, f.u2.Barcode: true}
		for _, u := range li.Units {
			if !want[u.Barcode] {
				t.Fatalf("%s: unexpected barcode %s", name, u.Barcode)
			}
			delete(want, u.Barcode)
		}
	}

	// The center's cari of the distributor and the distributor's own cari
	// with the center show the same lines and barcodes.
	check("center", accTok, cariOf(f.center, f.dist))
	check("distributor", f.distTok, cariOf(f.dist, f.center))
}
