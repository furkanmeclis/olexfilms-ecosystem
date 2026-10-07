package httpserver

import (
	"context"
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
		// TEC-381: planned payments.
		{"GET", "/v1/staff-payments?status=planned", nil},
		{"PATCH", "/v1/staff-payments/00000000-0000-0000-0000-000000000000", map[string]any{"amount": "1"}},
		{"POST", "/v1/staff-payments/00000000-0000-0000-0000-000000000000/cancel", nil},
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

// TEC-381: a payroll with a future payment day writes planned salaries with
// no ledger row; the planned list sums them; a planned payment is edited
// and cancelled, a posted one is not.
func TestIntegrationStaffPaymentsPlanned(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t381-dist", "distributor", center)
	dealer := it.org("t381-dealer", "dealer", dist)

	owner, opw := it.user("t381-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, opw, dealer)

	for _, name := range []string{"Usta", "Kalfa"} {
		it.accDo("POST", "/v1/staff-profiles", tok, map[string]any{
			"name": name + " " + it.suffix, "monthly_salary": "1000", "active": true,
		}, http.StatusCreated)
	}
	type payment struct {
		UUID             string  `json:"uuid"`
		StaffName        string  `json:"staff_name"`
		Status           string  `json:"status"`
		Amount           string  `json:"amount"`
		PaidOn           string  `json:"paid_on"`
		FinanceEntryUUID *string `json:"finance_entry_uuid"`
	}
	type payroll struct {
		Created int       `json:"created"`
		Items   []payment `json:"items"`
	}
	run := decodeData[payroll](t, it.accDo("POST", "/v1/staff-payments/payroll?period=2099-01&paid_on=2099-01-31", tok, nil, http.StatusCreated))
	if run.Created != 2 || run.Items[0].Status != "planned" || run.Items[0].FinanceEntryUUID != nil || run.Items[0].PaidOn != "2099-01-31" {
		t.Fatalf("planned payroll = %+v", run)
	}
	var ledgerRows int
	if err := it.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM finance_entries
		WHERE organization_id = $1 AND source_type = 'staff_payment'`, dealer.ID).Scan(&ledgerRows); err != nil || ledgerRows != 0 {
		t.Fatalf("ledger rows of planned payroll = %d (%v)", ledgerRows, err)
	}

	type page struct {
		Items       []payment `json:"items"`
		Total       int64     `json:"total"`
		TotalAmount string    `json:"total_amount"`
		Currency    string    `json:"currency"`
	}
	planned := decodeData[page](t, it.accDo("GET", "/v1/staff-payments?status=planned&sort=staff_name", tok, nil, http.StatusOK))
	if planned.Total != 2 || planned.TotalAmount != "2000.00" || planned.Currency == "" ||
		planned.Items[0].StaffName > planned.Items[1].StaffName {
		t.Fatalf("planned list = %+v", planned)
	}
	it.accDo("GET", "/v1/staff-payments?status=done", tok, nil, http.StatusBadRequest)
	it.accDo("GET", "/v1/staff-payments?sort=nope", tok, nil, http.StatusBadRequest)
	it.accDo("GET", "/v1/staff-payments?paid_on_from=2099-02-01&paid_on_to=2099-01-01", tok, nil, http.StatusBadRequest)
	if p := decodeData[page](t, it.accDo("GET", "/v1/staff-payments?paid_on_from=2099-02-01", tok, nil, http.StatusOK)); p.Total != 0 {
		t.Fatalf("paid_on_from filter = %+v", p)
	}

	first := planned.Items[0].UUID
	edited := decodeData[payment](t, it.accDo("PATCH", "/v1/staff-payments/"+first, tok, map[string]any{"amount": "1250"}, http.StatusOK))
	if edited.Status != "planned" || edited.Amount != "1250.00" {
		t.Fatalf("edited = %+v", edited)
	}
	cancelled := decodeData[payment](t, it.accDo("POST", "/v1/staff-payments/"+first+"/cancel", tok, nil, http.StatusOK))
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	if code, env := it.do("POST", "/v1/staff-payments/"+first+"/cancel", hostOlex, tok, nil); code != http.StatusUnprocessableEntity || errCode(env) != "STAFF_PAYMENT_NOT_PLANNED" {
		t.Fatalf("second cancel = %d %s", code, errCode(env))
	}
	if p := decodeData[page](t, it.accDo("GET", "/v1/staff-payments?status=planned", tok, nil, http.StatusOK)); p.Total != 1 || p.TotalAmount != "1000.00" {
		t.Fatalf("planned after cancel = %+v", p)
	}

	// Today's payroll is booked at once; posted payments are not changed.
	now := decodeData[payroll](t, it.accDo("POST", "/v1/staff-payments/payroll?period=2026-01", tok, nil, http.StatusCreated))
	if now.Created != 2 || now.Items[0].Status != "posted" || now.Items[0].FinanceEntryUUID == nil {
		t.Fatalf("today payroll = %+v", now)
	}
	if code, env := it.do("POST", "/v1/staff-payments/"+now.Items[0].UUID+"/cancel", hostOlex, tok, nil); code != http.StatusUnprocessableEntity || errCode(env) != "STAFF_PAYMENT_NOT_PLANNED" {
		t.Fatalf("cancel posted = %d %s", code, errCode(env))
	}
	it.accDo("PATCH", "/v1/staff-payments/00000000-0000-0000-0000-000000000000", tok, map[string]any{"amount": "1"}, http.StatusNotFound)
}
