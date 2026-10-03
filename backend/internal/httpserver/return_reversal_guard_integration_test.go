package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-229 (K24 / TEC-223) acceptance: a sale is reversed once, either by a
// dispute resolved with reversal or by a received return, never by both.
// Both scenarios continue the F1 chain (runF1Chain): the dealer bought the
// two rolls from the distributor on order B and consumed them in a service;
// the consumption is undone so the dealer can return the rolls.

// t229Setup runs the chain, opens the dealer's dispute on order B's
// purchase row and undoes the consumption (the rolls are back in the
// dealer's stock).
func t229Setup(t *testing.T) (*f1Chain, t219Dispute) {
	t.Helper()
	c := runF1Chain(t)
	it := c.it
	ctx := context.Background()
	_, dealerRows := it.chainBook(c.dealerTok, c.dealer, c.dist)
	var disputed string
	for _, r := range dealerRows {
		if r.SourceUUID != nil && *r.SourceUUID == c.b.UUID {
			disputed = r.UUID
		}
	}
	if disputed == "" {
		t.Fatalf("dealer row of order B not found: %+v", dealerRows)
	}
	d := decodeData[t219Dispute](t, it.accDo("POST", "/v1/accounting/disputes", c.dealerTok,
		map[string]any{"entry_uuid": disputed, "reason": "TEC-229 satış geri çevrilmeli"}, http.StatusCreated))
	if d.Status != "open" || d.SourceType != "order" || d.SourceUUID != c.b.UUID {
		t.Fatalf("dispute = %+v", d)
	}

	chain := it.stockChain()
	to := ledger.Owner{Type: ledger.OwnerOrganization, ID: c.dealer.ID, OrgID: c.dealer.ID}
	for _, u := range c.rolls {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		from := ledger.Owner{Type: ledger.OwnerService, ID: c.svcID, OrgID: c.dealer.ID}
		if _, err := chain.l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeReturn, UnitID: u.ID, From: &from, To: &to, Centimeters: 2500,
			Source: "test", RefType: "t229", RefID: c.svcID, Reason: "TEC-229 consumption undone",
		}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("return %s: %v", u.Barcode, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return c, d
}

// t229Return runs a dealer -> distributor return of both rolls to received.
func t229Return(t *testing.T, c *f1Chain) stockTransferView {
	t.Helper()
	it := c.it
	items := []map[string]any{{"barcode": c.rolls[0].Barcode}, {"barcode": c.rolls[1].Barcode}}
	r, _ := it.transferCall("POST", "/v1/stock-transfers", c.dealerTok,
		map[string]any{"kind": "return", "items": items}, http.StatusCreated)
	for _, m := range []struct{ tok, status string }{
		{c.distTok, "approved"}, {c.dealerTok, "shipped"}, {c.distTok, "received"},
	} {
		if code, ec := it.transferMove(m.tok, r.UUID, m.status); code != http.StatusOK {
			t.Fatalf("return %s -> %s = %d %s", r.UUID, m.status, code, ec)
		}
	}
	v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r.UUID, c.dealerTok, nil, http.StatusOK)
	if v.Status != "received" {
		t.Fatalf("return after receipt = %+v", v)
	}
	if got := c.stockOf(c.distTok, c.dist); got != 2 {
		t.Fatalf("distributor stock after the return = %d, want 2", got)
	}
	return v
}

// t229Books returns the dealer/distributor cari balances (both books) and
// the number of stock_return rows on them.
func t229Books(t *testing.T, c *f1Chain) (dealerBal, distBal string, returnRows int) {
	t.Helper()
	it := c.it
	read := func(token string, owner, cp db.Organization) (string, int) {
		ca, err := it.q.GetCariAccountByCounterpartyOrg(context.Background(), db.GetCariAccountByCounterpartyOrgParams{
			OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: cp.ID, Valid: true},
		})
		if err != nil {
			t.Fatalf("cari %s/%s: %v", owner.Slug, cp.Slug, err)
		}
		cari := decodeData[accCari](t, it.accDo("GET", "/v1/accounting/cari/"+ca.Uuid.String(), token, nil, http.StatusOK))
		page := decodeData[struct {
			Items []chainEntry `json:"items"`
		}](t, it.accDo("GET", "/v1/accounting/entries?limit=100&source_type="+posting.SourceStockReturn+
			"&cari_uuid="+ca.Uuid.String(), token, nil, http.StatusOK))
		return cari.Balance, len(page.Items)
	}
	dealerBal, n1 := read(c.dealerTok, c.dealer, c.dist)
	distBal, n2 := read(c.distTok, c.dist, c.dealer)
	return dealerBal, distBal, n1 + n2
}

