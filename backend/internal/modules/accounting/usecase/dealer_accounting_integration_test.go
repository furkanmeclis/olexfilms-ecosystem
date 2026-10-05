package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-342 (F3-07b) acceptance at the use-case level: the dealer_accounting
// module opens a dealer's manual writes, sourced rows stay protected, and a
// customer cari is opened only for a customer the dealer serves.

// moduleSwitch answers dealer_accounting from a field; other modules are on.
type moduleSwitch struct{ dealerAccounting bool }

func (m *moduleSwitch) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	if key == features.ModuleDealerAccounting {
		return m.dealerAccounting, nil
	}
	return true, nil
}

func dealerService(e *disputeEnv, sw *moduleSwitch) *acc.Service {
	return acc.New(e.pool, e.q, e.poster, sw)
}

func (e *disputeEnv) customer(t *testing.T, name string) db.User {
	t.Helper()
	u, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: "t342-" + name + "-" + e.suffix + "@example.test", Valid: true},
		PasswordHash: "x", Name: name, Surname: "Müşteri", Status: "active",
	})
	if err != nil {
		t.Fatalf("customer %s: %v", name, err)
	}
	return u
}

func (e *disputeEnv) serve(t *testing.T, o db.Organization, u db.User) {
	t.Helper()
	if _, err := e.q.LinkCustomerOrganization(e.ctx, db.LinkCustomerOrganizationParams{
		UserID: u.ID, OrganizationID: o.ID, BrandID: o.BrandID,
	}); err != nil {
		t.Fatalf("link customer: %v", err)
	}
}

func TestDealerAccounting_ModuleGatesManualWrites(t *testing.T) {
	e := newDisputeEnv(t)
	sw := &moduleSwitch{dealerAccounting: true}
	svc := dealerService(e, sw)
	c := caller(e.dealer, rbac.PermAccountingWrite)

	cash, err := svc.CreateAccount(e.ctx, c, acc.CreateAccountInput{Type: acc.AccountCash, Name: "Kasa " + e.suffix})
	if err != nil {
		t.Fatalf("module on: create account: %v", err)
	}
	exp, _, err := svc.CreateEntry(e.ctx, c, acc.EntryInput{
		Direction: "expense", Category: "rent", Amount: "750", AccountUUID: &cash.UUID, Description: "Kira",
	})
	if err != nil || exp.Amount != "750.00" || exp.Description == nil || *exp.Description != "Kira" {
		t.Fatalf("module on: manual expense = %+v, %v", exp, err)
	}

	sw.dealerAccounting = false
	if _, _, err := svc.CreateEntry(e.ctx, c, acc.EntryInput{
		Direction: "expense", Category: "rent", Amount: "1", AccountUUID: &cash.UUID,
	}); !errors.Is(err, acc.ErrForbidden) {
		t.Fatalf("module off: manual expense err = %v, want ErrForbidden", err)
	}
	if _, err := svc.CreateAccount(e.ctx, c, acc.CreateAccountInput{Type: acc.AccountCash, Name: "x"}); !errors.Is(err, acc.ErrForbidden) {
		t.Fatalf("module off: create account err = %v", err)
	}
	// Reads stay open.
	if _, err := svc.ListAccounts(e.ctx, caller(e.dealer, rbac.PermAccountingRead), acc.AccountFilter{}); err != nil {
		t.Fatalf("module off: read: %v", err)
	}
	// The distributor is not gated by the dealer module.
	if _, err := svc.CreateAccount(e.ctx, caller(e.dist, rbac.PermAccountingWrite),
		acc.CreateAccountInput{Type: acc.AccountCash, Name: "Dist kasa " + e.suffix}); err != nil {
		t.Fatalf("distributor with dealer module off: %v", err)
	}
}

