package usecase

// TEC-1210 (analytics A): the role reports — orders.fulfillment for the
// warehouse, network.summary for the distributor and the center,
// stock.movement for the stock keepers.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// orderFulfillment: shipped / received time series, the warehouse KPIs
// (shipped, received, average preparation hours approved -> shipped, open,
// overdue = open and approved more than 48 hours ago, see
// ReportFulfillmentByWarehouse) and one row per shipping warehouse.
func (s *Service) orderFulfillment(ctx context.Context, q query, a args, env *Envelope) error {
	from, to := ts(q.from), ts(q.to)
	trend, err := s.q.ReportFulfillmentTrend(ctx, db.ReportFulfillmentTrendParams{
		Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return fmt.Errorf("reports: fulfillment trend: %w", err)
	}
	vals := map[string]map[string]int64{}
	for _, r := range trend {
		add(vals, r.Series, r.Bucket, r.Value)
	}
	rows, err := s.q.ReportFulfillmentByWarehouse(ctx, db.ReportFulfillmentByWarehouseParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to, NowAt: ts(s.now()),
	})
	if err != nil {
		return fmt.Errorf("reports: fulfillment by warehouse: %w", err)
	}
	env.Columns = q.columns([][2]string{
		{"warehouse", "string"}, {"organization", "org"}, {"shipped_count", "integer"}, {"received_count", "integer"},
		{"open_count", "integer"}, {"overdue_count", "integer"}, {"avg_prep_hours", "duration"},
	})
	var shipped, received, open, overdue, prepCount int64
	var prepSeconds float64
	for _, r := range rows {
		shipped += r.ShippedCount
		received += r.ReceivedCount
		open += r.OpenCount
		overdue += r.OverdueCount
		prepCount += r.PrepCount
		prepSeconds += r.PrepSeconds
		row := map[string]any{
			"warehouse_uuid": nil, "warehouse": translate(q.locale, "row.no_warehouse"),
			"organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
			"shipped_count": r.ShippedCount, "received_count": r.ReceivedCount,
			"open_count": r.OpenCount, "overdue_count": r.OverdueCount, "avg_prep_hours": nil,
			rowLink: link(LinkOrganization, r.OrganizationUuid.String()),
		}
		if r.WarehouseUuid.Valid {
			row["warehouse_uuid"] = r.WarehouseUuid.String()
			row["warehouse"] = r.WarehouseName.String
		}
		if r.PrepCount > 0 {
			row["avg_prep_hours"] = round(r.PrepSeconds/float64(r.PrepCount)/3600, 1)
		}
		env.Rows = append(env.Rows, row)
	}
	var prepHours float64
	if prepCount > 0 {
		prepHours = round(prepSeconds/float64(prepCount)/3600, 1)
	}
	shippedSr, receivedSr := q.timeSeries("shipped", vals), q.timeSeries("received", vals)
	cards := q.series(seriesCards, unitCount)
	cards.Points = []Point{
		q.card("fulfillment_shipped", float64(shipped)),
		q.card("fulfillment_received", float64(received)),
		q.card("fulfillment_prep_hours", prepHours),
		q.card("fulfillment_open", float64(open)),
		q.card("fulfillment_overdue", float64(overdue)),
	}
	env.Series = []Series{cards, shippedSr, receivedSr}
	env.KPIs = kpisFromCards(ReportOrdersFulfillment, cards, map[string][]float64{
		"fulfillment_shipped": valuesOf(shippedSr), "fulfillment_received": valuesOf(receivedSr),
	})
	return nil
}

