package usecase_test

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
)

// TEC-346 (F3-07f) acceptance at the use-case level. Fixture (one month,
// the current one, in the dealer's book): two service incomes, one product
// sale (plus a voided one), one external purchase, two salaries; plus the
// distributor's sale to the dealer and two customer cari rows for aging.

type reportFixture struct {
	e        *disputeEnv
	svc      *acc.Service
	cashID   int64
	cash     uuid.UUID
	period   acc.StatementPeriod
	productA db.Product
	productB db.Product
}

func (f *reportFixture) post(t *testing.T, income bool, entry posting.Entry) db.FinanceEntry {
	t.Helper()
	tx, err := f.e.pool.Begin(f.e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.e.ctx) }()
	if entry.OrganizationID == 0 {
		entry.OrganizationID = f.e.dealer.ID
	}
	if entry.Currency == "" {
		entry.Currency = "TRY"
	}
	var res posting.Result
	if income {
		res, err = f.e.poster.PostIncome(f.e.ctx, tx, entry)
	} else {
		res, err = f.e.poster.PostExpense(f.e.ctx, tx, entry)
	}
	if err != nil {
		t.Fatalf("post %s: %v", entry.Category, err)
	}
	if err := tx.Commit(f.e.ctx); err != nil {
		t.Fatal(err)
	}
	return res.Entry
}

func (f *reportFixture) void(t *testing.T, src posting.Source) {
	t.Helper()
	tx, err := f.e.pool.Begin(f.e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.e.ctx) }()
	if _, err := f.e.poster.VoidBySourceTx(f.e.ctx, tx, src, "iptal", nil); err != nil {
		t.Fatalf("void: %v", err)
	}
	if err := tx.Commit(f.e.ctx); err != nil {
		t.Fatal(err)
	}
}

