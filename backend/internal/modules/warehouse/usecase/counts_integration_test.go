package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-206 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18): every count method; completion computes the differences and
// changes no stock; approval writes the correction movements through the
// ledger (missing unit void, extra/misplaced unit placed, fixed quantity and
// roll meters set to the counted value); blind counts never answer expected
// values; a location outside the scope cannot be counted.

type countSettings struct{}

func (countSettings) Scan(context.Context) sysconfig.Scan { return sysconfig.Scan{SKUEnabled: true} }

type countEnv struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	l      *ledger.Ledger
	brand  db.Brand
	center db.Organization
	piece  db.Product
	roll   db.Product
	fixed  db.Product
	suffix string
	tree   *wh.Service
	counts *wh.Counts
}

var countSeq atomic.Int64

func newCountEnv(t *testing.T) *countEnv {
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
	e := &countEnv{ctx: ctx, pool: pool, q: q, l: ledger.New(q, outbox.NewStore(pool, q)),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	if e.brand, err = q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	if e.center, err = q.GetBrandCenter(ctx, e.brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, Name: "t206-cat-" + e.suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku, unitType string, fixed bool) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: e.center.ID, BrandID: e.brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + e.suffix, Name: sku, Images: []byte("[]"), UnitType: unitType,
			UsesFixedBarcode: fixed, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	e.piece = product("t206-piece", "piece", false)
	e.roll = product("t206-roll", "roll_meter", false)
	e.fixed = product("t206-fixed", "piece", true)
	e.tree = wh.New(pool, q)
	e.counts = wh.NewCounts(pool, q, outbox.NewStore(pool, q), wh.NewScanner(q, countSettings{}))
	return e
}

// caller is a center warehouse user; adjust grants stock.adjust.
func (e *countEnv) caller(adjust bool) wh.ScanCaller {
	o := e.center
	scopes := map[string]rbac.Scope{rbac.PermStockRead: rbac.ScopeAll}
	if adjust {
		scopes[rbac.PermStockAdjust] = rbac.ScopeAll
	}
	return wh.ScanCaller{
		Caller: wh.Caller{
			Org:    orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, Slug: o.Slug, OrgType: o.Type, BrandID: o.BrandID, BrandSlug: e.brand.Slug},
			Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{o.ID}, OrgID: o.ID},
		},
		Principal: authctx.Principal{PermissionScopes: scopes},
	}
}

type countLoc struct {
	view wh.Location
	id   int64
}

func (l countLoc) owner(org int64) *ledger.Owner {
	return &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: l.id, OrgID: org}
}

func (l countLoc) qr() string { return "OFW:LOC:" + l.view.FullCode }

// warehouse creates warehouse -> room -> aisles for the center.
func (e *countEnv) warehouse(t *testing.T, code string, aisles ...string) (wh.Warehouse, wh.Room, []countLoc) {
	t.Helper()
	c := e.caller(false).Caller
	w, err := e.tree.CreateWarehouse(e.ctx, c, wh.WarehouseInput{Code: code + e.suffix[len(e.suffix)-6:], Name: "t206 " + code})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	r, err := e.tree.CreateRoom(e.ctx, c, w.UUID, wh.RoomInput{Code: "R1", Name: "room"})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	var out []countLoc
	for _, a := range aisles {
		l, err := e.tree.CreateLocation(e.ctx, c, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: a, Name: "aisle " + a})
		if err != nil {
			t.Fatalf("location %s: %v", a, err)
		}
		row, err := e.q.GetTypedLocationByUUID(e.ctx, db.GetTypedLocationByUUIDParams{Uuid: l.UUID, OrganizationID: e.center.ID})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, countLoc{view: l, id: row.ID})
	}
	return w, r, out
}

