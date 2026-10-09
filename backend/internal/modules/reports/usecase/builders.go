package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	perfmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	perfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

const unitCount = "count"

var (
	serviceStatuses     = []string{"draft", "pending", "processing", "ready", "completed", "cancelled"}
	orderStatuses       = []string{"draft", "submitted", "approved", "preparing", "ready", "processing", "shipped", "delivered", "received", "cancelling", "cancelled"}
	warrantyStatuses    = []string{"active", "expired", "void"}
	unitStatuses        = []string{"reserved", "printed", "available", "placed", "in_transit", "used", "void"}
	measurementStatuses = []string{"accepted", "vin_pending"}
	customerTypes       = []string{"individual", "corporate"}
	// performanceColumns are the dealers.performance metric columns.
	performanceColumns = []string{
		perfmodel.MetricServicesCount, perfmodel.MetricWarrantyStartRate, perfmodel.MetricMeasurementRate,
		perfmodel.MetricReviewAvg, perfmodel.MetricOrderVolume,
	}
)

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (s *Service) build(ctx context.Context, c Caller, f scopefilter.Filter, d Definition, q query, a args, env *Envelope) error {
	from, to := ts(q.from), ts(q.to)
	switch d.Key {
	case ReportServicesTrend:
		rows, err := s.q.ReportServiceTrend(ctx, db.ReportServiceTrendParams{
			Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs,
			CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: services trend: %w", err)
		}
		values := map[string]map[string]int64{}
		for _, r := range rows {
			add(values, r.Series, r.Bucket, r.Value)
		}
		env.Series = []Series{q.timeSeries("created", values), q.timeSeries("completed", values)}
	case ReportServicesStatusDistribution:
		rows, err := s.q.ReportServiceStatuses(ctx, db.ReportServiceStatusesParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: services statuses: %w", err)
		}
		counts := map[string]int64{}
		for _, r := range rows {
			counts[r.Status] = r.Value
		}
		env.Series = []Series{q.distribution("status", "services.status.", serviceStatuses, counts)}
	case ReportServicesTopBrands:
		rows, err := s.q.ServiceActivityCarBrands(ctx, db.ServiceActivityCarBrandsParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, PeriodFrom: from, PeriodTo: to, RowLimit: q.limit,
		})
		if err != nil {
			return fmt.Errorf("reports: top brands: %w", err)
		}
		sr := q.series("service_count", unitCount)
		for _, r := range rows {
			sr.Points = append(sr.Points, Point{Key: r.CarBrandUuid.String(), Label: r.CarBrandName, Value: float64(r.ServiceCount)})
		}
		env.Series = []Series{sr}
	case ReportServicesTopModels:
		rows, err := s.q.ReportServiceTopModels(ctx, db.ReportServiceTopModelsParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to, RowLimit: q.limit,
		})
		if err != nil {
			return fmt.Errorf("reports: top models: %w", err)
		}
		sr := q.series("service_count", unitCount)
		for _, r := range rows {
			sr.Points = append(sr.Points, Point{Key: r.CarModelUuid.String(), Label: r.CarBrandName + " " + r.CarModelName, Value: float64(r.ServiceCount)})
		}
		env.Series = []Series{sr}
	case ReportServicesTopProducts:
		rows, err := s.q.ServiceActivityTopProducts(ctx, db.ServiceActivityTopProductsParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, PeriodFrom: from, PeriodTo: to, RowLimit: q.limit,
		})
		if err != nil {
			return fmt.Errorf("reports: top products: %w", err)
		}
		sr := q.series("service_count", unitCount)
		for _, r := range rows {
			sr.Points = append(sr.Points, Point{Key: r.ProductUuid.String(), Label: r.ProductName, Value: float64(r.ServiceCount)})
		}
		env.Series = []Series{sr}
	case ReportOrdersTrend:
		rows, err := s.q.ReportOrderTrend(ctx, db.ReportOrderTrendParams{
			Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs,
			CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: orders trend: %w", err)
		}
		values := map[string]map[string]int64{}
		for _, r := range rows {
			add(values, r.Series, r.Bucket, r.Value)
		}
		env.Series = []Series{q.timeSeries("created", values), q.timeSeries("received", values)}
	case ReportOrdersStatusDistribution:
		rows, err := s.q.ReportOrderStatuses(ctx, db.ReportOrderStatusesParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: orders statuses: %w", err)
		}
		counts := map[string]int64{}
		for _, r := range rows {
			counts[r.Status] = r.Value
		}
		env.Series = []Series{q.distribution("status", "orders.status.", orderStatuses, counts)}
	case ReportCustomersTrend:
		rows, err := s.q.ReportCustomerTrend(ctx, db.ReportCustomerTrendParams{
			Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: customers trend: %w", err)
		}
		values := map[string]map[string]int64{}
		for _, r := range rows {
			add(values, r.Series, r.Bucket, r.Value)
		}
		for _, t := range customerTypes {
			env.Series = append(env.Series, q.timeSeries(t, values))
		}
	case ReportStockSummary:
		totals, err := s.q.ReportStockTotals(ctx, db.ReportStockTotalsParams{BrandID: a.brandID, OrgIds: a.orgIDs})
		if err != nil {
			return fmt.Errorf("reports: stock totals: %w", err)
		}
		rows, err := s.q.ReportStockUnitStatuses(ctx, db.ReportStockUnitStatusesParams{BrandID: a.brandID, OrgIds: a.orgIDs})
		if err != nil {
			return fmt.Errorf("reports: stock statuses: %w", err)
		}
		counts := map[string]int64{}
		for _, r := range rows {
			counts[r.Status] = r.Value
		}
		cards := q.series("cards", unitCount)
		cards.Points = []Point{
			q.card("stock_products", float64(totals.ProductCount)),
			q.card("stock_quantity", float64(totals.Quantity)),
			q.card("stock_meters", totals.Meters),
		}
		env.Series = []Series{cards, q.distribution("units", "units.status.", unitStatuses, counts)}
	case ReportWarrantiesSummary:
		counts, err := s.q.ReportWarrantyCounts(ctx, db.ReportWarrantyCountsParams{
			NowAt: ts(s.now()), RangeFrom: from, RangeTo: to, BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy,
		})
		if err != nil {
			return fmt.Errorf("reports: warranty counts: %w", err)
		}
		rows, err := s.q.ReportWarrantyTrend(ctx, db.ReportWarrantyTrendParams{
			Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs,
			CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: warranty trend: %w", err)
		}
		values := map[string]map[string]int64{}
		for _, r := range rows {
			add(values, "started", r.Bucket, r.Value)
		}
		cards := q.series("cards", unitCount)
		cards.Points = []Point{
			q.card("warranties_active", float64(counts.ActiveCount)),
			q.card("warranties_expiring", float64(counts.ExpiringCount)),
			q.card("warranties_started", float64(counts.StartedCount)),
			q.card("warranties_expired", float64(counts.ExpiredCount)),
			q.card("warranties_void", float64(counts.VoidCount)),
		}
		status := q.distribution("status", "warranties.status.", warrantyStatuses, map[string]int64{
			"active": counts.ActiveCount, "expired": counts.ExpiredCount, "void": counts.VoidCount,
		})
		env.Series = []Series{cards, status, q.timeSeries("started", values)}
	case ReportMeasurementsSummary:
		counts, err := s.q.ReportMeasurementCounts(ctx, db.ReportMeasurementCountsParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: measurement counts: %w", err)
		}
		rows, err := s.q.ReportMeasurementTrend(ctx, db.ReportMeasurementTrendParams{
			Granularity: q.granularity, Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs,
			CreatedByUserID: a.createdBy, RangeFrom: from, RangeTo: to,
		})
		if err != nil {
			return fmt.Errorf("reports: measurement trend: %w", err)
		}
		values := map[string]map[string]int64{}
		for _, r := range rows {
			add(values, "measurements", r.Bucket, r.Value)
		}
		cards := q.series("cards", unitCount)
		cards.Points = []Point{
			q.card("measurements_total", float64(counts.TotalCount)),
			q.card("measurements_linked", float64(counts.LinkedCount)),
			q.card("measurements_vehicles", float64(counts.VehicleCount)),
		}
		status := q.distribution("status", "measurements.status.", measurementStatuses, map[string]int64{
			"accepted": counts.AcceptedCount, "vin_pending": counts.VinPendingCount,
		})
		env.Series = []Series{cards, status, q.timeSeries("measurements", values)}
	case ReportDealersPerformance:
		return s.dealersPerformance(ctx, c, f, q, env)
	case ReportDealersTopByWarranty:
		rows, err := s.q.ReportTopDealersByWarranty(ctx, db.ReportTopDealersByWarrantyParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: from, RangeTo: to, RowLimit: q.limit,
		})
		if err != nil {
			return fmt.Errorf("reports: top dealers: %w", err)
		}
		sr := q.series("warranty_count", unitCount)
		sr.Label = translate(q.locale, "column.warranty_count")
		env.Columns = q.columns([][2]string{
			{"rank", "integer"}, {"organization", "string"}, {"city", "string"}, {"warranty_count", "integer"}, {"active_count", "integer"},
		})
		for i, r := range rows {
			sr.Points = append(sr.Points, Point{Key: r.OrganizationUuid.String(), Label: r.OrganizationName, Value: float64(r.WarrantyCount)})
			env.Rows = append(env.Rows, map[string]any{
				"rank": i + 1, "organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
				"city": r.City, "warranty_count": r.WarrantyCount, "active_count": r.ActiveCount,
			})
		}
		env.Series = []Series{sr}
	case ReportActivitiesRecent:
		rows, err := s.q.ReportRecentServices(ctx, db.ReportRecentServicesParams{
			BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, RowLimit: q.limit,
		})
		if err != nil {
			return fmt.Errorf("reports: recent services: %w", err)
		}
		env.Columns = q.columns([][2]string{
			{"service_no", "string"}, {"plate", "string"}, {"vehicle", "string"}, {"organization", "string"},
			{"status", "string"}, {"created_at", "datetime"},
		})
		for _, r := range rows {
			row := map[string]any{
				"uuid": r.Uuid, "service_no": r.ServiceNo, "plate": nil,
				"vehicle":           r.CarBrandName + " " + r.CarModelName,
				"organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
				"status": r.Status, "status_label": translate(q.locale, "services.status."+r.Status),
				"created_at": r.CreatedAt.Time.UTC(), "completed_at": nil,
			}
			if r.Plate.Valid {
				row["plate"] = r.Plate.String
			}
			if r.CompletedAt.Valid {
				row["completed_at"] = r.CompletedAt.Time.UTC()
			}
			env.Rows = append(env.Rows, row)
		}
	default:
		return ErrNotFound
	}
	return nil
}

