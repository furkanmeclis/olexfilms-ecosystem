package usecase_test

import (
	"fmt"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1066: the warehouse home reads the in-stock totals per warehouse.
func TestWarehouseSummary(t *testing.T) {
	e := newLabelEnv(t)
	ctx, q := e.ctx, e.q
	l := ledger.New(q, outbox.NewStore(e.pool, q))

	warehouse := func(code string) db.Warehouse {
		w, err := q.CreateWarehouse(ctx, db.CreateWarehouseParams{
			OrganizationID: e.center.ID, Code: code, Name: "wh " + code, Active: true,
		})
		if err != nil {
			t.Fatalf("warehouse %s: %v", code, err)
		}
		return w
	}
	location := func(w db.Warehouse, code string) ledger.Owner {
		room, err := q.CreateRoom(ctx, db.CreateRoomParams{
			OrganizationID: e.center.ID, WarehouseID: w.ID, Code: "R" + code, Name: "room " + code, Active: true,
		})
		if err != nil {
			t.Fatalf("room: %v", err)
		}
		loc, err := q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
			OrganizationID: e.center.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true},
			Type: pgtype.Text{String: "aisle", Valid: true}, Code: code, Name: "bin " + code, Active: true,
		})
		if err != nil {
			t.Fatalf("location: %v", err)
		}
		return ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: e.center.ID}
	}
	seq := 0
	unit := func(p db.Product, meters string) db.Unit {
		seq++
		arg := db.CreateUnitParams{
			OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: p.ID,
			Barcode:  fmt.Sprintf("T1066-%s-%d", e.suffix, seq),
			UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		}
		if meters != "" {
			var n pgtype.Numeric
			if err := n.Scan(meters); err != nil {
				t.Fatal(err)
			}
			arg.InitialMeters, arg.RemainingMeters = n, n
		}
		u, err := q.CreateUnit(ctx, arg)
		if err != nil {
			t.Fatalf("unit: %v", err)
		}
		return u
	}
	put := func(u db.Unit, to ledger.Owner) {
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		seq++
		if _, err := l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeEntry, UnitID: u.ID, To: &to, Source: "test", RefType: "t1066", RefID: int64(seq),
		}); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	tag := e.suffix[len(e.suffix)-9:]
	mainCode, secondCode := "T"+tag+"A", "T"+tag+"B"
	main := warehouse(mainCode)
	second := warehouse(secondCode)
	a, b := location(main, "A"), location(second, "B")
	put(unit(e.piece, ""), a)
	put(unit(e.piece, ""), a)
	put(unit(e.roll, "30"), a)
	put(unit(e.roll, "12.5"), b)
	put(unit(e.piece, ""), ledger.Owner{Type: ledger.OwnerOrganization, ID: e.center.ID})

	svc := stockusecase.New(q)
	f := scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{e.center.ID}, OrgID: e.center.ID}
	rows, err := svc.WarehouseSummary(ctx, f, e.center.ID)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]model.WarehouseStockSummary{}
	unplaced := false
	for i, r := range rows {
		if r.Warehouse == nil {
			unplaced = i == len(rows)-1 && r.Pieces >= 1
			continue
		}
		byCode[r.Warehouse.Code] = r
	}
	got := byCode[mainCode]
	if got.Products != 2 || got.Pieces != 2 || got.Rolls != 1 || got.Meters != "30.00" && got.Meters != "30" {
		t.Fatalf("main warehouse = %+v", got)
	}
	got = byCode[secondCode]
	if got.Products != 1 || got.Pieces != 0 || got.Rolls != 1 {
		t.Fatalf("second warehouse = %+v", got)
	}
	if !unplaced {
		t.Fatalf("the unplaced row (no warehouse) must come last: %+v", rows)
	}
}
