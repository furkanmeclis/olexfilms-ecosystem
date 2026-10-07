package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-337 acceptance: a completed re-application service books the claim
// through accounting/posting (no direct ledger writes). Everything runs in
// one rolled-back transaction, like the TEC-336 fixture.

// Center purchase price, center→distributor price and distributor→dealer
// price of the fixture product (brand currency TRY, one serial piece).
const (
	accCenterCost  = "100.00"
	accDistPrice   = "150.00"
	accDealerPrice = "200.00"
	accLabor       = "300.00"
)

type fakeSettings map[string]any

func (f fakeSettings) String(_ context.Context, key string) string {
	if v, ok := f[key].(string); ok {
		return v
	}
	d, _ := sysconfig.Lookup(key)
	s, _ := d.Default.(string)
	return s
}

func (f fakeSettings) Int(_ context.Context, key string) int64 {
	if v, ok := f[key].(int64); ok {
		return v
	}
	d, _ := sysconfig.Lookup(key)
	n, _ := d.Default.(int64)
	return n
}

type accFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	bus      events.Bus
	svc      *Service
	brand    db.Brand
	center   db.Organization
	dist     db.Organization
	dealer   db.Organization
	user     db.User
	settings fakeSettings
	today    time.Time
}

// newAccFixture builds center (TRY) → distributor → dealer with the given
// currencies, the F1 prices of the fixture product and the today rates
// TRY→EUR 0.025 and TRY→UAH 1.25.
func newAccFixture(t *testing.T, distCur, dealerCur string) *accFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	f := &accFixture{ctx: ctx, tx: tx, q: q, settings: fakeSettings{}}
	now := time.Now().UTC()
	f.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if f.brand, err = q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatalf("brand: %v", err)
	}
	if f.center, err = q.GetBrandCenter(ctx, f.brand.ID); err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	org := func(name, typ string, parent int64, cur string) db.Organization {
		o, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: "tec337-" + name + "-" + suffix, Name: "TEC337 " + name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
			BrandID: f.brand.ID, Currency: cur, Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("org %s: %v", name, err)
		}
		return o
	}
	f.dist = org("dist", "distributor", f.center.ID, distCur)
	f.dealer = org("dealer", "dealer", f.dist.ID, dealerCur)
	if f.user, err = q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC337", Surname: "Customer", Status: "active",
		Email: pgtype.Text{String: "tec337-" + suffix + "@example.test", Valid: true},
	}); err != nil {
		t.Fatalf("user: %v", err)
	}
	for _, r := range []struct{ quote, rate string }{{"EUR", "0.025"}, {"UAH", "1.25"}} {
		if err := q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: f.today, Valid: true}, Base: "TRY", Quote: r.quote,
			Rate: r.rate, Source: "manual", Note: pgtype.Text{String: "TEC-337 fixture", Valid: true},
		}); err != nil {
			t.Fatalf("rate fixture: %v", err)
		}
	}
	mem := outbox.NewMemory()
	f.bus = events.NewBus(slog.Default())
	f.svc = NewWithStore(txStore{Queries: q, pool: tx}, nil, mem)
	RegisterEventHandlers(f.bus, tx, q, mem, slog.Default())
	poster := posting.New(q, mem, fxrates.New(q, nil, nil))
	RegisterAccountingHandlers(f.bus, tx, poster, f.settings, slog.Default())
	return f
}

