package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-207 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18): the end-of-day summary is built from the day's ledger
// movements (TEC-204 stock entry, service consumption, transfers), per
// warehouse and for the whole organization; the cron writes the previous
// day once; the distributor preset opens a warehouse when the record is
// saved.

type eodMove struct {
	org, unit, product int64
	typ                string
	qty                int32
	meters             string
	fromType, toType   string
	fromID, toID       int64
	at                 time.Time
}

// rawMovement appends a ledger row directly (fixture only: the report
// reads stock_movements, the projections do not matter here).
func (e *entryEnv) rawMovement(t *testing.T, m eodMove) {
	t.Helper()
	opt := func(typ string, id int64) (any, any) {
		if typ == "" {
			return nil, nil
		}
		return typ, id
	}
	ft, fid := opt(m.fromType, m.fromID)
	tt, tid := opt(m.toType, m.toID)
	if m.meters == "" {
		m.meters = "0"
	}
	_, err := e.pool.Exec(e.ctx, `INSERT INTO stock_movements (organization_id, brand_id, unit_id, product_id, type,
		quantity_delta, meters_delta, from_owner_type, from_owner_id, to_owner_type, to_owner_id, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::numeric, $8, $9, $10, $11, $12, $13)`,
		m.org, e.brand.ID, m.unit, m.product, m.typ, m.qty, m.meters, ft, fid, tt, tid,
		"t207:"+uuid.NewString(), m.at)
	if err != nil {
		t.Fatalf("raw %s movement: %v", m.typ, err)
	}
}

func (e *entryEnv) locationID(t *testing.T, l wh.Location) int64 {
	t.Helper()
	var id int64
	if err := e.pool.QueryRow(e.ctx, `SELECT id FROM warehouse_locations WHERE uuid = $1`, l.UUID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func dayStart(t *testing.T, tz string, now time.Time) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc = time.UTC
	}
	start, _ := wh.EODDay(now.In(loc), loc)
	return start
}

func groupOf(t *testing.T, s wh.EODSummary, g string) wh.EODGroupTotal {
	t.Helper()
	for _, x := range s.Groups {
		if x.Group == g {
			return x
		}
	}
	t.Fatalf("group %s missing", g)
	return wh.EODGroupTotal{}
}

