package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-207 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18): the end-of-day summary is built from the day's ledger
// movements (TEC-204 stock entry, service consumption, moves, transfers,
// orders), per warehouse and for the whole organization; the cron writes
// the previous day once; the distributor preset opens a warehouse when the
// record is saved. Every movement goes through ledger.Post, so the shared
// database stays consistent for the projection rebuild checks of other
// tests.

// post writes one ledger movement in its own transaction.
func (e *entryEnv) post(t *testing.T, m ledger.Movement) {
	t.Helper()
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	if m.Source == "" {
		m.Source, m.RefType, m.RefID = "t207", "t207", time.Now().UnixNano()
	}
	if _, err := ledger.New(e.q, outbox.NewStore(e.pool, e.q)).Post(e.ctx, tx, m); err != nil {
		t.Fatalf("post %s: %v", m.Type, err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
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

// enterUnits confirms a TEC-204 generate_new stock entry of n pieces placed
// on loc of warehouse w (center) and returns the unit ids.
func (e *entryEnv) enterUnits(t *testing.T, w wh.Warehouse, loc wh.Location, n int, letter string) []int64 {
	t.Helper()
	c := e.caller(e.center)
	entry, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err = e.entries.AddLines(e.ctx, c, entry.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: n, Prefix: e.prefix(letter)}); err != nil {
		t.Fatalf("add lines: %v", err)
	}
	if _, err = e.entries.Place(e.ctx, c, entry.UUID, wh.PlaceInput{LocationCode: "OFW:LOC:" + loc.FullCode}); err != nil {
		t.Fatalf("place: %v", err)
	}
	if entry, err = e.entries.Confirm(e.ctx, c, entry.UUID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	units := make([]int64, 0, n)
	for _, l := range entry.Lines {
		u, err := e.q.GetUnitByUUID(e.ctx, l.UnitUUID)
		if err != nil {
			t.Fatal(err)
		}
		units = append(units, u.ID)
	}
	if len(units) != n {
		t.Fatalf("units = %v", units)
	}
	return units
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

// A confirmed stock entry (3 pieces) into warehouse E, then a service
// consumption, a void and a move of the third unit to warehouse F: the
// warehouse report counts exactly its own movements of the day, the
// previous day is empty, the system report includes both warehouses.
func TestEODReportSummarizesDay(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, loc := e.location(t, e.center, "E")
	w2, loc2 := e.location(t, e.center, "F")
	locID, loc2ID := e.locationID(t, loc), e.locationID(t, loc2)
	units := e.enterUnits(t, w, loc, 3, "D")

	from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: locID, OrgID: e.center.ID}
	e.post(t, ledger.Movement{Type: ledger.TypeConsumption, UnitID: units[0], From: &from,
		To: &ledger.Owner{Type: ledger.OwnerService, ID: 987654321, OrgID: e.center.ID}, Reason: "t207 service"})
	e.post(t, ledger.Movement{Type: ledger.TypeVoid, UnitID: units[1], From: &from, Reason: "t207 void"})
	e.post(t, ledger.Movement{Type: ledger.TypePlacement, UnitID: units[2], From: &from,
		To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc2ID, OrgID: e.center.ID}, Reason: "t207 move"})

	eod := wh.NewEOD(e.pool, e.q)
	wUUID := w.UUID.String()
	rep, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{WarehouseUUID: &wUUID})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if rep.Kind != wh.EODKindManual || rep.Warehouse == nil || rep.Warehouse.UUID != w.UUID {
		t.Fatalf("report = %+v", rep)
	}
	s := rep.Summary
	if g := groupOf(t, s, wh.EODGroupEntry); g.MovementCount != 3 || g.UnitCount != 3 || g.QuantityIn != 3 {
		t.Fatalf("entry = %+v", g)
	}
	// 3 entry placements + the move out to warehouse F.
	if g := groupOf(t, s, wh.EODGroupPlacement); g.MovementCount != 4 {
		t.Fatalf("placement = %+v", g)
	}
	if g := groupOf(t, s, wh.EODGroupConsumption); g.MovementCount != 1 || g.QuantityOut != 1 {
		t.Fatalf("consumption = %+v", g)
	}
	if g := groupOf(t, s, wh.EODGroupDisposal); g.MovementCount != 1 || g.QuantityOut != 1 {
		t.Fatalf("disposal = %+v", g)
	}
	if s.Totals.MovementCount != 9 || len(s.Products) != 4 || s.Products[0].ProductUUID != e.piece.Uuid {
		t.Fatalf("summary = %+v", s)
	}

	// Warehouse F only sees the move in.
	w2UUID := w2.UUID.String()
	rep2, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{WarehouseUUID: &w2UUID})
	if err != nil || rep2.Summary.Totals.MovementCount != 1 || groupOf(t, rep2.Summary, wh.EODGroupPlacement).MovementCount != 1 {
		t.Fatalf("warehouse F: %+v, %v", rep2.Summary, err)
	}
	// The previous day has nothing for warehouse E (created today).
	prev, err := time.Parse("2006-01-02", rep.ReportDate)
	if err != nil {
		t.Fatal(err)
	}
	yday, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{
		Date: prev.AddDate(0, 0, -1).Format("2006-01-02"), WarehouseUUID: &wUUID})
	if err != nil || yday.Summary.Totals.MovementCount != 0 {
		t.Fatalf("yesterday: %+v, %v", yday.Summary, err)
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
	if sys.Warehouse != nil || groupOf(t, sys.Summary, wh.EODGroupEntry).MovementCount < 3 ||
		groupOf(t, sys.Summary, wh.EODGroupConsumption).MovementCount < 1 || sys.Summary.Totals.MovementCount < 9 {
		t.Fatalf("system = %+v", sys.Summary)
	}
	got, err := eod.Get(e.ctx, c.Caller, rep.UUID)
	if err != nil || got.Summary.Totals.MovementCount != 9 {
		t.Fatalf("get: %+v, %v", got, err)
	}
	// Another organization cannot read it; a future day is refused.
	if _, err := eod.Get(e.ctx, e.caller(e.dist).Caller, rep.UUID); !errors.Is(err, wh.ErrEODReportNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	future := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	var ve *wh.ValidationError
	if _, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{Date: future}); !errors.As(err, &ve) {
		t.Fatalf("future day: %v", err)
	}
}