func (e *countEnv) post(t *testing.T, m ledger.Movement) {
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

// unit enters a new unit at the center: serial units go to loc (nil: stay
// unlocated with the organization), fixed barcodes enter qty at loc.
func (e *countEnv) unit(t *testing.T, p db.Product, meters string, loc *countLoc, qty int32) db.Unit {
	t.Helper()
	n := countSeq.Add(1)
	arg := db.CreateUnitParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T206-%s-%d", e.suffix, n),
		UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
	}
	if p.UsesFixedBarcode {
		arg.UnitKind = ledger.KindFixed
	}
	if meters != "" {
		var m pgtype.Numeric
		if err := m.Scan(meters); err != nil {
			t.Fatal(err)
		}
		arg.InitialMeters, arg.RemainingMeters = m, m
	}
	u, err := e.q.CreateUnit(e.ctx, arg)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	ref := time.Now().UnixNano()%1_000_000_000 + n
	if p.UsesFixedBarcode {
		e.post(t, ledger.Movement{Type: ledger.TypeEntry, UnitID: u.ID, To: loc.owner(e.center.ID), Quantity: qty, Source: "test", RefType: "t206", RefID: ref})
		return u
	}
	e.post(t, ledger.Movement{Type: ledger.TypeEntry, UnitID: u.ID, To: &ledger.Owner{Type: ledger.OwnerOrganization, ID: e.center.ID}, Source: "test", RefType: "t206", RefID: ref})
	if loc != nil {
		e.post(t, ledger.Movement{Type: ledger.TypePlacement, UnitID: u.ID, To: loc.owner(e.center.ID), Source: "test", RefType: "t206", RefID: ref})
	}
	return u
}

// state returns (owner_type, owner_id, status) of a serial unit.
func (e *countEnv) state(t *testing.T, u db.Unit) (string, int64, string) {
	t.Helper()
	st, err := e.q.GetUnitCurrentState(e.ctx, u.ID)
	if err != nil {
		t.Fatalf("state of %s: %v", u.Barcode, err)
	}
	return st.OwnerType, st.OwnerID, st.Status
}

