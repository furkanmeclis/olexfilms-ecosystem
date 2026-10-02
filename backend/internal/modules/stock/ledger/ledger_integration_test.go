package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-154 acceptance: ledger.Post against a migrated PostgreSQL
// (TEST_DATABASE_URL; CI runs PG18). Every Post commits its own
// transaction like a real request, so the concurrency tests see each
// other's locks. stock_movements is append-only, so the rows stay in the
// test database; every fixture name and barcode carries a unique suffix.

type env struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	l      *ledger.Ledger
	brand  int64
	center db.Organization
	dist   db.Organization
	dealer db.Organization
	cLoc   ledger.Owner // center warehouse location
	dLoc   ledger.Owner // distributor warehouse location
	piece  db.Product
	roll   db.Product
	fixed  db.Product
	suffix string
	// Shared customer vehicle of the services created by e.service.
	vehicle  db.Vehicle
	carBrand int64
	carModel int64
}

var refSeq atomic.Int64

func init() { refSeq.Store(time.Now().UnixNano() % 1_000_000_000_000) }

func nextRef() int64 { return refSeq.Add(1) }

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
	e := &env{ctx: ctx, pool: pool, q: q, l: ledger.New(q, outbox.NewStore(pool, q)),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano())}

	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	e.brand = brand.ID
	if e.center, err = q.GetBrandCenter(ctx, brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist = e.org(t, "dist", "distributor", e.center.ID)
	e.dealer = e.org(t, "dealer", "dealer", e.dist.ID)
	e.cLoc = e.location(t, e.center.ID, "C")
	e.dLoc = e.location(t, e.dist.ID, "D")

	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: e.center.ID, BrandID: brand.ID, Name: "t154-cat-" + e.suffix,
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
	e.piece = product("t154-piece", "piece", false)
	e.roll = product("t154-roll", "roll_meter", false)
	e.fixed = product("t154-fixed", "piece", true)
	return e
}

func (e *env) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t154-" + name + "-" + e.suffix, Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: e.brand, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
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

var serviceSeq atomic.Int64