// dealersPerformance reads the performance projection: the ranking of the
// organizations below a center / distributor, the own row of a dealer
// (legacy dealer_owner: one row). The month is the month of range_to.
func (s *Service) dealersPerformance(ctx context.Context, c Caller, f scopefilter.Filter, q query, env *Envelope) error {
	month := q.toDate.Format("2006-01")
	pc := perfuc.Caller{Principal: c.Principal, Org: c.Org, Filter: f}
	var rows []perfuc.RankingRow
	if c.Org.OrgType == "dealer" {
		b, err := s.perf.Benchmark(ctx, pc, month)
		if err != nil {
			return perfErr(err)
		}
		if b.Own.Metrics != nil {
			b.Own.Name = c.Org.Name
			rows = append(rows, b.Own)
		}
	} else {
		var err error
		rows, _, err = s.perf.ListRanking(ctx, pc, perfuc.RankingFilter{Period: month, Limit: q.limit})
		if err != nil {
			return perfErr(err)
		}
	}
	cols := [][2]string{{"rank", "integer"}, {"organization", "string"}, {"type", "string"}}
	for _, m := range performanceColumns {
		cols = append(cols, [2]string{m, "number"})
	}
	env.Columns = q.columns(cols)
	sr := q.series("service_count", unitCount)
	for _, r := range rows {
		row := map[string]any{"rank": r.Rank, "organization_uuid": r.OrganizationUUID, "organization": r.Name, "type": r.Type, "period": month}
		for _, m := range performanceColumns {
			row[m] = metricNumber(r.Metrics[m])
		}
		env.Rows = append(env.Rows, row)
		if v, ok := row[perfmodel.MetricServicesCount].(float64); ok {
			sr.Points = append(sr.Points, Point{Key: r.OrganizationUUID.String(), Label: r.Name, Value: v})
		}
	}
	env.Series = []Series{sr}
	return nil
}