// claim opens an approved claim on a dealer service of a fresh product with
// its F1 prices; publish approves it into a re-application service.
func (f *accFixture) claim(t *testing.T) (db.WarrantyClaim, db.Product, db.Unit) {
	t.Helper()
	q, ctx := f.q, f.ctx
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var carBrand, carModel int64
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "TEC337 "+suffix).Scan(&carBrand); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`, carBrand, "Model").Scan(&carModel); err != nil {
		t.Fatalf("car model: %v", err)
	}
	vehicle, err := q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: f.user.ID, BrandID: f.brand.ID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
		Vin: pgtype.Text{String: "1HGCM82633A004352", Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, Name: "tec337-cat-" + suffix,
		AvailableParts: []byte(`["body_kaput"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, CategoryID: cat.ID,
		Sku: "tec337-" + suffix, Name: "TEC337 Film", Images: []byte("[]"), UnitType: "piece", Active: true,
		WarrantyDurationMonths: pgtype.Int4{Int32: 12, Valid: true},
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	if _, err := q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: product.ID, BrandID: f.brand.ID, Currency: "TRY",
		PurchasePrice:          pgtype.Text{String: accCenterCost, Valid: true},
		SaleToDistributorPrice: pgtype.Text{String: accDistPrice, Valid: true},
	}); err != nil {
		t.Fatalf("list price: %v", err)
	}
	if _, err := q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: product.ID, BrandID: f.brand.ID, DistributorOrgID: f.dist.ID, Currency: "TRY", Price: accDealerPrice,
	}); err != nil {
		t.Fatalf("dealer price: %v", err)
	}
	unit, err := q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, ProductID: product.ID,
		Barcode: "TEC337-" + suffix, UnitKind: "serial", Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	service, err := q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo: "T337-" + suffix[len(suffix)-8:], OrganizationID: f.dealer.ID, BrandID: f.brand.ID,
		CustomerUserID: f.user.ID, VehicleID: vehicle.ID, CarBrandID: carBrand, CarModelID: carModel,
		Vin: pgtype.Text{String: "1HGCM82633A004352", Valid: true}, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	item, err := q.CreateServiceItem(ctx, db.CreateServiceItemParams{
		ServiceID: service.ID, ProductID: product.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`["body_kaput"]`),
	})
	if err != nil {
		t.Fatalf("service item: %v", err)
	}
	if _, err := q.CompleteService(ctx, db.CompleteServiceParams{ID: service.ID}); err != nil {
		t.Fatalf("complete original service: %v", err)
	}
	warranty, err := q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: f.user.ID,
		StartAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Now().AddDate(1, 0, 0), Valid: true},
	})
	if err != nil {
		t.Fatalf("warranty: %v", err)
	}
	claim, err := q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, WarrantyID: warranty.ID, ServiceID: service.ID,
		VehicleID: vehicle.ID, CustomerUserID: f.user.ID, Description: "Film kalkti",
		Status: StatusOpen, CoverageCheck: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := q.AddWarrantyClaimPart(ctx, db.AddWarrantyClaimPartParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		PartKey: "body_kaput", ServiceItemID: int8(item.ID), ProductID: int8(product.ID), UnitID: int8(unit.ID),
	}); err != nil {
		t.Fatalf("claim part: %v", err)
	}
	approved, err := q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
		ID: claim.ID, BrandID: claim.BrandID, FromStatus: StatusOpen, Status: StatusApproved,
	})
	if err != nil {
		t.Fatalf("approve claim: %v", err)
	}
	ev := events.New(events.WarrantyClaimStatusChanged).WithTenant(approved.OrganizationID).
		WithEntity("warranty_claim", &approved.ID, &approved.Uuid).
		WithPayload(map[string]any{"brand_id": approved.BrandID, "from": StatusCenterReview, "to": StatusApproved})
	if err := f.bus.Publish(ctx, ev); err != nil {
		t.Fatalf("publish approved: %v", err)
	}
	return approved, product, unit
}

// completeReapply completes the claim's re-application service and
// publishes service.completed `times` times.
func (f *accFixture) completeReapply(t *testing.T, claim db.WarrantyClaim, times int) db.Service {
	t.Helper()
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil || !got.ReapplyServiceID.Valid {
		t.Fatalf("claim has no reapply service: %+v %v", got, err)
	}
	done, err := f.q.CompleteService(f.ctx, db.CompleteServiceParams{ID: got.ReapplyServiceID.Int64})
	if err != nil {
		t.Fatalf("complete reapply service: %v", err)
	}
	for i := 0; i < times; i++ {
		ev := events.New(events.ServiceCompleted).WithTenant(done.OrganizationID).
			WithEntity("service", &done.ID, &done.Uuid).
			WithPayload(map[string]any{"service_id": done.ID, "brand_id": done.BrandID})
		if err := f.bus.Publish(f.ctx, ev); err != nil {
			t.Fatalf("publish completed: %v", err)
		}
	}
	return done
}

type accRow struct {
	org         int64
	role        string
	direction   string
	category    string
	origAmount  string
	origCur     string
	amount      string
	currency    string
	rate        string
	rateDate    time.Time
	cariCounter int64 // counterparty organization of the cari (0: none)
	hasAccount  bool
}