func TestDealerAccounting_SourcedEntryNotReversible(t *testing.T) {
	e := newDisputeEnv(t)
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true})
	_, row := e.sale(t, "1200.00")

	// Even with the module on, the dealer cannot void the distributor's
	// order row (K24) ...
	if _, err := svc.Void(e.ctx, caller(e.dealer, rbac.PermAccountingWrite), row.Uuid, "yanlış"); !errors.Is(err, acc.ErrNotVoidable) {
		t.Fatalf("void sourced row err = %v, want ErrNotVoidable", err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM finance_entries WHERE reversal_of_id = $1`, row.ID); n != 0 {
		t.Fatalf("reversals = %d", n)
	}
	// ... it can only open a dispute.
	d, err := svc.OpenDispute(e.ctx, caller(e.dealer, rbac.PermAccountingDispute),
		acc.DisputeInput{EntryUUID: row.Uuid, Reason: "Fiyat yanlış"})
	if err != nil || d.Status != "open" {
		t.Fatalf("open dispute = %+v, %v", d, err)
	}
}

func TestDealerAccounting_CustomerCari(t *testing.T) {
	e := newDisputeEnv(t)
	sw := &moduleSwitch{dealerAccounting: true}
	svc := dealerService(e, sw)
	c := caller(e.dealer, rbac.PermAccountingWrite)

	served := e.customer(t, "served")
	e.serve(t, e.dealer, served)
	stranger := e.customer(t, "stranger")
	e.serve(t, e.other, stranger) // served by another organization only

	// Not served by the dealer (or unknown): not found.
	for _, id := range []uuid.UUID{stranger.Uuid, uuid.New()} {
		if _, _, err := svc.OpenCustomerCari(e.ctx, c, acc.OpenCariInput{
			CounterpartyType: acc.CounterpartyTypeUser, CounterpartyUUID: &id,
		}); !errors.Is(err, acc.ErrCustomerNotFound) {
			t.Fatalf("open cari for %s err = %v, want ErrCustomerNotFound", id, err)
		}
	}
	var ve *acc.ValidationError
	if _, _, err := svc.OpenCustomerCari(e.ctx, c, acc.OpenCariInput{
		CounterpartyType: "organization", CounterpartyUUID: &served.Uuid,
	}); !errors.As(err, &ve) {
		t.Fatalf("organization type err = %v, want validation", err)
	}

	cari, created, err := svc.OpenCustomerCari(e.ctx, c, acc.OpenCariInput{
		CounterpartyType: acc.CounterpartyTypeUser, CounterpartyUUID: &served.Uuid,
	})
	if err != nil || !created || cari.Counterparty.Type != "user" || cari.Counterparty.UUID == nil ||
		*cari.Counterparty.UUID != served.Uuid || cari.Balance != "0.00" || cari.Currency != e.dealer.Currency {
		t.Fatalf("open cari = %+v created=%v, %v", cari, created, err)
	}
	again, created, err := svc.OpenCustomerCari(e.ctx, c, acc.OpenCariInput{
		CounterpartyType: acc.CounterpartyTypeUser, CounterpartyUUID: &served.Uuid,
	})
	if err != nil || created || again.UUID != cari.UUID {
		t.Fatalf("reopen = %+v created=%v, %v", again, created, err)
	}

	// Service income on credit, then the collection: balance back to 0.
	inc, _, err := svc.CreateEntry(e.ctx, c, acc.EntryInput{
		Direction: "income", Category: "service_income", Amount: "2500", CariUUID: &cari.UUID,
		Description: "Seramik kaplama",
	})
	if err != nil || inc.CariUUID == nil || *inc.CariUUID != cari.UUID {
		t.Fatalf("income on customer cari = %+v, %v", inc, err)
	}
	got, err := svc.GetCari(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, cari.UUID)
	if err != nil || got.Balance != "2500.00" {
		t.Fatalf("cari after income = %+v, %v", got, err)
	}
	cash, err := svc.CreateAccount(e.ctx, c, acc.CreateAccountInput{Type: acc.AccountCash, Name: "Kasa " + e.suffix})
	if err != nil {
		t.Fatal(err)
	}
	col, _, err := svc.Settle(e.ctx, c, "collection", acc.SettlementInput{
		AccountUUID: &cash.UUID, CariUUID: &cari.UUID, Amount: "2500", Description: "Nakit tahsilat",
	})
	if err != nil || col.Direction != "collection" {
		t.Fatalf("collection = %+v, %v", col, err)
	}
	got, err = svc.GetCari(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, cari.UUID)
	if err != nil || got.Balance != "0.00" || got.EntryCount != 2 {
		t.Fatalf("cari after collection = %+v, %v", got, err)
	}

	// Another organization's customer cari is not reachable by uuid.
	if _, _, err := svc.CreateEntry(e.ctx, caller(e.dist, rbac.PermAccountingWrite), acc.EntryInput{
		Direction: "charge", Category: "adjustment", Amount: "1", CariUUID: &cari.UUID,
	}); !errors.Is(err, acc.ErrCariNotFound) {
		t.Fatalf("foreign cari err = %v, want ErrCariNotFound", err)
	}

	// Module off: opening a customer cari is a write and is refused.
	sw.dealerAccounting = false
	if _, _, err := svc.OpenCustomerCari(e.ctx, c, acc.OpenCariInput{
		CounterpartyType: acc.CounterpartyTypeUser, CounterpartyUUID: &served.Uuid,
	}); !errors.Is(err, acc.ErrForbidden) {
		t.Fatalf("module off: open cari err = %v", err)
	}
}

func TestStaffPayments_PayrollIdempotentForActiveStaff(t *testing.T) {
	e := newDisputeEnv(t)
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true})
	c := caller(e.dealer, rbac.PermStaffManage)

	s1, err := svc.CreateStaffProfile(e.ctx, c, acc.CreateStaffProfileInput{
		Name: "Usta " + e.suffix, MonthlySalary: strPtr("1000"), Active: true,
	})
	if err != nil {
		t.Fatalf("staff 1: %v", err)
	}
	s2, err := svc.CreateStaffProfile(e.ctx, c, acc.CreateStaffProfileInput{
		Name: "Danışman " + e.suffix, MonthlySalary: strPtr("1500.50"), Active: true,
	})
	if err != nil {
		t.Fatalf("staff 2: %v", err)
	}

	pay, err := svc.RunPayroll(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), "2026-10")
	if err != nil {
		t.Fatalf("payroll: %v", err)
	}
	if pay.Created != 2 || pay.Skipped != 0 || len(pay.Items) != 2 {
		t.Fatalf("payroll first = %+v", pay)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM finance_entries
		WHERE organization_id = $1 AND source_type = 'staff_payment'
		  AND direction = 'expense' AND category = 'salary'`, e.dealer.ID); n != 2 {
		t.Fatalf("salary expense rows = %d", n)
	}
	if total := e.sum(t, `SELECT COALESCE(SUM(amount), 0)::text FROM finance_entries
		WHERE organization_id = $1 AND source_type = 'staff_payment'
		  AND direction = 'expense' AND category = 'salary'`, e.dealer.ID); total != "2500.50" {
		t.Fatalf("salary expense total = %s", total)
	}

	again, err := svc.RunPayroll(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), "2026-10")
	if err != nil {
		t.Fatalf("payroll second: %v", err)
	}
	if again.Created != 0 || again.Skipped != 2 || len(again.Items) != 0 {
		t.Fatalf("payroll second = %+v", again)
	}
	cash, err := svc.CreateAccount(e.ctx, caller(e.dealer, rbac.PermAccountingWrite), acc.CreateAccountInput{
		Type: acc.AccountCash, Name: "Kasa " + e.suffix,
	})
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	if _, err := svc.CreateStaffPayment(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), s1.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentSalary, Period: "2026-10", Amount: strPtr("1000"), AccountUUID: &cash.UUID,
	}); !errors.Is(err, acc.ErrStaffSalaryExists) {
		t.Fatalf("duplicate salary err = %v, want ErrStaffSalaryExists", err)
	}
	if s1.UUID == s2.UUID {
		t.Fatal("staff uuids unexpectedly equal")
	}
}