func t229Excluded(t *testing.T, c *f1Chain, transfer string) int {
	t.Helper()
	var n int
	if err := c.it.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM stock_transfer_request_items i
		JOIN stock_transfer_requests r ON r.id = i.request_id
		WHERE r.uuid = $1 AND i.accounting_excluded AND i.order_item_id IS NOT NULL`, transfer).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Dispute reversal first, return second: the return is received (the
// rolls move) but books nothing; the cari stays where the reversal left it.
func TestIntegrationDisputeReversalThenReturn(t *testing.T) {
	c, d := t229Setup(t)
	it := c.it

	resolved := decodeData[t219Dispute](t, it.accDo("POST", "/v1/accounting/disputes/"+d.UUID+"/resolve", c.distTok,
		map[string]any{"resolution": "reversal"}, http.StatusOK))
	if resolved.Status != "resolved_reversal" || resolved.ReversalEntryUUID == nil {
		t.Fatalf("resolved dispute = %+v", resolved)
	}
	dealerBal, distBal, n := t229Books(t, c)
	if !sameAmount(&dealerBal, "0") || !sameAmount(&distBal, "0") || n != 0 {
		t.Fatalf("after the dispute reversal: dealer %s, distributor %s, %d return rows", dealerBal, distBal, n)
	}

	r := t229Return(t, c)
	dealerAfter, distAfter, n := t229Books(t, c)
	if dealerAfter != dealerBal || distAfter != distBal || n != 0 {
		t.Fatalf("after the return: dealer %s (was %s), distributor %s (was %s), %d return rows, want 0",
			dealerAfter, dealerBal, distAfter, distBal, n)
	}
	if got := t229Excluded(t, c, r.UUID); got != 2 {
		t.Fatalf("excluded return lines = %d, want 2", got)
	}
}

// Return first, dispute reversal second: the reversal is refused with 422
// DISPUTE_SALE_RETURNED and writes nothing; the dispute stays open and can
// be rejected.
func TestIntegrationReturnThenDisputeReversal(t *testing.T) {
	c, d := t229Setup(t)
	it := c.it

	r := t229Return(t, c)
	dealerBal, distBal, n := t229Books(t, c)
	if !sameAmount(&dealerBal, "0") || !sameAmount(&distBal, "0") || n != 2 {
		t.Fatalf("after the return: dealer %s, distributor %s, %d return rows, want 0/0/2", dealerBal, distBal, n)
	}
	if got := t229Excluded(t, c, r.UUID); got != 0 {
		t.Fatalf("excluded return lines = %d, want 0", got)
	}

	code, env := it.do("POST", "/v1/accounting/disputes/"+d.UUID+"/resolve", hostOlex, c.distTok,
		map[string]any{"resolution": "reversal"})
	if code != http.StatusUnprocessableEntity || errCode(env) != "DISPUTE_SALE_RETURNED" {
		t.Fatalf("reversal after the return = %d %s, want 422 DISPUTE_SALE_RETURNED", code, errCode(env))
	}
	dealerAfter, distAfter, n := t229Books(t, c)
	if dealerAfter != dealerBal || distAfter != distBal || n != 2 {
		t.Fatalf("after the refused reversal: dealer %s (was %s), distributor %s (was %s), %d return rows",
			dealerAfter, dealerBal, distAfter, distBal, n)
	}
	_, dealerRows := it.chainBook(c.dealerTok, c.dealer, c.dist)
	for _, row := range dealerRows {
		if row.ReversalOfUUID != nil {
			t.Fatalf("order row reversed after the refused reversal: %+v", row)
		}
	}
	got := decodeData[t219Dispute](t, it.accDo("GET", "/v1/accounting/disputes/"+d.UUID, c.distTok, nil, http.StatusOK))
	if got.Status != "open" {
		t.Fatalf("dispute after the refused reversal = %+v", got)
	}
	it.accDo("POST", "/v1/accounting/disputes/"+d.UUID+"/resolve", c.distTok,
		map[string]any{"resolution": "reject", "note": "İade " + r.TransferNo + " ile geri çevrildi"}, http.StatusOK)
}
