package usecase_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1243 (K14): the stock view splits a product sold by the meter into
// closed boxes, open rolls, cut pieces and remnants; the box and meter
// thresholds of a reorder rule trigger and alert apart. No price anywhere.
func TestStockMeasuresAndTwoThresholds(t *testing.T) {
	e := newLabelEnv(t)
	ctx, q := e.ctx, e.q
	l := ledger.New(q, outbox.NewStore(e.pool, q))
	num := func(s string) pgtype.Numeric {
		var n pgtype.Numeric
		if err := n.Scan(s); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// The piece product is sold by the meter: 15 m boxes, 2 m minimum cut.
	if _, err := q.UpdateProductRollSettings(ctx, db.UpdateProductRollSettingsParams{
		DefaultRollMeters: num("15"), MinCutMeters: num("2"), SellableByMeter: true,
		Uuid: e.piece.Uuid, BrandID: e.brand.ID,
	}); err != nil {
		t.Fatalf("roll settings: %v", err)
	}
	tag := e.suffix[len(e.suffix)-9:]
	wh, err := q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: e.center.ID, Code: "M" + tag, Name: "measures " + tag, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	room, err := q.CreateRoom(ctx, db.CreateRoomParams{
		OrganizationID: e.center.ID, WarehouseID: wh.ID, Code: "R1", Name: "room", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	loc, err := q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
		OrganizationID: e.center.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true},
		Type: pgtype.Text{String: "aisle", Valid: true}, Code: "A", Name: "bin A", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	bin := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: e.center.ID}
	seq := 0
	unit := func(meters string) db.Unit {
		seq++
		arg := db.CreateUnitParams{
			OrganizationID: e.center.ID, BrandID: e.brand.ID, ProductID: e.piece.ID,
			Barcode:  fmt.Sprintf("T1243-%s-%d", e.suffix, seq),
			UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
		}
		if meters != "" {
			arg.InitialMeters, arg.RemainingMeters = num(meters), num(meters)
		}
		u, err := q.CreateUnit(ctx, arg)
		if err != nil {
			t.Fatalf("unit: %v", err)
		}
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeEntry, UnitID: u.ID, To: &bin, Source: "test", RefType: "t1243", RefID: u.ID,
		}); err != nil {
			t.Fatalf("entry: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return u
	}
	reserve := func(unitID int64, meters string) {
		var orderID, itemID int64
		if err := e.pool.QueryRow(ctx, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, currency, status)
VALUES ($1, $2, $1, $3, 'TRY', 'preparing') RETURNING id`, e.center.ID, e.brand.ID, e.dist.ID).Scan(&orderID); err != nil {
			t.Fatalf("order: %v", err)
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO order_items (order_id, organization_id, brand_id, product_id, meters, line_credits, unit_price, price_source, line_total)
VALUES ($1, $2, $3, $4, $5, 5, 10, 'list', 10) RETURNING id`, orderID, e.center.ID, e.brand.ID, e.piece.ID, meters).Scan(&itemID); err != nil {
			t.Fatalf("order item: %v", err)
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO stock_reservations (organization_id, brand_id, unit_id, unit_kind, order_item_id, meters)
VALUES ($1, $2, $3, 'serial', $4, $5)`, e.center.ID, e.brand.ID, unitID, itemID, meters); err != nil {
			t.Fatalf("reservation: %v", err)
		}
	}

	// 3 closed boxes; an opened 15 m box cut by hand (3 m piece, 12 m left
	// open); an opened 6 m roll reserved whole for a meter line (a cut
	// piece); a 1.20 m rest under the 2 m minimum (a remnant).
	unit("")
	unit("")
	unit("")
	opened := unit("15")
	whole := unit("6")
	unit("1.2")
	sp := stockusecase.NewSplits(e.pool, q, outbox.NewStore(e.pool, q))
	c := e.caller(e.center)
	if _, _, err := sp.Split(ctx, c, stockusecase.SplitInput{Barcode: opened.Barcode, Meters: "3", IdempotencyKey: "t1243-" + e.suffix}); err != nil {
		t.Fatalf("split: %v", err)
	}
	reserve(whole.ID, "6")

	svc := stockusecase.New(q)
	rows, _, err := svc.WarehouseStock(ctx, c.Filter, e.center.ID, wh.Uuid, model.StockFilter{ProductUUID: &e.piece.Uuid, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].Balance == nil {
		t.Fatalf("warehouse stock = %+v, %v", rows, err)
	}
	got := rows[0].Balance.Measures
	want := model.StockMeasures{
		Boxes: 3, AvailableBoxes: 3,
		OpenRolls: 1, OpenMeters: "12.00",
		CutPieces: 2, CutMeters: "9.00",
		Remnants: 1, RemnantMeters: "1.20",
		// 12 open + 9 cut - 6 reserved; the remnant does not count.
		AvailableMeters: "15.00",
	}
	if got != want {
		t.Fatalf("measures = %+v, want %+v", got, want)
	}
	if rows[0].Quantity != 7 {
		t.Fatalf("on hand quantity = %d", rows[0].Quantity)
	}

	// The warehouse stock card shows the same split.
	sums, err := svc.WarehouseSummary(ctx, c.Filter, e.center.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range sums {
		if r.Warehouse == nil || r.Warehouse.UUID != wh.Uuid {
			continue
		}
		found = true
		if r.Pieces != 3 || r.Rolls != 4 || r.OpenRolls != 1 || r.OpenMeters != "12.00" || r.CutPieces != 2 ||
			r.CutMeters != "9.00" || r.Remnants != 1 || r.RemnantMeters != "1.20" {
			t.Fatalf("stock card = %+v", r)
		}
	}
	if !found {
		t.Fatal("warehouse missing from the summary")
	}

	// Box threshold 4 (3 boxes: below), meter threshold 10 m (15 m: ok).
	rule, err := svc.UpsertReorderRule(ctx, c, stockusecase.ReorderRuleInput{
		WarehouseUUID: wh.Uuid.String(), ProductUUID: e.piece.Uuid.String(), MinQuantity: 4, MinMeters: ptr("10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Level != stockusecase.ReorderLevelBelowMin || rule.MeterLevel != stockusecase.ReorderLevelOK ||
		rule.MinMeters == nil || *rule.MinMeters != "10.00" || rule.SuggestedQuantity != 1 {
		t.Fatalf("box rule = %+v", rule)
	}
	// Box threshold 2 (ok), meter threshold 20 m (15 m: below).
	rule, err = svc.UpsertReorderRule(ctx, c, stockusecase.ReorderRuleInput{
		WarehouseUUID: wh.Uuid.String(), ProductUUID: e.piece.Uuid.String(), MinQuantity: 2, MinMeters: ptr("20"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Level != stockusecase.ReorderLevelOK || rule.MeterLevel != stockusecase.ReorderLevelBelowMin {
		t.Fatalf("meter rule = %+v", rule)
	}
	crit, n, err := svc.ReorderRules(ctx, c, stockusecase.ReorderFilter{CriticalOnly: true, WarehouseUUID: &wh.Uuid, Limit: 20})
	if err != nil || n != 1 || crit[0].UUID != rule.UUID {
		t.Fatalf("critical = %+v, %v", crit, err)
	}

	// The hourly check records the two states apart: the meter state goes
	// low, the box state stays ok; a second check sends nothing new.
	alerts := stockusecase.NewReorderAlerts(e.pool, svc, outbox.NewMemory())
	if _, err := alerts.Check(ctx, e.center.ID); err != nil {
		t.Fatal(err)
	}
	if again, err := alerts.Check(ctx, e.center.ID); err != nil || again != 0 {
		t.Fatalf("second check = %d, %v", again, err)
	}
	after, _, err := svc.ReorderRules(ctx, c, stockusecase.ReorderFilter{WarehouseUUID: &wh.Uuid, Limit: 20})
	if err != nil || len(after) != 1 || after[0].AlertState != "ok" || after[0].MeterAlertState != "low" || after[0].MeterAlertedAt == nil {
		t.Fatalf("alert states = %+v, %v", after, err)
	}
	raw, _ := json.Marshal(after)
	for _, k := range []string{"price", "amount", "cost"} {
		if strings.Contains(string(raw), k) {
			t.Fatalf("rule DTO carries %q: %s", k, raw)
		}
	}
	if err := svc.DeleteReorderRule(ctx, c, rule.UUID); err != nil {
		t.Fatal(err)
	}
}

func ptr(s string) *string { return &s }
