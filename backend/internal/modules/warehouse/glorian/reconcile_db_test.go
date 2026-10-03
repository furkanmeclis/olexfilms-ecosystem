package glorian_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-272: the reconcile drift report against the fake hub and the
// migrated database. The report reads the synced units of the connection
// (products pulled from the fake hub) and writes nothing but its
// integration_sync_runs row.

type reconcileUnits struct {
	inBin []db.Unit // entered into a glorian center bin
	label db.Unit   // printed label, no ledger state
}

func (f *pullFixture) reconcileBarcode(n int) string { return fmt.Sprintf("GT272-%s-%d", f.suffix, n) }

// reconcileBarcodePattern is a LIKE pattern matching every reconcileBarcode.
func (f *pullFixture) reconcileBarcodePattern() string { return "GT272-" + f.suffix + "-%" }

// reconcileUnits pulls the catalog (products become synced) and creates
// units of the synced product with remote id 10.
func (f *pullFixture) reconcileUnits(t *testing.T) reconcileUnits {
	t.Helper()
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("catalog pull: %v", err)
	}
	product, err := f.q.GetProductByConnectionExternalID(f.ctx, db.GetProductByConnectionExternalIDParams{
		ConnectionID: pgtype.Int8{Int64: f.conn.ID, Valid: true}, ExternalID: pgtype.Text{String: "10", Valid: true},
	})
	if err != nil {
		t.Fatalf("synced product: %v", err)
	}
	loc, err := f.q.CreateWarehouseLocation(f.ctx, db.CreateWarehouseLocationParams{
		OrganizationID: f.glorian.ID, Code: "T272-" + f.suffix, Name: "T272 bin", Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	l := ledger.New(f.q, outbox.NewStore(nil, f.q))
	unit := func(n int, enter bool) db.Unit {
		u, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
			OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, ProductID: product.ID,
			Barcode: f.reconcileBarcode(n), UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		})
		if err != nil {
			t.Fatalf("unit %d: %v", n, err)
		}
		if enter {
			if _, err := l.Post(f.ctx, f.tx, ledger.Movement{
				Type: ledger.TypeEntry, UnitID: u.ID,
				To:     &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: f.glorian.ID},
				Source: "test", RefType: "t272", RefID: time.Now().UnixNano(),
			}); err != nil {
				t.Fatalf("entry %d: %v", n, err)
			}
		}
		return u
	}
	var s reconcileUnits
	s.inBin = []db.Unit{unit(1, true), unit(2, true), unit(3, true)}
	s.label = unit(4, false)
	return s
}

func (f *pullFixture) hubItem(id, barcode, product string, dealer any, status, location string) fake.Row {
	return fake.Row{
		"id": id, "barcode": barcode, "product_id": product, "dealer_id": dealer,
		"status": status, "location": location, "updated_at": "2026-09-20T12:00:00+00:00",
	}
}

// inSyncHub is the hub state matching the local units.
func (f *pullFixture) inSyncHub() []fake.Row {
	return []fake.Row{
		f.hubItem("72001", f.reconcileBarcode(1), "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		f.hubItem("72002", f.reconcileBarcode(2), "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		f.hubItem("72003", f.reconcileBarcode(3), "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		f.hubItem("72004", f.reconcileBarcode(4), "10", nil, glorian.StockStatusReserved, glorian.StockLocationCenter),
	}
}

// fixtureUnits selects the units this fixture owns: units of the products
// synced through its own connection, plus any unit carrying one of its
// barcodes (so a unit the report wrongly created would still show up). Package tests run in parallel against
// the shared CI database and commit units, movements and projection rows
// for the same seeded brand, so the snapshot must not look beyond them.
const fixtureUnits = `SELECT u.id FROM units u JOIN products p ON p.id = u.product_id WHERE p.connection_id = $1 OR u.barcode LIKE $2`

// writeSnapshot fingerprints the business rows the report must not touch:
// the fixture's units, their stock ledger and their unit projection.
func (f *pullFixture) writeSnapshot(t *testing.T) string {
	t.Helper()
	var units, moves, state string
	if err := f.tx.QueryRow(f.ctx,
		`SELECT count(*)::text || ':' || coalesce(md5(string_agg(u::text, '|' ORDER BY u.id)), '') FROM units u WHERE u.id IN (`+fixtureUnits+`)`,
		f.conn.ID, f.reconcileBarcodePattern()).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx,
		`SELECT count(*)::text || ':' || coalesce(md5(string_agg(m::text, '|' ORDER BY m.id)), '') FROM stock_movements m WHERE m.unit_id IN (`+fixtureUnits+`)`,
		f.conn.ID, f.reconcileBarcodePattern()).Scan(&moves); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx,
		`SELECT count(*)::text || ':' || coalesce(md5(string_agg(s::text, '|' ORDER BY s.unit_id)), '') FROM unit_current_state s WHERE s.unit_id IN (`+fixtureUnits+`)`,
		f.conn.ID, f.reconcileBarcodePattern()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return units + "/" + moves + "/" + state
}

func (f *pullFixture) reconciler() *glorian.Reconciler {
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: f.srv.Client(), RetryBaseDelay: time.Millisecond})
	return glorian.NewReconciler(f.q, f.box, factory, nil)
}

