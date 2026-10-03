package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
)

// OrdersStep imports the legacy orders of both systems (TEC-261, design §7):
// hub orders / order_items / order_item_stock and warehouse orders /
// order_items / order_item_barcodes become orders / order_items /
// order_item_units.
//
// A hub order and a warehouse order with the same external_reference are
// one order (the warehouse pushed it to the hub under that reference); both
// legacy rows map to it. The more recently updated side decides the status
// (a tie goes to the warehouse). Lines are merged by product: the larger of
// the two quantities wins, and the assigned units of both sides are joined
// (a barcode present in both systems is one unit since TEC-257). An order of
// this database that already has the reference (brand_id, external_reference
// is unique) is linked, never rewritten.
//
// Parties (K6): the buyer is the dealer (hub dealer_id; warehouse customer ->
// its hub dealer through source/external_id or dealer_code), the seller is
// the dealer's parent distributor. Currency is the brand currency (TRY for
// Olex); an approved order freezes the rate of its order date, or 1 when no
// rate is stored (reported, K7). Legacy orders carry no prices, so lines are
// written at 0.
//
// Legacy orders are history: the step writes no status history row, no
// outbox event and no accounting entry (K9; a later receipt books nothing
// either, the total is 0). Lines of Glorian products are skipped (K2), roll
// products are skipped (the legacy rows carry no meters). Reruns are
// idempotent through migration_map checksums; a changed legacy order is
// rewritten only while the application has not moved it (no status history).
type OrdersStep struct {
	// System and WHSystem are the migration_map source systems of the hub
	// and warehouse rows; empty means SourceHub / SourceWH.
	System   string
	WHSystem string
}

// Name implements Step.
func (OrdersStep) Name() string { return "orders" }

