package glorian_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-269: the stock item pull mirrors the hub status onto
// units.external_status without a ledger movement, and the glorian units
// stay visible to the center warehouse role only (K20).

// stockUnits holds glorian units for the stock tests: inStock was entered
// into a glorian center bin through ledger.Post, label is a printed label.
type stockUnits struct {
	product db.Product
	loc     db.WarehouseLocation
	inStock db.Unit
	label   db.Unit
}

func (f *pullFixture) barcode(n int) string { return fmt.Sprintf("GT269-%s-%d", f.suffix, n) }

func (f *pullFixture) stockUnits(t *testing.T) stockUnits {
	t.Helper()
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, Name: f.name("T269 Local"),
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	var s stockUnits
	s.product, err = f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, CategoryID: cat.ID,
		Sku: "T269-" + f.suffix, Name: f.name("T269 Film"), Images: []byte("[]"),
		UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	s.loc, err = f.q.CreateWarehouseLocation(f.ctx, db.CreateWarehouseLocationParams{
		OrganizationID: f.glorian.ID, Code: "T269-" + f.suffix, Name: "T269 bin", Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	unit := func(n int) db.Unit {
		u, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
			OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, ProductID: s.product.ID,
			Barcode: f.barcode(n), UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		})
		if err != nil {
			t.Fatalf("unit %d: %v", n, err)
		}
		return u
	}
	s.inStock, s.label = unit(1), unit(2)
	l := ledger.New(f.q, outbox.NewStore(nil, f.q))
	if _, err := l.Post(f.ctx, f.tx, ledger.Movement{
		Type: ledger.TypeEntry, UnitID: s.inStock.ID,
		To:     &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: s.loc.ID, OrgID: f.glorian.ID},
		Source: "test", RefType: "t269", RefID: time.Now().UnixNano(),
	}); err != nil {
		t.Fatalf("entry: %v", err)
	}
	return s
}

