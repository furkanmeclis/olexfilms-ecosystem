package usecase_test

import (
	"errors"
	"testing"
	"time"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TEC-381 (F3-07j) acceptance: a staff payment is booked on its paid_on; a
// future one waits as planned (no ledger row) until the due job books it.

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// ledgerRows counts the staff_payment ledger rows of one payment.
func (e *disputeEnv) ledgerRows(t *testing.T, payment uuid.UUID) int {
	t.Helper()
	return e.count(t, `SELECT COUNT(*) FROM finance_entries
		WHERE source_type = 'staff_payment' AND source_uuid = $1`, payment)
}

// ledgerDay is the UTC calendar day of a payment's ledger row.
func (e *disputeEnv) ledgerDay(t *testing.T, payment uuid.UUID) string {
	t.Helper()
	return e.sum(t, `SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') FROM finance_entries
		WHERE source_type = 'staff_payment' AND source_uuid = $1 AND reversal_of_id IS NULL`, payment)
}

func TestStaffPayments_PlannedPostedOnPaidOn(t *testing.T) {
	e := newDisputeEnv(t)
	// 2031-03-10 12:00 Istanbul (the dealer's default time zone).
	clock := &fakeClock{now: time.Date(2031, 3, 10, 9, 0, 0, 0, time.UTC)}
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true}).WithClock(clock.Now)
	write := caller(e.dealer, rbac.PermStaffPaymentsWrite)
	staff, err := svc.CreateStaffProfile(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.CreateStaffProfileInput{
		Name: "Planlı " + e.suffix, MonthlySalary: strPtr("5000"), Active: true,
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

	// Future paid_on: planned, no ledger row.
	planned, err := svc.CreateStaffPayment(e.ctx, write, staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentAdvance, Period: "2031-03", Amount: strPtr("400"), AccountUUID: &cash.UUID,
		PaidOn: day(2031, 3, 31),
	})
	if err != nil {
		t.Fatalf("planned: %v", err)
	}
	if planned.Status != acc.StaffPaymentPlanned || planned.FinanceEntryUUID != nil || planned.PaidOn != "2031-03-31" {
		t.Fatalf("planned payment = %+v", planned)
	}
	if n := e.ledgerRows(t, planned.UUID); n != 0 {
		t.Fatalf("planned payment ledger rows = %d, want 0", n)
	}

	// Today: booked at once, on today.
	today, err := svc.CreateStaffPayment(e.ctx, write, staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentBonus, Period: "2031-03", Amount: strPtr("250"), AccountUUID: &cash.UUID,
		PaidOn: day(2031, 3, 10),
	})
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	if today.Status != acc.StaffPaymentPosted || today.FinanceEntryUUID == nil {
		t.Fatalf("today payment = %+v", today)
	}
	if d := e.ledgerDay(t, today.UUID); d != "2031-03-10" {
		t.Fatalf("today ledger day = %s", d)
	}

	// Past paid_on (entered late): booked on paid_on, not on the entry day.
	past, err := svc.CreateStaffPayment(e.ctx, write, staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentSalary, Period: "2031-02", AccountUUID: &cash.UUID, PaidOn: day(2031, 2, 27),
	})
	if err != nil {
		t.Fatalf("past: %v", err)
	}
	if past.Status != acc.StaffPaymentPosted {
		t.Fatalf("past payment = %+v", past)
	}
	if d := e.ledgerDay(t, past.UUID); d != "2031-02-27" {
		t.Fatalf("past ledger day = %s, want paid_on", d)
	}

	// Not due yet: the job leaves it planned.
	if _, err := svc.PostDueStaffPayments(e.ctx); err != nil {
		t.Fatalf("job before due: %v", err)
	}
	if n := e.ledgerRows(t, planned.UUID); n != 0 {
		t.Fatalf("job booked a payment before its day: %d rows", n)
	}

	// 2031-03-30 21:30 UTC is 2031-03-31 00:30 in Istanbul: due.
	clock.now = time.Date(2031, 3, 30, 21, 30, 0, 0, time.UTC)
	if _, err := svc.PostDueStaffPayments(e.ctx); err != nil {
		t.Fatalf("job: %v", err)
	}
	if _, err := svc.PostDueStaffPayments(e.ctx); err != nil {
		t.Fatalf("job again: %v", err)
	}
	if n := e.ledgerRows(t, planned.UUID); n != 1 {
		t.Fatalf("ledger rows after two job runs = %d, want 1", n)
	}
	if d := e.ledgerDay(t, planned.UUID); d != "2031-03-31" {
		t.Fatalf("job ledger day = %s, want paid_on", d)
	}
	page, err := svc.ListStaffPayments(e.ctx, caller(e.dealer, rbac.PermStaffManage), staff.UUID, acc.StaffPaymentFilter{
		Statuses: []string{acc.StaffPaymentPosted}, Limit: 20,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Total != 3 || page.TotalAmount != "5650.00" {
		t.Fatalf("posted payments = %d / %s, want 3 / 5650.00", page.Total, page.TotalAmount)
	}

	// P&L and staff cost fall in the same month.
	read := caller(e.dealer, rbac.PermAccountingRead)
	for _, m := range []struct {
		from, to *time.Time
		want     string
	}{
		{day(2031, 2, 1), day(2031, 2, 28), "5000.00"},
		{day(2031, 3, 1), day(2031, 3, 31), "650.00"},
	} {
		period := acc.StatementPeriod{From: m.from, To: m.to}
		cost, err := svc.GetStaffCostReport(e.ctx, read, nil, period)
		if err != nil {
			t.Fatalf("staff cost: %v", err)
		}
		pnl, err := svc.GetPnlReport(e.ctx, read, nil, period, acc.PnlGroupMonth, i18n.LocaleTR)
		if err != nil {
			t.Fatalf("pnl: %v", err)
		}
		if cost.Totals.Total != m.want || pnl.Totals.Expense != m.want {
			t.Fatalf("%s: staff cost %s, P&L expense %s, want both %s",
				m.from.Format("2006-01"), cost.Totals.Total, pnl.Totals.Expense, m.want)
		}
	}
}

func TestStaffPayments_PlannedEditCancel(t *testing.T) {
	e := newDisputeEnv(t)
	clock := &fakeClock{now: time.Date(2032, 5, 2, 9, 0, 0, 0, time.UTC)}
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true}).WithClock(clock.Now)
	write := caller(e.dealer, rbac.PermStaffPaymentsWrite)
	staff, err := svc.CreateStaffProfile(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.CreateStaffProfileInput{
		Name: "İptal " + e.suffix, MonthlySalary: strPtr("3000"), Active: true,
	})
	if err != nil {
		t.Fatalf("staff: %v", err)
	}

	// Payroll with a future payment day: planned salaries.
	pay, err := svc.RunPayroll(e.ctx, write, "2032-05", day(2032, 5, 31))
	if err != nil {
		t.Fatalf("payroll: %v", err)
	}
	if pay.Created != 1 || pay.Items[0].Status != acc.StaffPaymentPlanned || pay.Items[0].PaidOn != "2032-05-31" {
		t.Fatalf("payroll = %+v", pay)
	}
	salary := pay.Items[0]

	edited, err := svc.UpdateStaffPayment(e.ctx, write, salary.UUID, acc.UpdateStaffPaymentInput{
		Amount: strPtr("3200"), Description: strPtr("Mayıs maaşı"),
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.Status != acc.StaffPaymentPlanned || edited.Amount != "3200.00" || edited.Description == nil {
		t.Fatalf("edited = %+v", edited)
	}

	upcoming, err := svc.ListBookStaffPayments(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.StaffPaymentFilter{
		Statuses: []string{acc.StaffPaymentPlanned}, Limit: 20,
	})
	if err != nil {
		t.Fatalf("upcoming: %v", err)
	}
	if upcoming.Total != 1 || upcoming.TotalAmount != "3200.00" || upcoming.Items[0].StaffName != staff.Name {
		t.Fatalf("upcoming = %+v", upcoming)
	}

	cancelled, err := svc.CancelStaffPayment(e.ctx, write, salary.UUID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != acc.StaffPaymentCancelled {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	if _, err := svc.CancelStaffPayment(e.ctx, write, salary.UUID); !errors.Is(err, acc.ErrStaffPaymentNotPlanned) {
		t.Fatalf("second cancel err = %v", err)
	}
	if _, err := svc.UpdateStaffPayment(e.ctx, write, salary.UUID, acc.UpdateStaffPaymentInput{Amount: strPtr("1")}); !errors.Is(err, acc.ErrStaffPaymentNotPlanned) {
		t.Fatalf("edit cancelled err = %v", err)
	}

	// Past the payment day the job books nothing for a cancelled payment.
	clock.now = time.Date(2032, 6, 2, 9, 0, 0, 0, time.UTC)
	if _, err := svc.PostDueStaffPayments(e.ctx); err != nil {
		t.Fatalf("job: %v", err)
	}
	if n := e.ledgerRows(t, salary.UUID); n != 0 {
		t.Fatalf("cancelled payment ledger rows = %d, want 0", n)
	}
	cost, err := svc.GetStaffCostReport(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil,
		acc.StatementPeriod{From: day(2032, 5, 1), To: day(2032, 5, 31)})
	if err != nil {
		t.Fatalf("staff cost: %v", err)
	}
	if cost.Totals.Total != "0.00" {
		t.Fatalf("staff cost after cancel = %s", cost.Totals.Total)
	}

	// The cancelled salary frees its period; a posted payment is not
	// edited or cancelled (reversal only).
	again, err := svc.RunPayroll(e.ctx, write, "2032-05", nil)
	if err != nil {
		t.Fatalf("payroll again: %v", err)
	}
	if again.Created != 1 || again.Items[0].Status != acc.StaffPaymentPosted {
		t.Fatalf("payroll again = %+v", again)
	}
	if _, err := svc.CancelStaffPayment(e.ctx, write, again.Items[0].UUID); !errors.Is(err, acc.ErrStaffPaymentNotPlanned) {
		t.Fatalf("cancel posted err = %v", err)
	}
	if _, err := svc.UpdateStaffPayment(e.ctx, write, again.Items[0].UUID, acc.UpdateStaffPaymentInput{Amount: strPtr("1")}); !errors.Is(err, acc.ErrStaffPaymentNotPlanned) {
		t.Fatalf("edit posted err = %v", err)
	}
}

// Moving a planned payment's day to today books it at once.
func TestStaffPayments_PlannedEditToTodayPosts(t *testing.T) {
	e := newDisputeEnv(t)
	clock := &fakeClock{now: time.Date(2033, 7, 1, 9, 0, 0, 0, time.UTC)}
	svc := dealerService(e, &moduleSwitch{dealerAccounting: true}).WithClock(clock.Now)
	write := caller(e.dealer, rbac.PermStaffPaymentsWrite)
	staff, err := svc.CreateStaffProfile(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.CreateStaffProfileInput{
		Name: "Öne " + e.suffix, Active: true,
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
	p, err := svc.CreateStaffPayment(e.ctx, write, staff.UUID, acc.StaffPaymentInput{
		Type: acc.StaffPaymentBonus, Period: "2033-07", Amount: strPtr("90"), AccountUUID: &cash.UUID,
		PaidOn: day(2033, 7, 20),
	})
	if err != nil || p.Status != acc.StaffPaymentPlanned {
		t.Fatalf("planned = %+v, %v", p, err)
	}
	moved, err := svc.UpdateStaffPayment(e.ctx, write, p.UUID, acc.UpdateStaffPaymentInput{PaidOn: day(2033, 7, 1)})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.Status != acc.StaffPaymentPosted || moved.FinanceEntryUUID == nil {
		t.Fatalf("moved = %+v", moved)
	}
	if d := e.ledgerDay(t, p.UUID); d != "2033-07-01" {
		t.Fatalf("moved ledger day = %s", d)
	}
}