func (s OrdersStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

func (s OrdersStep) whSystem() string {
	if s.WHSystem == "" {
		return SourceWH
	}
	return s.WHSystem
}

// Legacy queries (single SELECTs, see source.CheckReadOnly).
const (
	hubOrdersQuery = `SELECT id, dealer_id, created_by, status, COALESCE(cargo_company, ''), COALESCE(tracking_number, ''),
	COALESCE(notes, ''), COALESCE(external_reference, ''), created_at, updated_at
FROM orders ORDER BY id`
	hubOrderItemsQuery  = `SELECT id, order_id, product_id, quantity, created_at FROM order_items ORDER BY order_id, id`
	hubOrderStockQuery  = `SELECT order_item_id, stock_item_id, created_at FROM order_item_stock ORDER BY order_item_id, id`
	hubDealerCodesQuery = `SELECT id, COALESCE(dealer_code, '') FROM dealers ORDER BY id`

	whOrdersQuery = `SELECT CAST(o.id AS CHAR(36)), o.status, o.external_reference, COALESCE(o.tracking_number, ''),
	COALESCE(o.notes, ''), o.confirmed_at, o.shipped_at, o.delivered_at, o.received_at, o.cancelled_at,
	o.deleted_at, o.created_at, o.updated_at,
	COALESCE(m.source, c.source, ''), COALESCE(m.external_id, c.external_id, ''), COALESCE(m.dealer_code, c.dealer_code, '')
FROM orders o
JOIN customers c ON c.id = o.customer_id
LEFT JOIN customers m ON m.id = c.merged_into_customer_id
ORDER BY o.created_at, o.id`
	whOrderItemsQuery = `SELECT CAST(i.id AS CHAR(36)), CAST(i.order_id AS CHAR(36)), CAST(i.product_id AS CHAR(36)),
	COALESCE(b.name, ''), i.quantity, i.created_at
FROM order_items i
JOIN products p ON p.id = i.product_id
LEFT JOIN brands b ON b.id = p.brand_id
ORDER BY i.order_id, i.id`
	whOrderBarcodesQuery = `SELECT CAST(order_item_id AS CHAR(36)), CAST(product_barcode_id AS CHAR(36)), created_at
FROM order_item_barcodes ORDER BY order_item_id, id`
)

// orderExternalRefMax is orders.external_reference VARCHAR(128);
// orderTrackingMax is orders.tracking_no VARCHAR(128).
const (
	orderExternalRefMax = 128
	orderTrackingMax    = 128
	tryCurrency         = "TRY"
)

// Order statuses of this database (chk_orders_status, 000049).
const (
	orderDraft      = "draft"
	orderSubmitted  = "submitted"
	orderApproved   = "approved"
	orderPreparing  = "preparing"
	orderReady      = "ready"
	orderProcessing = "processing"
	orderShipped    = "shipped"
	orderReceived   = "received"
	orderCancelling = "cancelling"
	orderCancelled  = "cancelled"
)

// hubOrderStatuses maps the hub OrderStatusEnum (app/Enums/OrderStatusEnum.php:
// pending, processing, shipped, delivered, cancelled). The hub has no
// receipt step: delivered is final there (the units were transferred to the
// dealer), which is received here. processing means the order went to the
// warehouse.
var hubOrderStatuses = map[string]string{
	"pending":    orderSubmitted,
	"processing": orderProcessing,
	"shipped":    orderShipped,
	"delivered":  orderReceived,
	"cancelled":  orderCancelled,
}

// whOrderStatuses maps the warehouse App\Enums\OrderStatus (draft,
// preparing, ready, processing, shipped, delivered, received, cancelled; the
// 2026_07_23 migration turned every legacy pending into one of them). The
// warehouse delivered is "the carrier delivered, the dealer has not
// received yet"; this application has no transition out of delivered, so it
// stays shipped (delivered_at keeps the moment) and the dealer can still
// receive it.
var whOrderStatuses = map[string]string{
	"draft":      orderDraft,
	"preparing":  orderPreparing,
	"ready":      orderReady,
	"processing": orderProcessing,
	"shipped":    orderShipped,
	"delivered":  orderShipped,
	"received":   orderReceived,
	"cancelled":  orderCancelled,
}

// approvedStatuses are the statuses past the seller's approval: the rate is
// frozen (chk_orders_rate_frozen).
var approvedStatuses = map[string]bool{
	orderApproved: true, orderPreparing: true, orderReady: true, orderProcessing: true,
	orderShipped: true, orderReceived: true, orderCancelling: true,
}

type hubOrder struct {
	ID, DealerID, CreatedBy int64
	Status, Cargo, Tracking string
	Notes, ExternalRef      string
	CreatedAt, UpdatedAt    sql.NullTime
}

type hubOrderItem struct {
	ID, OrderID, ProductID int64
	Quantity               int32
	CreatedAt              sql.NullTime
}

type hubOrderStock struct {
	ItemID, StockItemID int64
	CreatedAt           sql.NullTime
}

type whOrder struct {
	ID, Status, ExternalRef, Tracking, Notes                     string
	ConfirmedAt, ShippedAt, DeliveredAt, ReceivedAt, CancelledAt sql.NullTime
	DeletedAt, CreatedAt, UpdatedAt                              sql.NullTime
	CustSource, CustExternalID, CustDealerCode                   string
}

type whOrderItem struct {
	ID, OrderID, ProductID, Brand string
	Quantity                      int32
	CreatedAt                     sql.NullTime
}

type whOrderBarcode struct {
	ItemID, BarcodeID string
	CreatedAt         sql.NullTime
}

// legacyOrders is everything the step reads from both systems.
type legacyOrders struct {
	hub        []hubOrder
	hubItems   map[int64][]hubOrderItem
	hubStock   map[int64][]hubOrderStock
	dealerCode map[string]int64
	wh         []whOrder
	whItems    map[string][]whOrderItem
	whBarcodes map[string][]whOrderBarcode
}

// orderCandidate is one order of this database: a hub row, a warehouse row
// or both (same external_reference).
type orderCandidate struct {
	Hub *hubOrder
	WH  *whOrder
}

func (o *orderCandidate) label() string {
	if o.Hub != nil {
		return "hub:" + strconv.FormatInt(o.Hub.ID, 10)
	}
	return "wh:" + o.WH.ID
}

// skipOrder reports a skipped candidate: the total, then each legacy order
// and (lines true) order line it carries by id, as the validation report
// (report.go) matches skipped rows by id.
func skipOrder(c counts, l *legacyOrders, cand *orderCandidate, reason string, lines bool) {
	c.inc("order_skipped_" + reason)
	if h := cand.Hub; h != nil {
		c.inc("order_skipped_" + reason + ":hub:" + strconv.FormatInt(h.ID, 10))
		if lines {
			for _, it := range l.hubItems[h.ID] {
				c.inc("lines_skipped_order_" + reason + ":hub:" + strconv.FormatInt(it.ID, 10))
			}
		}
	}
	if w := cand.WH; w != nil {
		c.inc("order_skipped_" + reason + ":wh:" + w.ID)
		if lines {
			for _, it := range l.whItems[w.ID] {
				c.inc("lines_skipped_order_" + reason + ":wh:" + it.ID)
			}
		}
	}
}

// orderLine is one product line of a candidate, merged from both sides.
type orderLine struct {
	ProductID int64
	HubQty    int32
	WHQty     int32
	Keys      []Key
	Sums      []string
	Units     []orderLineUnit
	unitSeen  map[int64]bool
	CreatedAt sql.NullTime
}

type orderLineUnit struct {
	ID         int64
	Kind       string
	AssignedAt sql.NullTime
}

func (l *orderLine) quantity() int32 {
	q := max(l.HubQty, l.WHQty)
	return max(q, int32(len(l.Units)))
}

// orderCtx caches what the import needs from this database.
type orderCtx struct {
	q        *db.Queries
	m        *Mapper
	rates    *fxrates.Service
	brandID  int64
	currency string
	products map[uuid.UUID]db.MigratorProductForUnitRow
	dealers  map[int64]uuid.UUID
	users    map[int64]pgtype.Int8
}

// Run implements Step.
func (s OrdersStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	wh, err := src.Get(SourceWH)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	currency, err := dst.Q.MigratorBrandCurrency(ctx, brand.ID)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand currency: %w", err)
	}
	if currency != tryCurrency {
		c.inc("currency_not_try:" + currency)
	}
	legacy, err := readLegacyOrders(ctx, hub, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	u := &orderCtx{
		q: dst.Q, m: m, rates: fxrates.New(dst.Q, nil, nil), brandID: brand.ID, currency: currency,
		products: map[uuid.UUID]db.MigratorProductForUnitRow{}, dealers: map[int64]uuid.UUID{},
		users: map[int64]pgtype.Int8{},
	}

	var watermark time.Time
	seen := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}
	var cands []*orderCandidate
	byRef := map[string]*orderCandidate{}
	for i := range legacy.hub {
		o := &legacy.hub[i]
		c.inc("hub_read")
		seen(o.CreatedAt, o.UpdatedAt)
		cand := &orderCandidate{Hub: o}
		cands = append(cands, cand)
		if ref := strings.TrimSpace(o.ExternalRef); ref != "" {
			byRef[ref] = cand
		}
	}
	for i := range legacy.wh {
		o := &legacy.wh[i]
		c.inc("wh_read")
		seen(o.CreatedAt, o.UpdatedAt, o.DeletedAt)
		if o.DeletedAt.Valid {
			c.inc("wh_deleted_skipped")
			continue
		}
		if cand, ok := byRef[strings.TrimSpace(o.ExternalRef)]; ok && cand.WH == nil {
			cand.WH = o
			c.inc("orders_matched")
			continue
		}
		cands = append(cands, &orderCandidate{WH: o})
	}

	for _, cand := range cands {
		if err := s.importOrder(ctx, u, legacy, cand, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("order %s: %w", cand.label(), err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readLegacyOrders(ctx context.Context, hub, wh source.LegacySource) (*legacyOrders, error) {
	l := &legacyOrders{
		hubItems: map[int64][]hubOrderItem{}, hubStock: map[int64][]hubOrderStock{}, dealerCode: map[string]int64{},
		whItems: map[string][]whOrderItem{}, whBarcodes: map[string][]whOrderBarcode{},
	}
	steps := []struct {
		src   source.LegacySource
		query string
		what  string
		scan  func(rows source.Rows) error
	}{
		{hub, hubOrdersQuery, "hub orders", func(rows source.Rows) error {
			var o hubOrder
			if err := rows.Scan(&o.ID, &o.DealerID, &o.CreatedBy, &o.Status, &o.Cargo, &o.Tracking, &o.Notes,
				&o.ExternalRef, &o.CreatedAt, &o.UpdatedAt); err != nil {
				return err
			}
			l.hub = append(l.hub, o)
			return nil
		}},
		{hub, hubOrderItemsQuery, "hub order items", func(rows source.Rows) error {
			var it hubOrderItem
			if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Quantity, &it.CreatedAt); err != nil {
				return err
			}
			l.hubItems[it.OrderID] = append(l.hubItems[it.OrderID], it)
			return nil
		}},
		{hub, hubOrderStockQuery, "hub order item stock", func(rows source.Rows) error {
			var st hubOrderStock
			if err := rows.Scan(&st.ItemID, &st.StockItemID, &st.CreatedAt); err != nil {
				return err
			}
			l.hubStock[st.ItemID] = append(l.hubStock[st.ItemID], st)
			return nil
		}},
		{hub, hubDealerCodesQuery, "hub dealers", func(rows source.Rows) error {
			var id int64
			var code string
			if err := rows.Scan(&id, &code); err != nil {
				return err
			}
			if code = strings.ToUpper(strings.TrimSpace(code)); code != "" {
				l.dealerCode[code] = id
			}
			return nil
		}},
		{wh, whOrdersQuery, "warehouse orders", func(rows source.Rows) error {
			var o whOrder
			if err := rows.Scan(&o.ID, &o.Status, &o.ExternalRef, &o.Tracking, &o.Notes, &o.ConfirmedAt, &o.ShippedAt,
				&o.DeliveredAt, &o.ReceivedAt, &o.CancelledAt, &o.DeletedAt, &o.CreatedAt, &o.UpdatedAt,
				&o.CustSource, &o.CustExternalID, &o.CustDealerCode); err != nil {
				return err
			}
			l.wh = append(l.wh, o)
			return nil
		}},
		{wh, whOrderItemsQuery, "warehouse order items", func(rows source.Rows) error {
			var it whOrderItem
			if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.Brand, &it.Quantity, &it.CreatedAt); err != nil {
				return err
			}
			l.whItems[it.OrderID] = append(l.whItems[it.OrderID], it)
			return nil
		}},
		{wh, whOrderBarcodesQuery, "warehouse order item barcodes", func(rows source.Rows) error {
			var b whOrderBarcode
			if err := rows.Scan(&b.ItemID, &b.BarcodeID, &b.CreatedAt); err != nil {
				return err
			}
			l.whBarcodes[b.ItemID] = append(l.whBarcodes[b.ItemID], b)
			return nil
		}},
	}
	for _, st := range steps {
		rows, err := st.src.Query(ctx, st.query)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			if err := st.scan(rows); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan %s: %w", st.what, err)
			}
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("read %s: %w", st.what, err)
		}
	}
	return l, nil
}

