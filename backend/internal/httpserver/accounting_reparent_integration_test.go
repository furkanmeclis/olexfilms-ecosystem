package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TEC-198 acceptance (K25): moving a dealer to another distributor closes
// the open cari on the old parent's book and opens the same amount as an
// opening balance on the new parent's book (both sides: the parents' books
// and the dealer's own book), in the same transaction as the move, with
// audit rows and no income/expense. Moving to the current parent again and
// replaying the same change write nothing; moving back carries the balance
// back under a new change.
func TestIntegrationAccountingReparentTransfer(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	distA := it.org("t198-dist-a", "distributor", center)
	distB := it.org("t198-dist-b", "distributor", center)
	dealer := it.org("t198-dealer", "dealer", distA)

	// An open cari: distA sold 300.00 to the dealer (distA +300, dealer -300).
	poster := posting.New(it.q, nil, nil)
	tx, err := it.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := poster.PostHierarchicalSaleTx(ctx, tx, posting.Sale{
		Source:      posting.Source{Type: "order", UUID: uuid.New()},
		SellerOrgID: distA.ID, BuyerOrgID: dealer.ID, Amount: "300.00", Currency: distA.Currency,
		RateDate: time.Now().UTC(), Description: "TEC-198 sale",
	}); err != nil {
		t.Fatalf("sale: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if b := it.cariBalance(distA.ID, dealer.ID); b != "300.00" {
		t.Fatalf("distA cari before move = %s", b)
	}

	admin, apw := it.user("t198-admin", rbac.RoleSuperAdmin)
	atp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	}))
	it.stepUp(admin.Uuid)
	move := func(parent string) {
		t.Helper()
		if code, env := it.do("PATCH", "/v1/platform/organizations/"+dealer.Uuid.String(), hostOlex,
			atp.AccessToken, map[string]any{"parent_uuid": parent}); code != http.StatusOK {
			t.Fatalf("move to %s: %d %s", parent, code, errCode(env))
		}
	}
	transferRows := func() int {
		return it.countRows(`SELECT COUNT(*) FROM finance_entries e
			JOIN organization_parent_changes c ON c.uuid = e.source_uuid
			WHERE e.source_type = 'cari_transfer' AND c.organization_id = $1`, dealer.ID)
	}

	// 1. Move dealer distA -> distB.
	move(distB.Uuid.String())
	if b := it.cariBalance(distA.ID, dealer.ID); b != "0.00" {
		t.Fatalf("old parent cari after move = %s", b)
	}
	if b := it.cariBalance(distB.ID, dealer.ID); b != "300.00" {
		t.Fatalf("new parent cari after move = %s", b)
	}
	if b := it.cariBalance(dealer.ID, distA.ID); b != "0.00" {
		t.Fatalf("dealer cari with old parent = %s", b)
	}
	if b := it.cariBalance(dealer.ID, distB.ID); b != "-300.00" {
		t.Fatalf("dealer cari with new parent = %s", b)
	}
	// The new parent's row is an opening balance (charge, no account).
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1
		AND source_type = 'cari_transfer' AND role = 'open_parent' AND category = 'opening_balance'
		AND direction = 'charge' AND account_id IS NULL AND amount = 300`, distB.ID); n != 1 {
		t.Fatalf("new parent opening rows = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1
		AND source_type = 'cari_transfer' AND role = 'close_parent' AND category = 'cari_transfer'
		AND direction = 'collection' AND account_id IS NULL`, distA.ID); n != 1 {
		t.Fatalf("old parent closing rows = %d", n)
	}
	if n := transferRows(); n != 4 {
		t.Fatalf("transfer rows = %d", n)
	}
	// Never P&L, never cash.
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE source_type = 'cari_transfer'
		AND (direction IN ('income', 'expense') OR account_id IS NOT NULL)
		AND organization_id IN ($1, $2, $3)`, distA.ID, distB.ID, dealer.ID); n != 0 {
		t.Fatalf("transfer wrote %d P&L/cash rows", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM organization_parent_changes
		WHERE organization_id = $1 AND old_parent_id = $2 AND new_parent_id = $3`, dealer.ID, distA.ID, distB.ID); n != 1 {
		t.Fatalf("parent change rows = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE action = 'organization.parent_changed'
		AND resource_uuid = $1::uuid`, dealer.Uuid.String()); n != 1 {
		t.Fatalf("parent change audit rows = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events a
		JOIN finance_entries e ON e.uuid = a.resource_uuid
		JOIN organization_parent_changes c ON c.uuid = e.source_uuid
		WHERE a.action = 'accounting.cari_transferred' AND c.organization_id = $1`, dealer.ID); n != 4 {
		t.Fatalf("transfer audit rows = %d", n)
	}

	// 2. The same move again is a no-op.
	move(distB.Uuid.String())
	if n := transferRows(); n != 4 {
		t.Fatalf("transfer rows after repeated move = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM organization_parent_changes WHERE organization_id = $1`, dealer.ID); n != 1 {
		t.Fatalf("parent change rows after repeated move = %d", n)
	}

	// 3. Replaying the same change (the idempotency key) writes nothing.
	var change uuid.UUID
	if err := it.pool.QueryRow(ctx, `SELECT uuid FROM organization_parent_changes WHERE organization_id = $1`,
		dealer.ID).Scan(&change); err != nil {
		t.Fatalf("change: %v", err)
	}
	tx, err = it.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	res, err := poster.TransferCariTx(ctx, tx, posting.CariTransfer{
		Change: change, OrganizationID: dealer.ID, OldParentID: distA.ID, NewParentID: distB.ID,
	})
	if err != nil {
		t.Fatalf("replay transfer: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(res.Written) != 0 || transferRows() != 4 || it.cariBalance(distB.ID, dealer.ID) != "300.00" {
		t.Fatalf("replay wrote %d rows (total %d)", len(res.Written), transferRows())
	}

	// 4. Moving back carries the balance back under a new change.
	move(distA.Uuid.String())
	if b := it.cariBalance(distB.ID, dealer.ID); b != "0.00" {
		t.Fatalf("distB cari after moving back = %s", b)
	}
	if b := it.cariBalance(distA.ID, dealer.ID); b != "300.00" {
		t.Fatalf("distA cari after moving back = %s", b)
	}
	if b := it.cariBalance(dealer.ID, distA.ID); b != "-300.00" {
		t.Fatalf("dealer cari with distA after moving back = %s", b)
	}
	if n := transferRows(); n != 8 {
		t.Fatalf("transfer rows after moving back = %d", n)
	}
}

// TEC-198 acceptance: a cash/bank opening balance (step-up) shows in the
// account balance and the balance report, writes no income/expense (P&L)
// and no cari; the same request again is a replay, other values are 409;
// a reversal brings the balance back to zero.
func TestIntegrationAccountingAccountOpening(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	accUser, apw := it.user("t198-acc")
	it.member(center, accUser, "staff", rbac.RoleCenterAccounting)
	tok := it.loginOrg(accUser, apw, center)

	cash := decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "cash", "name": "T198 Cash " + it.suffix,
	}, http.StatusCreated))
	path := "/v1/accounting/accounts/" + cash.UUID + "/opening-balance"
	body := map[string]any{"amount": "500.00", "opening_date": "2021-03-01", "description": "TEC-198"}

	// 1. Step-up is required.
	code, env := it.do("POST", path, hostOlex, tok, body)
	if code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("account opening without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(accUser.Uuid)

	// Validation: amount, future date; unknown account 404.
	it.accDo("POST", path, tok, map[string]any{"amount": "-5", "opening_date": "2021-03-01"}, http.StatusBadRequest)
	it.accDo("POST", path, tok, map[string]any{
		"amount": "5", "opening_date": time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly),
	}, http.StatusBadRequest)
	it.accDo("POST", "/v1/accounting/accounts/"+uuid.NewString()+"/opening-balance", tok, body, http.StatusNotFound)

	// 2. Opening: direction opening, no cari, account balance 500.
	open := decodeData[accEntry](t, it.accDo("POST", path, tok, body, http.StatusCreated))
	if open.Direction != "opening" || open.Category != "opening_balance" || open.Amount != "500.00" ||
		open.CariUUID != nil || open.SourceType == nil || *open.SourceType != "account_opening" {
		t.Fatalf("account opening = %+v", open)
	}
	acct := "/v1/accounting/accounts/" + cash.UUID
	if a := decodeData[accAccount](t, it.accDo("GET", acct, tok, nil, http.StatusOK)); a.Balance != "500.00" {
		t.Fatalf("account after opening = %+v", a)
	}
	rep := decodeData[balReport](t, it.accDo("GET", "/v1/accounting/reports/balances", tok, nil, http.StatusOK))
	found := ""
	for _, a := range rep.Accounts {
		if a.UUID == cash.UUID {
			found = a.Balance
		}
	}
	if found != "500.00" {
		t.Fatalf("balance report cash = %q", found)
	}

	// 3. Not P&L: no income/expense rows on the account.
	for _, d := range []string{"income", "expense"} {
		got := decodeData[struct {
			Total int64 `json:"total"`
		}](t, it.accDo("GET", "/v1/accounting/entries?direction="+d+"&account_uuid="+cash.UUID, tok, nil, http.StatusOK))
		if got.Total != 0 {
			t.Fatalf("account opening wrote %d %s rows", got.Total, d)
		}
	}
	opening := decodeData[struct {
		Total int64 `json:"total"`
	}](t, it.accDo("GET", "/v1/accounting/entries?direction=opening&account_uuid="+cash.UUID, tok, nil, http.StatusOK))
	if opening.Total != 1 {
		t.Fatalf("opening rows = %d", opening.Total)
	}

	// 4. Replay and conflict.
	replay := decodeData[accEntry](t, it.accDo("POST", path, tok, body, http.StatusOK))
	if replay.UUID != open.UUID {
		t.Fatalf("replayed opening %s != %s", replay.UUID, open.UUID)
	}
	code, env = it.do("POST", path, hostOlex, tok, map[string]any{"amount": "499.00", "opening_date": "2021-03-01"})
	if code != http.StatusConflict || errCode(env) != "OPENING_BALANCE_EXISTS" {
		t.Fatalf("second account opening = %d %s", code, errCode(env))
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE action = 'accounting.account_opening_posted'
		AND resource_uuid = $1::uuid`, open.UUID); n != 1 {
		t.Fatalf("posted audit rows = %d", n)
	}

	// 5. Reversal: balance back to zero.
	rev := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries/"+open.UUID+"/void", tok,
		map[string]string{"reason": "wrong amount"}, http.StatusCreated))
	if rev.ReversalOfUUID == nil || *rev.ReversalOfUUID != open.UUID || rev.Amount != "-500.00" {
		t.Fatalf("reversal = %+v", rev)
	}
	if a := decodeData[accAccount](t, it.accDo("GET", acct, tok, nil, http.StatusOK)); a.Balance != "0.00" {
		t.Fatalf("account after reversal = %+v", a)
	}
}