// unit creates a printed label (not in stock yet).
func (e *env) unit(t *testing.T, p db.Product, meters string) db.Unit {
	t.Helper()
	arg := db.CreateUnitParams{
		OrganizationID: e.center.ID, BrandID: e.brand, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T154-%s-%d", e.suffix, unitSeq.Add(1)),
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

func orgOwner(o db.Organization) *ledger.Owner {
	return &ledger.Owner{Type: ledger.OwnerOrganization, ID: o.ID}
}

func ptr(o ledger.Owner) *ledger.Owner { return &o }

func mv(typ ledger.MovementType, u db.Unit, ref int64, to *ledger.Owner) ledger.Movement {
	return ledger.Movement{Type: typ, UnitID: u.ID, To: to, Source: "test", RefType: "t154", RefID: ref}
}

// post runs one movement in its own committed transaction.
func (e *env) post(m ledger.Movement) (ledger.Result, error) {
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		return ledger.Result{}, err
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	res, err := e.l.Post(e.ctx, tx, m)
	if err != nil {
		return res, err
	}
	return res, tx.Commit(e.ctx)
}

func (e *env) mustPost(t *testing.T, m ledger.Movement) ledger.Result {
	t.Helper()
	res, err := e.post(m)
	if err != nil {
		t.Fatalf("post %s: %v", m.Type, err)
	}
	return res
}

func (e *env) mustFail(t *testing.T, m ledger.Movement, want error) {
	t.Helper()
	if _, err := e.post(m); !errors.Is(err, want) {
		t.Fatalf("post %s: err = %v, want %v", m.Type, err, want)
	}
}

func (e *env) state(t *testing.T, u db.Unit) (db.UnitCurrentState, db.Unit) {
	t.Helper()
	s, err := e.q.GetUnitCurrentState(e.ctx, u.ID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	unit, err := e.q.GetUnit(e.ctx, u.ID)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	if unit.Status != s.Status {
		t.Fatalf("units.status %s != unit_current_state.status %s", unit.Status, s.Status)
	}
	return s, unit
}

func (e *env) orgStock(t *testing.T, orgID int64, p db.Product) (int32, string) {
	t.Helper()
	var qty int32
	var meters string
	err := e.pool.QueryRow(e.ctx, `SELECT quantity, meters::text FROM organization_product_stocks
		WHERE organization_id = $1 AND product_id = $2`, orgID, p.ID).Scan(&qty, &meters)
	if err != nil {
		return 0, "none"
	}
	return qty, meters
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// Barcode history: center warehouse -> distributor -> dealer -> service -> return.
func TestBarcodeHistory(t *testing.T) {
	e := newEnv(t)
	u := e.unit(t, e.piece, "")
	svc := *e.serviceOwner(t, e.dealer)
	transfer, order := nextRef(), nextRef()

	e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc)))
	e.mustPost(t, mv(ledger.TypeTransferOut, u, transfer, orgOwner(e.dist)))
	e.mustPost(t, mv(ledger.TypeTransferIn, u, transfer, ptr(e.dLoc)))
	e.mustPost(t, mv(ledger.TypeOrderOut, u, order, orgOwner(e.dealer)))
	e.mustPost(t, mv(ledger.TypeReceived, u, order, orgOwner(e.dealer)))
	e.mustPost(t, mv(ledger.TypeConsumption, u, nextRef(), ptr(svc)))
	e.mustPost(t, mv(ledger.TypeReturn, u, nextRef(), orgOwner(e.dealer)))

	rows, err := e.q.ListStockMovementsByUnit(e.ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		typ       ledger.MovementType
		ownerType ledger.OwnerType
		ownerID   int64
		status    ledger.Status
		org       int64
	}{
		{ledger.TypeEntry, ledger.OwnerWarehouseLocation, e.cLoc.ID, ledger.StatusAvailable, e.center.ID},
		{ledger.TypeTransferOut, ledger.OwnerOrganization, e.dist.ID, ledger.StatusInTransit, e.center.ID},
		{ledger.TypeTransferIn, ledger.OwnerWarehouseLocation, e.dLoc.ID, ledger.StatusAvailable, e.dist.ID},
		{ledger.TypeOrderOut, ledger.OwnerOrganization, e.dealer.ID, ledger.StatusInTransit, e.dist.ID},
		{ledger.TypeReceived, ledger.OwnerOrganization, e.dealer.ID, ledger.StatusAvailable, e.dealer.ID},
		{ledger.TypeConsumption, ledger.OwnerService, svc.ID, ledger.StatusUsed, e.dealer.ID},
		{ledger.TypeReturn, ledger.OwnerOrganization, e.dealer.ID, ledger.StatusAvailable, e.dealer.ID},
	}
	if len(rows) != len(want) {
		t.Fatalf("history has %d movements, want %d", len(rows), len(want))
	}
	for i, w := range want {
		r := rows[i]
		if r.Type != string(w.typ) || r.ToOwnerType.String != string(w.ownerType) || r.ToOwnerID.Int64 != w.ownerID ||
			r.ToStatus.String != string(w.status) || r.OrganizationID != w.org {
			t.Fatalf("movement %d = %s -> %s %d (%s, org %d), want %s -> %s %d (%s, org %d)", i,
				r.Type, r.ToOwnerType.String, r.ToOwnerID.Int64, r.ToStatus.String, r.OrganizationID,
				w.typ, w.ownerType, w.ownerID, w.status, w.org)
		}
		if i > 0 && (r.FromOwnerType.String != rows[i-1].ToOwnerType.String || r.FromOwnerID.Int64 != rows[i-1].ToOwnerID.Int64) {
			t.Fatalf("movement %d does not start where %d ended", i, i-1)
		}
	}

	s, _ := e.state(t, u)
	if s.OwnerType != "organization" || s.OwnerID != e.dealer.ID || s.HolderOrgID != e.dealer.ID || s.Status != "available" {
		t.Fatalf("state = %+v", s)
	}
	for _, c := range []struct {
		org  int64
		want int32
	}{{e.center.ID, 0}, {e.dist.ID, 0}, {e.dealer.ID, 1}} {
		if got, _ := e.orgStock(t, c.org, e.piece); got != c.want {
			t.Errorf("org %d stock = %d, want %d", c.org, got, c.want)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM outbox_events WHERE payload->'data'->>'barcode' = $1`, u.Barcode); n != len(want) {
		t.Fatalf("outbox events = %d, want %d", n, len(want))
	}
	if n := e.count(t, `SELECT count(*) FROM outbox_events WHERE payload->'data'->>'barcode' = $1
		AND event_name = 'stock.consumption'`, u.Barcode); n != 1 {
		t.Fatalf("stock.consumption events = %d", n)
	}
}

// A unit never has two owners: a second move while in transit, a second
// entry and a stale source are rejected; the state stays a single row.
func TestNoSecondOwner(t *testing.T) {
	e := newEnv(t)
	u := e.unit(t, e.piece, "")
	e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc)))
	e.mustPost(t, mv(ledger.TypeTransferOut, u, nextRef(), orgOwner(e.dist)))

	e.mustFail(t, mv(ledger.TypeTransferOut, u, nextRef(), orgOwner(e.dealer)), ledger.ErrTransitionNotAllowed)
	e.mustFail(t, mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc)), ledger.ErrTransitionNotAllowed)
	e.mustFail(t, mv(ledger.TypeConsumption, u, nextRef(),
		e.serviceOwner(t, e.dist)), ledger.ErrTransitionNotAllowed)
	stale := mv(ledger.TypeTransferCancelRestore, u, nextRef(), nil)
	stale.From = ptr(e.cLoc)
	e.mustFail(t, stale, ledger.ErrOwnerMismatch)

	if n := e.count(t, `SELECT count(*) FROM unit_current_state WHERE unit_id = $1`, u.ID); n != 1 {
		t.Fatalf("state rows = %d", n)
	}
	s, _ := e.state(t, u)
	if s.OwnerID != e.dist.ID || s.Status != "in_transit" {
		t.Fatalf("state = %+v", s)
	}

	// Two concurrent transfers of one available unit: one wins.
	v := e.unit(t, e.piece, "")
	e.mustPost(t, mv(ledger.TypeEntry, v, nextRef(), ptr(e.cLoc)))
	errs := race(e, mv(ledger.TypeTransferOut, v, nextRef(), orgOwner(e.dist)),
		mv(ledger.TypeTransferOut, v, nextRef(), orgOwner(e.dealer)))
	oneWins(t, errs, ledger.ErrTransitionNotAllowed)
	if n := e.count(t, `SELECT count(*) FROM stock_movements WHERE unit_id = $1 AND type = 'transfer_out'`, v.ID); n != 1 {
		t.Fatalf("transfer_out movements = %d", n)
	}
}

// 15 m roll: 5 m and 4 m out leaves 6 m; 7 m more is rejected.
func TestRollMeters(t *testing.T) {
	e := newEnv(t)
	u := e.unit(t, e.roll, "15")
	e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc)))
	for _, m := range []int64{500, 400} {
		p := mv(ledger.TypePartialConsumption, u, nextRef(), nil)
		p.Centimeters = m
		e.mustPost(t, p)
	}
	_, unit := e.state(t, u)
	if got, _ := unit.RemainingMeters.Float64Value(); got.Float64 != 6 {
		t.Fatalf("remaining = %v, want 6", got.Float64)
	}
	if qty, meters := e.orgStock(t, e.center.ID, e.roll); qty != 1 || meters != "6.00" {
		t.Fatalf("center roll stock = %d / %s, want 1 / 6.00", qty, meters)
	}
	var binMeters string
	if err := e.pool.QueryRow(e.ctx, `SELECT meters::text FROM bin_product_stocks
		WHERE location_id = $1 AND product_id = $2`, e.cLoc.ID, e.roll.ID).Scan(&binMeters); err != nil || binMeters != "6.00" {
		t.Fatalf("bin meters = %s, %v", binMeters, err)
	}

	tooMuch := mv(ledger.TypePartialConsumption, u, nextRef(), nil)
	tooMuch.Centimeters = 700
	e.mustFail(t, tooMuch, ledger.ErrInsufficientMeters)

	// The rest of the roll goes in one consumption.
	e.mustPost(t, mv(ledger.TypeConsumption, u, nextRef(),
		e.serviceOwner(t, e.center)))
	_, unit = e.state(t, u)
	if got, _ := unit.RemainingMeters.Float64Value(); got.Float64 != 0 || unit.Status != "used" {
		t.Fatalf("after consumption remaining = %v status %s", got.Float64, unit.Status)
	}
	if qty, meters := e.orgStock(t, e.center.ID, e.roll); qty != 0 || meters != "0.00" {
		t.Fatalf("center roll stock = %d / %s", qty, meters)
	}
}

// Double consumption and negative stock are rejected (serial and fixed).
func TestDoubleConsumptionAndNegativeStock(t *testing.T) {
	e := newEnv(t)
	svc := e.serviceOwner(t, e.center)
	u := e.unit(t, e.piece, "")
	e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), orgOwner(e.center)))
	e.mustPost(t, mv(ledger.TypeConsumption, u, nextRef(), svc))
	e.mustFail(t, mv(ledger.TypeConsumption, u, nextRef(), svc), ledger.ErrTransitionNotAllowed)

	f := e.unit(t, e.fixed, "")
	entry := mv(ledger.TypeEntry, f, nextRef(), ptr(e.cLoc))
	entry.Quantity = 3
	e.mustPost(t, entry)
	use := func(n int32) ledger.Movement {
		m := mv(ledger.TypeConsumption, f, nextRef(), nil)
		m.From, m.Quantity = ptr(e.cLoc), n
		return m
	}
	e.mustFail(t, use(5), ledger.ErrInsufficientStock)
	e.mustPost(t, use(2))

	// Fixed transfer: out of the center bin, into the distributor bin.
	transfer := nextRef()
	out := mv(ledger.TypeTransferOut, f, transfer, nil)
	out.From, out.Quantity = ptr(e.cLoc), 1
	e.mustPost(t, out)
	in := mv(ledger.TypeTransferIn, f, transfer, ptr(e.dLoc))
	in.Quantity = 2
	e.mustFail(t, in, ledger.ErrTransitionNotAllowed) // only 1 was sent
	in.Quantity = 1
	e.mustPost(t, in)
	e.mustFail(t, use(1), ledger.ErrInsufficientStock) // center bin is empty

	holding := func(o ledger.Owner) int32 {
		h, err := e.q.LockFixedBarcodeHolding(e.ctx, db.LockFixedBarcodeHoldingParams{
			UnitID: f.ID, OwnerType: string(o.Type), OwnerID: o.ID,
		})
		if err != nil {
			t.Fatalf("holding: %v", err)
		}
		return h.QuantityOnHand
	}
	if c, d := holding(e.cLoc), holding(e.dLoc); c != 0 || d != 1 {
		t.Fatalf("holdings center %d dist %d, want 0 / 1", c, d)
	}
	if qty, _ := e.orgStock(t, e.dist.ID, e.fixed); qty != 1 {
		t.Fatalf("distributor fixed stock = %d", qty)
	}
}

// The same idempotency key is written once.
func TestIdempotency(t *testing.T) {
	e := newEnv(t)
	u := e.unit(t, e.piece, "")
	m := mv(ledger.TypeEntry, u, nextRef(), ptr(e.cLoc))
	first := e.mustPost(t, m)
	second := e.mustPost(t, m)
	if first.Replayed || !second.Replayed || second.Movement.ID != first.Movement.ID {
		t.Fatalf("first %+v / second %+v", first, second)
	}
	want := fmt.Sprintf("test:t154:%d:entry:%s", m.RefID, u.Barcode)
	if first.Movement.IdempotencyKey != want {
		t.Fatalf("key = %s, want %s", first.Movement.IdempotencyKey, want)
	}
	if n := e.count(t, `SELECT count(*) FROM stock_movements WHERE unit_id = $1`, u.ID); n != 1 {
		t.Fatalf("movements = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM outbox_events WHERE payload->'data'->>'idempotency_key' = $1`, want); n != 1 {
		t.Fatalf("outbox events = %d", n)
	}
	if qty, _ := e.orgStock(t, e.center.ID, e.piece); qty != 1 {
		t.Fatalf("center stock = %d", qty)
	}
}

// Two concurrent consumptions of one unit: one succeeds, one fails.
func TestConcurrentConsumption(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 3; i++ {
		u := e.unit(t, e.piece, "")
		e.mustPost(t, mv(ledger.TypeEntry, u, nextRef(), orgOwner(e.center)))
		svc := func() *ledger.Owner {
			return e.serviceOwner(t, e.center)
		}
		errs := race(e, mv(ledger.TypeConsumption, u, nextRef(), svc()), mv(ledger.TypeConsumption, u, nextRef(), svc()))
		oneWins(t, errs, ledger.ErrTransitionNotAllowed)
		if n := e.count(t, `SELECT count(*) FROM stock_movements WHERE unit_id = $1 AND type = 'consumption'`, u.ID); n != 1 {
			t.Fatalf("consumptions = %d", n)
		}
	}
	if qty, _ := e.orgStock(t, e.center.ID, e.piece); qty != 0 {
		t.Fatalf("center stock = %d", qty)
	}
}

// race posts a and b from two goroutines at the same time.
func race(e *env, a, b ledger.Movement) [2]error {
	var errs [2]error
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, m := range []ledger.Movement{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = e.post(m)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func oneWins(t *testing.T, errs [2]error, loser error) {
	t.Helper()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, loser):
			t.Fatalf("loser error = %v, want %v", err, loser)
		}
	}
	if ok != 1 {
		t.Fatalf("successes = %d (%v), want 1", ok, errs)
	}
}

// service creates a real draft service of org: since TEC-178 (000050) a
// service owner is a foreign key to services(id, organization_id), and the
// holder organization must be the service organization.
func (e *env) service(t *testing.T, org db.Organization) int64 {
	t.Helper()
	if e.vehicle.ID == 0 {
		user, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
			PasswordHash: "x", Name: "T178", Surname: e.suffix, Status: "active",
			Email: pgtype.Text{String: "t178-ledger-" + e.suffix + "@example.test", Valid: true},
		})
		if err != nil {
			t.Fatalf("customer: %v", err)
		}
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`,
			"t178-"+e.suffix).Scan(&e.carBrand); err != nil {
			t.Fatalf("car brand: %v", err)
		}
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`,
			e.carBrand, "t178-"+e.suffix).Scan(&e.carModel); err != nil {
			t.Fatalf("car model: %v", err)
		}
		if e.vehicle, err = e.q.CreateVehicle(e.ctx, db.CreateVehicleParams{
			UserID: user.ID, BrandID: e.brand,
			CarBrandID: pgtype.Int8{Int64: e.carBrand, Valid: true},
			CarModelID: pgtype.Int8{Int64: e.carModel, Valid: true},
		}); err != nil {
			t.Fatalf("vehicle: %v", err)
		}
	}
	s, err := e.q.CreateService(e.ctx, db.CreateServiceParams{
		// services.service_no is VARCHAR(32): "L" + 19-digit nanoseconds + sequence.
		ServiceNo:      fmt.Sprintf("L%d-%d", time.Now().UnixNano(), serviceSeq.Add(1)),
		OrganizationID: org.ID, BrandID: e.brand,
		CustomerUserID: e.vehicle.UserID, VehicleID: e.vehicle.ID,
		CarBrandID: e.carBrand, CarModelID: e.carModel, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return s.ID
}

// serviceOwner is a service owner held by the service organization.
func (e *env) serviceOwner(t *testing.T, org db.Organization) *ledger.Owner {
	t.Helper()
	return &ledger.Owner{Type: ledger.OwnerService, ID: e.service(t, org), OrgID: org.ID}
}