// whDealerID resolves the hub dealer a warehouse customer mirrors: the
// olexfilms source external_id, else the dealer code (0: none).
func whDealerID(o *whOrder, codes map[string]int64) int64 {
	if strings.EqualFold(strings.TrimSpace(o.CustSource), "olexfilms") {
		if id, err := strconv.ParseInt(strings.TrimSpace(o.CustExternalID), 10, 64); err == nil && id > 0 {
			return id
		}
	}
	if code := strings.ToUpper(strings.TrimSpace(o.CustDealerCode)); code != "" {
		return codes[code]
	}
	return 0
}

// mapOrderStatus maps a legacy status ("" when unknown).
func mapOrderStatus(table map[string]string, status string) string {
	return table[strings.ToLower(strings.TrimSpace(status))]
}

// orderState is the resolved status and timestamps of a candidate.
type orderState struct {
	Status                                  string
	Created                                 sql.NullTime
	Submitted, Approved, Shipped, Delivered sql.NullTime
	Received, Cancelled                     sql.NullTime
}

// resolveOrderState picks the winning status (the more recently updated
// side, the warehouse on a tie) and derives the timestamps the status needs
// (chk_orders_cancelled, chk_orders_rate_frozen). ok false: a side has an
// unknown status (reason names it).
func resolveOrderState(cand *orderCandidate) (orderState, string, bool) {
	var st orderState
	var hubStatus, whStatus string
	if cand.Hub != nil {
		if hubStatus = mapOrderStatus(hubOrderStatuses, cand.Hub.Status); hubStatus == "" {
			return st, "hub:" + strings.TrimSpace(cand.Hub.Status), false
		}
	}
	if cand.WH != nil {
		if whStatus = mapOrderStatus(whOrderStatuses, cand.WH.Status); whStatus == "" {
			return st, "wh:" + strings.TrimSpace(cand.WH.Status), false
		}
	}
	var updated time.Time
	switch {
	case cand.WH == nil:
		st.Status, updated = hubStatus, latest(cand.Hub.CreatedAt, cand.Hub.UpdatedAt)
	case cand.Hub == nil:
		st.Status, updated = whStatus, latest(cand.WH.CreatedAt, cand.WH.UpdatedAt)
	default:
		hubUpd, whUpd := latest(cand.Hub.CreatedAt, cand.Hub.UpdatedAt), latest(cand.WH.CreatedAt, cand.WH.UpdatedAt)
		st.Status, updated = whStatus, whUpd
		if hubUpd.After(whUpd) {
			st.Status, updated = hubStatus, hubUpd
		}
	}
	var wh whOrder
	if cand.WH != nil {
		wh = *cand.WH
	}
	for _, t := range []sql.NullTime{wh.CreatedAt, hubCreated(cand)} {
		if t.Valid && (!st.Created.Valid || t.Time.Before(st.Created.Time)) {
			st.Created = t
		}
	}
	when := st.Created
	if !updated.IsZero() {
		when = sql.NullTime{Time: updated, Valid: true}
	}
	or := func(a, b sql.NullTime) sql.NullTime {
		if a.Valid {
			return a
		}
		return b
	}

	if st.Status != orderDraft {
		st.Submitted = st.Created
	}
	if approvedStatuses[st.Status] || (st.Status == orderCancelled && wh.ConfirmedAt.Valid) {
		st.Approved = or(wh.ConfirmedAt, st.Created)
	}
	switch st.Status {
	case orderShipped, orderReceived, orderCancelling:
		st.Shipped = or(wh.ShippedAt, when)
	default:
		st.Shipped = wh.ShippedAt
	}
	st.Delivered = wh.DeliveredAt
	if st.Status == orderReceived {
		st.Received = or(wh.ReceivedAt, when)
	}
	if st.Status == orderCancelled {
		st.Cancelled = or(wh.CancelledAt, when)
	}
	return st, "", true
}

