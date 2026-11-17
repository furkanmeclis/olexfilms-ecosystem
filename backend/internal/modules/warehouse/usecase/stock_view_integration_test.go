package usecase_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-1229 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL):
// the warehouse product list splits on hand into reserved / available /
// incoming; the movement list filters by warehouse and type; a reorder
// rule turns critical and notifies once; the end-of-day report carries the
// opening and closing balance and the pending work. No DTO carries a price.

// reserve writes an active order reservation of one serial unit (an order
// of the center to the distributor being prepared).
func (e *entryEnv) reserve(t *testing.T, unitID int64) {
	t.Helper()
	var orderID, itemID int64
	if err := e.pool.QueryRow(e.ctx, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, currency, status)
VALUES ($1, $2, $1, $3, 'TRY', 'preparing') RETURNING id`, e.center.ID, e.brand.ID, e.dist.ID).Scan(&orderID); err != nil {
		t.Fatalf("order: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx, `INSERT INTO order_items (order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total)
VALUES ($1, $2, $3, $4, 1, 10, 'list', 10) RETURNING id`, orderID, e.center.ID, e.brand.ID, e.piece.ID).Scan(&itemID); err != nil {
		t.Fatalf("order item: %v", err)
	}
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO stock_reservations (organization_id, brand_id, unit_id, unit_kind, order_item_id, quantity)
VALUES ($1, $2, $3, 'serial', $4, 1)`, e.center.ID, e.brand.ID, unitID, itemID); err != nil {
		t.Fatalf("reservation: %v", err)
	}
}

func noPrice(t *testing.T, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"price", "amount", "total\"", "currency", "value"} {
		if strings.Contains(string(raw), k) {
			t.Fatalf("payload carries %q: %s", k, raw)
		}
	}
}