func (f *accFixture) rows(t *testing.T, claim db.WarrantyClaim) map[string]accRow {
	t.Helper()
	rs, err := f.tx.Query(f.ctx, `
SELECT e.organization_id, e.role, e.direction, e.category, e.orig_amount::text, e.orig_currency,
       e.amount::text, e.currency, e.rate::text, e.rate_date, COALESCE(c.counterparty_org_id, 0), e.account_id IS NOT NULL
FROM finance_entries e
LEFT JOIN cari_accounts c ON c.id = e.cari_id
WHERE e.source_type = $1 AND e.source_uuid = $2
ORDER BY e.id`, AccountingSourceType, claim.Uuid)
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	defer rs.Close()
	out := map[string]accRow{}
	for rs.Next() {
		var r accRow
		var day pgtype.Date
		if err := rs.Scan(&r.org, &r.role, &r.direction, &r.category, &r.origAmount, &r.origCur,
			&r.amount, &r.currency, &r.rate, &day, &r.cariCounter, &r.hasAccount); err != nil {
			t.Fatalf("scan: %v", err)
		}
		r.rateDate = day.Time
		key := fmt.Sprintf("%d/%s", r.org, r.role)
		if _, dup := out[key]; dup {
			t.Fatalf("duplicate row %s", key)
		}
		out[key] = r
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func (f *accFixture) key(o db.Organization, role string) string {
	return fmt.Sprintf("%d/%s", o.ID, role)
}

func wantRow(t *testing.T, rows map[string]accRow, key, direction, category, orig, amount string, counter int64) accRow {
	t.Helper()
	r, ok := rows[key]
	if !ok {
		t.Fatalf("missing row %s in %+v", key, rows)
	}
	if r.direction != direction || r.category != category || trim2(r.origAmount) != orig ||
		trim2(r.amount) != amount || r.cariCounter != counter || r.hasAccount {
		t.Fatalf("row %s = %+v; want %s/%s orig %s amount %s cari %d", key, r, direction, category, orig, amount, counter)
	}
	return r
}

func trim2(s string) string {
	var r pgtype.Numeric
	if err := r.Scan(s); err != nil {
		return s
	}
	return posting.FormatNumeric(r)
}

// Product chain rows of the TRY fixture: center cost, center→distributor
// refund at the distributor price, distributor→dealer refund at the dealer
// price.
func assertProductRows(t *testing.T, f *accFixture, rows map[string]accRow) {
	t.Helper()
	wantRow(t, rows, f.key(f.center, RoleWarrantyCost), accounting.DirectionExpense, accounting.CategoryWarrantyCost, accCenterCost, accCenterCost, 0)
	wantRow(t, rows, f.key(f.center, posting.RoleReturnIn), accounting.DirectionExpense, accounting.CategoryPurchase, accDistPrice, accDistPrice, f.dist.ID)
	wantRow(t, rows, f.key(f.dist, posting.RoleReturnOut), accounting.DirectionIncome, accounting.CategorySale, accDistPrice, accDistPrice, f.center.ID)
	wantRow(t, rows, f.key(f.dist, posting.RoleReturnIn), accounting.DirectionExpense, accounting.CategoryPurchase, accDealerPrice, accDealerPrice, f.dealer.ID)
	wantRow(t, rows, f.key(f.dealer, posting.RoleReturnOut), accounting.DirectionIncome, accounting.CategorySale, accDealerPrice, accDealerPrice, f.dist.ID)
}

func assertLaborRows(t *testing.T, f *accFixture, rows map[string]accRow, amount string) {
	t.Helper()
	wantRow(t, rows, f.key(f.center, RoleLaborOut), accounting.DirectionExpense, accounting.CategoryWarrantyLabor, amount, amount, f.dist.ID)
	wantRow(t, rows, f.key(f.dist, RoleLaborIn), accounting.DirectionIncome, accounting.CategoryWarrantyLaborIncome, amount, amount, f.center.ID)
	wantRow(t, rows, f.key(f.dist, RoleLaborOut), accounting.DirectionExpense, accounting.CategoryWarrantyLabor, amount, amount, f.dealer.ID)
	wantRow(t, rows, f.key(f.dealer, RoleLaborIn), accounting.DirectionIncome, accounting.CategoryWarrantyLaborIncome, amount, amount, f.dist.ID)
}

func TestWarrantyAccountingDealerRule(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	claim, _, _ := f.claim(t)
	f.completeReapply(t, claim, 1)

	rows := f.rows(t, claim)
	if len(rows) != 5 {
		t.Fatalf("rows = %d (%+v), want 5 (cost + two refund hops, no labor)", len(rows), rows)
	}
	assertProductRows(t, f, rows)
	if _, ok := rows[f.key(f.center, RoleLaborOut)]; ok {
		t.Fatal("dealer rule wrote a labor row")
	}
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil || got.Status != StatusClosed {
		t.Fatalf("claim = %+v, %v; want closed", got, err)
	}

	// Detail cost summary: center caller with accounting.read only.
	center := Caller{
		UserID: 1, OrganizationID: f.center.ID, BrandID: f.brand.ID, OrgType: "center",
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand},
		Permissions: map[string]rbac.Scope{
			rbac.PermWarrantyClaimsRead: rbac.ScopeBrand, rbac.PermAccountingRead: rbac.ScopeBrand,
		},
	}
	v, err := f.svc.Get(f.ctx, center, claim.Uuid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v.CostSummary == nil || v.CostSummary.ProductCost != accCenterCost || v.CostSummary.Labor != "0.00" || v.CostSummary.Currency != "TRY" {
		t.Fatalf("cost summary = %+v", v.CostSummary)
	}
	dealer := Caller{
		UserID: 1, OrganizationID: f.dealer.ID, BrandID: f.brand.ID, OrgType: "dealer",
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{f.dealer.ID}},
		Permissions: map[string]rbac.Scope{
			rbac.PermWarrantyClaimsRead: rbac.ScopeManaged, rbac.PermAccountingRead: rbac.ScopeManaged,
		},
	}
	if v, err := f.svc.Get(f.ctx, dealer, claim.Uuid); err != nil || v.CostSummary != nil {
		t.Fatalf("dealer detail = %+v, %v; want no cost summary", v.CostSummary, err)
	}
}

func TestWarrantyAccountingCenterRule(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	f.settings[sysconfig.KeyWarrantyClaimsLaborRule] = sysconfig.LaborRuleCenter
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = accLabor
	claim, _, _ := f.claim(t)
	f.completeReapply(t, claim, 1)

	rows := f.rows(t, claim)
	if len(rows) != 9 {
		t.Fatalf("rows = %d (%+v), want 9", len(rows), rows)
	}
	assertProductRows(t, f, rows)
	assertLaborRows(t, f, rows, accLabor)
}

func TestWarrantyAccountingSharedRule(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	f.settings[sysconfig.KeyWarrantyClaimsLaborRule] = sysconfig.LaborRuleShared
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = accLabor
	f.settings[sysconfig.KeyWarrantyClaimsLaborSharePercent] = int64(50)
	claim, _, _ := f.claim(t)
	f.completeReapply(t, claim, 1)

	rows := f.rows(t, claim)
	if len(rows) != 9 {
		t.Fatalf("rows = %d (%+v), want 9", len(rows), rows)
	}
	assertProductRows(t, f, rows)
	assertLaborRows(t, f, rows, "150.00")
}

func TestWarrantyAccountingSecondEventWritesNothing(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	f.settings[sysconfig.KeyWarrantyClaimsLaborRule] = sysconfig.LaborRuleCenter
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = accLabor
	claim, _, _ := f.claim(t)
	done := f.completeReapply(t, claim, 2)
	if rows := f.rows(t, claim); len(rows) != 9 {
		t.Fatalf("rows after two events = %d, want 9", len(rows))
	}
	// A direct third call, even with another labor setting, is skipped.
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = "999.00"
	acc := NewAccounting(f.tx, posting.New(f.q, nil, fxrates.New(f.q, nil, nil)), f.settings, nil)
	res, err := acc.PostReapplyService(f.ctx, done.ID, done.BrandID, nil)
	if err != nil || !res.Skipped {
		t.Fatalf("third post = %+v, %v; want skipped", res, err)
	}
	if rows := f.rows(t, claim); len(rows) != 9 {
		t.Fatalf("rows after third call = %d, want 9", len(rows))
	}
}

// EUR distributor / UAH dealer: every row is converted at the posting day's
// rate and keeps it (K7).
func TestWarrantyAccountingFrozenRatesEURUAH(t *testing.T) {
	f := newAccFixture(t, "EUR", "UAH")
	f.settings[sysconfig.KeyWarrantyClaimsLaborRule] = sysconfig.LaborRuleCenter
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = accLabor
	claim, _, _ := f.claim(t)
	f.completeReapply(t, claim, 1)

	rows := f.rows(t, claim)
	if len(rows) != 9 {
		t.Fatalf("rows = %d (%+v), want 9", len(rows), rows)
	}
	wantRow(t, rows, f.key(f.center, RoleWarrantyCost), "expense", accounting.CategoryWarrantyCost, accCenterCost, accCenterCost, 0)
	wantRow(t, rows, f.key(f.center, posting.RoleReturnIn), "expense", accounting.CategoryPurchase, accDistPrice, accDistPrice, f.dist.ID)
	// 150 TRY × 0.025 = 3.75 EUR; 200 TRY × 0.025 = 5.00 EUR; 200 TRY × 1.25 = 250.00 UAH.
	eur := []accRow{
		wantRow(t, rows, f.key(f.dist, posting.RoleReturnOut), "income", accounting.CategorySale, accDistPrice, "3.75", f.center.ID),
		wantRow(t, rows, f.key(f.dist, posting.RoleReturnIn), "expense", accounting.CategoryPurchase, accDealerPrice, "5.00", f.dealer.ID),
		// Labor 300 TRY: 7.50 EUR at the distributor, 375.00 UAH at the dealer.
		wantRow(t, rows, f.key(f.dist, RoleLaborIn), "income", accounting.CategoryWarrantyLaborIncome, accLabor, "7.50", f.center.ID),
		wantRow(t, rows, f.key(f.dist, RoleLaborOut), "expense", accounting.CategoryWarrantyLabor, accLabor, "7.50", f.dealer.ID),
	}
	uah := []accRow{
		wantRow(t, rows, f.key(f.dealer, posting.RoleReturnOut), "income", accounting.CategorySale, accDealerPrice, "250.00", f.dist.ID),
		wantRow(t, rows, f.key(f.dealer, RoleLaborIn), "income", accounting.CategoryWarrantyLaborIncome, accLabor, "375.00", f.dist.ID),
	}
	for _, r := range eur {
		if r.currency != "EUR" || r.origCur != "TRY" || trimRate(r.rate) != "0.025" || !r.rateDate.Equal(f.today) {
			t.Fatalf("EUR row = %+v; want TRY→EUR 0.025 on %s", r, f.today.Format(time.DateOnly))
		}
	}
	for _, r := range uah {
		if r.currency != "UAH" || r.origCur != "TRY" || trimRate(r.rate) != "1.25" || !r.rateDate.Equal(f.today) {
			t.Fatalf("UAH row = %+v; want TRY→UAH 1.25 on %s", r, f.today.Format(time.DateOnly))
		}
	}
}

// The dealer's latest received order price of the unit wins over its F1
// purchase price.
func TestWarrantyAccountingUsesReceivedOrderPrice(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	claim, product, unit := f.claim(t)
	var orderID, itemID int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, currency)
VALUES ($1, $2, $1, $3, 'TRY') RETURNING id`, f.dist.ID, f.brand.ID, f.dealer.ID).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	item, err := f.q.CreateOrderItem(f.ctx, db.CreateOrderItemParams{
		OrderID: orderID, ProductID: product.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
		UnitPrice: num(t, "180.00"), PriceSource: "distributor_dealer", LineTotal: num(t, "180.00"),
	})
	if err != nil {
		t.Fatalf("order item: %v", err)
	}
	itemID = item.ID
	if _, err := f.q.CreateOrderItemUnit(f.ctx, db.CreateOrderItemUnitParams{
		OrderItemID: itemID, UnitID: unit.ID, Quantity: pgtype.Int4{Int32: 1, Valid: true},
	}); err != nil {
		t.Fatalf("order item unit: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE orders SET status = 'received', approved_at = NOW(),
rate_snapshot = '{}'::jsonb, try_rate = 1 WHERE id = $1`, orderID); err != nil {
		t.Fatalf("receive order: %v", err)
	}
	f.completeReapply(t, claim, 1)
	rows := f.rows(t, claim)
	wantRow(t, rows, f.key(f.dist, posting.RoleReturnIn), "expense", accounting.CategoryPurchase, "180.00", "180.00", f.dealer.ID)
	wantRow(t, rows, f.key(f.dealer, posting.RoleReturnOut), "income", accounting.CategorySale, "180.00", "180.00", f.dist.ID)
	// The distributor has no received order of the unit: F1 price.
	wantRow(t, rows, f.key(f.dist, posting.RoleReturnOut), "income", accounting.CategorySale, accDistPrice, accDistPrice, f.center.ID)
}