func perfErr(err error) error {
	if errors.Is(err, perfuc.ErrForbidden) {
		return ErrForbidden
	}
	return err
}

func metricNumber(m *perfuc.MetricValue) any {
	if m == nil {
		return nil
	}
	v, err := strconv.ParseFloat(m.Value, 64)
	if err != nil {
		return nil
	}
	return v
}

// add stores one bucket value of a series.
func add(values map[string]map[string]int64, series string, bucket pgtype.Date, v int64) {
	if !bucket.Valid {
		return
	}
	if values[series] == nil {
		values[series] = map[string]int64{}
	}
	values[series][bucket.Time.Format(dateLayout)] += v
}

func (q query) series(key, unit string) Series {
	return Series{Key: key, Label: translate(q.locale, "series."+key), Unit: unit, Points: []Point{}}
}

// timeSeries fills every bucket of the range (missing = 0).
func (q query) timeSeries(key string, values map[string]map[string]int64) Series {
	sr := q.series(key, unitCount)
	for _, b := range q.buckets() {
		sr.Points = append(sr.Points, Point{Key: b, Label: b, Value: float64(values[key][b])})
	}
	return sr
}

// distribution lists every known value in a fixed order (missing = 0).
func (q query) distribution(key, labelPrefix string, order []string, counts map[string]int64) Series {
	sr := q.series(key, unitCount)
	for _, k := range order {
		sr.Points = append(sr.Points, Point{Key: k, Label: translate(q.locale, labelPrefix+k), Value: float64(counts[k])})
	}
	return sr
}

