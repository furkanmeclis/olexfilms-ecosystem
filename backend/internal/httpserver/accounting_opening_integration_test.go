package httpserver

import (
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-177 acceptance: the opening balance of a cari is written once (step-up),
// shows in the cari balance and at its opening date in the statement; the
// same request again is a replay, other values are 409; a reversal brings
// the balance back to zero and a new opening balance may follow. Dealers
// get 403, a distributor cannot book on a non-neighbour or on the center's
// cari (404).
func TestIntegrationAccountingOpeningBalance(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t177-dist", "distributor", center)
	other := it.org("t177-other", "distributor", center)
	dealer := it.org("t177-dealer", "dealer", dist)

	accUser, apw := it.user("t177-acc")
	it.member(center, accUser, "staff", rbac.RoleCenterAccounting)
	distOwner, dpw := it.user("t177-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerStaff, rpw := it.user("t177-dealer-staff")
	it.member(dealer, dealerStaff, "staff")
	tok := it.loginOrg(accUser, apw, center)

	const path = "/v1/accounting/opening-balances"
	body := map[string]any{
		"counterparty_organization_uuid": dist.Uuid.String(), "side": "debit",
		"amount": "1000.00", "opening_date": "2020-01-15", "description": "TEC-177",
	}

	// 1. Step-up is required.
	code, env := it.do("POST", path, hostOlex, tok, body)
	if code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("opening without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(accUser.Uuid)

	// Validation: side, date in the future.
	it.accDo("POST", path, tok, map[string]any{
		"counterparty_organization_uuid": dist.Uuid.String(), "side": "both", "amount": "1", "opening_date": "2020-01-15",
	}, http.StatusBadRequest)
	it.accDo("POST", path, tok, map[string]any{
		"counterparty_organization_uuid": dist.Uuid.String(), "side": "debit", "amount": "1",
		"opening_date": time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly),
	}, http.StatusBadRequest)

	// 2. Debit opening: a charge on the distributor's cari, dated 2020-01-15.
	open := decodeData[accEntry](t, it.accDo("POST", path, tok, body, http.StatusCreated))
	if open.Direction != "charge" || open.Category != "opening_balance" || open.Amount != "1000.00" ||
		open.SourceType == nil || *open.SourceType != "opening_balance" || open.CariUUID == nil {
		t.Fatalf("opening = %+v", open)
	}
	cariUUID := *open.CariUUID
	cariPath := "/v1/accounting/cari/" + cariUUID
	if c := decodeData[accCari](t, it.accDo("GET", cariPath, tok, nil, http.StatusOK)); c.Balance != "1000.00" {
		t.Fatalf("cari after opening = %+v", c)
	}

	// 3. Statement: the opening is the first line at its date; a period
	// after it starts with it as the opening balance.
	all := decodeData[stmt](t, it.accDo("GET", cariPath+"/statement?locale=en", tok, nil, http.StatusOK))
	if len(all.Lines) != 1 || all.Lines[0].Date[:10] != "2020-01-15" || all.Lines[0].Debit != "1000.00" ||
		all.Lines[0].Balance != "1000.00" || all.Lines[0].SourceLabel != "Opening balance" || all.ClosingBalance != "1000.00" {
		t.Fatalf("statement = %+v", all)
	}
	later := decodeData[stmt](t, it.accDo("GET", cariPath+"/statement?from=2020-02-01", tok, nil, http.StatusOK))
	if later.OpeningBalance != "1000.00" || len(later.Lines) != 0 {
		t.Fatalf("statement from february = %+v", later)
	}

	// 4. The same opening balance again is a replay; other values are 409,
	// also through cari_uuid.
	replay := decodeData[accEntry](t, it.accDo("POST", path, tok, body, http.StatusOK))
	if replay.UUID != open.UUID {
		t.Fatalf("replayed opening %s != %s", replay.UUID, open.UUID)
	}
	code, env = it.do("POST", path, hostOlex, tok, map[string]any{
		"cari_uuid": cariUUID, "side": "debit", "amount": "999.00", "opening_date": "2020-01-15",
	})
	if code != http.StatusConflict || errCode(env) != "OPENING_BALANCE_EXISTS" {
		t.Fatalf("second opening = %d %s", code, errCode(env))
	}
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE source_type = 'opening_balance' AND source_uuid = $1::uuid`, cariUUID); n != 1 {
		t.Fatalf("opening rows = %d", n)
	}

	// 5. Reversal: balance back to 0; audit and outbox rows.
	rev := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries/"+open.UUID+"/void", tok,
		map[string]string{"reason": "wrong amount"}, http.StatusCreated))
	if rev.ReversalOfUUID == nil || *rev.ReversalOfUUID != open.UUID || rev.Amount != "-1000.00" {
		t.Fatalf("reversal = %+v", rev)
	}
	if c := decodeData[accCari](t, it.accDo("GET", cariPath, tok, nil, http.StatusOK)); c.Balance != "0.00" {
		t.Fatalf("cari after reversal = %+v", c)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE action = 'accounting.opening_balance_posted'
		AND resource = 'finance_entry' AND resource_uuid = $1::uuid`, open.UUID); n != 1 {
		t.Fatalf("posted audit rows = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE action = 'accounting.opening_balance_voided'
		AND resource_uuid = $1::uuid`, rev.UUID); n != 1 {
		t.Fatalf("voided audit rows = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'cari.opening_balance_posted'
		AND payload->'data'->>'entry_uuid' = $1`, open.UUID); n != 1 {
		t.Fatalf("opening events = %d", n)
	}

	// 6. After the reversal a new (credit) opening balance is booked as the
	// next revision: we owe the distributor 250.
	credit := decodeData[accEntry](t, it.accDo("POST", path, tok, map[string]any{
		"cari_uuid": cariUUID, "side": "credit", "amount": "250.00", "opening_date": "2020-01-15",
	}, http.StatusCreated))
	if credit.Direction != "collection" || credit.Category != "opening_balance" {
		t.Fatalf("credit opening = %+v", credit)
	}
	if c := decodeData[accCari](t, it.accDo("GET", cariPath, tok, nil, http.StatusOK)); c.Balance != "-250.00" {
		t.Fatalf("cari after credit opening = %+v", c)
	}
	incomes := decodeData[struct {
		Total int64 `json:"total"`
	}](t, it.accDo("GET", "/v1/accounting/entries?direction=income&cari_uuid="+cariUUID, tok, nil, http.StatusOK))
	if incomes.Total != 0 {
		t.Fatalf("opening wrote %d income rows", incomes.Total)
	}

	// 7. Scope: dealer staff 403 (TEC-341 opens own-book writes to the dealer
	// owner and dealer accounting); the distributor books on its own book only.
	dealerTok := it.loginOrg(dealerStaff, rpw, dealer)
	it.stepUp(dealerStaff.Uuid)
	code, env = it.do("POST", path, hostOlex, dealerTok, map[string]any{
		"counterparty_organization_uuid": dist.Uuid.String(), "side": "debit", "amount": "1", "opening_date": "2020-01-15",
	})
	if code != http.StatusForbidden {
		t.Fatalf("dealer staff opening = %d %s", code, errCode(env))
	}
	distTok := it.loginOrg(distOwner, dpw, dist)
	it.stepUp(distOwner.Uuid)
	it.accDo("POST", path, distTok, map[string]any{
		"counterparty_organization_uuid": other.Uuid.String(), "side": "debit", "amount": "1", "opening_date": "2020-01-15",
	}, http.StatusNotFound)
	it.accDo("POST", path, distTok, map[string]any{
		"cari_uuid": cariUUID, "side": "debit", "amount": "1", "opening_date": "2020-01-15",
	}, http.StatusNotFound)
	own := decodeData[accEntry](t, it.accDo("POST", path, distTok, map[string]any{
		"counterparty_organization_uuid": center.Uuid.String(), "side": "credit", "amount": "250.00", "opening_date": "2020-01-15",
	}, http.StatusCreated))
	if own.Direction != "collection" || own.CariUUID == nil || *own.CariUUID == cariUUID {
		t.Fatalf("distributor opening = %+v", own)
	}
}