func trimRate(s string) string {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return s
	}
	return strings.TrimRight(strings.TrimRight(numRat(n).FloatString(6), "0"), ".")
}

func num(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("numeric %q: %v", s, err)
	}
	return n
}

// TEC-382: cancelling the completed re-application service reverses every
// warranty_claim row (same amount, opposite sign, linked to the original),
// nets the claim to zero, leaves the originals untouched, adds one timeline
// note and writes nothing on a second cancellation.

type accLedgerRow struct {
	id, org     int64
	role        string
	direction   string
	category    string
	origAmount  string
	amount      string
	currency    string
	cari        int64
	reversalOf  int64
	description string
}

func (f *accFixture) ledgerRows(t *testing.T, claim db.WarrantyClaim) []accLedgerRow {
	t.Helper()
	rs, err := f.tx.Query(f.ctx, `
SELECT id, organization_id, role, direction, category, orig_amount::text, amount::text, currency,
       COALESCE(cari_id, 0), COALESCE(reversal_of_id, 0), COALESCE(description, '')
FROM finance_entries WHERE source_type = $1 AND source_uuid = $2 ORDER BY id`, AccountingSourceType, claim.Uuid)
	if err != nil {
		t.Fatalf("ledger rows: %v", err)
	}
	defer rs.Close()
	var out []accLedgerRow
	for rs.Next() {
		var r accLedgerRow
		if err := rs.Scan(&r.id, &r.org, &r.role, &r.direction, &r.category, &r.origAmount, &r.amount, &r.currency,
			&r.cari, &r.reversalOf, &r.description); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("ledger rows: %v", err)
	}
	return out
}