// A confirmed stock entry (3 pieces), a consumption, a partial roll
// consumption and a transfer out of another warehouse: the warehouse report
// counts exactly its own movements of the day; yesterday's movement and
// the other warehouse stay out; the system report includes both
// warehouses.
func TestEODReportSummarizesDay(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, loc := e.location(t, e.center, "E")
	_, loc2 := e.location(t, e.center, "F")
	locID, loc2ID := e.locationID(t, loc), e.locationID(t, loc2)

	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err = e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: 3, Prefix: e.prefix("D")}); err != nil {
		t.Fatalf("add lines: %v", err)
	}
	if _, err = e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{LocationCode: "OFW:LOC:" + loc.FullCode}); err != nil {
		t.Fatalf("place: %v", err)
	}
	if entry, err = e.entries.Confirm(e.ctx, c, entry.UUID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	units := make([]int64, 0, 3)
	for _, l := range entry.Lines {
		u, err := e.q.GetUnitByUUID(e.ctx, l.UnitUUID)
		if err != nil {
			t.Fatal(err)
		}
		units = append(units, u.ID)
	}
	if len(units) != 3 {
		t.Fatalf("units = %v", units)
	}

	now := time.Now()
	start := dayStart(t, e.center.Timezone, now)
	// Service consumption of unit 1 and a partial (roll) consumption of
	// unit 2 out of the warehouse; a transfer out of the other warehouse;
	// a placement yesterday.
	e.rawMovement(t, eodMove{org: e.center.ID, unit: units[0], product: e.piece.ID, typ: "consumption", qty: -1,
		fromType: "warehouse_location", fromID: locID, toType: "service", toID: 987654321, at: now})
	e.rawMovement(t, eodMove{org: e.center.ID, unit: units[1], product: e.piece.ID, typ: "partial_consumption",
		meters: "-1.50", fromType: "warehouse_location", fromID: locID, at: now})
	e.rawMovement(t, eodMove{org: e.center.ID, unit: units[2], product: e.piece.ID, typ: "transfer_out", qty: -1,
		fromType: "warehouse_location", fromID: loc2ID, toType: "organization", toID: e.center.ID, at: now})
	e.rawMovement(t, eodMove{org: e.center.ID, unit: units[2], product: e.piece.ID, typ: "placement",
		fromType: "organization", fromID: e.center.ID, toType: "warehouse_location", toID: locID, at: start.Add(-time.Hour)})

	eod := wh.NewEOD(e.pool, e.q)
	wUUID := w.UUID.String()
	rep, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{WarehouseUUID: &wUUID})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if rep.Kind != wh.EODKindManual || rep.Warehouse == nil || rep.Warehouse.UUID != w.UUID || !rep.PeriodStart.Equal(start) {
		t.Fatalf("report = %+v", rep)
	}
	s := rep.Summary
	if g := groupOf(t, s, wh.EODGroupEntry); g.MovementCount != 3 || g.UnitCount != 3 || g.QuantityIn != 3 {
		t.Fatalf("entry = %+v", g)
	}
	if g := groupOf(t, s, wh.EODGroupPlacement); g.MovementCount != 3 {
		t.Fatalf("placement (yesterday's must stay out) = %+v", g)
	}
	if g := groupOf(t, s, wh.EODGroupConsumption); g.MovementCount != 2 || g.QuantityOut != 1 || g.MetersOut != "1.50" {
		t.Fatalf("consumption = %+v", g)
	}
	if g := groupOf(t, s, wh.EODGroupTransfer); g.MovementCount != 0 {
		t.Fatalf("the other warehouse's transfer leaked in: %+v", g)
	}
	if s.Totals.MovementCount != 8 || len(s.Products) != 4 || s.Products[0].ProductUUID != e.piece.Uuid {
		t.Fatalf("summary = %+v", s)
	}

	// Regenerating the same day rewrites the row (same uuid).
	again, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{Date: rep.ReportDate, WarehouseUUID: &wUUID})
	if err != nil || again.UUID != rep.UUID {
		t.Fatalf("regenerate: %+v, %v", again, err)
	}
	// The center system report covers both warehouses (other tests may add
	// movements to the center in parallel: lower bounds only).
	sys, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{})
	if err != nil {
		t.Fatalf("system: %v", err)
	}
	if sys.Warehouse != nil || groupOf(t, sys.Summary, wh.EODGroupTransfer).MovementCount < 1 ||
		groupOf(t, sys.Summary, wh.EODGroupEntry).MovementCount < 3 || sys.Summary.Totals.MovementCount < 9 {
		t.Fatalf("system = %+v", sys.Summary)
	}
	got, err := eod.Get(e.ctx, c.Caller, rep.UUID)
	if err != nil || got.Summary.Totals.MovementCount != 8 {
		t.Fatalf("get: %+v, %v", got, err)
	}
	// Another organization cannot read it; a future day is refused.
	if _, err := eod.Get(e.ctx, e.caller(e.dist).Caller, rep.UUID); !errors.Is(err, wh.ErrEODReportNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	future := now.AddDate(0, 0, 3).Format("2006-01-02")
	var ve *wh.ValidationError
	if _, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{Date: future}); !errors.As(err, &ve) {
		t.Fatalf("future day: %v", err)
	}
}

// The cron writes the previous local day of a distributor once (auto);
// movements recorded on the distributor or landing on it are counted
// exactly; a manual run then rewrites the row.
func TestEODDailyRunDistributor(t *testing.T) {
	e := newEntryEnv(t)
	dc := e.caller(e.dist)
	if _, err := e.tree.CreateWarehouse(e.ctx, dc.Caller, wh.WarehouseInput{Code: "D" + e.suffix[len(e.suffix)-6:], Name: "dist wh"}); err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	unit, err := e.q.CreateUnit(e.ctx, db.CreateUnitParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.piece.ID,
		Barcode: e.prefix("U") + "01", Status: "printed", UnitKind: "serial", Source: "generated",
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	now := time.Now()
	// Order received by the distributor (recorded on it) and a transfer
	// recorded on the center that lands on the distributor.
	e.rawMovement(t, eodMove{org: e.dist.ID, unit: unit.ID, product: e.piece.ID, typ: "received", qty: 1,
		toType: "organization", toID: e.dist.ID, at: now})
	e.rawMovement(t, eodMove{org: e.center.ID, unit: unit.ID, product: e.piece.ID, typ: "transfer_in", qty: 1,
		toType: "organization", toID: e.dist.ID, at: now})

	eod := wh.NewEOD(e.pool, e.q)
	eod.SetClock(func() time.Time { return now.AddDate(0, 0, 1) })
	n, err := eod.RunOrganization(e.ctx, e.dist.ID)
	if err != nil || n != 2 {
		t.Fatalf("run: %d, %v (system + one warehouse)", n, err)
	}
	if n, err := eod.RunOrganization(e.ctx, e.dist.ID); err != nil || n != 0 {
		t.Fatalf("rerun must write nothing: %d, %v", n, err)
	}
	list, total, err := eod.List(e.ctx, dc.Caller, wh.EODListInput{Scope: wh.EODScopeSystem})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %d %+v %v", total, list, err)
	}
	sys := list[0]
	if sys.Kind != wh.EODKindAuto || sys.Warehouse != nil {
		t.Fatalf("system = %+v", sys)
	}
	if g := groupOf(t, sys.Summary, wh.EODGroupOrder); g.MovementCount != 1 || g.QuantityIn != 1 {
		t.Fatalf("order = %+v", g)
	}
	if g := groupOf(t, sys.Summary, wh.EODGroupTransfer); g.MovementCount != 1 || g.QuantityIn != 1 {
		t.Fatalf("transfer = %+v", g)
	}
	if sys.Summary.Totals.MovementCount != 2 {
		t.Fatalf("totals = %+v", sys.Summary.Totals)
	}
	if _, total, err := eod.List(e.ctx, dc.Caller, wh.EODListInput{Scope: wh.EODScopeWarehouse}); err != nil || total != 1 {
		t.Fatalf("warehouse reports: %d, %v", total, err)
	}
	// A manual run of the same day replaces the auto row.
	manual, err := eod.Generate(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.EODGenerateInput{Date: sys.ReportDate})
	if err != nil || manual.UUID != sys.UUID || manual.Kind != wh.EODKindManual || manual.Summary.Totals.MovementCount != 2 {
		t.Fatalf("manual: %+v, %v", manual, err)
	}
}