func (e *countEnv) holding(t *testing.T, u db.Unit, loc countLoc) int32 {
	t.Helper()
	n, err := e.q.GetFixedHoldingQuantity(e.ctx, db.GetFixedHoldingQuantityParams{UnitID: u.ID, OwnerType: "warehouse_location", OwnerID: loc.id})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *countEnv) remaining(t *testing.T, u db.Unit) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(e.ctx, `SELECT remaining_meters::text FROM units WHERE id = $1`, u.ID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *countEnv) movements(t *testing.T, u db.Unit) []string {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `SELECT type || '|' || idempotency_key FROM stock_movements WHERE unit_id = $1 ORDER BY id`, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func lineFor(t *testing.T, rep wh.CountReport, barcode string) wh.CountLine {
	t.Helper()
	for _, l := range rep.Lines {
		if l.Unit != nil && l.Unit.Barcode == barcode {
			return l
		}
	}
	t.Fatalf("no line for %s in %+v", barcode, rep.Lines)
	return wh.CountLine{}
}

func mustScan(t *testing.T, e *countEnv, c wh.ScanCaller, id uuid.UUID, in wh.CountScanInput) wh.CountScanResult {
	t.Helper()
	res, err := e.counts.Scan(e.ctx, c, id, in)
	if err != nil {
		t.Fatalf("scan %q: %v", in.Code, err)
	}
	return res
}

// location_first, blind, warehouse scope: the full flow.
func TestStockCountLocationFirstBlind(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "B", "A1", "A2")
	l1, l2 := locs[0], locs[1]
	_, _, other := e.warehouse(t, "O", "X1")

	a := e.unit(t, e.piece, "", &l1, 0)
	b := e.unit(t, e.piece, "", &l1, 0)
	moved := e.unit(t, e.piece, "", &l2, 0) // found on A1
	missing := e.unit(t, e.piece, "", &l1, 0)
	roll := e.unit(t, e.roll, "10", &l1, 0)
	fixed := e.unit(t, e.fixed, "", &l1, 10)

	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cnt.Status != wh.CountStatusDraft || cnt.ScopeType != wh.CountScopeWarehouse || cnt.StartApprovalRequired {
		t.Fatalf("draft = %+v", cnt)
	}
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: a.Barcode}); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("scan draft: %v", err)
	}
	if cnt, err = e.counts.Start(e.ctx, c, cnt.UUID); err != nil || cnt.Status != wh.CountStatusInProgress {
		t.Fatalf("start: %+v %v", cnt, err)
	}
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: a.Barcode}); !errors.Is(err, wh.ErrCountLocationRequired) {
		t.Fatalf("unit before location: %v", err)
	}
	// Out of scope: another warehouse's location, by QR or by uuid.
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: other[0].qr()}); !errors.Is(err, wh.ErrCountLocationOutOfScope) {
		t.Fatalf("out of scope location: %v", err)
	}
	ou := other[0].view.UUID.String()
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: a.Barcode, LocationUUID: &ou}); !errors.Is(err, wh.ErrCountLocationOutOfScope) {
		t.Fatalf("out of scope location_uuid: %v", err)
	}

	res := mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: l1.qr()})
	if res.Scan.Kind != wh.CountScanLocation || res.LocationContext == nil || res.LocationContext.UUID != l1.view.UUID {
		t.Fatalf("location scan = %+v", res)
	}
	meters, qty := "8.5", int32(7)
	for _, in := range []wh.CountScanInput{
		{Code: a.Barcode}, {Code: "OFW:UNIT:" + b.Barcode}, {Code: moved.Barcode},
		{Code: roll.Barcode, Meters: &meters}, {Code: fixed.Barcode, Quantity: &qty},
	} {
		res := mustScan(t, e, c, cnt.UUID, in)
		if res.Expected != nil {
			t.Fatalf("blind scan answered expected: %+v", res.Expected)
		}
		raw, _ := json.Marshal(res)
		if strings.Contains(string(raw), `"expected"`) {
			t.Fatalf("blind scan JSON has expected: %s", raw)
		}
		if res.LocationContext == nil || res.LocationContext.UUID != l1.view.UUID {
			t.Fatalf("context = %+v", res.LocationContext)
		}
	}
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: a.Barcode}); !errors.Is(err, wh.ErrCountUnitScanned) {
		t.Fatalf("double scan: %v", err)
	}
	detail, err := e.counts.Get(e.ctx, c, cnt.UUID)
	if err != nil || detail.Expected != nil || detail.Progress == nil || detail.Progress.SerialUnits != 4 || detail.Progress.FixedQuantity != 7 {
		t.Fatalf("blind detail = %+v %v", detail, err)
	}
	if raw, _ := json.Marshal(detail); strings.Contains(string(raw), `"expected"`) {
		t.Fatalf("blind detail JSON has expected: %s", raw)
	}
	if _, err := e.counts.Report(e.ctx, c, cnt.UUID); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("report before completion: %v", err)
	}

	if cnt, err = e.counts.Complete(e.ctx, c, cnt.UUID); err != nil || cnt.Status != wh.CountStatusPendingReview || cnt.Summary == nil {
		t.Fatalf("complete: %+v %v", cnt, err)
	}
	rep, err := e.counts.Report(e.ctx, c, cnt.UUID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	want := map[string]string{
		a.Barcode: wh.CountResultMatched, b.Barcode: wh.CountResultMatched, moved.Barcode: wh.CountResultWrongLocation,
		missing.Barcode: wh.CountResultMissing, roll.Barcode: wh.CountResultMeterVariance, fixed.Barcode: wh.CountResultQtyVariance,
	}
	if len(rep.Lines) != len(want) {
		t.Fatalf("lines = %+v", rep.Lines)
	}
	for bc, result := range want {
		if l := lineFor(t, rep, bc); l.Result != result {
			t.Fatalf("%s result = %s, want %s", bc, l.Result, result)
		}
	}
	mv := lineFor(t, rep, moved.Barcode)
	if mv.ExpectedLocation == nil || mv.ExpectedLocation.UUID != l2.view.UUID || mv.CountedLocation == nil || mv.CountedLocation.UUID != l1.view.UUID {
		t.Fatalf("wrong location line = %+v", mv)
	}
	fl := lineFor(t, rep, fixed.Barcode)
	if fl.ExpectedQuantity != 10 || fl.CountedQuantity != 7 {
		t.Fatalf("fixed line = %+v", fl)
	}
	if rl := lineFor(t, rep, roll.Barcode); rl.CountedMeters == nil || *rl.CountedMeters != "8.50" || rl.ExpectedMeters == nil || *rl.ExpectedMeters != "10.00" {
		t.Fatalf("roll line = %+v", rl)
	}
	if cnt.Summary.ByResult[wh.CountResultMatched] != 2 || cnt.Summary.Unresolved != 4 {
		t.Fatalf("summary = %+v", cnt.Summary)
	}
	var csvOut strings.Builder
	if err := e.counts.Export(e.ctx, c, cnt.UUID, &csvOut); err != nil || !strings.Contains(csvOut.String(), missing.Barcode+",") {
		t.Fatalf("export: %v %q", err, csvOut.String())
	}

	// Completion changed no stock.
	if typ, id, st := e.state(t, missing); typ != "warehouse_location" || id != l1.id || st != "placed" {
		t.Fatalf("missing before approval = %s %d %s", typ, id, st)
	}
	if typ, id, _ := e.state(t, moved); typ != "warehouse_location" || id != l2.id {
		t.Fatalf("moved before approval = %s %d", typ, id)
	}
	if n := e.holding(t, fixed, l1); n != 10 {
		t.Fatalf("fixed before approval = %d", n)
	}
	if got := e.remaining(t, roll); got != "10.00" {
		t.Fatalf("roll before approval = %s", got)
	}

	resolutions := []wh.CountResolution{
		{LineUUID: mv.UUID.String(), Resolution: wh.CountResolveRelocate},
		{LineUUID: lineFor(t, rep, missing.Barcode).UUID.String(), Resolution: wh.CountResolveVoidMissing},
		{LineUUID: lineFor(t, rep, roll.Barcode).UUID.String(), Resolution: wh.CountResolveIncreaseUnlocated},
	}
	var ve *wh.ValidationError
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, resolutions); !errors.As(err, &ve) {
		t.Fatalf("approve with an unresolved line: %v", err)
	}
	bad := append([]wh.CountResolution{{LineUUID: fl.UUID.String(), Resolution: wh.CountResolveVoidMissing}}, resolutions...)
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, bad); !errors.As(err, &ve) {
		t.Fatalf("approve with a disallowed resolution: %v", err)
	}
	resolutions = append(resolutions, wh.CountResolution{LineUUID: fl.UUID.String(), Resolution: wh.CountResolveIncreaseUnlocated})
	if _, err := e.counts.Approve(e.ctx, e.caller(false), cnt.UUID, resolutions); !errors.Is(err, wh.ErrCountAdjustForbidden) {
		t.Fatalf("approve without stock.adjust: %v", err)
	}
	if typ, _, st := e.state(t, missing); typ != "warehouse_location" || st != "placed" {
		t.Fatalf("refused approvals changed stock: %s %s", typ, st)
	}

	rep, err = e.counts.Approve(e.ctx, c, cnt.UUID, resolutions)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if rep.Count.Status != wh.CountStatusApproved || rep.Count.ApprovedAt == nil {
		t.Fatalf("approved = %+v", rep.Count)
	}
	// Projections equal the counted values.
	if typ, id, st := e.state(t, moved); typ != "warehouse_location" || id != l1.id || st != "placed" {
		t.Fatalf("moved after approval = %s %d %s", typ, id, st)
	}
	if typ, _, st := e.state(t, missing); typ != "trash" || st != "void" {
		t.Fatalf("missing after approval = %s %s", typ, st)
	}
	if n := e.holding(t, fixed, l1); n != 7 {
		t.Fatalf("fixed after approval = %d", n)
	}
	if got := e.remaining(t, roll); got != "8.50" {
		t.Fatalf("roll after approval = %s", got)
	}
	var onL1, onL2 int32
	if err := e.pool.QueryRow(e.ctx, `SELECT COALESCE(SUM(quantity) FILTER (WHERE location_id = $1), 0)::int,
		COALESCE(SUM(quantity) FILTER (WHERE location_id = $2), 0)::int FROM bin_product_stocks WHERE product_id = $3`,
		l1.id, l2.id, e.piece.ID).Scan(&onL1, &onL2); err != nil {
		t.Fatal(err)
	}
	if onL1 != 3 || onL2 != 0 { // a, b, moved counted on A1; missing voided
		t.Fatalf("bin stock A1=%d A2=%d", onL1, onL2)
	}
	ml := lineFor(t, rep, missing.Barcode)
	if got := e.movements(t, missing); got[len(got)-1] != fmt.Sprintf("void|stock_count:stock_count_line:%d:void:%s", lineID(t, e, ml.UUID), missing.Barcode) {
		t.Fatalf("void movement = %v", got)
	}
	if got := e.movements(t, moved); !strings.HasPrefix(got[len(got)-1], "placement|stock_count:stock_count_line:") {
		t.Fatalf("placement movement = %v", got)
	}
	if got := e.movements(t, fixed); !strings.HasPrefix(got[len(got)-1], "count_adjustment|stock_count:") {
		t.Fatalf("fixed movement = %v", got)
	}
	if got := e.movements(t, a); len(got) != 2 {
		t.Fatalf("matched unit moved: %v", got)
	}
	for _, l := range rep.Lines {
		if l.Result != wh.CountResultMatched && (l.Resolution == nil || l.ResolvedAt == nil) {
			t.Fatalf("unresolved after approval: %+v", l)
		}
	}

	// Approving again is refused and writes nothing.
	before := len(e.movements(t, fixed))
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, resolutions); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("second approve: %v", err)
	}
	if n := len(e.movements(t, fixed)); n != before {
		t.Fatalf("second approve wrote %d movements", n-before)
	}
	if _, err := e.counts.Cancel(e.ctx, c, cnt.UUID); !errors.Is(err, wh.ErrCountStatus) {
		t.Fatalf("cancel approved: %v", err)
	}
}