func (f *pullFixture) reconcileOnce(t *testing.T) (glorian.ReconcileReport, int, glorian.ReconcileCounts) {
	t.Helper()
	before := f.writeSnapshot(t)
	rep, err := f.reconciler().ReconcileConnection(f.ctx, f.conn)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if after := f.writeSnapshot(t); after != before {
		t.Fatalf("reconcile changed units/stock_movements/unit_current_state:\n%s\n%s", before, after)
	}
	run := latestRun(t, f.runs(t), glorian.KindReconcile)
	if run.Status != glorian.RunSucceeded || run.Uuid != rep.RunUUID || run.Watermark.Valid {
		t.Fatalf("reconcile run = %+v", run)
	}
	var c glorian.ReconcileCounts
	if err := json.Unmarshal(run.Counts, &c); err != nil {
		t.Fatal(err)
	}
	return rep, glorian.ReconcileExitCode([]glorian.ReconcileReport{rep}, nil), c
}

func TestReconcileDBInSync(t *testing.T) {
	f := newPullFixture(t, true)
	f.reconcileUnits(t)
	f.srv.SetFixture(fake.FixtureStockItems, f.inSyncHub())
	// The stock pull mirrors the hub status first (external_status).
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("stock pull: %v", err)
	}
	rep, code, c := f.reconcileOnce(t)
	if rep.HasDrift() || code != glorian.ReconcileExitClean || rep.Remote != 4 || rep.Local != 4 {
		t.Fatalf("report = %+v (exit %d); want 4/4 without drift", rep.Summary, code)
	}
	if c.Total() != 0 || c.Remote != 4 || c.Local != 4 {
		t.Fatalf("run counts = %+v", c)
	}
}

func TestReconcileDBCountsEveryDriftKind(t *testing.T) {
	f := newPullFixture(t, true)
	s := f.reconcileUnits(t)
	f.srv.SetFixture(fake.FixtureStockItems, f.inSyncHub())
	if err := f.puller().Run(f.ctx); err != nil {
		t.Fatalf("stock pull: %v", err)
	}
	// A unit entered after the pull is not on the hub: only_local.
	extra, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
		OrganizationID: f.glorian.ID, BrandID: f.glorian.BrandID, ProductID: s.label.ProductID,
		Barcode: f.reconcileBarcode(5), UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		t.Fatalf("extra unit: %v", err)
	}
	// The hub drifts without a new pull.
	f.srv.SetFixture(fake.FixtureStockItems, []fake.Row{
		// status_drift: mirrored available, now used on the hub.
		f.hubItem("72001", f.reconcileBarcode(1), "10", nil, glorian.StockStatusUsed, glorian.StockLocationCenter),
		// owner_drift: in a center bin locally, at a dealer on the hub.
		f.hubItem("72002", f.reconcileBarcode(2), "10", "1", glorian.StockStatusAvailable, glorian.StockLocationDealer),
		f.hubItem("72003", f.reconcileBarcode(3), "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
		// product_drift: hub product 11, local product 10.
		f.hubItem("72004", f.reconcileBarcode(4), "11", nil, glorian.StockStatusReserved, glorian.StockLocationCenter),
		// only_remote: no local unit.
		f.hubItem("72099", f.reconcileBarcode(99), "10", nil, glorian.StockStatusAvailable, glorian.StockLocationCenter),
	})
	rep, code, c := f.reconcileOnce(t)
	want := glorian.ReconcileSummary{OnlyRemote: 1, OnlyLocal: 1, StatusDrift: 1, ProductDrift: 1, OwnerDrift: 1}
	if rep.Summary != want {
		t.Fatalf("summary = %+v, want %+v", rep.Summary, want)
	}
	if code == glorian.ReconcileExitClean {
		t.Fatal("drift must give a non-zero exit code")
	}
	if c.ReconcileSummary != want {
		t.Fatalf("run counts = %+v, want %+v", c.ReconcileSummary, want)
	}
	checks := []struct {
		name    string
		rows    []glorian.DriftRow
		barcode string
	}{
		{glorian.DriftOnlyRemote, c.Details.OnlyRemote, f.reconcileBarcode(99)},
		{glorian.DriftOnlyLocal, c.Details.OnlyLocal, extra.Barcode},
		{glorian.DriftStatus, c.Details.StatusDrift, s.inBin[0].Barcode},
		{glorian.DriftOwner, c.Details.OwnerDrift, s.inBin[1].Barcode},
		{glorian.DriftProduct, c.Details.ProductDrift, s.label.Barcode},
	}
	for _, ch := range checks {
		if len(ch.rows) != 1 || ch.rows[0].Barcode != ch.barcode {
			t.Fatalf("%s details = %+v, want %s", ch.name, ch.rows, ch.barcode)
		}
	}
}