func (q query) card(key string, v float64) Point {
	return Point{Key: key, Label: translate(q.locale, "card."+key), Value: v}
}

func (q query) columns(defs [][2]string) []Column {
	out := make([]Column, 0, len(defs))
	for _, d := range defs {
		out = append(out, Column{Key: d[0], Label: translate(q.locale, "column."+d[0]), Type: d[1]})
	}
	return out
}

// overviewDomain is one group of overview cards gated like its report.
type overviewDomain struct {
	def    Definition
	filter scopefilter.Filter
}

// overviewDomains lists the card groups open to the caller.
func (s *Service) overviewDomains(ctx context.Context, c Caller) ([]overviewDomain, error) {
	var out []overviewDomain
	for _, key := range []string{ReportServicesTrend, ReportOrdersTrend, ReportCustomersTrend, ReportWarrantiesSummary, ReportStockSummary, ReportMeasurementsSummary, networkDef.Key} {
		d := networkDef
		if key != networkDef.Key {
			d, _ = DefinitionByKey(key)
		}
		f, err := s.access(ctx, c, d)
		switch {
		case err == nil:
			out = append(out, overviewDomain{def: d, filter: f})
		case errors.Is(err, ErrForbidden), errors.Is(err, ErrFeatureDisabled):
		default:
			return nil, err
		}
	}
	return out, nil
}