func hubCreated(cand *orderCandidate) sql.NullTime {
	if cand.Hub == nil {
		return sql.NullTime{}
	}
	return cand.Hub.CreatedAt
}

func (s OrdersStep) importOrder(ctx context.Context, u *orderCtx, l *legacyOrders, cand *orderCandidate, c counts) error {
	label := cand.label()
	state, reason, ok := resolveOrderState(cand)
	if !ok {
		c.inc("skipped_status:" + reason)
		skipOrder(c, l, cand, "status", true)
		return nil
	}

	// Buyer: the hub dealer decides; a warehouse-only order uses its
	// customer's hub dealer.
	var dealerID int64
	if cand.WH != nil {
		dealerID = whDealerID(cand.WH, l.dealerCode)
	}
	if cand.Hub != nil {
		if dealerID != 0 && dealerID != cand.Hub.DealerID {
			c.inc("buyer_conflict:" + label)
		}
		dealerID = cand.Hub.DealerID
	}
	if dealerID == 0 {
		c.inc("skipped_buyer_unmapped:" + label)
		skipOrder(c, l, cand, "buyer_unmapped", true)
		return nil
	}
	buyerUUID, ok, err := u.m.Lookup(ctx, s.system(), "dealers", strconv.FormatInt(dealerID, 10))
	if err != nil {
		return err
	}
	if !ok {
		c.inc("skipped_buyer_unmapped:" + label)
		skipOrder(c, l, cand, "buyer_unmapped", true)
		return nil
	}
	buyer, err := u.q.MigratorOrderBuyer(ctx, buyerUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.inc("skipped_parties:" + label)
		skipOrder(c, l, cand, "parties", true)
		return nil
	}
	if err != nil {
		return fmt.Errorf("buyer: %w", err)
	}
	if buyer.BrandID != u.brandID || buyer.Type != "dealer" || buyer.ParentType != "distributor" {
		c.inc("skipped_parties:" + label)
		skipOrder(c, l, cand, "parties", true)
		return nil
	}

	lines, sum, err := s.orderLines(ctx, u, l, cand, c)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		c.inc("skipped_no_lines:" + label)
		// Each line was skipped (and reported) on its own.
		skipOrder(c, l, cand, "no_lines", false)
		return nil
	}

	ref := ""
	if cand.Hub != nil {
		ref = strings.TrimSpace(cand.Hub.ExternalRef)
	}
	if ref == "" && cand.WH != nil {
		ref = strings.TrimSpace(cand.WH.ExternalRef)
	}
	if len([]rune(ref)) > orderExternalRefMax {
		c.inc("external_reference_too_long:" + label)
		ref = ""
	}

	var keys []Key
	if cand.Hub != nil {
		keys = append(keys, Key{System: s.system(), Table: "orders", ID: strconv.FormatInt(cand.Hub.ID, 10), TargetTable: "orders"})
	}
	if cand.WH != nil {
		keys = append(keys, Key{System: s.whSystem(), Table: "orders", ID: cand.WH.ID, TargetTable: "orders"})
	}
	orderSum := Checksum(orderChecksumParts(cand, dealerID, sum)...)

	// Resolve the order: a mapping of either side, else an order of this
	// database with the reference (linked, never rewritten), else new.
	var target uuid.UUID
	for _, k := range keys {
		got, mapped, err := u.m.Lookup(ctx, k.System, k.Table, k.ID)
		if err != nil {
			return err
		}
		if !mapped {
			continue
		}
		if target != uuid.Nil && got != target {
			c.inc("order_map_conflict:" + label)
			return nil
		}
		target = got
	}
	if target == uuid.Nil && ref != "" {
		existing, err := u.q.MigratorFindOrderByExternalReference(ctx, db.MigratorFindOrderByExternalReferenceParams{
			BrandID: u.brandID, ExternalReference: ref,
		})
		switch {
		case err == nil:
			for _, k := range keys {
				if _, err := u.m.Link(ctx, k, existing, orderSum); err != nil {
					return err
				}
			}
			c.inc("orders_linked_existing")
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find order by reference: %w", err)
		}
	}
	changed := false
	for _, k := range keys {
		_, mapped, err := u.m.Lookup(ctx, k.System, k.Table, k.ID)
		if err != nil {
			return err
		}
		if !mapped && target != uuid.Nil {
			if _, err := u.m.Link(ctx, k, target, orderSum); err != nil {
				return err
			}
			changed = true
			continue
		}
		res, err := u.m.Upsert(ctx, k, orderSum)
		if err != nil {
			return err
		}
		target = res.UUID
		changed = changed || res.Created || res.Changed
	}

	rate, rateDefaulted, err := u.rateFor(ctx, state)
	if err != nil {
		return err
	}
	tracking, note := orderTrackingAndNote(cand)

	row, err := u.q.MigratorOrderByUUID(ctx, db.MigratorOrderByUUIDParams{Uuid: target, BrandID: u.brandID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		createdBy, err := s.createdBy(ctx, u, cand)
		if err != nil {
			return err
		}
		id, err := u.q.MigratorInsertOrder(ctx, db.MigratorInsertOrderParams{
			Uuid: target, SellerOrgID: buyer.ParentID, BrandID: u.brandID, BuyerOrgID: buyer.ID,
			Status: state.Status, Currency: u.currency, RateSnapshot: rate.snapshot, TryRate: rate.try,
			TrackingNo: pgText(tracking), ExternalReference: pgText(ref), Note: pgText(note),
			CreatedByUserID: createdBy, SubmittedAt: pgTime(state.Submitted), ApprovedAt: pgTime(state.Approved),
			ShippedAt: pgTime(state.Shipped), DeliveredAt: pgTime(state.Delivered), ReceivedAt: pgTime(state.Received),
			CancelledAt: pgTime(state.Cancelled), CreatedAt: pgTime(state.Created),
		})
		if err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
		c.inc("orders_created")
		c.inc("orders_status:" + state.Status)
		if rateDefaulted {
			c.inc("rate_defaulted")
		}
		return s.writeLines(ctx, u, id, buyer.ParentID, lines, c)
	case err != nil:
		return fmt.Errorf("read order: %w", err)
	}
	if !changed {
		c.inc("orders_unchanged")
		return nil
	}
	if row.Touched {
		c.inc("orders_kept_app")
		return nil
	}
	n, err := u.q.MigratorUpdateOrder(ctx, db.MigratorUpdateOrderParams{
		ID: row.ID, BrandID: u.brandID, Status: state.Status, RateSnapshot: rate.snapshot, TryRate: rate.try,
		TrackingNo: pgText(tracking), Note: pgText(note), SubmittedAt: pgTime(state.Submitted),
		ApprovedAt: pgTime(state.Approved), ShippedAt: pgTime(state.Shipped), DeliveredAt: pgTime(state.Delivered),
		ReceivedAt: pgTime(state.Received), CancelledAt: pgTime(state.Cancelled),
	})
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}
	if n == 0 {
		c.inc("orders_kept_app")
		return nil
	}
	c.inc("orders_updated")
	if rateDefaulted {
		c.inc("rate_defaulted")
	}
	return s.writeLines(ctx, u, row.ID, row.OrganizationID, lines, c)
}

