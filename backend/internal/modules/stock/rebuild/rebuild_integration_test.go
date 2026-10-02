package rebuild_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-156 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18). The test database is shared with other packages, so every
// scan is scoped to a fresh distributor; the chain starts at the brand
// center (entries go to a center, K14) and ends at the distributor.

type env struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	l      *ledger.Ledger
	svc    *rebuild.Service
	brand  int64
	center db.Organization
	dist   db.Organization
	cLoc   ledger.Owner
	dLoc   ledger.Owner
	dLoc2  ledger.Owner
	piece  db.Product
	roll   db.Product
	fixed  db.Product
	suffix string
}

var seq atomic.Int64

func init() { seq.Store(time.Now().UnixNano() % 1_000_000_000_000) }

func next() int64 { return seq.Add(1) }

func newEnv(t *testing.T) *env {
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
	q := db.New(pool)
	e := &env{ctx: ctx, pool: pool, q: q, l: ledger.New(q, nil), svc: rebuild.New(pool, q),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	e.brand = brand.ID
	if e.center, err = q.GetBrandCenter(ctx, brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist, err = q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t156-dist-" + e.suffix, Name: "dist", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: e.center.ID, Valid: true},
		BrandID: e.brand, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	e.cLoc = e.location(t, e.center.ID, "C")
	e.dLoc = e.location(t, e.dist.ID, "D")
	e.dLoc2 = e.location(t, e.dist.ID, "D2")

	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: e.center.ID, BrandID: brand.ID, Name: "t156-cat-" + e.suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku, unitType string, fixed bool) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: e.center.ID, BrandID: brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + e.suffix, Name: sku, Images: []byte("[]"),
			UnitType: unitType, UsesFixedBarcode: fixed, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	e.piece = product("t156-piece", "piece", false)
	e.roll = product("t156-roll", "roll_meter", false)
	e.fixed = product("t156-fixed", "piece", true)
	return e
}

func (e *env) location(t *testing.T, orgID int64, code string) ledger.Owner {
	t.Helper()
	loc, err := e.q.CreateWarehouseLocation(e.ctx, db.CreateWarehouseLocationParams{
		OrganizationID: orgID, Code: code + "-" + e.suffix, Name: "bin " + code, Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: orgID}
}

var unitSeq atomic.Int64

func (e *env) unit(t *testing.T, p db.Product, meters string) db.Unit {
	t.Helper()
	arg := db.CreateUnitParams{
		OrganizationID: e.center.ID, BrandID: e.brand, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T156-%s-%d", e.suffix, unitSeq.Add(1)),
		UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
	}
	if p.UsesFixedBarcode {
		arg.UnitKind = ledger.KindFixed
	}
	if meters != "" {
		var n pgtype.Numeric
		if err := n.Scan(meters); err != nil {
			t.Fatal(err)
		}
		arg.InitialMeters, arg.RemainingMeters = n, n
	}
	u, err := e.q.CreateUnit(e.ctx, arg)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	return u
}

func mv(typ ledger.MovementType, u db.Unit, ref int64, to *ledger.Owner) ledger.Movement {
	return ledger.Movement{Type: typ, UnitID: u.ID, To: to, Source: "test", RefType: "t156", RefID: ref}
}

func ptr(o ledger.Owner) *ledger.Owner { return &o }

func (e *env) post(t *testing.T, m ledger.Movement) {
	t.Helper()
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	if _, err := e.l.Post(e.ctx, tx, m); err != nil {
		t.Fatalf("post %s: %v", m.Type, err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	tag, err := e.pool.Exec(e.ctx, sql, args...)
	if err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("exec %q: %d rows", sql, tag.RowsAffected())
	}
}

// chain builds center -> distributor stock through ledger.Post: a placed
// piece, a 15 m roll cut twice (6 m left) and a fixed barcode split 6 / 4.
func (e *env) chain(t *testing.T) (piece, roll, fixed db.Unit) {
	t.Helper()
	distOrg := &ledger.Owner{Type: ledger.OwnerOrganization, ID: e.dist.ID}

	piece = e.unit(t, e.piece, "")
	e.post(t, mv(ledger.TypeEntry, piece, next(), ptr(e.cLoc)))
	ref := next()
	e.post(t, mv(ledger.TypeTransferOut, piece, ref, distOrg))
	e.post(t, mv(ledger.TypeTransferIn, piece, ref, ptr(e.dLoc)))
	e.post(t, mv(ledger.TypePlacement, piece, next(), ptr(e.dLoc2)))

	roll = e.unit(t, e.roll, "15")
	e.post(t, mv(ledger.TypeEntry, roll, next(), ptr(e.cLoc)))
	ref = next()
	e.post(t, mv(ledger.TypeTransferOut, roll, ref, distOrg))
	e.post(t, mv(ledger.TypeTransferIn, roll, ref, ptr(e.dLoc)))
	for _, cm := range []int64{500, 400} {
		p := mv(ledger.TypePartialConsumption, roll, next(), nil)
		p.Centimeters = cm
		e.post(t, p)
	}

	fixed = e.unit(t, e.fixed, "")
	entry := mv(ledger.TypeEntry, fixed, next(), ptr(e.cLoc))
	entry.Quantity = 10
	e.post(t, entry)
	ref = next()
	out := mv(ledger.TypeTransferOut, fixed, ref, nil)
	out.From, out.Quantity = ptr(e.cLoc), 4
	e.post(t, out)
	in := mv(ledger.TypeTransferIn, fixed, ref, distOrg)
	in.Quantity = 4
	e.post(t, in)
	return piece, roll, fixed
}

// snapshot renders every projection row of the scope and the movements of
// the units, to prove what a run wrote.
func (e *env) snapshot(t *testing.T, units []int64) string {
	t.Helper()
	var b strings.Builder
	queries := []string{
		`SELECT unit_id, owner_type, owner_id, holder_org_id, status, last_movement_id, version FROM unit_current_state WHERE unit_id = ANY($1) ORDER BY unit_id`,
		`SELECT unit_id, owner_type, owner_id, holder_org_id, quantity_on_hand, last_movement_id, version FROM fixed_barcode_holdings WHERE unit_id = ANY($1) ORDER BY id`,
		`SELECT id, status, remaining_meters::text FROM units WHERE id = ANY($1) ORDER BY id`,
		`SELECT id, type, quantity_delta, meters_delta::text FROM stock_movements WHERE unit_id = ANY($1) ORDER BY id`,
	}
	for _, sql := range queries {
		rows, err := e.pool.Query(e.ctx, sql, units)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
	}
	for _, sql := range []string{
		`SELECT location_id, product_id, quantity, meters::text FROM bin_product_stocks WHERE organization_id = $1 ORDER BY 1, 2`,
		`SELECT organization_id, product_id, quantity, meters::text FROM organization_product_stocks WHERE organization_id = $1 ORDER BY 1, 2`,
	} {
		rows, err := e.pool.Query(e.ctx, sql, e.dist.ID)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
	}
	return b.String()
}

func (e *env) audits(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM activity_events
		WHERE action = $1 AND payload->>'organization_id' = $2`,
		rebuild.AuditAction, fmt.Sprint(e.dist.ID)).Scan(&n); err != nil {
		t.Fatalf("audits: %v", err)
	}
	return n
}

func hasDiff(rep rebuild.Report, table, field string, unitID int64) bool {
	for _, d := range rep.Diffs {
		if d.Table == table && strings.HasSuffix(d.Field, field) && (unitID == 0 || d.UnitID == unitID) {
			return true
		}
	}
	return false
}

func TestRebuildScanRepair(t *testing.T) {
	e := newEnv(t)
	piece, roll, fixed := e.chain(t)
	units := []int64{piece.ID, roll.ID, fixed.ID}

	// A chain posted through ledger.Post has no drift.
	rep, err := e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.DiffCount != 0 || len(rep.Anomalies) != 0 {
		t.Fatalf("clean chain: diffs %+v anomalies %v", rep.Diffs, rep.Anomalies)
	}
	if rep.UnitsScanned != 3 || rep.MovementsReplayed != 12 {
		t.Fatalf("scanned %d units / %d movements, want 3 / 12", rep.UnitsScanned, rep.MovementsReplayed)
	}

	// Corrupt the projections directly; the movements stay untouched.
	e.exec(t, `UPDATE unit_current_state SET owner_id = $2, holder_org_id = $3 WHERE unit_id = $1`,
		piece.ID, e.dLoc.ID, e.dist.ID)
	e.exec(t, `UPDATE units SET remaining_meters = 10 WHERE id = $1`, roll.ID)
	e.exec(t, `UPDATE fixed_barcode_holdings SET quantity_on_hand = 1 WHERE unit_id = $1 AND owner_type = 'organization' AND owner_id = $2`,
		fixed.ID, e.dist.ID)
	e.exec(t, `UPDATE organization_product_stocks SET quantity = quantity + 5 WHERE organization_id = $1 AND product_id = $2`,
		e.dist.ID, e.piece.ID)
	e.exec(t, `UPDATE bin_product_stocks SET meters = 0 WHERE location_id = $1 AND product_id = $2`,
		e.dLoc.ID, e.roll.ID)

	before := e.snapshot(t, units)
	auditsBefore := e.audits(t)

	// The dry run finds every corruption and writes nothing.
	rep, err = e.svc.Run(e.ctx, rebuild.Options{OrganizationID: e.dist.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		table, field string
		unit         int64
	}{
		{rebuild.TableUnitState, "owner_id", piece.ID},
		{rebuild.TableUnits, "remaining_meters", roll.ID},
		{rebuild.TableFixedHoldings, "quantity_on_hand", fixed.ID},
		{rebuild.TableOrgStocks, "quantity", 0},
		{rebuild.TableBinStocks, "meters", 0},
	} {
		if !hasDiff(rep, want.table, want.field, want.unit) {
			t.Errorf("missing diff %s.%s (unit %d) in %+v", want.table, want.field, want.unit, rep.Diffs)
		}
	}
	if rep.Applied || rep.DiffCount != len(rep.Diffs) || rep.DiffCount < 5 {
		t.Fatalf("dry run report: applied %v, %d diffs", rep.Applied, rep.DiffCount)
	}
	if after := e.snapshot(t, units); after != before {
		t.Fatalf("dry run wrote:\nbefore\n%s\nafter\n%s", before, after)
	}
	if n := e.audits(t); n != auditsBefore {
		t.Fatalf("dry run wrote %d audit rows", n-auditsBefore)
	}

	// Repair: one transaction, audit row, movements untouched.
	movementsBefore := movementRows(t, e, units)
	rep, err = e.svc.Run(e.ctx, rebuild.Options{OrganizationID: e.dist.ID, Apply: true, Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Applied || rep.DiffCount < 5 {
		t.Fatalf("repair report: applied %v, %d diffs", rep.Applied, rep.DiffCount)
	}
	if n := e.audits(t); n != auditsBefore+1 {
		t.Fatalf("audit rows = %d, want %d", n, auditsBefore+1)
	}
	if got := movementRows(t, e, units); got != movementsBefore {
		t.Fatalf("repair touched movements:\n%s\n%s", movementsBefore, got)
	}

	rep, err = e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.DiffCount != 0 || len(rep.Anomalies) != 0 {
		t.Fatalf("after repair: %+v %v", rep.Diffs, rep.Anomalies)
	}
	var qty int32
	var meters string
	if err := e.pool.QueryRow(e.ctx, `SELECT quantity, meters::text FROM organization_product_stocks
		WHERE organization_id = $1 AND product_id = $2`, e.dist.ID, e.roll.ID).Scan(&qty, &meters); err != nil ||
		qty != 1 || meters != "6.00" {
		t.Fatalf("distributor roll stock = %d / %s (%v), want 1 / 6.00", qty, meters, err)
	}

	// A repaired ledger keeps working: the next movement posts cleanly.
	cut := mv(ledger.TypePartialConsumption, roll, next(), nil)
	cut.Centimeters = 100
	e.post(t, cut)
	if rep, err = e.svc.Check(e.ctx, e.dist.ID); err != nil || rep.DiffCount != 0 {
		t.Fatalf("after next post: %v %+v", err, rep.Diffs)
	}

	// Nothing to repair: no write, no audit row.
	rep, err = e.svc.Run(e.ctx, rebuild.Options{OrganizationID: e.dist.ID, Apply: true, Source: "test"})
	if err != nil || rep.Applied || rep.DiffCount != 0 {
		t.Fatalf("clean apply: %v applied %v diffs %d", err, rep.Applied, rep.DiffCount)
	}
	if n := e.audits(t); n != auditsBefore+1 {
		t.Fatalf("clean apply wrote an audit row")
	}
}

// A projection row without any movement (a phantom) is reported and removed.
func TestRebuildPhantomRows(t *testing.T) {
	e := newEnv(t)
	e.chain(t)
	e.exec(t, `INSERT INTO bin_product_stocks (location_id, organization_id, brand_id, product_id, quantity, meters)
		VALUES ($1, $2, $3, $4, 3, 0)
		ON CONFLICT (location_id, product_id) DO UPDATE SET quantity = 3`, e.dLoc2.ID, e.dist.ID, e.brand, e.fixed.ID)
	rep, err := e.svc.Check(e.ctx, e.dist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.DiffCount != 1 || !hasDiff(rep, rebuild.TableBinStocks, "quantity", 0) {
		t.Fatalf("phantom bin: %+v", rep.Diffs)
	}
	if _, err := e.svc.Run(e.ctx, rebuild.Options{OrganizationID: e.dist.ID, Apply: true, Source: "test"}); err != nil {
		t.Fatal(err)
	}
	if rep, err = e.svc.Check(e.ctx, e.dist.ID); err != nil || rep.DiffCount != 0 {
		t.Fatalf("after repair: %v %+v", err, rep.Diffs)
	}
}

func movementRows(t *testing.T, e *env, units []int64) string {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `SELECT id, organization_id, type, quantity_delta, meters_delta::text,
		coalesce(from_owner_type, ''), coalesce(from_owner_id, 0), coalesce(to_owner_type, ''), coalesce(to_owner_id, 0),
		coalesce(from_status, ''), coalesce(to_status, ''), idempotency_key
		FROM stock_movements WHERE unit_id = ANY($1) ORDER BY id`, units)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&b, vals...)
	}
	return b.String()
}
