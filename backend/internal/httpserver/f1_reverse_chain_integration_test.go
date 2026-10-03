package httpserver

import (
	"context"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// t219Dispute is one /v1/accounting/disputes row of the reverse chain test.
type t219Dispute struct {
	UUID         string `json:"uuid"`
	Status       string `json:"status"`
	Organization struct {
		UUID string `json:"uuid"`
	} `json:"organization"`
	CounterpartyOrganization struct {
		UUID string `json:"uuid"`
	} `json:"counterparty_organization"`
	Entry struct {
		UUID string `json:"uuid"`
	} `json:"entry"`
	SourceType        string  `json:"source_type"`
	SourceUUID        string  `json:"source_uuid"`
	ResolutionNote    *string `json:"resolution_note"`
	ReversalEntryUUID *string `json:"reversal_entry_uuid"`
	RevisionEntryUUID *string `json:"revision_entry_uuid"`
}

// TEC-219 (F1-13b) acceptance, the F1 gate chain of TEC-217 wound back:
// after runF1Chain (center -> EUR distributor -> UAH dealer, two whole
// rolls consumed in a completed service, two warranties) the dealer
// disputes the distributor's sale on its book (K24), and the chain is
// undone through the cancel / take-back paths the product has:
//
//   - the completed service cannot be cancelled (completed is final): the
//     center voids its warranties (POST /v1/warranties/{uuid}/void);
//   - the consumption is undone with the ledger's return movement
//     (used in the service -> available at the dealer, 25 m back on each
//     roll). No HTTP route writes it yet, so the test posts it through
//     ledger.Post, the same way the other stock tests seed movements;
//   - the received orders cannot be cancelled (received is final): the
//     rolls go back up by return requests (TEC-223), dealer -> distributor
//     and distributor -> center; each receipt books the reversal of the
//     parent's sale (stock_return) on both books;
//   - the distributor closes the dispute without a second reversal (the
//     return already booked it): a dispute reversal on top would book the
//     sale back twice.
//
// Every step asserts the ownership and the stock ledger of the rolls; at
// the end the stock, the ownership and the three books are back where they
// were before the chain: the rolls available at the center with 25 m each,
// no stock at the distributor and the dealer, every cari between the three
// organizations at zero, the order rows untouched (append-only) and the
// warranties void.
func TestIntegrationF1ReverseChain(t *testing.T) {
	c := runF1Chain(t)
	it := c.it
	ctx := context.Background()
	chainMoves := "entry,order_out,received,order_out,received,consumption"
	atOrg := func(o db.Organization) ledger.Owner {
		return ledger.Owner{Type: ledger.OwnerOrganization, ID: o.ID, OrgID: o.ID}
	}
	stocks := func(step string, center, dist, dealer int32) {
		t.Helper()
		for _, s := range []struct {
			tok  string
			org  db.Organization
			want int32
		}{{c.staffTok, c.center, center}, {c.distTok, c.dist, dist}, {c.dealerTok, c.dealer, dealer}} {
			if got := c.stockOf(s.tok, s.org); got != s.want {
				t.Fatalf("%s: stock of %s = %d, want %d", step, s.org.Slug, got, s.want)
			}
		}
	}
	meters := func(step, want string) {
		t.Helper()
		for _, u := range c.rolls {
			cur, err := it.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: c.center.BrandID, Barcode: u.Barcode})
			if err != nil {
				t.Fatal(err)
			}
			if !cur.RemainingMeters.Valid || ratOfText(t, posting.FormatNumeric(cur.RemainingMeters)).Cmp(ratOfText(t, want)) != 0 {
				t.Fatalf("%s: %s remaining = %v, want %s", step, u.Barcode, cur.RemainingMeters, want)
			}
		}
	}
	entries := func(token string, owner, cp db.Organization, source string) (accCari, []chainEntry) {
		t.Helper()
		ca, err := it.q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
			OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: cp.ID, Valid: true},
		})
		if err != nil {
			t.Fatalf("cari %s/%s: %v", owner.Slug, cp.Slug, err)
		}
		cari := decodeData[accCari](t, it.accDo("GET", "/v1/accounting/cari/"+ca.Uuid.String(), token, nil, http.StatusOK))
		page := decodeData[struct {
			Items []chainEntry `json:"items"`
		}](t, it.accDo("GET", "/v1/accounting/entries?limit=100&source_type="+source+"&cari_uuid="+ca.Uuid.String(),
			token, nil, http.StatusOK))
		return cari, page.Items
	}
	// orderRowsIntact: each order row of the chain is still the one row of
	// its order on its book, never reversed (the ledger is append-only).
	orderRowsIntact := func(step string) {
		t.Helper()
		for _, w := range c.books {
			_, rows := c.it.chainBook(w.token, w.owner, w.cp)
			n := 0
			for _, r := range rows {
				if r.ReversalOfUUID != nil {
					t.Fatalf("%s %s: order row reversed: %+v", step, w.name, r)
				}
				if r.SourceUUID != nil && *r.SourceUUID == w.order {
					if r.Direction != w.direction || !sameAmount(&r.OrigAmount, w.total) {
						t.Fatalf("%s %s: order row = %+v", step, w.name, r)
					}
					n++
				}
			}
			if n != 1 {
				t.Fatalf("%s %s: %d rows of the order, want 1", step, w.name, n)
			}
		}
	}

	// 1. The dealer disputes the distributor's sale on its book (K24): the
	// purchase row of order B. The distributor sees the open dispute; the
	// books do not move.
	_, dealerRows := c.it.chainBook(c.dealerTok, c.dealer, c.dist)
	var disputed string
	for _, r := range dealerRows {
		if r.SourceUUID != nil && *r.SourceUUID == c.b.UUID {
			disputed = r.UUID
		}
	}
	if disputed == "" {
		t.Fatalf("dealer row of order B not found: %+v", dealerRows)
	}
	reason := "Rulolar hizmetten geri alınıp iade ediliyor; satış geri çevrilmeli"
	d := decodeData[t219Dispute](t, it.accDo("POST", "/v1/accounting/disputes", c.dealerTok,
		map[string]any{"entry_uuid": disputed, "reason": reason}, http.StatusCreated))
	if d.Status != "open" || d.Entry.UUID != disputed || d.SourceType != "order" || d.SourceUUID != c.b.UUID ||
		d.Organization.UUID != c.dealer.Uuid.String() || d.CounterpartyOrganization.UUID != c.dist.Uuid.String() {
		t.Fatalf("dispute = %+v", d)
	}
	it.accDo("POST", "/v1/accounting/disputes", c.dealerTok,
		map[string]any{"entry_uuid": disputed, "reason": reason}, http.StatusConflict)
	open := decodeData[struct {
		Items []t219Dispute `json:"items"`
	}](t, it.accDo("GET", "/v1/accounting/disputes?status=open&limit=100", c.distTok, nil, http.StatusOK))
	seen := false
	for _, o := range open.Items {
		seen = seen || o.UUID == d.UUID
	}
	if !seen {
		t.Fatalf("distributor's open disputes miss %s: %+v", d.UUID, open.Items)
	}
	c.readBooks("dispute open")

	// 2. Service: a completed service is final, so it cannot be cancelled;
	// the center voids its warranties instead.
	if _, ec := it.svcCall("POST", "/v1/services/"+c.svcUUID+"/transitions", c.staffTok,
		map[string]string{"status": "cancelled", "note": "TEC-219"}, http.StatusConflict); ec == "" {
		t.Fatal("cancelling a completed service: no error code")
	}
	ws := it.serviceWarranties(c.svcID, c.dealer.BrandID)
	if len(ws) != 2 {
		t.Fatalf("warranties before the void = %+v", ws)
	}
	it.stepUp(c.staff.Uuid)
	for _, w := range ws {
		if w.Status != "active" {
			t.Fatalf("warranty before the void = %+v", w)
		}
		v := decodeData[warrantyRow](t, it.custDo("POST", "/v1/warranties/"+w.Uuid.String()+"/void", c.staffTok,
			map[string]any{"reason": "TEC-219 hizmet geri alındı"}, http.StatusOK))
		if v.Status != "void" || v.CanVoid {
			t.Fatalf("voided warranty = %+v", v)
		}
	}
	for _, w := range it.serviceWarranties(c.svcID, c.dealer.BrandID) {
		if w.Status != "void" {
			t.Fatalf("warranty after the void = %+v", w)
		}
	}
	if page, _ := it.listWarranties("/v1/warranties?limit=100&status=active", c.dealerTok); page.Total != 0 {
		t.Fatalf("dealer active warranties = %+v", page)
	}
	c.owned("warranties void", "used", "service", c.svcID, c.dealer.ID, chainMoves)

	// 3. The consumption is undone: each roll comes back from the service
	// into the dealer's stock with its 25 m (ledger return movement).
	chain := it.stockChain()
	for _, u := range c.rolls {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		from, to := ledger.Owner{Type: ledger.OwnerService, ID: c.svcID, OrgID: c.dealer.ID}, atOrg(c.dealer)
		if _, err := chain.l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeReturn, UnitID: u.ID, From: &from, To: &to, Centimeters: 2500,
			Source: "test", RefType: "t219", RefID: c.svcID, Reason: "TEC-219 consumption undone",
		}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("return %s: %v", u.Barcode, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	chainMoves += ",return"
	c.owned("consumption undone", "available", "organization", c.dealer.ID, c.dealer.ID, chainMoves)
	meters("consumption undone", "25")
	stocks("consumption undone", 0, 0, 2)

	// 4. A received order cannot be cancelled; the dealer returns both
	// rolls to the distributor (TEC-223): approve, ship, receive. The
	// receipt books the reversal of order B on both books.
	if code, _ := it.transition(c.distTok, c.b.UUID, "cancelled"); code == http.StatusOK {
		t.Fatal("a received order was cancelled")
	}
	if v := it.orderCall("GET", "/v1/orders/"+c.b.UUID, c.dealerTok, nil, http.StatusOK); v.Status != "received" {
		t.Fatalf("order B after the cancel attempt = %s", v.Status)
	}
	items := []map[string]any{{"barcode": c.rolls[0].Barcode}, {"barcode": c.rolls[1].Barcode}}
	move := func(tok, id, status string) {
		t.Helper()
		if code, ec := it.transferMove(tok, id, status); code != http.StatusOK {
			t.Fatalf("return %s -> %s = %d %s", id, status, code, ec)
		}
	}
	r1, _ := it.transferCall("POST", "/v1/stock-transfers", c.dealerTok,
		map[string]any{"kind": "return", "items": items}, http.StatusCreated)
	if r1.Status != "requested" || r1.Receiver.UUID != c.dist.Uuid.String() {
		t.Fatalf("dealer return = %+v", r1)
	}
	move(c.distTok, r1.UUID, "approved")
	move(c.dealerTok, r1.UUID, "shipped")
	chainMoves += ",transfer_out"
	c.owned("R1 shipped", "in_transit", "organization", c.dist.ID, c.dist.ID, chainMoves)
	move(c.distTok, r1.UUID, "received")
	chainMoves += ",transfer_in"
	c.owned("R1 received", "available", "organization", c.dist.ID, c.dist.ID, chainMoves)
	stocks("R1 received", 0, 2, 0)

	// 5. The distributor returns both rolls to the center. No center role
	// holds transfers.approve, so a super admin member of the center
	// decides the return.
	admin, apw := it.user("t219-admin", rbac.RoleSuperAdmin)
	it.member(c.center, admin, "staff", rbac.RoleCenterStaff)
	adminTok := it.loginOrg(admin, apw, c.center)
	r2, _ := it.transferCall("POST", "/v1/stock-transfers", c.distTok,
		map[string]any{"kind": "return", "items": items}, http.StatusCreated)
	if r2.Status != "requested" || r2.Receiver.UUID != c.center.Uuid.String() {
		t.Fatalf("distributor return = %+v", r2)
	}
	move(adminTok, r2.UUID, "approved")
	move(c.distTok, r2.UUID, "shipped")
	chainMoves += ",transfer_out"
	c.owned("R2 shipped", "in_transit", "organization", c.center.ID, c.center.ID, chainMoves)
	move(adminTok, r2.UUID, "received")
	chainMoves += ",transfer_in"
	c.owned("R2 received", "available", "organization", c.center.ID, c.center.ID, chainMoves)
	if c.centerStockBefore != 2 {
		t.Fatalf("center stock before the chain = %d, want 2", c.centerStockBefore)
	}
	stocks("R2 received", c.centerStockBefore, 0, 0)
	meters("R2 received", "25")

	// 6. The books: each return receipt is one purchase expense at the
	// parent (on the child's cari) and one sale income at the child (on the
	// parent's cari), at the order price, in each organization's currency;
	// every cari between the three organizations is back at zero and the
	// order rows are untouched.
	day := c.books[0].frozen.snap.RateDate
	type returnRow struct {
		name      string
		token     string
		owner, cp db.Organization
		transfer  string
		direction string
		category  string
		total     string
	}
	for _, w := range []returnRow{
		{"distributor takes R1 back", c.distTok, c.dist, c.dealer, r1.UUID, "expense", "purchase", "300"},
		{"dealer returns R1", c.dealerTok, c.dealer, c.dist, r1.UUID, "income", "sale", "300"},
		{"center takes R2 back", c.staffTok, c.center, c.dist, r2.UUID, "expense", "purchase", "200"},
		{"distributor returns R2", c.distTok, c.dist, c.center, r2.UUID, "income", "sale", "200"},
	} {
		oc := strings.TrimSpace(w.owner.Currency)
		rate := c.toOrg[oc]
		if rate == nil {
			t.Fatalf("%s: no expected rate for %s", w.name, oc)
		}
		amount := new(big.Rat).Mul(ratOfText(t, w.total), rate).FloatString(2)
		cari, rows := entries(w.token, w.owner, w.cp, posting.SourceStockReturn)
		if len(rows) != 1 {
			t.Fatalf("%s: %d return rows, want 1: %+v", w.name, len(rows), rows)
		}
		r := rows[0]
		if r.SourceUUID == nil || *r.SourceUUID != w.transfer || r.Direction != w.direction || r.Category != w.category ||
			r.ReversalOfUUID != nil || strings.TrimSpace(r.OrigCurrency) != c.cur || !sameAmount(&r.OrigAmount, w.total) ||
			strings.TrimSpace(r.Currency) != oc || !sameAmount(&r.Amount, amount) || ratOfText(t, r.Rate).Cmp(rate) != 0 ||
			r.RateDate != day {
			t.Fatalf("%s row = %+v, want %s %s %s %s on %s", w.name, r, w.direction, w.category, amount, oc, day)
		}
		if !sameAmount(&cari.Balance, "0") {
			t.Fatalf("%s: cari balance = %s, want 0", w.name, cari.Balance)
		}
	}
	orderRowsIntact("after the returns")

	// 7. The distributor closes the dispute: the return booked the reversal
	// of the sale, so it is rejected with a note naming the return and
	// nothing more is posted. The books stay at zero.
	note := "İade " + r1.TransferNo + " ile satış geri çevrildi"
	resolved := decodeData[t219Dispute](t, it.accDo("POST", "/v1/accounting/disputes/"+d.UUID+"/resolve", c.distTok,
		map[string]any{"resolution": "reject", "note": note}, http.StatusOK))
	if resolved.Status != "rejected" || resolved.ReversalEntryUUID != nil || resolved.RevisionEntryUUID != nil ||
		resolved.ResolutionNote == nil || *resolved.ResolutionNote != note {
		t.Fatalf("resolved dispute = %+v", resolved)
	}
	orderRowsIntact("after the dispute")
	for _, w := range c.books {
		cari, _ := c.it.chainBook(w.token, w.owner, w.cp)
		if !sameAmount(&cari.Balance, "0") {
			t.Fatalf("%s: cari balance after the dispute = %s, want 0", w.name, cari.Balance)
		}
	}
	c.owned("reverse chain", "available", "organization", c.center.ID, c.center.ID, chainMoves)
}
