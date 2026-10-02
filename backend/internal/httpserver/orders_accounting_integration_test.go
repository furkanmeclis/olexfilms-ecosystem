package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// orderEntry is one finance_entries row of an order source.
type orderEntry struct {
	OrgID, Counterparty int64
	Direction, Category string
	OrigCurrency        string
	OrigAmount          string
	Converted           bool // amount = ROUND(orig_amount * rate, 2)
	InOrgCurrency       bool // currency = the organization's currency
	ReversalOf          *int64
}

func (it *itest) orderEntries(orderUUID string) []orderEntry {
	it.t.Helper()
	rows, err := it.pool.Query(context.Background(), `
		SELECT f.organization_id, COALESCE(ca.counterparty_org_id, 0), f.direction, f.category,
		       f.orig_currency, f.orig_amount::text, f.amount = ROUND(f.orig_amount * f.rate, 2),
		       f.currency = o.currency, f.reversal_of_id
		FROM finance_entries f
		JOIN organizations o ON o.id = f.organization_id
		LEFT JOIN cari_accounts ca ON ca.id = f.cari_id
		WHERE f.source_type = 'order' AND f.source_uuid = $1
		ORDER BY f.id`, orderUUID)
	if err != nil {
		it.t.Fatal(err)
	}
	defer rows.Close()
	var out []orderEntry
	for rows.Next() {
		var e orderEntry
		if err := rows.Scan(&e.OrgID, &e.Counterparty, &e.Direction, &e.Category, &e.OrigCurrency,
			&e.OrigAmount, &e.Converted, &e.InOrgCurrency, &e.ReversalOf); err != nil {
			it.t.Fatal(err)
		}
		e.OrigCurrency = strings.TrimSpace(e.OrigCurrency)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		it.t.Fatal(err)
	}
	return out
}

func (it *itest) cariBalance(owner, counterparty int64) string {
	it.t.Helper()
	ctx := context.Background()
	c, err := it.q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: owner, CounterpartyOrgID: pgtype.Int8{Int64: counterparty, Valid: true},
	})
	if err != nil {
		it.t.Fatalf("cari %d/%d: %v", owner, counterparty, err)
	}
	b, err := it.q.GetCariBalance(ctx, db.GetCariBalanceParams{CariID: c.ID, OrganizationID: owner})
	if err != nil {
		it.t.Fatalf("balance: %v", err)
	}
	return posting.FormatNumeric(b.Balance)
}

func (it *itest) orderRow(orderUUID string, brandID int64) db.Order {
	it.t.Helper()
	o, err := it.q.GetOrderByUUID(context.Background(), db.GetOrderByUUIDParams{Uuid: uuid.MustParse(orderUUID), BrandID: brandID})
	if err != nil {
		it.t.Fatal(err)
	}
	return o
}