func TestStockViewBalancesMovementsAndReorder(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	src := e.site(t, "S")
	dst := e.site(t, "D")
	lines := e.stocked(t, src.w, src.a, 5, "B")
	unitIDs := make([]int64, len(lines))
	for i, l := range lines {
		u, _ := e.state(t, l.UnitUUID)
		unitIDs[i] = u.ID
	}
	e.reserve(t, unitIDs[0])
	// TEC-1226: one returned unit waits in quarantine (not on hand).
	e.post(t, ledger.Movement{Type: ledger.TypeQuarantine, UnitID: unitIDs[4], Reason: "t1229 quarantine"})

	// One unit on its way to the destination warehouse.
	tr := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	doc, err := tr.CreateTransfer(e.ctx, c, wh.TransferInput{FromWarehouseUUID: src.w.UUID.String(), ToWarehouseUUID: dst.w.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddTransferLines(e.ctx, c, doc.UUID, []string{lines[3].Barcode}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Ship(e.ctx, c, doc.UUID); err != nil {
		t.Fatal(err)
	}

	svc := stockusecase.New(e.q)
	sc := e.stockCaller(e.center)
	srcItems, _, err := svc.WarehouseStock(e.ctx, sc.Filter, e.center.ID, src.w.UUID, stockFilter())
	if err != nil {
		t.Fatal(err)
	}
	if len(srcItems) != 1 || srcItems[0].Balance == nil {
		t.Fatalf("source items = %+v", srcItems)
	}
	b := srcItems[0].Balance
	if b.OnHandQuantity != 3 || b.ReservedQuantity != 1 || b.AvailableQuantity != 2 || b.IncomingQuantity != 0 || b.QuarantineQuantity != 1 {
		t.Fatalf("source balance = %+v", b)
	}
	noPrice(t, srcItems)
	// The destination warehouse has nothing on hand yet (no bin row, so
	// not listed) and one unit incoming.
	bal, err := svc.Balances(e.ctx, stockusecase.BalanceScope{OrganizationID: e.center.ID, WarehouseID: e.locWarehouse(t, dst)},
		map[int64]stockusecase.Amount{e.piece.ID: {}})
	if err != nil {
		t.Fatal(err)
	}
	if got := bal[e.piece.ID]; got.IncomingQuantity != 1 || got.OnHandQuantity != 0 || got.AvailableQuantity != 0 {
		t.Fatalf("destination balance = %+v", got)
	}
	// The location list shows the same numbers as the warehouse list.
	locItems, _, err := svc.LocationStock(e.ctx, sc.Filter, src.a.UUID, stockFilter())
	if err != nil || len(locItems) != 1 || locItems[0].Balance.AvailableQuantity != 2 || locItems[0].Balance.ReservedQuantity != 1 {
		t.Fatalf("location items = %+v, %v", locItems, err)
	}

	// Movement list: the source warehouse, transfer_out only.
	mf, err := stockusecase.ParseMovementFilter(map[string][]string{
		"warehouse_uuid": {src.w.UUID.String()}, "type": {"transfer_out"},
	})
	if err != nil {
		t.Fatal(err)
	}
	mvs, total, err := svc.Movements(e.ctx, sc.Filter, e.center.ID, e.center.BrandID, mf)
	if err != nil || total != 1 || len(mvs) != 1 || mvs[0].Unit.Barcode != lines[3].Barcode ||
		mvs[0].From == nil || mvs[0].From.Warehouse == nil || mvs[0].From.Warehouse.UUID != src.w.UUID {
		t.Fatalf("movements = %+v (%d), %v", mvs, total, err)
	}
	noPrice(t, mvs)
	if _, err := stockusecase.ParseMovementFilter(map[string][]string{"type": {"nope"}}); err == nil {
		t.Fatal("unknown movement type accepted")
	}

	// Reorder rule: available 2 <= reorder point 2 -> reorder; suggested
	// tops up to max 10.
	two, ten := int32(2), int32(10)
	rule, err := svc.UpsertReorderRule(e.ctx, sc, stockusecase.ReorderRuleInput{
		WarehouseUUID: src.w.UUID.String(), ProductUUID: e.piece.Uuid.String(), MinQuantity: 1, ReorderPoint: &two, MaxQuantity: &ten,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Level != stockusecase.ReorderLevelReorder || rule.Balance.AvailableQuantity != 2 || rule.SuggestedQuantity != 8 {
		t.Fatalf("rule = %+v", rule)
	}
	noPrice(t, rule)
	if _, err := svc.UpsertReorderRule(e.ctx, sc, stockusecase.ReorderRuleInput{
		WarehouseUUID: src.w.UUID.String(), ProductUUID: e.piece.Uuid.String(), MinQuantity: 5, ReorderPoint: &two,
	}); err == nil {
		t.Fatal("reorder point below min accepted")
	}
	crit, n, err := svc.ReorderRules(e.ctx, sc, stockusecase.ReorderFilter{CriticalOnly: true, WarehouseUUID: &src.w.UUID, Limit: 20})
	if err != nil || n != 1 || crit[0].UUID != rule.UUID {
		t.Fatalf("critical = %+v, %v", crit, err)
	}
	// The hourly check notifies once on the ok -> low edge.
	mem := outbox.NewMemory()
	alerts := stockusecase.NewReorderAlerts(e.pool, svc, mem)
	first, err := alerts.Check(e.ctx, e.center.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := alerts.Check(e.ctx, e.center.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("second check notified %d", again)
	}
	_ = first // recipients depend on the shared center's members
	if err := svc.DeleteReorderRule(e.ctx, sc, rule.UUID); err != nil {
		t.Fatal(err)
	}
	// Another organization cannot read the center's movements.
	if _, _, err := svc.Movements(e.ctx, e.stockCaller(e.dist).Filter, e.center.ID, e.center.BrandID, mf); err == nil {
		t.Fatal("foreign movements readable")
	}
}

func stockFilter() stockmodel.StockFilter { return stockmodel.StockFilter{Limit: apiquery.MaxLimit} }

func (e *entryEnv) locWarehouse(t *testing.T, s site) int64 {
	t.Helper()
	var id int64
	if err := e.pool.QueryRow(e.ctx, `SELECT id FROM warehouses WHERE uuid = $1`, s.w.UUID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The end-of-day report of a warehouse carries the opening and closing
// on-hand balance per product and the open work; yesterday's report
// subtracts today's movements from the current stock.
func TestEODReportBalances(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, loc := e.location(t, e.center, "H")
	w2, loc2 := e.location(t, e.center, "J")
	locID, loc2ID := e.locationID(t, loc), e.locationID(t, loc2)
	units := e.enterUnits(t, w, loc, 3, "H")
	from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: locID, OrgID: e.center.ID}
	e.post(t, ledger.Movement{Type: ledger.TypeConsumption, UnitID: units[0], From: &from,
		To: &ledger.Owner{Type: ledger.OwnerService, ID: e.service(t, e.center), OrgID: e.center.ID}, Reason: "t1229 service"})
	e.post(t, ledger.Movement{Type: ledger.TypePlacement, UnitID: units[1], From: &from,
		To: &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc2ID, OrgID: e.center.ID}, Reason: "t1229 move"})

	eod := wh.NewEOD(e.pool, e.q)
	wUUID := w.UUID.String()
	rep, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{WarehouseUUID: &wUUID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Summary.Balances) != 1 {
		t.Fatalf("balances = %+v", rep.Summary.Balances)
	}
	b := rep.Summary.Balances[0]
	if b.ProductUUID != e.piece.Uuid || b.OpeningQuantity != 0 || b.ClosingQuantity != 1 || b.ChangeQuantity != 1 {
		t.Fatalf("today balance = %+v", b)
	}
	if rep.Summary.Pending == nil {
		t.Fatal("pending work missing")
	}
	noPrice(t, rep.Summary.Balances)
	w2UUID := w2.UUID.String()
	rep2, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{WarehouseUUID: &w2UUID})
	if err != nil || len(rep2.Summary.Balances) != 1 || rep2.Summary.Balances[0].ClosingQuantity != 1 || rep2.Summary.Balances[0].OpeningQuantity != 0 {
		t.Fatalf("warehouse J balances = %+v, %v", rep2.Summary.Balances, err)
	}
	day, err := time.Parse("2006-01-02", rep.ReportDate)
	if err != nil {
		t.Fatal(err)
	}
	yday, err := eod.Generate(e.ctx, wh.EODCaller{Caller: c.Caller}, wh.EODGenerateInput{
		Date: day.AddDate(0, 0, -1).Format("2006-01-02"), WarehouseUUID: &wUUID})
	if err != nil || len(yday.Summary.Balances) != 0 {
		t.Fatalf("yesterday balances = %+v, %v", yday.Summary.Balances, err)
	}
	html := wh.EODPDFHTML(wh.EODPDFDoc{Report: rep, Organization: "org"}, "tr", "Gün sonu")
	if !strings.Contains(html, "Açılış ve kapanış bakiyesi") || !strings.Contains(html, "Bekleyen işler") {
		t.Fatal("pdf misses the balance or pending sections")
	}
}