func TestStaffPayments_PayrollSkipsInactiveStaff(t *testing.T) {
	e := newDisputeEnv(t)
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true})
	c := caller(e.dealer, rbac.PermStaffManage)
	if _, err := svc.CreateStaffProfile(e.ctx, c, acc.CreateStaffProfileInput{
		Name: "Aktif " + e.suffix, MonthlySalary: strPtr("900"), Active: true,
	}); err != nil {
		t.Fatalf("active staff: %v", err)
	}
	inactive, err := svc.CreateStaffProfile(e.ctx, c, acc.CreateStaffProfileInput{
		Name: "Pasif " + e.suffix, MonthlySalary: strPtr("800"), Active: false,
	})
	if err != nil {
		t.Fatalf("inactive staff: %v", err)
	}
	pay, err := svc.RunPayroll(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), "2026-11")
	if err != nil {
		t.Fatalf("payroll: %v", err)
	}
	if pay.Created != 1 || pay.Skipped != 0 {
		t.Fatalf("payroll = %+v", pay)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff_payments WHERE staff_id = (
		SELECT id FROM staff_profiles WHERE uuid = $1)`, inactive.UUID); n != 0 {
		t.Fatalf("inactive staff payments = %d", n)
	}
}

func TestStaffPayments_AdvanceAndSalarySamePeriod(t *testing.T) {
	e := newDisputeEnv(t)
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true})
	c := caller(e.dealer, rbac.PermStaffManage)
	staff, err := svc.CreateStaffProfile(e.ctx, c, acc.CreateStaffProfileInput{
		Name: "Primli " + e.suffix, MonthlySalary: strPtr("2000"), Active: true,
	})
	if err != nil {
		t.Fatalf("staff: %v", err)
	}
	cash, err := svc.CreateAccount(e.ctx, caller(e.dealer, rbac.PermAccountingWrite), acc.CreateAccountInput{
		Type: acc.AccountCash, Name: "Kasa " + e.suffix,
	})
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	paidOn := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)
	advance, err := svc.CreateStaffPayment(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentAdvance, Period: "2026-12", Amount: strPtr("300"), AccountUUID: &cash.UUID,
		Description: strPtr("Yol avansı"), PaidOn: &paidOn,
	})
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	salary, err := svc.CreateStaffPayment(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentSalary, Period: "2026-12", AccountUUID: &cash.UUID, Description: strPtr("Aralık maaşı"),
	})
	if err != nil {
		t.Fatalf("salary: %v", err)
	}
	if advance.Type != acc.StaffPaymentAdvance || salary.Type != acc.StaffPaymentSalary || salary.PeriodAdvances != "300.00" {
		t.Fatalf("payments advance=%+v salary=%+v", advance, salary)
	}
	if total := e.sum(t, `SELECT COALESCE(SUM(amount), 0)::text FROM finance_entries
		WHERE organization_id = $1 AND source_type = 'staff_payment'
		  AND direction = 'expense' AND category IN ('salary', 'staff_advance')`, e.dealer.ID); total != "2300.00" {
		t.Fatalf("expense total = %s", total)
	}
}

func TestStaffPayments_ManualPaymentRequiresAccount(t *testing.T) {
	e := newDisputeEnv(t)
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true})
	staff, err := svc.CreateStaffProfile(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.CreateStaffProfileInput{
		Name: "Hesapsız " + e.suffix, MonthlySalary: strPtr("1000"), Active: true,
	})
	if err != nil {
		t.Fatalf("staff: %v", err)
	}
	_, err = svc.CreateStaffPayment(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentAdvance, Period: "2027-01", Amount: strPtr("100"),
	})
	var ve *acc.ValidationError
	if !errors.As(err, &ve) || ve.Field != "account_uuid" {
		t.Fatalf("manual payment without account err = %v, want account_uuid validation", err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM staff_payments WHERE staff_id = (
		SELECT id FROM staff_profiles WHERE uuid = $1)`, staff.UUID); n != 0 {
		t.Fatalf("staff payments after rejected manual payment = %d", n)
	}
}

func (e *disputeEnv) sum(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var total string
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&total); err != nil {
		t.Fatalf("sum: %v", err)
	}
	return total
}

func strPtr(v string) *string { return &v }