// K4 / TEC-207: saving a distributor with the "register as warehouse"
// preset opens its warehouse in the same transaction; a failing preset
// rolls the record back.
func TestDistributorPresetOpensWarehouse(t *testing.T) {
	e := newEntryEnv(t)
	ctx := brandctx.WithBrand(e.ctx, brandctx.Brand{ID: e.brand.ID, UUID: e.brand.Uuid, Slug: e.brand.Slug, Name: e.brand.Name, Status: "active"})
	owner, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "t207-" + e.suffix + "@example.test", Valid: true}, PasswordHash: "x",
		Name: "Preset", Surname: "Owner", Status: "active",
	})
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	orgs := orgusecase.New(e.pool, e.q)
	orgs.SetWarehousePresetHook(e.tree)

	res, err := orgs.RegisterOrganization(ctx, orgusecase.RegisterInput{
		OrganizationName: "t207 dist " + e.suffix, Type: "distributor", RegisterAsWarehouse: true,
		Address: "Depo cad. 1",
	}, owner.ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	org, err := e.q.GetOrganizationByUUID(e.ctx, res.Organization.UUID)
	if err != nil {
		t.Fatal(err)
	}
	whs, err := e.q.ListWarehouses(e.ctx, db.ListWarehousesParams{OrganizationID: org.ID})
	if err != nil || len(whs) != 1 {
		t.Fatalf("warehouses = %+v, %v", whs, err)
	}
	if w := whs[0]; w.Code != wh.PresetWarehouseCode || w.Name != org.Name || !w.Active || w.Address.String != "Depo cad. 1" {
		t.Fatalf("preset warehouse = %+v", w)
	}
	if res.Organization.Settings["register_as_warehouse"] != true {
		t.Fatalf("settings = %+v", res.Organization.Settings)
	}

	// Without the preset no warehouse is opened.
	plain, err := orgs.RegisterOrganization(ctx, orgusecase.RegisterInput{
		OrganizationName: "t207 plain " + e.suffix, Type: "distributor",
	}, owner.ID)
	if err != nil {
		t.Fatalf("register plain: %v", err)
	}
	porg, err := e.q.GetOrganizationByUUID(e.ctx, plain.Organization.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if whs, err := e.q.ListWarehouses(e.ctx, db.ListWarehousesParams{OrganizationID: porg.ID}); err != nil || len(whs) != 0 {
		t.Fatalf("plain warehouses = %+v, %v", whs, err)
	}

	// A failing preset leaves no organization behind.
	boom := errors.New("boom")
	orgs.SetWarehousePresetHook(orgusecase.WarehousePresetHookFunc(func(_ context.Context, _ pgx.Tx, _ db.Organization) error { return boom }))
	name := "t207 rollback " + e.suffix
	if _, err := orgs.RegisterOrganization(ctx, orgusecase.RegisterInput{
		OrganizationName: name, Type: "distributor", RegisterAsWarehouse: true,
	}, owner.ID); !errors.Is(err, boom) {
		t.Fatalf("failing preset: %v", err)
	}
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM organizations WHERE name = $1`, name).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rolled back organization count = %d, %v", n, err)
	}
}