func (f *reportFixture) exec(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := f.e.pool.QueryRow(f.e.ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
	return id
}

// service creates a draft dealer service and records its income (F3-07c).
func (f *reportFixture) service(t *testing.T, name, amount string) {
	t.Helper()
	e := f.e
	user := e.customer(t, "svc-"+name)
	carBrand := f.exec(t, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "t346-"+name+"-"+e.suffix)
	carModel := f.exec(t, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`, carBrand, "t346-"+name)
	veh, err := e.q.CreateVehicle(e.ctx, db.CreateVehicleParams{
		UserID: user.ID, BrandID: e.dealer.BrandID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	svc, err := e.q.CreateService(e.ctx, db.CreateServiceParams{
		ServiceNo: "R" + name + e.suffix[len(e.suffix)-12:], OrganizationID: e.dealer.ID, BrandID: e.dealer.BrandID,
		CustomerUserID: user.ID, VehicleID: veh.ID, CarBrandID: carBrand, CarModelID: carModel, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	row := f.post(t, true, posting.Entry{
		Source: posting.Source{Type: "service_income", UUID: svc.Uuid}, Category: "service_income",
		Amount: amount, AccountID: f.cashID, Description: "Hizmet " + name,
	})
	var n pgtype.Numeric
	_ = n.Scan(amount)
	if _, err := e.q.SetServiceIncome(e.ctx, db.SetServiceIncomeParams{
		ID: svc.ID, BrandID: svc.BrandID, IncomeEntryID: pgtype.Int8{Int64: row.ID, Valid: true}, IncomeAmount: n,
	}); err != nil {
		t.Fatalf("set income: %v", err)
	}
}

// productSale writes a quick sale row (F3-07d snapshot) and its income.
func (f *reportFixture) productSale(t *testing.T, p db.Product, qty, unitPrice, cost, total string) posting.Source {
	t.Helper()
	e := f.e
	src := posting.Source{Type: "product_sale", UUID: uuid.New()}
	row := f.post(t, true, posting.Entry{
		Source: src, Category: "product_sale", Amount: total, AccountID: f.cashID, Description: "Product sale",
	})
	saleID := f.exec(t, `INSERT INTO product_sales (uuid, organization_id, brand_id, payment_method, currency, total, finance_entry_id)
		VALUES ($1, $2, $3, 'cash', 'TRY', $4, $5) RETURNING id`, src.UUID, e.dealer.ID, e.dealer.BrandID, total, row.ID)
	f.exec(t, `INSERT INTO product_sale_lines (sale_id, organization_id, brand_id, product_id, quantity, unit_price, line_total, purchase_unit_cost)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		saleID, e.dealer.ID, e.dealer.BrandID, p.ID, qty, unitPrice, total, cost)
	return src
}

func newReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	e := newDisputeEnv(t)
	f := &reportFixture{e: e, svc: dealerService(e, &moduleSwitch{dealerAccounting: true})}
	cash, err := f.svc.CreateAccount(e.ctx, caller(e.dealer, rbac.PermAccountingWrite),
		acc.CreateAccountInput{Type: acc.AccountCash, Name: "Kasa " + e.suffix})
	if err != nil {
		t.Fatalf("cash: %v", err)
	}
	f.cash = cash.UUID
	f.cashID = f.exec(t, `SELECT id FROM finance_accounts WHERE uuid = $1`, cash.UUID)
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	f.period = acc.StatementPeriod{From: &first, To: &today}

	center, err := e.q.GetBrandCenter(e.ctx, e.brand)
	if err != nil {
		t.Fatal(err)
	}
	catID := f.exec(t, `INSERT INTO product_categories (organization_id, brand_id, name) VALUES ($1, $2, $3) RETURNING id`,
		center.ID, e.brand, "t346-"+e.suffix)
	for i, p := range []*db.Product{&f.productA, &f.productB} {
		id := f.exec(t, `INSERT INTO products (organization_id, brand_id, category_id, sku, name) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			center.ID, e.brand, catID, "T346-"+strconv.Itoa(i)+"-"+e.suffix, "Ürün "+strconv.Itoa(i))
		if *p, err = e.q.GetProduct(e.ctx, db.GetProductParams{ID: id, BrandID: e.brand}); err != nil {
			t.Fatalf("product: %v", err)
		}
	}

	// Two service incomes, one product sale (2 x 1500, cost 900 each) and a
	// voided sale of product B, one external purchase, two salaries.
	f.service(t, "a", "15000")
	f.service(t, "b", "5000")
	f.productSale(t, f.productA, "2", "1500", "900", "3000")
	f.void(t, f.productSale(t, f.productB, "1", "800", "500", "800"))
	f.post(t, false, posting.Entry{
		Source: posting.Source{Type: "purchase_external", UUID: uuid.New()}, Category: "purchase_external",
		Amount: "2000", AccountID: f.cashID, Description: "External purchase",
	})
	period := now.Format("2006-01")
	for _, s := range []struct{ name, amount string }{{"Usta", "4000"}, {"Çırak", "3500"}} {
		st, err := f.svc.CreateStaffProfile(e.ctx, caller(e.dealer, rbac.PermStaffManage), acc.CreateStaffProfileInput{
			Name: s.name + " " + e.suffix, MonthlySalary: strPtr(s.amount), Active: true,
		})
		if err != nil {
			t.Fatalf("staff: %v", err)
		}
		if _, err := f.svc.CreateStaffPayment(e.ctx, caller(e.dealer, rbac.PermStaffPaymentsWrite), st.UUID, acc.StaffPaymentInput{
			Type: acc.StaffPaymentSalary, Period: period, AccountUUID: &f.cash, PaidOn: &today,
		}); err != nil {
			t.Fatalf("salary: %v", err)
		}
	}
	// The distributor's sale to the dealer: a purchase expense on the
	// dealer's cari with the distributor (payable).
	e.sale(t, "1200.00")
	return f
}

func TestDealerReports_Pnl(t *testing.T) {
	f := newReportFixture(t)
	e := f.e
	c := caller(e.dealer, rbac.PermAccountingRead)

	rep, err := f.svc.GetPnlReport(e.ctx, c, nil, f.period, "", i18n.LocaleTR)
	if err != nil {
		t.Fatalf("pnl: %v", err)
	}
	// income 15000 + 5000 + 3000 (the voided sale nets out); expense 2000 +
	// 4000 + 3500 + 1200 (distributor sale).
	if rep.Group != acc.PnlGroupMonth || len(rep.Lines) != 1 || rep.Currency != "TRY" ||
		rep.Totals.Income != "23000.00" || rep.Totals.Expense != "10700.00" || rep.Totals.Net != "12300.00" {
		t.Fatalf("pnl by month = %+v", rep)
	}
	if l := rep.Lines[0]; l.Key != time.Now().UTC().Format("2006-01") || l.Income != "23000.00" || l.Net != "12300.00" {
		t.Fatalf("pnl month line = %+v", l)
	}

	byCat, err := f.svc.GetPnlReport(e.ctx, c, nil, f.period, acc.PnlGroupCategory, i18n.LocaleEN)
	if err != nil {
		t.Fatalf("pnl by category: %v", err)
	}
	want := []struct{ key, income, expense string }{
		{"service_income", "20000.00", "0.00"},
		{"product_sale", "3000.00", "0.00"},
		{"salary", "0.00", "7500.00"},
		{"purchase_external", "0.00", "2000.00"},
		{"purchase", "0.00", "1200.00"},
	}
	if len(byCat.Lines) != len(want) {
		t.Fatalf("pnl by category lines = %+v", byCat.Lines)
	}
	for i, w := range want {
		l := byCat.Lines[i]
		if l.Key != w.key || l.Income != w.income || l.Expense != w.expense {
			t.Fatalf("category line %d = %+v, want %+v", i, l, w)
		}
	}
	if byCat.Lines[0].Label != "Service income" || byCat.Totals != rep.Totals {
		t.Fatalf("category labels/totals = %+v / %+v", byCat.Lines[0], byCat.Totals)
	}

	// A period without rows is empty.
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	empty, err := f.svc.GetPnlReport(e.ctx, c, nil, acc.StatementPeriod{From: &old, To: &old}, "", i18n.LocaleTR)
	if err != nil || len(empty.Lines) != 0 || empty.Totals.Net != "0.00" {
		t.Fatalf("empty pnl = %+v, %v", empty, err)
	}
	var ve *acc.ValidationError
	if _, err := f.svc.GetPnlReport(e.ctx, c, nil, f.period, "week", i18n.LocaleTR); !errors.As(err, &ve) || ve.Field != "group" {
		t.Fatalf("bad group err = %v", err)
	}

	// The distributor reads its own P&L (its sale to the dealer) ...
	dist, err := f.svc.GetPnlReport(e.ctx, caller(e.dist, rbac.PermAccountingRead), nil, f.period, "", i18n.LocaleTR)
	if err != nil || dist.Totals.Income != "1200.00" || dist.Organization.UUID != e.dist.Uuid {
		t.Fatalf("distributor own pnl = %+v, %v", dist, err)
	}
	// ... but never the dealer's: every report reads as not found.
	distC := caller(e.dist, rbac.PermAccountingRead)
	if _, err := f.svc.GetPnlReport(e.ctx, distC, &e.dealer.Uuid, f.period, "", i18n.LocaleTR); !errors.Is(err, acc.ErrBookNotFound) {
		t.Fatalf("distributor reads dealer pnl err = %v, want ErrBookNotFound", err)
	}
	if _, err := f.svc.GetMarginReport(e.ctx, distC, &e.dealer.Uuid, f.period, true); !errors.Is(err, acc.ErrBookNotFound) {
		t.Fatalf("distributor reads dealer margin err = %v", err)
	}
	if _, err := f.svc.GetCariAgingReport(e.ctx, distC, &e.dealer.Uuid, nil); !errors.Is(err, acc.ErrBookNotFound) {
		t.Fatalf("distributor reads dealer aging err = %v", err)
	}
	if _, err := f.svc.GetStaffCostReport(e.ctx, distC, &e.dealer.Uuid, f.period); !errors.Is(err, acc.ErrBookNotFound) {
		t.Fatalf("distributor reads dealer staff cost err = %v", err)
	}
	if _, err := f.svc.ResolveOwnBook(e.ctx, distC, &e.dealer.Uuid); !errors.Is(err, acc.ErrBookNotFound) {
		t.Fatalf("distributor export of dealer book err = %v", err)
	}

	// XLSX export rows equal the report (adapter path of the export job).
	ds, err := acc.NewPnlAdapter(f.svc).Export(e.ctx, ioengine.ExportQuery{
		ioengine.QueryOrganizationID: strconv.FormatInt(e.dealer.ID, 10),
		acc.QueryFrom:                f.period.From.Format(time.DateOnly),
		acc.QueryTo:                  f.period.To.Format(time.DateOnly),
		acc.QueryGroup:               acc.PnlGroupCategory,
	}, i18n.LocaleEN)
	if err != nil {
		t.Fatalf("pnl export: %v", err)
	}
	got := xlsxTable(t, ds, "Category")
	if len(got) != len(byCat.Lines)+1 {
		t.Fatalf("xlsx rows = %d, report lines = %d (+ totals)", len(got), len(byCat.Lines))
	}
	for i, l := range byCat.Lines {
		if w := []string{l.Label, l.Income, l.Expense, l.Net}; !equalRow(got[i], w) {
			t.Fatalf("xlsx row %d = %v, want %v", i, got[i], w)
		}
	}
	if w := []string{"Total", rep.Totals.Income, rep.Totals.Expense, rep.Totals.Net}; !equalRow(got[len(got)-1], w) {
		t.Fatalf("xlsx totals = %v, want %v", got[len(got)-1], w)
	}
}

func TestDealerReports_Margin(t *testing.T) {
	f := newReportFixture(t)
	e := f.e
	c := caller(e.dealer, rbac.PermAccountingRead)

	rep, err := f.svc.GetMarginReport(e.ctx, c, nil, f.period, true)
	if err != nil {
		t.Fatalf("margin: %v", err)
	}
	if rep.Services.ServiceCount != 2 || rep.Services.Revenue != "20000.00" || *rep.Services.Cost != "0.00" {
		t.Fatalf("service margin = %+v", rep.Services)
	}
	// Product A only (the sale of product B was voided): 2 x 1500, cost 2 x
	// 900, profit 1200, margin 40%.
	if len(rep.Products) != 1 {
		t.Fatalf("products = %+v", rep.Products)
	}
	p := rep.Products[0]
	if p.ProductUUID != f.productA.Uuid || p.Quantity != "2.00" || p.SaleCount != 1 || p.Revenue != "3000.00" ||
		*p.Cost != "1800.00" || *p.GrossProfit != "1200.00" || *p.MarginPct != "40.00" || *p.CostIncomplete {
		t.Fatalf("product margin = %+v", p)
	}
	// Total: revenue 23000, cost 1800, profit 21200, margin 92.17%.
	if rep.Total.Revenue != "23000.00" || *rep.Total.Cost != "1800.00" || *rep.Total.GrossProfit != "21200.00" ||
		*rep.Total.MarginPct != "92.17" || rep.ProductSales.Revenue != "3000.00" {
		t.Fatalf("total margin = %+v", rep.Total)
	}

	// Without pricing.purchase.read the cost side is hidden.
	hidden, err := f.svc.GetMarginReport(e.ctx, c, nil, f.period, false)
	if err != nil || hidden.CostVisible || hidden.Total.Cost != nil || hidden.Products[0].GrossProfit != nil ||
		hidden.Products[0].CostIncomplete != nil || hidden.Total.Revenue != "23000.00" {
		t.Fatalf("margin without cost = %+v, %v", hidden, err)
	}

	// Export: one row per report line (services + products); the cost
	// columns are dropped without the grant and kept with it.
	q := ioengine.ExportQuery{
		ioengine.QueryOrganizationID: strconv.FormatInt(e.dealer.ID, 10),
		acc.QueryFrom:                f.period.From.Format(time.DateOnly),
		acc.QueryTo:                  f.period.To.Format(time.DateOnly),
	}
	ds, err := acc.NewMarginAdapter(f.svc).Export(e.ctx, q, i18n.LocaleEN)
	if err != nil {
		t.Fatal(err)
	}
	ds = ioengine.ApplyColumnVisibility(ds, ioengine.GrantedPermissions(q))
	if len(ds.Columns) != 5 || len(ds.Rows) != 1+len(rep.Products) {
		t.Fatalf("margin dataset without grant = %+v", ds.Columns)
	}
	ioengine.SetGrantedPermissions(q, []string{rbac.PermPricingPurchaseRead})
	ds, err = acc.NewMarginAdapter(f.svc).Export(e.ctx, q, i18n.LocaleEN)
	if err != nil {
		t.Fatal(err)
	}
	ds = ioengine.ApplyColumnVisibility(ds, ioengine.GrantedPermissions(q))
	rows := xlsxTable(t, ds, "Type")
	if len(rows) != 1+len(rep.Products)+1 {
		t.Fatalf("margin xlsx rows = %v", rows)
	}
	if w := []string{"Product sale", p.Name, p.SKU, "2.00", "3000.00", "1800.00", "1200.00", "40.00"}; !equalRow(rows[1], w) {
		t.Fatalf("margin xlsx product row = %v, want %v", rows[1], w)
	}
}

func TestDealerReports_CariAgingAndStaffCost(t *testing.T) {
	f := newReportFixture(t)
	e := f.e
	w := caller(e.dealer, rbac.PermAccountingWrite)
	cust := e.customer(t, "aging")
	e.serve(t, e.dealer, cust)
	cari, _, err := f.svc.OpenCustomerCari(e.ctx, w, acc.OpenCariInput{CounterpartyType: acc.CounterpartyTypeUser, CounterpartyUUID: &cust.Uuid})
	if err != nil {
		t.Fatalf("cari: %v", err)
	}
	cariID := f.exec(t, `SELECT id FROM cari_accounts WHERE uuid = $1`, cari.UUID)
	today := *f.period.To
	// A 45-day-old receivable of 1000 and a fresh charge of 500.
	f.post(t, true, posting.Entry{
		Source: posting.Source{Type: "service_income", UUID: uuid.New()}, Category: "service_income",
		Amount: "1000", CariID: cariID, Description: "Eski hizmet", PostedAt: today.AddDate(0, 0, -45).Add(10 * time.Hour),
	})
	if _, _, err := f.svc.CreateEntry(e.ctx, w, acc.EntryInput{
		Direction: "charge", Category: "adjustment", Amount: "500", CariUUID: &cari.UUID,
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}

	rep, err := f.svc.GetCariAgingReport(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, &today)
	if err != nil {
		t.Fatalf("aging: %v", err)
	}
	if len(rep.Lines) != 2 {
		t.Fatalf("aging lines = %+v", rep.Lines)
	}
	recv, pay := rep.Lines[0], rep.Lines[1]
	if recv.Side != acc.AgingReceivable || recv.CariUUID != cari.UUID || recv.Balance != "1500.00" ||
		recv.Buckets != (acc.AgingBuckets{Days0To30: "500.00", Days31To60: "1000.00", Days61To90: "0.00", Days90Plus: "0.00"}) {
		t.Fatalf("receivable aging = %+v", recv)
	}
	if pay.Side != acc.AgingPayable || pay.Counterparty.Type != "organization" || pay.Balance != "1200.00" ||
		pay.Buckets.Days0To30 != "1200.00" {
		t.Fatalf("payable aging = %+v", pay)
	}
	if rep.Totals.Receivable.Balance != "1500.00" || rep.Totals.Payable.Balance != "1200.00" ||
		rep.Totals.Receivable.Buckets.Days31To60 != "1000.00" {
		t.Fatalf("aging totals = %+v", rep.Totals)
	}
	// A partial collection settles the oldest row first (FIFO).
	if _, _, err := f.svc.Settle(e.ctx, w, "collection", acc.SettlementInput{
		AccountUUID: &f.cash, CariUUID: &cari.UUID, Amount: "600",
	}); err != nil {
		t.Fatalf("collection: %v", err)
	}
	rep, err = f.svc.GetCariAgingReport(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, &today)
	if err != nil || rep.Lines[0].Balance != "900.00" || rep.Lines[0].Buckets.Days0To30 != "500.00" ||
		rep.Lines[0].Buckets.Days31To60 != "400.00" {
		t.Fatalf("aging after collection = %+v, %v", rep.Lines, err)
	}
	ds := acc.CariAgingDataset(rep, i18n.LocaleEN)
	if got := xlsxTable(t, ds, "Side"); len(got) != len(rep.Lines) {
		t.Fatalf("aging xlsx rows = %d, report lines = %d", len(got), len(rep.Lines))
	}

	staff, err := f.svc.GetStaffCostReport(e.ctx, caller(e.dealer, rbac.PermAccountingRead), nil, f.period)
	if err != nil {
		t.Fatalf("staff cost: %v", err)
	}
	if len(staff.Lines) != 2 || staff.Totals.Salary != "7500.00" || staff.Totals.Total != "7500.00" || staff.Totals.Advance != "0.00" {
		t.Fatalf("staff cost = %+v", staff)
	}
	for _, l := range staff.Lines {
		if l.PaymentCount != 1 || l.Salary != l.Total || (l.Total != "4000.00" && l.Total != "3500.00") {
			t.Fatalf("staff line = %+v", l)
		}
	}
	got := xlsxTable(t, acc.StaffCostDataset(staff, i18n.LocaleEN), "Name")
	if len(got) != len(staff.Lines)+1 || got[0][0] != staff.Lines[0].Name || got[0][2] != staff.Lines[0].Salary {
		t.Fatalf("staff xlsx = %v", got)
	}
}

// xlsxTable encodes ds as XLSX (no letterhead) and returns the cells under
// the header row whose first cell is header.
func xlsxTable(t *testing.T, ds ioengine.Dataset, header string) [][]string {
	t.Helper()
	data, err := ioengine.EncodeExport(ioengine.ExportXLSX, ds, "en", nil, "report")
	if err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = x.Close() }()
	rows, err := x.GetRows(x.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rows {
		if len(r) > 0 && r[0] == header {
			return rows[i+1:]
		}
	}
	t.Fatalf("header %q not found in %v", header, rows)
	return nil
}

func equalRow(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