// The cron writes the previous local day of a distributor once (auto): a
// transfer and an order from the center to the distributor (out on the
// center, in on the distributor) are counted; a manual run then rewrites
// the row.
func TestEODDailyRunDistributor(t *testing.T) {
	e := newEntryEnv(t)
	dc := e.caller(e.dist)
	if _, err := e.tree.CreateWarehouse(e.ctx, dc.Caller, wh.WarehouseInput{Code: "D" + e.suffix[len(e.suffix)-6:], Name: "dist wh"}); err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	w, loc := e.location(t, e.center, "G")
	units := e.enterUnits(t, w, loc, 2, "G")
	from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: e.locationID(t, loc), OrgID: e.center.ID}
	dist := ledger.Owner{Type: ledger.OwnerOrganization, ID: e.dist.ID, OrgID: e.dist.ID}
	e.post(t, ledger.Movement{Type: ledger.TypeTransferOut, UnitID: units[0], From: &from, To: &dist, Reason: "t207 transfer"})
	e.post(t, ledger.Movement{Type: ledger.TypeTransferIn, UnitID: units[0], To: &dist, Reason: "t207 transfer in"})
	e.post(t, ledger.Movement{Type: ledger.TypeOrderOut, UnitID: units[1], From: &from, To: &dist, Reason: "t207 order"})
	e.post(t, ledger.Movement{Type: ledger.TypeReceived, UnitID: units[1], To: &dist, Reason: "t207 received"})

	now := time.Now()
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
	if g := groupOf(t, sys.Summary, wh.EODGroupTransfer); g.MovementCount != 2 || g.QuantityIn != 1 {
		t.Fatalf("transfer = %+v", g)
	}
	if g := groupOf(t, sys.Summary, wh.EODGroupOrder); g.MovementCount != 2 || g.QuantityIn != 1 {
		t.Fatalf("order = %+v", g)
	}
	if sys.Summary.Totals.MovementCount != 4 {
		t.Fatalf("totals = %+v", sys.Summary.Totals)
	}
	if _, total, err := eod.List(e.ctx, dc.Caller, wh.EODListInput{Scope: wh.EODScopeWarehouse}); err != nil || total != 1 {
		t.Fatalf("warehouse reports: %d, %v", total, err)
	}
	// A manual run of the same day replaces the auto row.
	manual, err := eod.Generate(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.EODGenerateInput{Date: sys.ReportDate})
	if err != nil || manual.UUID != sys.UUID || manual.Kind != wh.EODKindManual || manual.Summary.Totals.MovementCount != 4 {
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
