package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// finance_entries is append-only, so the rows of these tests stay; every
// fixture carries the run suffix (new distributor, new accounts).

type accAccount struct {
	UUID     string  `json:"uuid"`
	Type     string  `json:"type"`
	Currency string  `json:"currency"`
	IBAN     *string `json:"iban"`
	Active   bool    `json:"active"`
	Balance  string  `json:"balance"`
}

type accCari struct {
	UUID         string `json:"uuid"`
	Balance      string `json:"balance"`
	EntryCount   int64  `json:"entry_count"`
	Counterparty struct {
		Type string `json:"type"`
		UUID string `json:"uuid"`
	} `json:"counterparty"`
}

type accEntry struct {
	UUID           string  `json:"uuid"`
	Direction      string  `json:"direction"`
	Category       string  `json:"category"`
	Amount         string  `json:"amount"`
	CariUUID       *string `json:"cari_uuid"`
	SourceType     *string `json:"source_type"`
	ReversalOfUUID *string `json:"reversal_of_uuid"`
	Voided         bool    `json:"voided"`
}

func decodeData[T any](t *testing.T, env envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode %s: %v", env.Data, err)
	}
	return v
}

func (it *itest) accDo(method, path, token string, body any, want int) envelope {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	return env
}

// TEC-172 acceptance: the center opens an account, writes a manual charge
// to a distributor, collects it; the cari balance is back to 0 and the
// collection wrote no income. Dealer staff get 403 on writes; a void needs a
// step-up.
func TestIntegrationAccountingFlow(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t172-dist", "distributor", center)
	dealer := it.org("t172-dealer", "dealer", dist)

	accUser, apw := it.user("t172-acc")
	it.member(center, accUser, "staff", rbac.RoleCenterAccounting)
	distOwner, dpw := it.user("t172-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rpw := it.user("t172-dealer-owner")
	it.member(dealer, dealerOwner, "owner")

	tok := it.loginOrg(accUser, apw, center)

	// Categories carry labels in the request language.
	cats := decodeData[struct {
		Items []struct {
			Key, Direction, Label string
		} `json:"items"`
	}](t, it.accDo("GET", "/v1/accounting/categories?direction=charge", tok, nil, http.StatusOK))
	if len(cats.Items) == 0 || cats.Items[0].Label == "" {
		t.Fatalf("charge categories = %+v", cats.Items)
	}

	// 1. Accounts: IBAN is validated; only bank accounts carry one.
	it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "bank", "name": "Bank " + it.suffix, "iban": "TR330006100519786457841327",
	}, http.StatusBadRequest)
	it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "cash", "name": "Cash " + it.suffix, "iban": "TR330006100519786457841326",
	}, http.StatusBadRequest)
	bank := decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "bank", "name": "Bank " + it.suffix, "iban": "TR33 0006 1005 1978 6457 8413 26",
	}, http.StatusCreated))
	if bank.IBAN == nil || *bank.IBAN != "TR330006100519786457841326" || bank.Currency != center.Currency || bank.Balance != "0.00" {
		t.Fatalf("bank account = %+v", bank)
	}
	cash := decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "cash", "name": "Cash " + it.suffix,
	}, http.StatusCreated))

	// 2. Manual charge to the distributor (a direct child): the cari opens.
	charge := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "charge", "category": "opening_balance", "amount": "1500.00",
		"counterparty_organization_uuid": dist.Uuid.String(), "description": "TEC-172",
	}, http.StatusCreated))
	if charge.CariUUID == nil || charge.SourceType == nil || *charge.SourceType != "manual" || charge.Amount != "1500.00" {
		t.Fatalf("charge = %+v", charge)
	}
	cariPath := "/v1/accounting/cari/" + *charge.CariUUID
	cari := decodeData[accCari](t, it.accDo("GET", cariPath, tok, nil, http.StatusOK))
	if cari.Balance != "1500.00" || cari.Counterparty.UUID != dist.Uuid.String() {
		t.Fatalf("cari after charge = %+v", cari)
	}
	list := decodeData[struct {
		Items []accCari `json:"items"`
		Total int64     `json:"total"`
	}](t, it.accDo("GET", "/v1/accounting/cari?limit=100&q="+it.suffix, tok, nil, http.StatusOK))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].UUID != cari.UUID {
		t.Fatalf("cari list = %+v", list)
	}

	// A system category and a non-neighbour counterparty are refused.
	it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "income", "category": "sale", "amount": "10", "account_uuid": cash.UUID,
	}, http.StatusBadRequest)
	it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "charge", "category": "adjustment", "amount": "10",
		"counterparty_organization_uuid": dealer.Uuid.String(),
	}, http.StatusNotFound)

	// 3. Collection into the cash account closes the cari (no income).
	key := uuid.NewString()
	collectBody := map[string]any{
		"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "1500.00", "idempotency_key": key,
	}
	col := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/collections", tok, collectBody, http.StatusCreated))
	if col.Direction != "collection" || col.Category != "collection" {
		t.Fatalf("collection = %+v", col)
	}
	replay := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/collections", tok, collectBody, http.StatusOK))
	if replay.UUID != col.UUID {
		t.Fatalf("replayed collection %s != %s", replay.UUID, col.UUID)
	}
	cari = decodeData[accCari](t, it.accDo("GET", cariPath, tok, nil, http.StatusOK))
	if cari.Balance != "0.00" || cari.EntryCount != 2 {
		t.Fatalf("cari after collection = %+v", cari)
	}
	acct := decodeData[accAccount](t, it.accDo("GET", "/v1/accounting/accounts/"+cash.UUID, tok, nil, http.StatusOK))
	if acct.Balance != "1500.00" {
		t.Fatalf("cash balance = %s", acct.Balance)
	}
	incomes := decodeData[struct {
		Total int64 `json:"total"`
	}](t, it.accDo("GET", "/v1/accounting/entries?direction=income&cari_uuid="+cari.UUID, tok, nil, http.StatusOK))
	if incomes.Total != 0 {
		t.Fatalf("collection wrote %d income rows", incomes.Total)
	}
	byCari := decodeData[struct {
		Items []accEntry `json:"items"`
		Total int64      `json:"total"`
	}](t, it.accDo("GET", "/v1/accounting/entries?source_type=manual&cari_uuid="+cari.UUID, tok, nil, http.StatusOK))
	if byCari.Total != 2 || byCari.Items[0].UUID != col.UUID {
		t.Fatalf("cari entries = %+v", byCari)
	}

	// 4. Manual expense, then its void: step-up first.
	exp := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "expense", "category": "rent", "amount": "200", "account_uuid": cash.UUID,
	}, http.StatusCreated))
	code, env := it.do("POST", "/v1/accounting/entries/"+exp.UUID+"/void", hostOlex, tok, map[string]string{"reason": "typo"})
	if code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("void without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(accUser.Uuid)
	rev := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries/"+exp.UUID+"/void", tok,
		map[string]string{"reason": "typo"}, http.StatusCreated))
	if rev.ReversalOfUUID == nil || *rev.ReversalOfUUID != exp.UUID || rev.Amount != "-200.00" {
		t.Fatalf("reversal = %+v", rev)
	}
	it.accDo("POST", "/v1/accounting/entries/"+exp.UUID+"/void", tok, map[string]string{"reason": "again"}, http.StatusConflict)
	voided := decodeData[accEntry](t, it.accDo("GET", "/v1/accounting/entries/"+exp.UUID, tok, nil, http.StatusOK))
	if !voided.Voided {
		t.Fatal("expense not marked voided")
	}
	acct = decodeData[accAccount](t, it.accDo("GET", "/v1/accounting/accounts/"+cash.UUID, tok, nil, http.StatusOK))
	if acct.Balance != "1500.00" {
		t.Fatalf("cash balance after void = %s", acct.Balance)
	}

	// 5. Deactivate (no delete); an inactive account takes no entries.
	it.accDo("PATCH", "/v1/accounting/accounts/"+bank.UUID, tok, map[string]any{"active": false, "iban": nil}, http.StatusOK)
	it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "income", "category": "other_income", "amount": "5", "account_uuid": bank.UUID,
	}, http.StatusBadRequest)

	// 6. Scope: the center reads the distributor's book; the distributor
	// cannot read the center's.
	it.accDo("GET", "/v1/accounting/accounts?organization_uuid="+dist.Uuid.String(), tok, nil, http.StatusOK)
	distTok := it.loginOrg(distOwner, dpw, dist)
	it.accDo("GET", "/v1/accounting/accounts", distTok, nil, http.StatusOK)
	it.accDo("GET", "/v1/accounting/accounts?organization_uuid="+center.Uuid.String(), distTok, nil, http.StatusNotFound)
	it.accDo("GET", cariPath, distTok, nil, http.StatusNotFound)

	// 7. Dealer owner reads and, since TEC-341 (F3-07, dealer_accounting
	// module on), writes its own book; dealer staff gets 403 on every write.
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	it.accDo("GET", "/v1/accounting/accounts", dealerTok, nil, http.StatusOK)
	it.accDo("GET", "/v1/accounting/entries", dealerTok, nil, http.StatusOK)
	it.accDo("POST", "/v1/accounting/accounts", dealerTok, map[string]any{"type": "cash", "name": "Dealer cash " + it.suffix}, http.StatusCreated)
	dealerStaff, spw := it.user("t172-dealer-staff")
	it.member(dealer, dealerStaff, "staff")
	staffTok := it.loginOrg(dealerStaff, spw, dealer)
	for _, w := range []struct {
		path string
		body map[string]any
	}{
		{"/v1/accounting/accounts", map[string]any{"type": "cash", "name": "x"}},
		{"/v1/accounting/entries", map[string]any{"direction": "expense", "category": "rent", "amount": "1", "counterparty_organization_uuid": dist.Uuid.String()}},
		{"/v1/accounting/payments", map[string]any{"amount": "1", "counterparty_organization_uuid": dist.Uuid.String()}},
	} {
		code, env := it.do("POST", w.path, hostOlex, staffTok, w.body)
		if code != http.StatusForbidden || errCode(env) != "FORBIDDEN" {
			t.Fatalf("dealer staff POST %s = %d %s", w.path, code, errCode(env))
		}
	}
}