// orderChecksumParts lists the legacy values of a candidate the order and its
// lines are built from.
func orderChecksumParts(cand *orderCandidate, dealerID int64, lineSum string) []any {
	parts := []any{dealerID, lineSum}
	if h := cand.Hub; h != nil {
		parts = append(parts, "hub", h.Status, h.Cargo, h.Tracking, h.Notes, h.ExternalRef, h.CreatedBy,
			h.CreatedAt.Time, h.UpdatedAt.Time)
	}
	if w := cand.WH; w != nil {
		parts = append(parts, "wh", w.Status, w.ExternalRef, w.Tracking, w.Notes, w.ConfirmedAt.Time, w.ShippedAt.Time,
			w.DeliveredAt.Time, w.ReceivedAt.Time, w.CancelledAt.Time, w.CreatedAt.Time, w.UpdatedAt.Time)
	}
	return parts
}

// orderTrackingAndNote joins the tracking number and notes of both sides.
func orderTrackingAndNote(cand *orderCandidate) (string, string) {
	var tracking string
	var notes []string
	add := func(n string) {
		if n = strings.TrimSpace(n); n == "" {
			return
		}
		for _, have := range notes {
			if have == n {
				return
			}
		}
		notes = append(notes, n)
	}
	if w := cand.WH; w != nil {
		tracking = strings.TrimSpace(w.Tracking)
		add(w.Notes)
	}
	if h := cand.Hub; h != nil {
		if tracking == "" {
			tracking = strings.TrimSpace(h.Tracking)
		}
		add(h.Notes)
		if cargo := strings.TrimSpace(h.Cargo); cargo != "" {
			add("Kargo: " + cargo)
		}
	}
	return truncate(tracking, orderTrackingMax), strings.Join(notes, "\n")
}

