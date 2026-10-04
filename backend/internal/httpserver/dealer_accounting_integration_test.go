package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-342 (F3-07b) acceptance over HTTP: dealer accounting writes need the
// dealer_accounting module (403 FEATURE_DISABLED when the distributor closes
// it; reads stay open; the distributor itself is not gated), dealer_staff
// reaches no accounting endpoint, and the customer cari opens only for a
// customer the dealer serves and closes to 0 with a collection.
func TestIntegrationDealerAccountingGate(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t342-dist", "distributor", center)
	dealer := it.org("t342-dealer", "dealer", dist)
	other := it.org("t342-other", "dealer", dist)

	distOwner, dpw := it.user("t342-dist-owner")
	it.member(dist, distOwner, "owner")
	owner, opw := it.user("t342-dealer-owner")
	it.member(dealer, owner, "owner")
	accUser, apw := it.user("t342-dealer-acc")
	it.member(dealer, accUser, "staff", rbac.RoleDealerAccounting)
	staff, spw := it.user("t342-dealer-staff")
	it.member(dealer, staff, "staff")

	distTok := it.loginOrg(distOwner, dpw, dist)
	ownerTok := it.loginOrg(owner, opw, dealer)
	accTok := it.loginOrg(accUser, apw, dealer)
	staffTok := it.loginOrg(staff, spw, dealer)

	// 1. Module on (standard default): the dealer owner and the dealer
	// accounting role write manual entries.
	cash := decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", ownerTok, map[string]any{
		"type": "cash", "name": "Kasa " + it.suffix,
	}, http.StatusCreated))
	exp := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries", ownerTok, map[string]any{
		"direction": "expense", "category": "rent", "amount": "900", "account_uuid": cash.UUID,
		"description": "Dükkan kirası",
	}, http.StatusCreated))
	if exp.Direction != "expense" || exp.Amount != "900.00" {
		t.Fatalf("dealer manual expense = %+v", exp)
	}
	it.accDo("POST", "/v1/accounting/entries", accTok, map[string]any{
		"direction": "income", "category": "other_income", "amount": "50", "account_uuid": cash.UUID,
	}, http.StatusCreated)

	// 2. Customer cari: only a customer the dealer serves.
	served, _ := it.user("t342-customer")
	stranger, _ := it.user("t342-stranger")
	it.serveCustomer(dealer, served)
	it.serveCustomer(other, stranger)
	it.accDo("POST", "/v1/accounting/cari", ownerTok, map[string]any{
		"counterparty_type": "user", "counterparty_uuid": stranger.Uuid.String(),
	}, http.StatusNotFound)
	it.accDo("POST", "/v1/accounting/cari", ownerTok, map[string]any{
		"counterparty_type": "user", "counterparty_uuid": uuid.NewString(),
	}, http.StatusNotFound)
	it.accDo("POST", "/v1/accounting/cari", ownerTok, map[string]any{
		"counterparty_type": "organization", "counterparty_uuid": dist.Uuid.String(),
	}, http.StatusBadRequest)
	cari := decodeData[accCari](t, it.accDo("POST", "/v1/accounting/cari", ownerTok, map[string]any{
		"counterparty_type": "user", "counterparty_uuid": served.Uuid.String(),
	}, http.StatusCreated))
	if cari.Counterparty.Type != "user" || cari.Counterparty.UUID != served.Uuid.String() || cari.Balance != "0.00" {
		t.Fatalf("customer cari = %+v", cari)
	}
	again := decodeData[accCari](t, it.accDo("POST", "/v1/accounting/cari", accTok, map[string]any{
		"counterparty_type": "user", "counterparty_uuid": served.Uuid.String(),
	}, http.StatusOK))
	if again.UUID != cari.UUID {
		t.Fatalf("reopened cari %s != %s", again.UUID, cari.UUID)
	}

	// Service on credit, then the collection: the balance is back to 0.
	it.accDo("POST", "/v1/accounting/entries", ownerTok, map[string]any{
		"direction": "income", "category": "service_income", "amount": "3200", "cari_uuid": cari.UUID,
		"description": "PPF tam kaplama",
	}, http.StatusCreated)
	cariPath := "/v1/accounting/cari/" + cari.UUID
	if got := decodeData[accCari](t, it.accDo("GET", cariPath, ownerTok, nil, http.StatusOK)); got.Balance != "3200.00" {
		t.Fatalf("customer cari after income = %+v", got)
	}
	it.accDo("POST", "/v1/accounting/collections", accTok, map[string]any{
		"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "3200", "description": "Kart",
	}, http.StatusCreated)
	if got := decodeData[accCari](t, it.accDo("GET", cariPath, ownerTok, nil, http.StatusOK)); got.Balance != "0.00" || got.EntryCount != 2 {
		t.Fatalf("customer cari after collection = %+v", got)
	}
	st := decodeData[struct {
		ClosingBalance string `json:"closing_balance"`
		Lines          []any  `json:"lines"`
	}](t, it.accDo("GET", cariPath+"/statement", ownerTok, nil, http.StatusOK))
	if st.ClosingBalance != "0.00" || len(st.Lines) != 2 {
		t.Fatalf("customer statement = %+v", st)
	}

	// 3. dealer_staff reaches no accounting endpoint, read or write.
	for _, c := range []struct {
		method, path string
		body         map[string]any
	}{
		{"GET", "/v1/accounting/accounts", nil},
		{"GET", "/v1/accounting/entries", nil},
		{"GET", "/v1/accounting/cari", nil},
		{"GET", cariPath + "/statement", nil},
		{"POST", "/v1/accounting/accounts", map[string]any{"type": "cash", "name": "x"}},
		{"POST", "/v1/accounting/entries", map[string]any{"direction": "expense", "category": "rent", "amount": "1", "account_uuid": cash.UUID}},
		{"POST", "/v1/accounting/cari", map[string]any{"counterparty_type": "user", "counterparty_uuid": served.Uuid.String()}},
		{"POST", "/v1/accounting/collections", map[string]any{"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "1"}},
		{"POST", "/v1/accounting/payments", map[string]any{"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "1"}},
		{"POST", "/v1/accounting/entries/" + exp.UUID + "/void", map[string]any{"reason": "x"}},
		{"POST", "/v1/accounting/disputes", map[string]any{"entry_uuid": exp.UUID, "reason": "x"}},
	} {
		if code, env := it.do(c.method, c.path, hostOlex, staffTok, c.body); code != http.StatusForbidden {
			t.Fatalf("dealer staff %s %s = %d %s, want 403", c.method, c.path, code, errCode(env))
		}
	}

	// 4. The distributor closes dealer_accounting for the dealer: every
	// write answers 403 FEATURE_DISABLED, reads stay open.
	code, env := it.do("PUT", "/v1/tenant/modules/dealers/"+dealer.Uuid.String()+"/"+features.ModuleDealerAccounting,
		hostOlex, distTok, map[string]any{"enabled": false})
	if code != http.StatusOK {
		t.Fatalf("close dealer_accounting = %d %s", code, errCode(env))
	}
	it.stepUp(owner.Uuid)
	for _, c := range []struct {
		path string
		body map[string]any
	}{
		{"/v1/accounting/accounts", map[string]any{"type": "cash", "name": "y"}},
		{"/v1/accounting/entries", map[string]any{"direction": "expense", "category": "rent", "amount": "1", "account_uuid": cash.UUID}},
		{"/v1/accounting/cari", map[string]any{"counterparty_type": "user", "counterparty_uuid": served.Uuid.String()}},
		{"/v1/accounting/collections", map[string]any{"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "1"}},
		{"/v1/accounting/payments", map[string]any{"account_uuid": cash.UUID, "cari_uuid": cari.UUID, "amount": "1"}},
		{"/v1/accounting/entries/" + exp.UUID + "/void", map[string]any{"reason": "x"}},
		{"/v1/accounting/opening-balances", map[string]any{"counterparty_organization_uuid": dist.Uuid.String(), "side": "debit", "amount": "1"}},
	} {
		code, env := it.do("POST", c.path, hostOlex, ownerTok, c.body)
		if code != http.StatusForbidden || errCode(env) != response.CodeFeatureDisabled {
			t.Fatalf("module off: POST %s = %d %s, want 403 FEATURE_DISABLED", c.path, code, errCode(env))
		}
	}
	it.accDo("PATCH", "/v1/accounting/accounts/"+cash.UUID, accTok, map[string]any{"name": "z"}, http.StatusForbidden)
	it.accDo("GET", "/v1/accounting/accounts", ownerTok, nil, http.StatusOK)
	it.accDo("GET", cariPath+"/statement", accTok, nil, http.StatusOK)
	it.accDo("GET", "/v1/accounting/disputes", ownerTok, nil, http.StatusOK)

	// The distributor (not a dealer) keeps writing its own book.
	it.accDo("POST", "/v1/accounting/accounts", distTok, map[string]any{
		"type": "cash", "name": "Dist kasa " + it.suffix,
	}, http.StatusCreated)
}

func (it *itest) serveCustomer(o db.Organization, u db.User) {
	it.t.Helper()
	if _, err := it.q.LinkCustomerOrganization(context.Background(), db.LinkCustomerOrganizationParams{
		UserID: u.ID, OrganizationID: o.ID, BrandID: o.BrandID,
	}); err != nil {
		it.t.Fatalf("link customer: %v", err)
	}
}