// overview is the legacy 15+ card dashboard: one card group per domain the
// caller reaches (module on + read permission), each in its own scope.
// Network cards (dealer counts) only for a multi-organization reach.
func (s *Service) overview(ctx context.Context, c Caller, d Definition, q query) (Envelope, error) {
	domains, err := s.overviewDomains(ctx, c)
	if err != nil {
		return Envelope{}, err
	}
	if len(domains) == 0 {
		return Envelope{}, ErrForbidden
	}
	env := s.envelope(d, q, domains[0].filter)
	cards := q.series("cards", unitCount)
	from, to := ts(q.from), ts(q.to)
	for _, dom := range domains {
		f := dom.filter
		orgIDs := f.OrgIDsArg()
		var createdBy pgtype.Int8
		if f.UserOnly() {
			createdBy = pgtype.Int8{Int64: f.UserID, Valid: true}
		}
		switch dom.def.Key {
		case ReportServicesTrend:
			r, err := s.q.ReportServiceOverview(ctx, db.ReportServiceOverviewParams{
				RangeFrom: from, RangeTo: to, BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
			})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview services: %w", err)
			}
			cards.Points = append(cards.Points, q.card("services_created", float64(r.CreatedCount)),
				q.card("services_completed", float64(r.CompletedCount)), q.card("services_open", float64(r.OpenCount)))
		case ReportOrdersTrend:
			r, err := s.q.ReportOrderOverview(ctx, db.ReportOrderOverviewParams{
				RangeFrom: from, RangeTo: to, BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
			})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview orders: %w", err)
			}
			cards.Points = append(cards.Points, q.card("orders_created", float64(r.CreatedCount)), q.card("orders_open", float64(r.OpenCount)))
		case ReportCustomersTrend:
			r, err := s.q.ReportCustomerOverview(ctx, db.ReportCustomerOverviewParams{
				RangeFrom: from, RangeTo: to, BrandID: c.Org.BrandID, OrgIds: orgIDs,
			})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview customers: %w", err)
			}
			cards.Points = append(cards.Points, q.card("customers_total", float64(r.TotalCount)), q.card("customers_new", float64(r.NewCount)))
		case ReportWarrantiesSummary:
			r, err := s.q.ReportWarrantyCounts(ctx, db.ReportWarrantyCountsParams{
				NowAt: ts(s.now()), RangeFrom: from, RangeTo: to, BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
			})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview warranties: %w", err)
			}
			cards.Points = append(cards.Points, q.card("warranties_active", float64(r.ActiveCount)),
				q.card("warranties_expiring", float64(r.ExpiringCount)), q.card("warranties_started", float64(r.StartedCount)))
		case ReportStockSummary:
			r, err := s.q.ReportStockTotals(ctx, db.ReportStockTotalsParams{BrandID: c.Org.BrandID, OrgIds: orgIDs})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview stock: %w", err)
			}
			cards.Points = append(cards.Points, q.card("stock_products", float64(r.ProductCount)),
				q.card("stock_quantity", float64(r.Quantity)), q.card("stock_meters", r.Meters))
		case ReportMeasurementsSummary:
			r, err := s.q.ReportMeasurementCounts(ctx, db.ReportMeasurementCountsParams{
				BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy, RangeFrom: from, RangeTo: to,
			})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview measurements: %w", err)
			}
			cards.Points = append(cards.Points, q.card("measurements_total", float64(r.TotalCount)))
		case networkDef.Key:
			r, err := s.q.ReportNetworkCounts(ctx, db.ReportNetworkCountsParams{BrandID: c.Org.BrandID, OrgIds: orgIDs})
			if err != nil {
				return Envelope{}, fmt.Errorf("reports: overview network: %w", err)
			}
			cards.Points = append(cards.Points, q.card("dealers_total", float64(r.DealerCount)), q.card("dealers_active", float64(r.ActiveDealerCount)))
		}
	}
	env.Series = []Series{cards}
	return env, nil
}
