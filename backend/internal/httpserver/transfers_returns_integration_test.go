package httpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-223 acceptance: dealer A returns units to its distributor (its direct
// parent). The distributor approves, A ships (transfer_out), the
// distributor receives (transfer_in): the unit is the distributor's, and
// both ledgers carry the reversal of the distributor's sale (A income on
// the distributor's cari, the distributor a purchase expense on A's cari)
// at A's frozen purchase price. A rejected return writes no stock movement;
// a return to anyone but the direct parent is refused (422, and by the
// database trigger); only the parent decides.
func TestIntegrationStockReturnRequests(t *testing.T) {
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
	dist := it.org("t223-dist", "distributor", center)
	dealerA := it.org("t223-dealer-a", "dealer", dist)
	dealerB := it.org("t223-dealer-b", "dealer", dist)
	dist2 := it.org("t223-dist2", "distributor", center)

	ua, apw := it.user("t223-a-owner")
	it.member(dealerA, ua, "owner")
	ub, bpw := it.user("t223-b-owner")
	it.member(dealerB, ub, "owner")
	ud, dpw := it.user("t223-dist-owner")
	it.member(dist, ud, "owner")
	aTok := it.loginOrg(ua, apw, dealerA)
	bTok := it.loginOrg(ub, bpw, dealerB)
	dTok := it.loginOrg(ud, dpw, dist)

	piece := it.product(center, "T223P")
	it.setListPrice(piece, cur, "50")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: piece.ID, BrandID: piece.BrandID, DistributorOrgID: dist.ID, Currency: cur, Price: "70",
	}); err != nil {
		t.Fatal(err)
	}
	chain := it.stockChain()
	loc := chain.location(center, "T223")
	atA := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID}
	var units []db.Unit
	for i := 1; i <= 2; i++ {
		u := chain.unit(center, piece, 2230+i)
		chain.post(ledger.TypeEntry, u, chain.nextRef(), loc)
		chain.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dealerA, atA)
		units = append(units, u)
	}

	createReturn := func(u db.Unit) stockTransferView {
		t.Helper()
		v, _ := it.transferCall("POST", "/v1/stock-transfers", aTok, map[string]any{
			"kind":  "return",
			"items": []map[string]any{{"barcode": u.Barcode}},
		}, http.StatusCreated)
		return v
	}
	move := func(tok, id, status string) {
		t.Helper()
		if code, ec := it.transferMove(tok, id, status); code != http.StatusOK {
			t.Fatalf("%s %s = %d %s", id, status, code, ec)
		}
	}
	movements := func(u db.Unit) int {
		t.Helper()
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1`, u.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	finance := func(id string) []orderEntry {
		t.Helper()
		rows, err := it.pool.Query(ctx, `
			SELECT f.organization_id, COALESCE(ca.counterparty_org_id, 0), f.direction, f.category,
			       f.orig_currency, f.orig_amount::text, f.reversal_of_id
			FROM finance_entries f LEFT JOIN cari_accounts ca ON ca.id = f.cari_id
			WHERE f.source_type = 'stock_return' AND f.source_uuid = $1 ORDER BY f.id`, id)
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

	// 1. Only the direct parent: another dealer or another distributor is
	// refused; the targets list holds the parent only.
	for _, other := range []db.Organization{dealerB, dist2} {
		if _, ec := it.transferCall("POST", "/v1/stock-transfers", aTok, map[string]any{
			"kind": "return", "to_org_uuid": other.Uuid.String(),
			"items": []map[string]any{{"barcode": units[0].Barcode}},
		}, http.StatusUnprocessableEntity); ec != "TRANSFER_NOT_PARENT" {
			t.Fatalf("return to %s = %s", other.Name, ec)
		}
	}
	code, env := it.do("GET", "/v1/stock-transfers/targets?kind=return", hostOlex, aTok, nil)
	var targets struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
	}
	if code != http.StatusOK || json.Unmarshal(env.Data, &targets) != nil ||
		len(targets.Items) != 1 || targets.Items[0].UUID != dist.Uuid.String() {
		t.Fatalf("return targets = %d %s", code, env.Data)
	}
	// The database refuses a return that skips the parent as well.
	if _, err := it.q.InsertTransferRequest(ctx, db.InsertTransferRequestParams{
		FromOrgID: dealerA.ID, BrandID: dealerA.BrandID, ToOrgID: dist2.ID, ApproverOrgID: dist2.ID,
		Currency: cur, Kind: "return",
	}); err == nil {
		t.Fatal("database accepted a return to another distributor")
	}

	// 2. Requested -> approved (parent) -> shipped -> received.
	r1 := createReturn(units[0])
	if r1.Role != "sender" || r1.Receiver.UUID != dist.Uuid.String() || r1.Status != "requested" {
		t.Fatalf("return = %+v", r1)
	}
	if v, _ := it.transferCall("GET", "/v1/stock-transfers/"+r1.UUID, dTok, nil, http.StatusOK); v.Role != "parent" ||
		strings.Join(v.AvailableTransitions, ",") != "approved,rejected,cancelled" {
		t.Fatalf("parent view = %+v", v)
	}
	it.transferCall("GET", "/v1/stock-transfers/"+r1.UUID, bTok, nil, http.StatusNotFound)
	if code, _ := it.transferMove(aTok, r1.UUID, "approved"); code != http.StatusForbidden {
		t.Fatalf("child approves its own return = %d", code)
	}
	move(dTok, r1.UUID, "approved")
	move(aTok, r1.UUID, "shipped")
	if rows := finance(r1.UUID); len(rows) != 0 {
		t.Fatalf("rows before receipt = %+v", rows)
	}
	move(dTok, r1.UUID, "received")
	st, err := it.q.GetUnitCurrentState(ctx, units[0].ID)
	if err != nil || st.HolderOrgID != dist.ID || st.Status != "available" {
		t.Fatalf("returned unit = %+v, %v", st, err)
	}
	rows := finance(r1.UUID)
	if len(rows) != 2 ||
		rows[0].OrgID != dist.ID || rows[0].Counterparty != dealerA.ID || rows[0].Direction != "expense" || rows[0].Category != "purchase" ||
		rows[1].OrgID != dealerA.ID || rows[1].Counterparty != dist.ID || rows[1].Direction != "income" || rows[1].Category != "sale" {
		t.Fatalf("rows after receipt = %+v", rows)
	}
	for _, r := range rows {
		if r.OrigAmount != "70.00" || r.OrigCurrency != cur || r.ReversalOf != nil {
			t.Fatalf("row amounts = %+v", r)
		}
	}
	want := new(big.Rat).Mul(big.NewRat(70, 1), rate).FloatString(2)
	if got := it.cariBalance(dist.ID, dealerA.ID); got != "-"+want {
		t.Fatalf("distributor/A balance = %s, want -%s", got, want)
	}
	if got := it.cariBalance(dealerA.ID, dist.ID); got != want {
		t.Fatalf("A/distributor balance = %s, want %s", got, want)
	}
	var list struct {
		Items []stockTransferView `json:"items"`
	}
	code, env = it.do("GET", "/v1/stock-transfers?kind=return&direction=approval", hostOlex, dTok, nil)
	if code != http.StatusOK || json.Unmarshal(env.Data, &list) != nil || len(list.Items) != 1 || list.Items[0].UUID != r1.UUID {
		t.Fatalf("parent return list = %d %s", code, env.Data)
	}

	// 3. A rejected return leaves the stock alone.
	before := movements(units[1])
	r2 := createReturn(units[1])
	move(dTok, r2.UUID, "rejected")
	if n := movements(units[1]); n != before {
		t.Fatalf("movements after rejection = %d, want %d", n, before)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, units[1].ID); err != nil || st.HolderOrgID != dealerA.ID || st.Status != "available" {
		t.Fatalf("rejected unit = %+v, %v", st, err)
	}
	if rows := finance(r2.UUID); len(rows) != 0 {
		t.Fatalf("rows after rejection = %+v", rows)
	}
}
