package httpserver

import (
	"net/http"
	"testing"
)

type staffProfileView struct {
	UUID          string  `json:"uuid"`
	Name          string  `json:"name"`
	MonthlySalary *string `json:"monthly_salary"`
	Active        bool    `json:"active"`
}

func TestIntegrationStaffPaymentsDealerStaffForbidden(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t345-dist", "distributor", center)
	dealer := it.org("t345-dealer", "dealer", dist)

	owner, opw := it.user("t345-owner")
	it.member(dealer, owner, "owner")
	staff, spw := it.user("t345-staff")
	it.member(dealer, staff, "staff")
	ownerTok := it.loginOrg(owner, opw, dealer)
	staffTok := it.loginOrg(staff, spw, dealer)

	profile := decodeData[staffProfileView](t, it.accDo("POST", "/v1/staff-profiles", ownerTok, map[string]any{
		"name": "Usta " + it.suffix, "monthly_salary": "1200", "active": true,
	}, http.StatusCreated))
	if profile.UUID == "" || profile.MonthlySalary == nil || *profile.MonthlySalary != "1200.00" {
		t.Fatalf("staff profile = %+v", profile)
	}

	for _, c := range []struct {
		method string
		path   string
		body   map[string]any
	}{
		{"GET", "/v1/staff-profiles", nil},
		{"POST", "/v1/staff-profiles", map[string]any{"name": "x", "monthly_salary": "1"}},
		{"PATCH", "/v1/staff-profiles/" + profile.UUID, map[string]any{"active": false}},
		{"POST", "/v1/staff-profiles/" + profile.UUID + "/payments", map[string]any{
			"type": "salary", "period": "2026-10",
		}},
		{"POST", "/v1/staff-payments/payroll?period=2026-10", nil},
		{"GET", "/v1/staff-profiles/" + profile.UUID + "/payments", nil},
	} {
		if code, env := it.do(c.method, c.path, hostOlex, staffTok, c.body); code != http.StatusForbidden {
			t.Fatalf("dealer_staff %s %s = %d %s, want 403", c.method, c.path, code, errCode(env))
		}
	}
}

// TEC-349: payment history of a staff card.
func TestIntegrationStaffPaymentsHistory(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t349-dist", "distributor", center)
	dealer := it.org("t349-dealer", "dealer", dist)

	owner, opw := it.user("t349-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, opw, dealer)

	profile := decodeData[staffProfileView](t, it.accDo("POST", "/v1/staff-profiles", tok, map[string]any{
		"name": "Usta " + it.suffix, "monthly_salary": "1500", "active": true,
	}, http.StatusCreated))
	it.accDo("POST", "/v1/staff-payments/payroll?period=2026-09", tok, nil, http.StatusCreated)

	type paymentPage struct {
		Items []struct {
			Type             string `json:"type"`
			Period           string `json:"period"`
			Amount           string `json:"amount"`
			FinanceEntryUUID string `json:"finance_entry_uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	base := "/v1/staff-profiles/" + profile.UUID + "/payments"
	page := decodeData[paymentPage](t, it.accDo("GET", base, tok, nil, http.StatusOK))
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Type != "salary" ||
		page.Items[0].Period != "2026-09" || page.Items[0].Amount != "1500.00" || page.Items[0].FinanceEntryUUID == "" {
		t.Fatalf("history = %+v", page)
	}
	if p := decodeData[paymentPage](t, it.accDo("GET", base+"?type=bonus", tok, nil, http.StatusOK)); p.Total != 0 {
		t.Fatalf("bonus history = %+v", p)
	}
	if p := decodeData[paymentPage](t, it.accDo("GET", base+"?period=2026-10", tok, nil, http.StatusOK)); p.Total != 0 {
		t.Fatalf("2026-10 history = %+v", p)
	}
	it.accDo("GET", base+"?period=2026-13", tok, nil, http.StatusBadRequest)
	it.accDo("GET", "/v1/staff-profiles/00000000-0000-0000-0000-000000000000/payments", tok, nil, http.StatusNotFound)
}