type orderRate struct {
	snapshot []byte
	try      pgtype.Numeric
}

// rateFor freezes the rate of the order date for an approved order (K7):
// brand currency -> TRY, 1 when no rate is stored (defaulted true).
func (u *orderCtx) rateFor(ctx context.Context, st orderState) (orderRate, bool, error) {
	if !st.Approved.Valid {
		return orderRate{}, false, nil
	}
	on := st.Created.Time
	if !st.Created.Valid {
		on = st.Approved.Time
	}
	defaulted := false
	snap, err := u.rates.ResolveRate(ctx, on, u.currency, tryCurrency)
	if errors.Is(err, fxrates.ErrRateNotFound) {
		snap = fxrates.Snapshot{Base: u.currency, Quote: tryCurrency, Rate: "1",
			RateDate: on.UTC().Format("2006-01-02"), Source: "migrator_default"}
		defaulted, err = true, nil
	}
	if err != nil {
		return orderRate{}, false, fmt.Errorf("rate: %w", err)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return orderRate{}, false, fmt.Errorf("rate snapshot: %w", err)
	}
	var n pgtype.Numeric
	if err := n.Scan(snap.Rate); err != nil {
		return orderRate{}, false, fmt.Errorf("rate %q: %w", snap.Rate, err)
	}
	return orderRate{snapshot: raw, try: n}, defaulted, nil
}

// createdBy is the hub creator's account (warehouse users are not migrated).
func (s OrdersStep) createdBy(ctx context.Context, u *orderCtx, cand *orderCandidate) (pgtype.Int8, error) {
	if cand.Hub == nil || cand.Hub.CreatedBy == 0 {
		return pgtype.Int8{}, nil
	}
	if id, ok := u.users[cand.Hub.CreatedBy]; ok {
		return id, nil
	}
	target, ok, err := u.m.Lookup(ctx, s.system(), "users", strconv.FormatInt(cand.Hub.CreatedBy, 10))
	if err != nil {
		return pgtype.Int8{}, err
	}
	var out pgtype.Int8
	if ok {
		id, err := u.q.MigratorUserIDByUUID(ctx, target)
		switch {
		case err == nil:
			out = pgInt8(id)
		case !errors.Is(err, pgx.ErrNoRows):
			return out, fmt.Errorf("creator: %w", err)
		}
	}
	u.users[cand.Hub.CreatedBy] = out
	return out, nil
}