// networkSummary: the services completed, orders and warranties started in
// the reach (time series and KPIs), the active dealers and one row per
// dealer, most services first.
func (s *Service) networkSummary(ctx context.Context, q query, a args, env *Envelope) error {
	from, to := ts(q.from), ts(q.to)
	gran, tz := q.granularity, q.tz.String()
	svcRows, err := s.q.ReportServiceTrend(ctx, db.ReportServiceTrendParams{
		Granularity: gran, Tz: tz, BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return fmt.Errorf("reports: network services: %w", err)
	}
	ordRows, err := s.q.ReportOrderTrend(ctx, db.ReportOrderTrendParams{
		Granularity: gran, Tz: tz, BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return fmt.Errorf("reports: network orders: %w", err)
	}
	warRows, err := s.q.ReportWarrantyTrend(ctx, db.ReportWarrantyTrendParams{
		Granularity: gran, Tz: tz, BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return fmt.Errorf("reports: network warranties: %w", err)
	}
	counts, err := s.q.ReportNetworkCounts(ctx, db.ReportNetworkCountsParams{BrandID: a.brandID, OrgIds: a.orgIDs})
	if err != nil {
		return fmt.Errorf("reports: network counts: %w", err)
	}
	dealers, err := s.q.ReportNetworkDealers(ctx, db.ReportNetworkDealersParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to, RowLimit: q.limit,
	})
	if err != nil {
		return fmt.Errorf("reports: network dealers: %w", err)
	}
	vals := map[string]map[string]int64{}
	for _, r := range svcRows {
		if r.Series == "completed" {
			add(vals, "services", r.Bucket, r.Value)
		}
	}
	for _, r := range ordRows {
		if r.Series == "created" {
			add(vals, "orders", r.Bucket, r.Value)
		}
	}
	for _, r := range warRows {
		add(vals, "warranties", r.Bucket, r.Value)
	}
	services, orders, warranties := q.timeSeries("services", vals), q.timeSeries("orders", vals), q.timeSeries("warranties", vals)
	cards := q.series(seriesCards, unitCount)
	cards.Points = []Point{
		q.card("network_services", sum(services)),
		q.card("network_orders", sum(orders)),
		q.card("network_warranties", sum(warranties)),
		q.card("network_active_dealers", float64(counts.ActiveDealerCount)),
	}
	env.Columns = q.columns([][2]string{
		{"rank", "integer"}, {"organization", "org"}, {"city", "string"}, {"status", "status"},
		{"services_count", "integer"}, {"orders_count", "integer"}, {"warranties_count", "integer"},
	})
	for i, r := range dealers {
		env.Rows = append(env.Rows, map[string]any{
			"rank": i + 1, "organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName, "city": r.City,
			"status": r.Status, "services_count": r.ServicesCount, "orders_count": r.OrdersCount, "warranties_count": r.WarrantiesCount,
			rowLink:   link(LinkOrganization, r.OrganizationUuid.String()),
			rowStatus: map[string]StatusCell{"status": q.statusCell("organizations", "organizations.status.", r.Status)},
		})
	}
	env.Series = []Series{cards, services, orders, warranties}
	env.KPIs = kpisFromCards(ReportNetworkSummary, cards, map[string][]float64{
		"network_services": valuesOf(services), "network_orders": valuesOf(orders), "network_warranties": valuesOf(warranties),
	})
	return nil
}

// stockMovement: serial units into and out of the warehouses in reach (time
// series) and one row per warehouse (in, out, net).
func (s *Service) stockMovement(ctx context.Context, q query, a args, env *Envelope) error {
	from, to := ts(q.from), ts(q.to)
	trend, err := s.q.ReportStockMovementTrend(ctx, db.ReportStockMovementTrendParams{
		Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return fmt.Errorf("reports: stock movement trend: %w", err)
	}
	rows, err := s.q.ReportStockMovementByWarehouse(ctx, db.ReportStockMovementByWarehouseParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to, RowLimit: q.limit,
	})
	if err != nil {
		return fmt.Errorf("reports: stock movement by warehouse: %w", err)
	}
	vals := map[string]map[string]int64{}
	for _, r := range trend {
		add(vals, r.Series, r.Bucket, r.Value)
	}
	env.Columns = q.columns([][2]string{
		{"warehouse", "string"}, {"organization", "org"}, {"in_count", "integer"}, {"out_count", "integer"}, {"net_count", "integer"},
	})
	for _, r := range rows {
		env.Rows = append(env.Rows, map[string]any{
			"warehouse_uuid": r.WarehouseUuid, "warehouse": r.WarehouseName,
			"organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
			"in_count": r.InCount, "out_count": r.OutCount, "net_count": r.InCount - r.OutCount,
			rowLink: link(LinkOrganization, r.OrganizationUuid.String()),
		})
	}
	in, out := q.timeSeries("in", vals), q.timeSeries("out", vals)
	env.Series = []Series{in, out}
	return nil
}

// AgingBuckets are the age buckets (days since the last ledger movement)
// of stock.aging.
var AgingBuckets = []string{"qty_0_30", "qty_30_60", "qty_60_90", "qty_90_180", "qty_180_plus"}

// stockAging (TEC-1229): stock on hand of the organizations in reach per
// product and warehouse, bucketed by the days since the last ledger
// movement, oldest first (the limit parameter caps the rows). The unit price
// and stock value columns (recommended sale price of the brand price
// list) need pricing.purchase.read or pricing.sale.read; a warehouse role
// never sees them.
func (s *Service) stockAging(ctx context.Context, c Caller, q query, a args, env *Envelope) error {
	rows, err := s.q.ReportStockAging(ctx, db.ReportStockAgingParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, AsOf: ts(s.now()), MinDays: 0, RowLimit: q.limit,
	})
	if err != nil {
		return fmt.Errorf("reports: stock aging: %w", err)
	}
	priced := orderAmountsVisible(c.Principal)
	defs := [][2]string{
		{"product", "string"}, {"sku", "string"}, {"warehouse", "string"}, {"organization", "org"},
		{"quantity", "integer"}, {"meters", "number"},
		{"qty_0_30", "integer"}, {"qty_30_60", "integer"}, {"qty_60_90", "integer"},
		{"qty_90_180", "integer"}, {"qty_180_plus", "integer"},
		{"oldest_days", "integer"}, {"oldest_movement_at", "datetime"},
	}
	if priced {
		defs = append(defs, [2]string{"unit_price", "money"}, [2]string{"stock_value", "money"})
	}
	env.Columns = q.columns(defs)
	var total, old float64
	for _, r := range rows {
		row := map[string]any{
			"product_uuid": r.ProductUuid, "product": r.ProductName, "sku": r.Sku,
			"warehouse": nil, "warehouse_uuid": nil,
			"organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
			"quantity": r.Quantity, "meters": parseNum(r.Meters),
			"qty_0_30": r.Qty030, "qty_30_60": r.Qty3060, "qty_60_90": r.Qty6090,
			"qty_90_180": r.Qty90180, "qty_180_plus": r.Qty180Plus,
			"oldest_days": r.OldestDays, "oldest_movement_at": r.OldestMovementAt.Time.UTC(),
			rowLink: link(LinkProduct, r.ProductUuid.String()),
		}
		if r.WarehouseUuid.Valid {
			row["warehouse_uuid"], row["warehouse"] = uuid.UUID(r.WarehouseUuid.Bytes).String(), r.WarehouseName.String
		}
		if priced {
			row["unit_price"], row["stock_value"], row["currency"] = nil, nil, nil
			if r.UnitPrice != "" {
				price := parseNum(r.UnitPrice)
				row["unit_price"] = round(price, 2)
				row["stock_value"] = round(price*float64(r.Quantity), 2)
				row["currency"] = r.Currency
			}
		}
		total += float64(r.Quantity)
		old += float64(r.Qty90180 + r.Qty180Plus)
		env.Rows = append(env.Rows, row)
	}
	env.KPIs = append(env.KPIs,
		KPI{Key: "aging_quantity", Label: translate(q.locale, "kpi.aging_quantity"), Unit: unitCount, Value: total, Good: "down", Spark: []float64{}},
		KPI{Key: "aging_over_90", Label: translate(q.locale, "kpi.aging_over_90"), Unit: unitCount, Value: old, Good: "down", Spark: []float64{}},
	)
	return nil
}

func parseNum(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