func (f *pullFixture) movementCount(t *testing.T, ids ...int64) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM stock_movements WHERE unit_id = ANY($1)`, ids).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *pullFixture) stockItem(id, barcode, status, updated string) fake.Row {
	return fake.Row{
		"id": id, "barcode": barcode, "product_id": "10", "dealer_id": nil,
		"status": status, "location": "center", "updated_at": updated,
	}
}

func TestPullStockMirrorsExternalStatus(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.stockUnits(t)
	stateBefore, err := f.q.GetUnitCurrentState(f.ctx, s.inStock.ID)
	if err != nil {
		t.Fatal(err)
	}
	movesBefore := f.movementCount(t, s.inStock.ID, s.label.ID)
	if movesBefore != 1 {
		t.Fatalf("movements before = %d, want the entry only", movesBefore)
	}
	f.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		f.stockItem("9001", f.barcode(1), glorian.StockStatusUsed, "2026-09-01T12:00:00+00:00"),
		// Lower case on the hub still matches the local barcode.
		f.stockItem("9002", "gt269-"+f.suffix+"-2", glorian.StockStatusReserved, "2026-09-02T12:00:00+00:00"),
		f.stockItem("9003", f.barcode(99), glorian.StockStatusAvailable, "2026-09-03T12:00:00+00:00"),
	})
	p := f.puller()
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, c := range []struct {
		unit   db.Unit
		remote string
		status string
	}{
		{s.inStock, "9001", glorian.StockStatusUsed},
		{s.label, "9002", glorian.StockStatusReserved},
	} {
		u, err := f.q.GetUnit(f.ctx, c.unit.ID)
		if err != nil {
			t.Fatal(err)
		}
		if u.ExternalStatus.String != c.status || u.ExternalID.String != c.remote || u.ConnectionID.Int64 != f.conn.ID {
			t.Fatalf("unit %s mirror = %v/%v/%v, want %s/%s/%d",
				u.Barcode, u.ExternalStatus, u.ExternalID, u.ConnectionID, c.status, c.remote, f.conn.ID)
		}
	}
	// Mirror only: no ledger movement, no projection change.
	if n := f.movementCount(t, s.inStock.ID, s.label.ID); n != movesBefore {
		t.Fatalf("stock movements = %d, want %d (pull must not post to the ledger)", n, movesBefore)
	}
	stateAfter, err := f.q.GetUnitCurrentState(f.ctx, s.inStock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfter.Status != stateBefore.Status || stateAfter.OwnerID != stateBefore.OwnerID || stateAfter.HolderOrgID != stateBefore.HolderOrgID {
		t.Fatalf("unit state changed: %+v -> %+v", stateBefore, stateAfter)
	}
	inStock, err := f.q.GetUnit(f.ctx, s.inStock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inStock.Status != string(ledger.StatusAvailable) {
		t.Fatalf("in-stock unit status = %s, want available", inStock.Status)
	}
	// The local lifecycle is untouched: the hub says used/reserved, the
	// units keep their ledger status.
	label, err := f.q.GetUnit(f.ctx, s.label.ID)
	if err != nil {
		t.Fatal(err)
	}
	if label.Status != string(ledger.StatusPrinted) {
		t.Fatalf("label status = %s, want printed", label.Status)
	}

	run := latestRun(t, f.runs(t), glorian.KindPullStock)
	c := runCounts(t, run)
	if run.Status != glorian.RunSucceeded || c.Fetched != 3 || c.Updated != 2 || c.Unmatched != 1 {
		t.Fatalf("pull_stock run = %s %+v; want 3 fetched, 2 updated, 1 unmatched", run.Status, c)
	}
	wantWM := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if !run.Watermark.Valid || !run.Watermark.Time.Equal(wantWM) {
		t.Fatalf("watermark = %v, want %v", run.Watermark, wantWM)
	}

	// Second run: only the overlapping newest row is fetched again; a
	// changed status is mirrored again, still without a movement.
	f.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		f.stockItem("9001", f.barcode(1), glorian.StockStatusExternalOutbound, "2026-09-10T12:00:00+00:00"),
		f.stockItem("9002", "gt269-"+f.suffix+"-2", glorian.StockStatusReserved, "2026-09-02T12:00:00+00:00"),
		f.stockItem("9003", f.barcode(99), glorian.StockStatusAvailable, "2026-09-03T12:00:00+00:00"),
	})
	if err := p.Run(f.ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	c = runCounts(t, latestRun(t, f.runs(t), glorian.KindPullStock))
	if c.Fetched != 2 || c.Updated != 1 || c.Unmatched != 1 {
		t.Fatalf("second pull_stock counts = %+v; want 9001 updated and 9003 unmatched", c)
	}
	u, err := f.q.GetUnit(f.ctx, s.inStock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.ExternalStatus.String != glorian.StockStatusExternalOutbound {
		t.Fatalf("external_status = %v after second run", u.ExternalStatus)
	}
	if n := f.movementCount(t, s.inStock.ID, s.label.ID); n != movesBefore {
		t.Fatalf("stock movements after second run = %d, want %d", n, movesBefore)
	}
}

// viewerOf builds the stock.read viewer of a system role in organization o,
// resolved the same way middleware.RequireScope does.
func (f *pullFixture) viewerOf(t *testing.T, role string, o db.Organization) stockusecase.UnitViewer {
	t.Helper()
	def, ok := rbac.RoleBySlug(role)
	if !ok {
		t.Fatalf("role %s", role)
	}
	p := authctx.Principal{UserInternal: 1, Roles: []string{role}, PermissionScopes: rbac.RoleGrants(def)}
	org := orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, OrgType: o.Type, BrandID: o.BrandID}
	filter, err := scopefilter.Resolve(f.ctx, f.q, p, &org, rbac.PermStockRead)
	if err != nil {
		t.Fatalf("%s stock.read scope: %v", role, err)
	}
	return stockusecase.UnitViewer{Principal: p, Org: org, Filter: filter}
}

// K20: glorian units are seen by the center warehouse role only; Olex
// operation screens (dealer stock, service consumption, orders) never
// find them.
func TestGlorianUnitsVisibleToCenterWarehouseOnly(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.stockUnits(t)
	f.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		f.stockItem("9101", f.barcode(1), glorian.StockStatusAvailable, "2026-09-01T12:00:00+00:00"),
	})
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	dealer, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "t269-dealer-" + f.suffix, Name: "T269 Olex Bayi", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: f.olex.ID, Valid: true},
		BrandID: f.olex.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	svc := stockusecase.New(f.q)
	barcode := s.inStock.Barcode
	unitFilter := stockmodel.UnitFilter{Barcode: barcode, Limit: 50}

	// Olex dealer owner: lookup, scanner and list find nothing.
	d := f.viewerOf(t, rbac.RoleDealerOwner, dealer)
	if _, err := svc.UnitHistory(f.ctx, d.Org, d.Filter, barcode, 50, 0); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("dealer unit history: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ReachableUnit(f.ctx, d.Org, d.Filter, barcode); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("dealer scanner lookup: err = %v, want ErrNotFound", err)
	}
	if _, _, err := svc.OrganizationUnits(f.ctx, d, f.glorian.Uuid, unitFilter); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("dealer list of glorian center: err = %v, want ErrNotFound", err)
	}
	if rows, total, err := svc.OrganizationUnits(f.ctx, d, dealer.Uuid, unitFilter); err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("dealer own list = %d/%d, %v; want empty", len(rows), total, err)
	}

	// Olex service consumption and order fulfillment resolve barcodes in
	// their own brand (K20): the glorian barcode does not exist there.
	if _, err := f.q.GetUnitByBarcode(f.ctx, db.GetUnitByBarcodeParams{BrandID: f.olex.BrandID, Barcode: barcode}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("olex brand barcode lookup: err = %v, want no rows", err)
	}

	// Center warehouse (Olex center, stock.read scope all): sees the unit.
	w := f.viewerOf(t, rbac.RoleCenterWarehouse, f.olex)
	if w.Filter.Scope != rbac.ScopeAll {
		t.Fatalf("center warehouse stock.read scope = %s, want all", w.Filter.Scope)
	}
	h, err := svc.UnitHistory(f.ctx, w.Org, w.Filter, barcode, 50, 0)
	if err != nil {
		t.Fatalf("center warehouse unit history: %v", err)
	}
	if h.Unit.UUID != s.inStock.Uuid {
		t.Fatalf("center warehouse history unit = %s, want %s", h.Unit.UUID, s.inStock.Uuid)
	}
	ru, err := svc.ReachableUnit(f.ctx, w.Org, w.Filter, barcode)
	if err != nil || ru.Unit.ID != s.inStock.ID || ru.Unit.ExternalStatus.String != glorian.StockStatusAvailable {
		t.Fatalf("center warehouse scanner = %+v, %v", ru.Unit, err)
	}
	rows, total, err := svc.OrganizationUnits(f.ctx, w, f.glorian.Uuid, unitFilter)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].UUID != s.inStock.Uuid {
		t.Fatalf("center warehouse list of glorian center = %d/%d, %v; want the unit", len(rows), total, err)
	}
}