func (u *orderCtx) product(ctx context.Context, target uuid.UUID) (db.MigratorProductForUnitRow, bool, error) {
	if p, ok := u.products[target]; ok {
		return p, true, nil
	}
	p, err := u.q.MigratorProductForUnit(ctx, db.MigratorProductForUnitParams{Uuid: target, BrandID: u.brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, fmt.Errorf("product: %w", err)
	}
	u.products[target] = p
	return p, true, nil
}

// orderLines merges the legacy items of a candidate into product lines and
// returns a checksum of the legacy item and unit rows.
func (s OrdersStep) orderLines(ctx context.Context, u *orderCtx, l *legacyOrders, cand *orderCandidate, c counts) ([]*orderLine, string, error) {
	byProduct := map[int64]*orderLine{}
	var out []*orderLine
	var sumParts []any
	line := func(p db.MigratorProductForUnitRow) *orderLine {
		if ln, ok := byProduct[p.ID]; ok {
			return ln
		}
		ln := &orderLine{ProductID: p.ID, unitSeen: map[int64]bool{}}
		byProduct[p.ID] = ln
		out = append(out, ln)
		return ln
	}
	resolve := func(system, table, id, what string) (db.MigratorProductForUnitRow, bool, error) {
		target, ok, err := u.m.Lookup(ctx, system, table, id)
		if err != nil {
			return db.MigratorProductForUnitRow{}, false, err
		}
		if !ok {
			c.inc("lines_skipped_product_unmapped:" + what)
			return db.MigratorProductForUnitRow{}, false, nil
		}
		p, found, err := u.product(ctx, target)
		if err != nil {
			return p, false, err
		}
		if !found {
			c.inc("lines_skipped_product_unmapped:" + what)
			return p, false, nil
		}
		if p.UnitType == "roll_meter" {
			c.inc("lines_skipped_roll_meter:" + what)
			return p, false, nil
		}
		return p, true, nil
	}
	addUnit := func(ln *orderLine, system, table, id string, at sql.NullTime) error {
		target, ok, err := u.m.Lookup(ctx, system, table, id)
		if err != nil {
			return err
		}
		if !ok {
			c.inc("units_skipped_unmapped")
			return nil
		}
		unit, err := u.q.MigratorUnitByUUID(ctx, db.MigratorUnitByUUIDParams{Uuid: target, BrandID: u.brandID})
		if errors.Is(err, pgx.ErrNoRows) {
			c.inc("units_skipped_unmapped")
			return nil
		}
		if err != nil {
			return fmt.Errorf("unit: %w", err)
		}
		if unit.ProductID != ln.ProductID {
			c.inc("units_skipped_product_mismatch:" + table + ":" + id)
			return nil
		}
		if ln.unitSeen[unit.ID] {
			return nil
		}
		ln.unitSeen[unit.ID] = true
		ln.Units = append(ln.Units, orderLineUnit{ID: unit.ID, Kind: unit.UnitKind, AssignedAt: at})
		return nil
	}

	if h := cand.Hub; h != nil {
		hubQty := map[int64]int32{}
		for _, it := range l.hubItems[h.ID] {
			id := strconv.FormatInt(it.ID, 10)
			sumParts = append(sumParts, "hi", it.ID, it.ProductID, it.Quantity)
			for _, st := range l.hubStock[it.ID] {
				sumParts = append(sumParts, "hs", st.StockItemID)
			}
			if it.Quantity <= 0 {
				c.inc("lines_skipped_quantity:hub:" + id)
				continue
			}
			p, ok, err := resolve(s.system(), "products", strconv.FormatInt(it.ProductID, 10), "hub:"+id)
			if err != nil {
				return nil, "", err
			}
			if !ok {
				continue
			}
			ln := line(p)
			hubQty[p.ID] += it.Quantity
			ln.Keys = append(ln.Keys, Key{System: s.system(), Table: "order_items", ID: id, TargetTable: "order_items"})
			ln.Sums = append(ln.Sums, Checksum(it.ProductID, it.Quantity))
			if !ln.CreatedAt.Valid {
				ln.CreatedAt = it.CreatedAt
			}
			for _, st := range l.hubStock[it.ID] {
				if err := addUnit(ln, s.system(), "stock_items", strconv.FormatInt(st.StockItemID, 10), st.CreatedAt); err != nil {
					return nil, "", err
				}
			}
		}
		for pid, q := range hubQty {
			byProduct[pid].HubQty = q
		}
	}
	if w := cand.WH; w != nil {
		whQty := map[int64]int32{}
		for _, it := range l.whItems[w.ID] {
			sumParts = append(sumParts, "wi", it.ID, it.ProductID, it.Quantity)
			for _, b := range l.whBarcodes[it.ID] {
				sumParts = append(sumParts, "wb", b.BarcodeID)
			}
			switch whBrand(it.Brand) {
			case "glorian":
				c.inc("lines_glorian_skipped")
				continue
			case "":
				c.inc("lines_brand_unknown:" + strings.TrimSpace(it.Brand))
				c.inc("lines_skipped_brand_unknown:wh:" + it.ID)
				continue
			}
			if it.Quantity <= 0 {
				c.inc("lines_skipped_quantity:wh:" + it.ID)
				continue
			}
			p, ok, err := resolve(s.whSystem(), "products", it.ProductID, "wh:"+it.ID)
			if err != nil {
				return nil, "", err
			}
			if !ok {
				continue
			}
			ln := line(p)
			whQty[p.ID] += it.Quantity
			ln.Keys = append(ln.Keys, Key{System: s.whSystem(), Table: "order_items", ID: it.ID, TargetTable: "order_items"})
			ln.Sums = append(ln.Sums, Checksum(it.ProductID, it.Quantity))
			if !ln.CreatedAt.Valid {
				ln.CreatedAt = it.CreatedAt
			}
			for _, b := range l.whBarcodes[it.ID] {
				if err := addUnit(ln, s.whSystem(), "product_barcodes", b.BarcodeID, b.CreatedAt); err != nil {
					return nil, "", err
				}
			}
		}
		for pid, q := range whQty {
			byProduct[pid].WHQty = q
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ProductID < out[j].ProductID })
	return out, Checksum(sumParts...), nil
}

// writeLines writes the lines and their units of an order the migrator owns:
// a mapped line is updated in place, a new one is inserted; units are only
// added (order_item_units is unique by line and unit).
func (s OrdersStep) writeLines(ctx context.Context, u *orderCtx, orderID, orgID int64, lines []*orderLine, c counts) error {
	for _, ln := range lines {
		var target uuid.UUID
		for _, k := range ln.Keys {
			got, mapped, err := u.m.Lookup(ctx, k.System, k.Table, k.ID)
			if err != nil {
				return err
			}
			if mapped && target == uuid.Nil {
				target = got
			}
		}
		if target == uuid.Nil {
			existing, err := u.q.MigratorFindOrderItem(ctx, db.MigratorFindOrderItemParams{OrderID: orderID, ProductID: ln.ProductID})
			switch {
			case err == nil:
				target = existing
			case !errors.Is(err, pgx.ErrNoRows):
				return fmt.Errorf("find order line: %w", err)
			}
		}
		for i, k := range ln.Keys {
			_, mapped, err := u.m.Lookup(ctx, k.System, k.Table, k.ID)
			if err != nil {
				return err
			}
			if mapped || target != uuid.Nil {
				if !mapped {
					if _, err := u.m.Link(ctx, k, target, ln.Sums[i]); err != nil {
						return err
					}
				} else if _, err := u.m.Upsert(ctx, k, ln.Sums[i]); err != nil {
					return err
				}
				continue
			}
			res, err := u.m.Upsert(ctx, k, ln.Sums[i])
			if err != nil {
				return err
			}
			target = res.UUID
		}

		qty := ln.quantity()
		item, err := u.q.MigratorOrderItemByUUID(ctx, db.MigratorOrderItemByUUIDParams{Uuid: target, BrandID: u.brandID})
		var itemID int64
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			itemID, err = u.q.MigratorInsertOrderItem(ctx, db.MigratorInsertOrderItemParams{
				Uuid: target, OrderID: orderID, OrganizationID: orgID, BrandID: u.brandID, ProductID: ln.ProductID,
				Quantity: pgtype.Int4{Int32: qty, Valid: true}, CreatedAt: pgTime(ln.CreatedAt),
			})
			if err != nil {
				return fmt.Errorf("insert order line: %w", err)
			}
			c.inc("lines_created")
		case err != nil:
			return fmt.Errorf("read order line: %w", err)
		default:
			if item.OrderID != orderID || item.ProductID != ln.ProductID {
				c.inc("line_map_conflict:" + target.String())
				continue
			}
			itemID = item.ID
			if !item.Quantity.Valid || item.Quantity.Int32 != qty {
				if err := u.q.MigratorUpdateOrderItemQuantity(ctx, db.MigratorUpdateOrderItemQuantityParams{
					ID: itemID, BrandID: u.brandID, Quantity: pgtype.Int4{Int32: qty, Valid: true},
				}); err != nil {
					return fmt.Errorf("update order line: %w", err)
				}
				c.inc("lines_updated")
			} else {
				c.inc("lines_unchanged")
			}
		}

		for _, unit := range ln.Units {
			uq := int32(1)
			if unit.Kind == "fixed" && len(ln.Units) == 1 {
				uq = qty
			}
			n, err := u.q.MigratorInsertOrderItemUnit(ctx, db.MigratorInsertOrderItemUnitParams{
				OrderItemID: itemID, OrganizationID: orgID, BrandID: u.brandID, UnitID: unit.ID,
				Quantity: pgtype.Int4{Int32: uq, Valid: true}, AssignedAt: pgTime(unit.AssignedAt),
			})
			if err != nil {
				return fmt.Errorf("assign unit: %w", err)
			}
			if n > 0 {
				c.inc("units_assigned")
			}
		}
	}
	return nil
}