// cancelService cancels a completed service and publishes service.cancelled
// `times` times with the payload of POST /v1/services/{uuid}/cancel-completed
// (no service_id; the entity id carries it).
func (f *accFixture) cancelService(t *testing.T, svc db.Service, reason string, times int) {
	t.Helper()
	done, err := f.q.CancelCompletedService(f.ctx, db.CancelCompletedServiceParams{
		ID: svc.ID, CancelReason: pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		t.Fatalf("cancel service: %v", err)
	}
	for i := 0; i < times; i++ {
		ev := events.New(events.ServiceCancelled).WithTenant(done.OrganizationID).
			WithEntity("service", &done.ID, &done.Uuid).
			WithPayload(map[string]any{
				"service_uuid": done.Uuid.String(), "service_no": done.ServiceNo, "status": done.Status,
				"organization_id": done.OrganizationID, "brand_id": done.BrandID,
				"from_status": "completed", "reason": reason,
			})
		if err := f.bus.Publish(f.ctx, ev); err != nil {
			t.Fatalf("publish cancelled: %v", err)
		}
	}
}

func TestWarrantyAccountingCancelReversesRows(t *testing.T) {
	f := newAccFixture(t, "EUR", "UAH")
	f.settings[sysconfig.KeyWarrantyClaimsLaborRule] = sysconfig.LaborRuleCenter
	f.settings[sysconfig.KeyWarrantyClaimsLaborAmount] = accLabor
	claim, _, _ := f.claim(t)
	done := f.completeReapply(t, claim, 1)
	before := f.ledgerRows(t, claim)
	if len(before) != 9 {
		t.Fatalf("rows before cancel = %d, want 9", len(before))
	}

	f.cancelService(t, done, "Müşteri vazgeçti", 2)

	after := f.ledgerRows(t, claim)
	if len(after) != 18 {
		t.Fatalf("rows after two cancellations = %d, want 9 originals + 9 reversals", len(after))
	}
	originals := map[int64]accLedgerRow{}
	for _, r := range before {
		originals[r.id] = r
	}
	reversed := map[int64]int{}
	net := map[string]*big.Rat{}
	for _, r := range after {
		k := fmt.Sprintf("%d/%s", r.org, r.currency)
		if net[k] == nil {
			net[k] = new(big.Rat)
		}
		amt, _ := new(big.Rat).SetString(r.amount)
		net[k].Add(net[k], amt)
		if r.reversalOf == 0 {
			if r != originals[r.id] {
				t.Fatalf("original row changed: %+v -> %+v", originals[r.id], r)
			}
			continue
		}
		o, ok := originals[r.reversalOf]
		if !ok {
			t.Fatalf("reversal %+v of an unknown row", r)
		}
		reversed[o.id]++
		if r.org != o.org || r.role != o.role || r.direction != o.direction || r.category != o.category ||
			r.cari != o.cari || r.currency != o.currency || trim2(r.amount) != trim2("-"+o.amount) ||
			trim2(r.origAmount) != trim2("-"+o.origAmount) {
			t.Fatalf("reversal %+v does not mirror %+v", r, o)
		}
		if !strings.Contains(r.description, done.ServiceNo) || !strings.Contains(r.description, "Müşteri vazgeçti") {
			t.Fatalf("reversal description = %q", r.description)
		}
	}
	for _, o := range before {
		if reversed[o.id] != 1 {
			t.Fatalf("row %+v reversed %d times, want 1", o, reversed[o.id])
		}
	}
	for k, v := range net {
		if v.Sign() != 0 {
			t.Fatalf("net of %s = %s, want 0", k, v.FloatString(2))
		}
	}

	events, err := f.q.ListWarrantyClaimEvents(f.ctx, claim.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	notes := 0
	for _, ev := range events {
		if ev.EventType == EventNote && strings.Contains(string(ev.Payload), ReversalEventKind) {
			notes++
			if !strings.Contains(ev.Note.String, done.ServiceNo) {
				t.Fatalf("reversal note = %q", ev.Note.String)
			}
		}
	}
	if notes != 1 {
		t.Fatalf("reversal timeline events = %d, want 1", notes)
	}

	center := Caller{
		UserID: 1, OrganizationID: f.center.ID, BrandID: f.brand.ID, OrgType: "center",
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand},
		Permissions: map[string]rbac.Scope{
			rbac.PermWarrantyClaimsRead: rbac.ScopeBrand, rbac.PermAccountingRead: rbac.ScopeBrand,
		},
	}
	v, err := f.svc.Get(f.ctx, center, claim.Uuid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if v.CostSummary == nil || v.CostSummary.ProductCost != "0.00" || v.CostSummary.Labor != "0.00" || v.CostSummary.Currency != "TRY" {
		t.Fatalf("cost summary after cancel = %+v, want net 0", v.CostSummary)
	}

	// A direct repeat writes nothing either.
	acc := NewAccounting(f.tx, posting.New(f.q, nil, fxrates.New(f.q, nil, nil)), f.settings, nil)
	res, err := acc.ReverseReapplyService(f.ctx, done.ID, done.BrandID, "tekrar", nil)
	if err != nil || !res.Skipped {
		t.Fatalf("repeat reversal = %+v, %v; want skipped", res, err)
	}
	if rows := f.ledgerRows(t, claim); len(rows) != 18 {
		t.Fatalf("rows after repeat = %d, want 18", len(rows))
	}
}

// A cancelled service without a claim, and a cancelled claim service that
// is not the claim's re-application service, write nothing.
func TestWarrantyAccountingCancelIgnoresOtherServices(t *testing.T) {
	f := newAccFixture(t, "TRY", "TRY")
	claim, _, _ := f.claim(t)
	f.completeReapply(t, claim, 1)
	if rows := f.ledgerRows(t, claim); len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	// The claim's original (non-claim) service.
	original, err := f.q.GetService(f.ctx, db.GetServiceParams{ID: claim.ServiceID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("original service: %v", err)
	}
	f.cancelService(t, original, "Garanti dışı iptal", 1)
	acc := NewAccounting(f.tx, posting.New(f.q, nil, fxrates.New(f.q, nil, nil)), f.settings, nil)
	if res, err := acc.ReverseReapplyService(f.ctx, original.ID, original.BrandID, "x", nil); err != nil || !res.Skipped {
		t.Fatalf("non-claim service reversal = %+v, %v; want skipped", res, err)
	}
	// A completed (not cancelled) re-application service is not reversed.
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res, err := acc.ReverseReapplyService(f.ctx, got.ReapplyServiceID.Int64, got.BrandID, "x", nil); err != nil || !res.Skipped {
		t.Fatalf("completed service reversal = %+v, %v; want skipped", res, err)
	}
	for _, r := range f.ledgerRows(t, claim) {
		if r.reversalOf != 0 {
			t.Fatalf("unexpected reversal %+v", r)
		}
	}
}
