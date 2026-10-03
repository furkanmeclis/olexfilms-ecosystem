package httpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	transfersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-200 acceptance (K13): a received sibling transfer books the giver A
// income on B's cari (A alacak) and the receiver B a purchase expense on
// A's cari (B borç) for the line totals frozen at approval, in the receipt
// transaction. The A–B balances move by that amount; a repeated post writes
// nothing; the reversal (VoidBySourceTx) brings both balances back. A cancel
// after shipping books nothing. Every transfers.* event carries the right
// recipients (notify_user_ids). A missing rate answers 400 RATE_NOT_FOUND
// and rolls the receipt back.
func TestIntegrationStockTransferAccounting(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	rate := big.NewRat(1, 1)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
		rate = big.NewRat(35, 1)
	}
	dist := it.org("t200-dist", "distributor", center)
	dealerA := it.org("t200-dealer-a", "dealer", dist)
	dealerB := it.org("t200-dealer-b", "dealer", dist)
	ua, apw := it.user("t200-a-owner")
	it.member(dealerA, ua, "owner")
	ub, bpw := it.user("t200-b-owner")
	it.member(dealerB, ub, "owner")
	ud, dpw := it.user("t200-dist-owner")
	it.member(dist, ud, "owner")
	aTok := it.loginOrg(ua, apw, dealerA)
	bTok := it.loginOrg(ub, bpw, dealerB)
	dTok := it.loginOrg(ud, dpw, dist)

	piece := it.product(center, "T200P")
	it.setListPrice(piece, cur, "50")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: piece.ID, BrandID: piece.BrandID, DistributorOrgID: dist.ID, Currency: cur, Price: "70",
	}); err != nil {
		t.Fatal(err)
	}
	chain := it.stockChain()
	loc := chain.location(center, "T200")
	atA := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID}
	var units []db.Unit
	for i := 1; i <= 3; i++ {
		u := chain.unit(center, piece, 2000+i)
		chain.post(ledger.TypeEntry, u, chain.nextRef(), loc)
		chain.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dealerA, atA)
		units = append(units, u)
	}

	create := func(u db.Unit) stockTransferView {
		t.Helper()
		v, _ := it.transferCall("POST", "/v1/stock-transfers", aTok, map[string]any{
			"to_org_uuid": dealerB.Uuid.String(),
			"items":       []map[string]any{{"barcode": u.Barcode}},
		}, http.StatusCreated)
		return v
	}
	move := func(tok, id, status string) {
		t.Helper()
		if code, ec := it.transferMove(tok, id, status); code != http.StatusOK {
			t.Fatalf("%s %s = %d %s", id, status, code, ec)
		}
	}
	recipients := func(id, event string) []int64 {
		t.Helper()
		var raw []byte
		if err := it.pool.QueryRow(ctx, `
			SELECT payload->'data'->'notify_user_ids' FROM outbox_events
			WHERE event_name = $1 AND payload->'data'->>'transfer_uuid' = $2
			ORDER BY id DESC LIMIT 1`, event, id).Scan(&raw); err != nil {
			t.Fatalf("%s %s: %v", id, event, err)
		}
		var ids []int64
		if err := json.Unmarshal(raw, &ids); err != nil {
			t.Fatalf("%s %s recipients %s: %v", id, event, raw, err)
		}
		slices.Sort(ids)
		return ids
	}
	sorted := func(ids ...int64) []int64 { slices.Sort(ids); return ids }
	finance := func(id string) []orderEntry {
		t.Helper()
		rows, err := it.pool.Query(ctx, `
			SELECT f.organization_id, COALESCE(ca.counterparty_org_id, 0), f.direction, f.category,
			       f.orig_currency, f.orig_amount::text, f.reversal_of_id
			FROM finance_entries f LEFT JOIN cari_accounts ca ON ca.id = f.cari_id
			WHERE f.source_type = 'stock_transfer' AND f.source_uuid = $1 ORDER BY f.id`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []orderEntry
		for rows.Next() {
			var e orderEntry
			if err := rows.Scan(&e.OrgID, &e.Counterparty, &e.Direction, &e.Category, &e.OrigCurrency,
				&e.OrigAmount, &e.ReversalOf); err != nil {
				t.Fatal(err)
			}
			e.OrigCurrency = strings.TrimSpace(e.OrigCurrency)
			out = append(out, e)
		}
		return out
	}
	want := new(big.Rat).Mul(big.NewRat(70, 1), rate).FloatString(2)

	// 1. Requested -> approved -> shipped -> received.
	r1 := create(units[0])
	if got := recipients(r1.UUID, "transfers.requested"); !slices.Equal(got, sorted(ub.ID, ud.ID)) {
		t.Fatalf("requested recipients = %v", got)
	}
	move(bTok, r1.UUID, "approved")
	if got := recipients(r1.UUID, "transfers.approved"); !slices.Equal(got, []int64{ua.ID}) {
		t.Fatalf("approved recipients = %v", got)
	}
	move(aTok, r1.UUID, "shipped")
	if got := recipients(r1.UUID, "transfers.shipped"); !slices.Equal(got, []int64{ub.ID}) {
		t.Fatalf("shipped recipients = %v", got)
	}
	if rows := finance(r1.UUID); len(rows) != 0 {
		t.Fatalf("rows before receipt = %+v", rows)
	}
	move(bTok, r1.UUID, "received")
	if got := recipients(r1.UUID, "transfers.received"); !slices.Equal(got, []int64{ua.ID}) {
		t.Fatalf("received recipients = %v", got)
	}
	rows := finance(r1.UUID)
	if len(rows) != 2 ||
		rows[0].OrgID != dealerA.ID || rows[0].Counterparty != dealerB.ID || rows[0].Direction != "income" || rows[0].Category != "sale" ||
		rows[1].OrgID != dealerB.ID || rows[1].Counterparty != dealerA.ID || rows[1].Direction != "expense" || rows[1].Category != "purchase" {
		t.Fatalf("rows after receipt = %+v", rows)
	}
	for _, r := range rows {
		if r.OrigAmount != "70.00" || r.OrigCurrency != cur || r.ReversalOf != nil {
			t.Fatalf("row amounts = %+v", r)
		}
	}
	if got := it.cariBalance(dealerA.ID, dealerB.ID); got != want {
		t.Fatalf("A/B balance = %s, want %s", got, want)
	}
	if got := it.cariBalance(dealerB.ID, dealerA.ID); got != "-"+want {
		t.Fatalf("B/A balance = %s, want -%s", got, want)
	}

	// A repeated post (retry) writes nothing.
	poster := posting.New(it.q, outbox.NewStore(it.pool, it.q), nil)
	r1UUID := uuid.MustParse(r1.UUID)
	tx, err := it.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := poster.PostSiblingTransferTx(ctx, tx, posting.SiblingTransfer{
		Source:      posting.Source{Type: posting.SourceStockTransfer, UUID: r1UUID},
		SenderOrgID: dealerA.ID, ReceiverOrgID: dealerB.ID, Amount: "70.00", Currency: cur,
	})
	if err != nil || !res.Replayed() {
		t.Fatalf("replay = %+v, %v", res, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if rows := finance(r1.UUID); len(rows) != 2 {
		t.Fatalf("rows after replay = %+v", rows)
	}

	// The reversal brings both balances back; a second one writes nothing.
	svc := transfersusecase.New(it.pool, it.q, outbox.NewStore(it.pool, it.q)).WithAccounting(poster)
	req, err := it.q.GetTransferRequestByUUID(ctx, db.GetTransferRequestByUUIDParams{Uuid: r1UUID, BrandID: center.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	void := func() int {
		t.Helper()
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		v, err := svc.VoidAccountingTx(ctx, tx, req, "returned", nil)
		if err != nil {
			t.Fatalf("void: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return len(v.Reversals)
	}
	if n := void(); n != 2 {
		t.Fatalf("reversals = %d, want 2", n)
	}
	if n := void(); n != 0 {
		t.Fatalf("second void reversals = %d", n)
	}
	for _, pair := range [][2]int64{{dealerA.ID, dealerB.ID}, {dealerB.ID, dealerA.ID}} {
		if got := it.cariBalance(pair[0], pair[1]); got != "0.00" {
			t.Fatalf("balance %d/%d after void = %s", pair[0], pair[1], got)
		}
	}

	// 2. Approved by the parent, shipped, cancelled: nothing booked; the
	// cancel goes to the receiver and the parent.
	r2 := create(units[1])
	move(dTok, r2.UUID, "approved")
	if got := recipients(r2.UUID, "transfers.approved"); !slices.Equal(got, sorted(ua.ID, ub.ID)) {
		t.Fatalf("parent approval recipients = %v", got)
	}
	move(aTok, r2.UUID, "shipped")
	move(aTok, r2.UUID, "cancelled")
	if got := recipients(r2.UUID, "transfers.cancelled"); !slices.Equal(got, sorted(ub.ID, ud.ID)) {
		t.Fatalf("cancelled recipients = %v", got)
	}
	if rows := finance(r2.UUID); len(rows) != 0 {
		t.Fatalf("rows after cancel = %+v", rows)
	}

	// 3. No rate for the receiver's currency: 400, the receipt rolls back.
	r3 := create(units[2])
	move(bTok, r3.UUID, "approved")
	move(aTok, r3.UUID, "shipped")
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET currency = 'XTS' WHERE id = $1`, dealerB.ID); err != nil {
		t.Fatal(err)
	}
	if code, ec := it.transferMove(bTok, r3.UUID, "received"); code != http.StatusBadRequest || ec != "RATE_NOT_FOUND" {
		t.Fatalf("receive without rate = %d %s", code, ec)
	}
	if v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r3.UUID, bTok, nil, http.StatusOK); v.Status != "shipped" {
		t.Fatalf("status after failed receipt = %s", v.Status)
	}
	if rows := finance(r3.UUID); len(rows) != 0 {
		t.Fatalf("rows after failed receipt = %+v", rows)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, units[2].ID); err != nil || st.Status != "in_transit" {
		t.Fatalf("unit after failed receipt = %+v, %v", st, err)
	}
}