// TEC-169 acceptance: along center -> distributor -> dealer every received
// order books one income row at the seller (on the buyer's cari, category
// sale) and one expense row at the buyer (on the seller's cari, category
// purchase) for the order total in the order currency. A second receipt
// writes nothing. VoidBySourceTx reverses both sides with reversal_of_id
// and the cari balances go back to zero. When the accounting write fails
// the receipt is rolled back as a whole: no received movement, the order
// stays shipped.
func TestIntegrationOrdersAccountingBridge(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t169", 1690)
	item := []any{map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 1}}

	checkSale := func(name, orderUUID string, seller, buyer int64, total string) {
		t.Helper()
		rows := it.orderEntries(orderUUID)
		if len(rows) != 2 {
			t.Fatalf("%s: %d finance rows, want 2: %+v", name, len(rows), rows)
		}
		want := []orderEntry{
			{OrgID: seller, Counterparty: buyer, Direction: "income", Category: "sale"},
			{OrgID: buyer, Counterparty: seller, Direction: "expense", Category: "purchase"},
		}
		for i, r := range rows {
			w := want[i]
			if r.OrgID != w.OrgID || r.Counterparty != w.Counterparty || r.Direction != w.Direction || r.Category != w.Category {
				t.Fatalf("%s row %d = %+v, want %+v", name, i, r, w)
			}
			if r.OrigCurrency != f.cur || r.OrigAmount != total || !r.Converted || !r.InOrgCurrency || r.ReversalOf != nil {
				t.Fatalf("%s row %d amounts = %+v (total %s %s)", name, i, r, total, f.cur)
			}
		}
	}

	// 1. Center -> distributor (list price 50).
	a := it.openPreparing(f.distTok, f.staffTok, item)
	it.shipOrder(f.staffTok, a, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	if rows := it.orderEntries(a.UUID); len(rows) != 0 {
		t.Fatalf("rows before receipt = %+v", rows)
	}
	if code, ec := it.transition(f.distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive A = %d %s", code, ec)
	}
	if o := it.orderRow(a.UUID, f.center.BrandID); posting.FormatNumeric(o.Total) != "50.00" {
		t.Fatalf("A total = %s", posting.FormatNumeric(o.Total))
	}
	checkSale("A", a.UUID, f.center.ID, f.dist.ID, "50.00")
	// A second receipt writes nothing.
	if code, ec := it.transition(f.distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("second receive A = %d %s", code, ec)
	}
	checkSale("A again", a.UUID, f.center.ID, f.dist.ID, "50.00")

	// 2. Distributor -> dealer (dealer price 70).
	b := it.openPreparing(f.dealerTok, f.distTok, item)
	it.shipOrder(f.distTok, b, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	if code, ec := it.transition(f.dealerTok, b.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive B = %d %s", code, ec)
	}
	checkSale("B", b.UUID, f.dist.ID, f.dealer.ID, "70.00")
	if got := it.cariBalance(f.center.ID, f.dist.ID); got == "0.00" {
		t.Fatalf("center/dist balance after A = %s", got)
	}

	// 3. The reversal (return flow): both sides of both orders reversed,
	// the caris back to zero; a repeated void writes nothing.
	bridge := ordersusecase.NewAccountingBridge(posting.New(it.q, outbox.NewStore(it.pool, it.q), nil))
	void := func(orderUUID string) int {
		t.Helper()
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		r, err := bridge.VoidOrder(ctx, tx, it.orderRow(orderUUID, f.center.BrandID), "returned", nil)
		if err != nil {
			t.Fatalf("void %s: %v", orderUUID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return len(r.Reversals)
	}
	for _, id := range []string{a.UUID, b.UUID} {
		if n := void(id); n != 2 {
			t.Fatalf("void %s: %d reversals, want 2", id, n)
		}
		rows := it.orderEntries(id)
		if len(rows) != 4 || rows[2].ReversalOf == nil || rows[3].ReversalOf == nil {
			t.Fatalf("rows after void %s = %+v", id, rows)
		}
		if n := void(id); n != 0 {
			t.Fatalf("second void %s: %d reversals", id, n)
		}
	}
	for _, pair := range [][2]int64{
		{f.center.ID, f.dist.ID}, {f.dist.ID, f.center.ID}, {f.dist.ID, f.dealer.ID}, {f.dealer.ID, f.dist.ID},
	} {
		if got := it.cariBalance(pair[0], pair[1]); got != "0.00" {
			t.Fatalf("balance %d/%d after void = %s", pair[0], pair[1], got)
		}
	}

	// 4. A failing accounting write rolls the receipt back: the source key
	// of order C already holds a different seller row.
	c := it.openPreparing(f.distTok, f.staffTok, item)
	it.shipOrder(f.staffTok, c, [][]map[string]any{{{"barcode": f.u2.Barcode}}})
	cUUID := uuid.MustParse(c.UUID)
	poster := posting.New(it.q, nil, nil)
	tx, err := it.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := poster.PostIncome(ctx, tx, posting.Entry{
		OrganizationID: f.center.ID, CounterpartyOrgID: f.dist.ID,
		Source: posting.Source{Type: "order", UUID: cUUID}, Role: posting.RoleSale,
		Category: posting.CategorySale, Amount: "1.00", Currency: f.cur,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("conflicting row: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _ := it.transition(f.distTok, c.UUID, "received"); code == http.StatusOK {
		t.Fatalf("receive C with a conflicting accounting row = %d", code)
	}
	if o := it.orderRow(c.UUID, f.center.BrandID); o.Status != "shipped" || o.ReceivedAt.Valid {
		t.Fatalf("C after failed receipt = %s %v", o.Status, o.ReceivedAt)
	}
	var received int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1 AND type = 'received'`,
		f.u2.ID).Scan(&received); err != nil {
		t.Fatal(err)
	}
	if received != 0 {
		t.Fatalf("received movements of u2 after rollback = %d", received)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, f.u2.ID); err != nil || st.Status != "in_transit" {
		t.Fatalf("u2 after rollback = %+v %v", st, err)
	}
	if rows := it.orderEntries(c.UUID); len(rows) != 1 {
		t.Fatalf("C finance rows after rollback = %+v", rows)
	}
	var history int
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM order_status_history h JOIN orders o ON o.id = h.order_id
		WHERE o.uuid = $1 AND h.to_status = 'received'`, c.UUID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 0 {
		t.Fatalf("received history rows of C = %d", history)
	}
}