func lineID(t *testing.T, e *countEnv, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := e.pool.QueryRow(e.ctx, `SELECT id FROM stock_count_lines WHERE uuid = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// unit_first, guided, location scope: expected values are answered, a
// unit counted without a location matches, an unscanned unit is missing
// and "ignore" leaves the stock as it is. A cancelled count changes nothing.
func TestStockCountUnitFirstGuided(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "G", "A1", "A2")
	l1, l2 := locs[0], locs[1]
	here := e.unit(t, e.piece, "", &l1, 0)
	gone := e.unit(t, e.piece, "", &l1, 0)
	e.unit(t, e.piece, "", &l2, 0) // outside the location scope

	locID := l1.view.UUID.String()
	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodUnitFirst,
		Visibility: wh.CountVisibilityGuided, ScopeType: wh.CountScopeLocation, LocationUUID: &locID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cnt, err = e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatalf("start: %v", err)
	}
	detail, err := e.counts.Get(e.ctx, c, cnt.UUID)
	if err != nil || detail.Expected == nil || detail.Expected.SerialUnits != 2 || len(detail.Expected.Locations) != 1 {
		t.Fatalf("guided detail = %+v %v", detail.Expected, err)
	}
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: l2.qr()}); !errors.Is(err, wh.ErrCountLocationOutOfScope) {
		t.Fatalf("sibling location: %v", err)
	}
	res := mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: here.Barcode})
	if res.Scan.Location != nil || res.Expected == nil || !res.Expected.InScope || res.Expected.Location == nil || res.Expected.Location.UUID != l1.view.UUID {
		t.Fatalf("guided unit scan = %+v / %+v", res.Scan, res.Expected)
	}
	if cnt, err = e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	rep, err := e.counts.Report(e.ctx, c, cnt.UUID)
	if err != nil || len(rep.Lines) != 2 {
		t.Fatalf("report = %+v %v", rep.Lines, err)
	}
	if l := lineFor(t, rep, here.Barcode); l.Result != wh.CountResultMatched {
		t.Fatalf("here = %+v", l)
	}
	gl := lineFor(t, rep, gone.Barcode)
	if gl.Result != wh.CountResultMissing || len(gl.AllowedResolutions) != 2 {
		t.Fatalf("gone = %+v", gl)
	}
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, []wh.CountResolution{{LineUUID: gl.UUID.String(), Resolution: wh.CountResolveIgnore}}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if typ, id, st := e.state(t, gone); typ != "warehouse_location" || id != l1.id || st != "placed" {
		t.Fatalf("ignored unit moved: %s %d %s", typ, id, st)
	}

	// Cancelled count: no stock change.
	cnt2, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt2.UUID); err != nil {
		t.Fatal(err)
	}
	mustScan(t, e, c, cnt2.UUID, wh.CountScanInput{Code: l2.qr()})
	mustScan(t, e, c, cnt2.UUID, wh.CountScanInput{Code: here.Barcode})
	if cnt2, err = e.counts.Cancel(e.ctx, c, cnt2.UUID); err != nil || cnt2.Status != wh.CountStatusCancelled {
		t.Fatalf("cancel: %+v %v", cnt2, err)
	}
	if typ, id, _ := e.state(t, here); typ != "warehouse_location" || id != l1.id {
		t.Fatalf("cancelled count moved the unit: %s %d", typ, id)
	}
}

// product_qty: SKUs of serial products and fixed barcodes are counted by
// quantity; a serial unit scan is refused; the fixed quantity is set to the
// counted value on approval.
func TestStockCountProductQty(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "P", "A1")
	l1 := locs[0]
	s1 := e.unit(t, e.piece, "", &l1, 0)
	e.unit(t, e.piece, "", &l1, 0)
	fixed := e.unit(t, e.fixed, "", &l1, 5)

	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodProductQty, Visibility: wh.CountVisibilityBlind})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: l1.qr()})
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: s1.Barcode}); !errors.Is(err, wh.ErrCountScanKind) {
		t.Fatalf("serial scan in product_qty: %v", err)
	}
	if _, err := e.counts.Scan(e.ctx, c, cnt.UUID, wh.CountScanInput{Code: e.fixed.Sku}); !errors.Is(err, wh.ErrCountScanKind) {
		t.Fatalf("fixed product SKU: %v", err)
	}
	one, six := int32(1), int32(6)
	res := mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: e.piece.Sku, Quantity: &one})
	if res.Scan.Kind != wh.CountScanProduct || res.Scan.Quantity != 1 {
		t.Fatalf("product scan = %+v", res.Scan)
	}
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: fixed.Barcode, Quantity: &six})
	if _, err := e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	rep, err := e.counts.Report(e.ctx, c, cnt.UUID)
	if err != nil || len(rep.Lines) != 2 {
		t.Fatalf("report = %+v %v", rep.Lines, err)
	}
	var res2 []wh.CountResolution
	for _, l := range rep.Lines {
		switch l.LineKind {
		case wh.CountScanProduct:
			if l.Result != wh.CountResultQtyVariance || l.ExpectedQuantity != 2 || l.CountedQuantity != 1 || len(l.AllowedResolutions) != 1 {
				t.Fatalf("product line = %+v", l)
			}
			res2 = append(res2, wh.CountResolution{LineUUID: l.UUID.String(), Resolution: wh.CountResolveIgnore})
		case wh.CountScanFixed:
			if l.Result != wh.CountResultQtyVariance || l.ExpectedQuantity != 5 || l.CountedQuantity != 6 {
				t.Fatalf("fixed line = %+v", l)
			}
			res2 = append(res2, wh.CountResolution{LineUUID: l.UUID.String(), Resolution: wh.CountResolveIncreaseUnlocated})
		}
	}
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, res2); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if n := e.holding(t, fixed, l1); n != 6 {
		t.Fatalf("fixed after approval = %d", n)
	}
}

// initial_placement: the start needs an approval; unlocated units scanned
// into a location are placed on approval.
func TestStockCountInitialPlacement(t *testing.T) {
	e := newCountEnv(t)
	c := e.caller(true)
	w, _, locs := e.warehouse(t, "I", "A1")
	l1 := locs[0]
	loose := e.unit(t, e.piece, "", nil, 0)

	if _, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodInitialPlacement,
		Visibility: wh.CountVisibilityGuided, ScopeType: wh.CountScopeProduct, ProductUUID: ptrStr(e.piece.Uuid.String())}); err == nil {
		t.Fatal("initial placement with a product scope accepted")
	}
	cnt, err := e.counts.Create(e.ctx, c, wh.CountInput{WarehouseUUID: w.UUID.String(), Method: wh.CountMethodInitialPlacement, Visibility: wh.CountVisibilityGuided})
	if err != nil {
		t.Fatal(err)
	}
	if !cnt.StartApprovalRequired {
		t.Fatalf("count = %+v", cnt)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt.UUID); !errors.Is(err, wh.ErrCountStartApproval) {
		t.Fatalf("start without approval: %v", err)
	}
	if cnt, err = e.counts.ApproveStart(e.ctx, c, cnt.UUID); err != nil || cnt.StartApprovedAt == nil {
		t.Fatalf("approve start: %+v %v", cnt, err)
	}
	if _, err := e.counts.Start(e.ctx, c, cnt.UUID); err != nil {
		t.Fatalf("start: %v", err)
	}
	mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: l1.qr()})
	res := mustScan(t, e, c, cnt.UUID, wh.CountScanInput{Code: loose.Barcode})
	if res.Expected == nil || !res.Expected.InScope || res.Expected.Location != nil {
		t.Fatalf("guided initial scan = %+v", res.Expected)
	}
	if _, err := e.counts.Complete(e.ctx, c, cnt.UUID); err != nil {
		t.Fatal(err)
	}
	rep, err := e.counts.Report(e.ctx, c, cnt.UUID)
	if err != nil {
		t.Fatal(err)
	}
	// Every other unlocated stock of the center is a difference too: ignore it.
	var in []wh.CountResolution
	for _, l := range rep.Lines {
		switch {
		case l.Unit != nil && l.Unit.Barcode == loose.Barcode:
			if l.Result != wh.CountResultUnlocated {
				t.Fatalf("loose line = %+v", l)
			}
			in = append(in, wh.CountResolution{LineUUID: l.UUID.String(), Resolution: wh.CountResolveRelocate})
		case l.Result != wh.CountResultMatched:
			in = append(in, wh.CountResolution{LineUUID: l.UUID.String(), Resolution: wh.CountResolveIgnore})
		}
	}
	if typ, _, _ := e.state(t, loose); typ != "organization" {
		t.Fatalf("loose placed before approval: %s", typ)
	}
	if _, err := e.counts.Approve(e.ctx, c, cnt.UUID, in); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if typ, id, st := e.state(t, loose); typ != "warehouse_location" || id != l1.id || st != "placed" {
		t.Fatalf("loose after approval = %s %d %s", typ, id, st)
	}
}

func ptrStr(s string) *string { return &s }
