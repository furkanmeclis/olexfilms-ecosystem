package httpserver

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
)

// TEC-194 acceptance (warranty:repair_scan): a completed service whose
// service.completed event never reached the listener is found by the scan
// and gets its warranties (and warranty.created events) through the
// listener's own path; a second run writes nothing; excluded units
// (no period, external, external_outbound, Glorian) get no warranty from
// the scan either; services outside the look-back window are left alone.
// The fixtures reuse the TEC-186 helpers (directService, serviceWarranties,
// createdEvents).
func TestIntegrationWarrantyRepairScan(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t194-dist", "distributor", center)
	dealer := it.org("t194-dealer", "dealer", dist)
	cust, veh := it.svcCustomer(dealer, "t194-cust", "34R194"+it.suffix[len(it.suffix)-4:])

	p12 := it.product(center, "T194A")
	p0 := it.product(center, "T194Z")
	it.setWarrantyMonths(p12, 12)
	it.setWarrantyMonths(p0, 0)

	c := it.stockChain()
	cLoc := c.location(center, "C194")
	uA := c.unit(center, p12, 19401)
	uZero := c.unit(center, p0, 19402)
	uOut := c.unit(center, p12, 19403)
	uB := c.unit(center, p12, 19404)
	uOld := c.unit(center, p12, 19405)
	c.post(ledger.TypeEntry, uOut, c.nextRef(), cLoc)
	func() {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := c.l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeExternalOutbound, UnitID: uOut.ID, Source: "test", RefType: "t194", RefID: c.nextRef(),
		}); err != nil {
			t.Fatalf("external_outbound: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	uExt, err := it.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: center.BrandID, ProductID: p12.ID,
		Barcode: "T194X-" + it.suffix, UnitKind: ledger.KindSerial, Source: "external", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		t.Fatalf("external unit: %v", err)
	}
	full := func(p db.Product, u db.Unit) db.CreateServiceItemParams {
		return db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"}
	}

	listener := warrantymodule.NewListener(it.pool, it.q, "http://localhost:3000", nil)
	scanner := warrantyusecase.NewRepairScanner(listener, 30, nil)
	// The scan leaves services completed within the grace period to the
	// listener, so the test runs it as if an hour later.
	scanAt := func() time.Time { return time.Now().Add(time.Hour) }

	// A: completed straight in the database, the listener never ran (the
	// outbox row was never processed). One eligible item, three excluded.
	sA := it.directService(dealer, cust, veh, 1, full(p12, uA), full(p0, uZero), full(p12, uOut), full(p12, uExt))
	// B: the listener ran; only its excluded item has no warranty.
	sB := it.directService(dealer, cust, veh, 2, full(p12, uB), full(p0, uZero))
	if _, err := listener.CreateForService(ctx, sB.ID); err != nil {
		t.Fatalf("listener B: %v", err)
	}
	if len(it.serviceWarranties(sB.ID, sB.BrandID)) != 1 || it.createdEvents(sB.ID) != 1 {
		t.Fatal("B: listener did not open its warranty")
	}
	// Old: completed 40 days ago, outside the 30 day window.
	sOld := it.directService(dealer, cust, veh, 3, full(p12, uOld))
	if _, err := it.pool.Exec(ctx, `UPDATE services SET completed_at = now() - interval '40 days' WHERE id = $1`, sOld.ID); err != nil {
		t.Fatalf("old completed_at: %v", err)
	}

	if len(it.serviceWarranties(sA.ID, sA.BrandID)) != 0 || it.createdEvents(sA.ID) != 0 {
		t.Fatal("A must start without warranties")
	}

	// 1. First run: A is repaired (one warranty, one event), B is excluded,
	// the old service is not scanned.
	res, err := scanner.Scan(ctx, scanAt(), dealer.ID)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := warrantyusecase.RepairResult{
		ServicesScanned: 2, ServicesExcluded: 1, ServicesRetried: 1, ServicesRepaired: 1, WarrantiesCreated: 1,
	}
	if res != want {
		t.Fatalf("first scan = %+v, want %+v", res, want)
	}
	wsA := it.serviceWarranties(sA.ID, sA.BrandID)
	if len(wsA) != 1 || wsA[0].UnitID != uA.ID || wsA[0].Status != "active" || wsA[0].HolderUserID != cust.ID ||
		!wsA[0].StartAt.Time.Equal(sA.CompletedAt.Time) {
		t.Fatalf("A warranties = %+v", wsA)
	}
	if n := it.createdEvents(sA.ID); n != 1 {
		t.Fatalf("A warranty.created = %d, want 1", n)
	}
	if len(it.serviceWarranties(sB.ID, sB.BrandID)) != 1 || it.createdEvents(sB.ID) != 1 {
		t.Fatal("B must keep exactly its listener warranty")
	}
	if len(it.serviceWarranties(sOld.ID, sOld.BrandID)) != 0 || it.createdEvents(sOld.ID) != 0 {
		t.Fatal("the service outside the window must not be repaired")
	}
	var excluded int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM warranties WHERE unit_id = ANY($1)`,
		[]int64{uZero.ID, uOut.ID, uExt.ID}).Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if excluded != 0 {
		t.Fatalf("excluded units got %d warranties", excluded)
	}

	// 2. Second run: nothing new (A and B only have excluded items left).
	countAll := func() (rows, evs int) {
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM warranties WHERE organization_id = $1`, dealer.ID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'warranty.created'
			AND (payload->'data'->>'organization_id')::bigint = $1`, dealer.ID).Scan(&evs); err != nil {
			t.Fatal(err)
		}
		return rows, evs
	}
	rowsBefore, evsBefore := countAll()
	res, err = scanner.Scan(ctx, scanAt(), dealer.ID)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if res.WarrantiesCreated != 0 || res.ServicesRetried != 0 || res.ServicesExcluded != 2 {
		t.Fatalf("second scan = %+v", res)
	}
	if rowsAfter, evsAfter := countAll(); rowsAfter != rowsBefore || evsAfter != evsBefore {
		t.Fatalf("second scan wrote rows %d -> %d, events %d -> %d", rowsBefore, rowsAfter, evsBefore, evsAfter)
	}

	// 3. Glorian: a completed Glorian service is excluded by the scan too.
	gCenter := it.brandCenter("glorian")
	gDist := it.org("t194-gdist", "distributor", gCenter)
	gDealer := it.org("t194-gdealer", "dealer", gDist)
	gCust, gVeh := it.svcCustomer(gDealer, "t194-gcust", "34H194"+it.suffix[len(it.suffix)-4:])
	gp := it.product(gCenter, "T194G")
	it.setWarrantyMonths(gp, 12)
	gu := c.unit(gCenter, gp, 19406)
	gs := it.directService(gDealer, gCust, gVeh, 4, full(gp, gu))
	res, err = scanner.Scan(ctx, scanAt(), gDealer.ID)
	if err != nil {
		t.Fatalf("glorian scan: %v", err)
	}
	if res.ServicesScanned != 1 || res.ServicesExcluded != 1 || res.WarrantiesCreated != 0 ||
		len(it.serviceWarranties(gs.ID, gs.BrandID)) != 0 || it.createdEvents(gs.ID) != 0 {
		t.Fatalf("glorian scan = %+v", res)
	}
}

// TEC-260 (F2-FIX-5): the repair scan leaves migrated legacy services
// alone. A service the migrator brought over already completed (its
// completed_at is not after migration_map.migrated_at) gets no warranty
// and no warranty.created event from the scan; a migrated service that was
// completed in the new app after its migration is still repaired.
func TestIntegrationWarrantyRepairScanSkipsMigrated(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t260-dist", "distributor", center)
	dealer := it.org("t260-dealer", "dealer", dist)
	cust, veh := it.svcCustomer(dealer, "t260-cust", "34R260"+it.suffix[len(it.suffix)-4:])

	p12 := it.product(center, "T260A")
	it.setWarrantyMonths(p12, 12)
	c := it.stockChain()
	uLegacy := c.unit(center, p12, 26001)
	uAfter := c.unit(center, p12, 26002)
	full := func(u db.Unit) db.CreateServiceItemParams {
		return db.CreateServiceItemParams{ProductID: p12.ID, UnitID: u.ID, Kind: "full"}
	}

	// mapService records svc as migrated at migratedAt (own source_system so
	// the migrator tests' cleanups never see it)
	// and moves its completion to completedAt.
	mapService := func(svc db.Service, completedAt, migratedAt string) {
		t.Helper()
		if _, err := it.pool.Exec(ctx, `UPDATE services SET completed_at = now() - $2::interval WHERE id = $1`,
			svc.ID, completedAt); err != nil {
			t.Fatalf("completed_at: %v", err)
		}
		if _, err := it.pool.Exec(ctx, `INSERT INTO migration_map
			(source_system, source_table, source_id, target_table, target_uuid, migrated_at)
			VALUES ('t260-test', 'services', $1, 'services', $2, now() - $3::interval)`,
			svc.Uuid.String(), svc.Uuid, migratedAt); err != nil {
			t.Fatalf("migration_map: %v", err)
		}
	}

	// Legacy: completed in the old hub 3 days ago, migrated 2 days ago.
	sLegacy := it.directService(dealer, cust, veh, 1, full(uLegacy))
	mapService(sLegacy, "3 days", "2 days")
	// After: migrated 2 days ago (still open then), completed 1 day ago in
	// the new app without the listener running.
	sAfter := it.directService(dealer, cust, veh, 2, full(uAfter))
	mapService(sAfter, "1 day", "2 days")

	listener := warrantymodule.NewListener(it.pool, it.q, "http://localhost:3000", nil)
	scanner := warrantyusecase.NewRepairScanner(listener, 30, nil)
	res, err := scanner.Scan(ctx, time.Now().Add(time.Hour), dealer.ID)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := warrantyusecase.RepairResult{
		ServicesScanned: 1, ServicesRetried: 1, ServicesRepaired: 1, WarrantiesCreated: 1,
	}
	if res != want {
		t.Fatalf("scan = %+v, want %+v", res, want)
	}
	if n := len(it.serviceWarranties(sLegacy.ID, sLegacy.BrandID)); n != 0 {
		t.Fatalf("legacy service got %d warranties, want 0", n)
	}
	if n := it.createdEvents(sLegacy.ID); n != 0 {
		t.Fatalf("legacy service warranty.created = %d, want 0", n)
	}
	ws := it.serviceWarranties(sAfter.ID, sAfter.BrandID)
	if len(ws) != 1 || ws[0].UnitID != uAfter.ID {
		t.Fatalf("after-migration service warranties = %+v", ws)
	}
	if n := it.createdEvents(sAfter.ID); n != 1 {
		t.Fatalf("after-migration service warranty.created = %d, want 1", n)
	}
}
