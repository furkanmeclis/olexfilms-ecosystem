package usecase

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/jackc/pgx/v5/pgtype"
)

type RunResult struct {
	Organizations int
	Periods       int
	Metrics       int
}

func (s *Service) DailyTask(ctx context.Context, organizationID int64, computedAt time.Time) error {
	if computedAt.IsZero() {
		computedAt = s.now()
	}
	_, err := s.Run(ctx, organizationID, computedAt)
	return err
}

func (s *Service) Run(ctx context.Context, organizationID int64, at time.Time) (RunResult, error) {
	var orgArg pgtype.Int8
	if organizationID > 0 {
		orgArg = pgtype.Int8{Int64: organizationID, Valid: true}
	}
	orgs, err := s.q.ListPerformanceOrganizations(ctx, orgArg)
	if err != nil {
		return RunResult{}, fmt.Errorf("performance: list organizations: %w", err)
	}
	var res RunResult
	for _, org := range orgs {
		if organizationID == 0 && localHour(at, org.Timezone) != 3 {
			continue
		}
		on, err := s.featureOn(ctx, org.ID, features.ModulePerformance)
		if err != nil {
			return res, err
		}
		if !on {
			continue
		}
		one, err := s.RunOrganization(ctx, org, at)
		if err != nil {
			return res, err
		}
		res.Organizations++
		res.Periods += one.Periods
		res.Metrics += one.Metrics
	}
	return res, nil
}

func (s *Service) RunOrganization(ctx context.Context, org db.Organization, at time.Time) (RunResult, error) {
	local := at.In(loadLocation(org.Timezone))
	periods := []time.Time{
		time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC),
		time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0),
	}
	var res RunResult
	for _, month := range periods {
		n, err := s.ComputeMonth(ctx, org, month, at)
		if err != nil {
			return res, err
		}
		res.Periods++
		res.Metrics += n
	}
	if org.Type == "dealer" && local.Day() == 1 {
		previous := periods[1].Format("2006-01")
		if _, err := s.calculateBonuses(ctx, org, previous, false); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (s *Service) ComputeMonth(ctx context.Context, org db.Organization, month, at time.Time) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("performance: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	flags, err := s.metricFlags(ctx, org.ID)
	if err != nil {
		return 0, err
	}
	total := 0
	own, err := s.computeScope(ctx, q, org, ScopeOrg, []int64{org.ID}, month, at, flags)
	if err != nil {
		return 0, err
	}
	total += own
	if org.Type == "center" || org.Type == "distributor" {
		ids, err := q.ListPerformanceSubtreeOrgIDs(ctx, org.ID)
		if err != nil {
			return 0, fmt.Errorf("performance: subtree ids: %w", err)
		}
		n, err := s.computeScope(ctx, q, org, ScopeSubtree, ids, month, at, flags)
		if err != nil {
			return 0, err
		}
		total += n
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, computedEvent(org, month, total)); err != nil {
			return 0, fmt.Errorf("performance: computed event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("performance: commit: %w", err)
	}
	return total, nil
}

type metricFlags struct {
	measurements bool
	reviews      bool
	certificates bool
	efficiency   bool
	leads        bool
}

func (s *Service) metricFlags(ctx context.Context, orgID int64) (metricFlags, error) {
	on := func(key string) (bool, error) { return s.featureOn(ctx, orgID, key) }
	measurements, err := on(features.ModuleMeasurements)
	if err != nil {
		return metricFlags{}, err
	}
	reviews, err := on(features.ModuleReviews)
	if err != nil {
		return metricFlags{}, err
	}
	certificates, err := on(features.ModuleCertificates)
	if err != nil {
		return metricFlags{}, err
	}
	efficiency, err := on(features.ModuleEfficiency)
	if err != nil {
		return metricFlags{}, err
	}
	leads, err := on(features.ModuleLeads)
	if err != nil {
		return metricFlags{}, err
	}
	return metricFlags{measurements: measurements, reviews: reviews, certificates: certificates, efficiency: efficiency, leads: leads}, nil
}

func (s *Service) computeScope(ctx context.Context, q *db.Queries, org db.Organization, scope string, orgIDs []int64, month, at time.Time, flags metricFlags) (int, error) {
	from := month
	to := month.AddDate(0, 1, 0)
	if _, err := q.DeletePerformanceMetricsForScope(ctx, db.DeletePerformanceMetricsForScopeParams{
		OrganizationID: org.ID, Period: period(month), Scope: scope,
	}); err != nil {
		return 0, fmt.Errorf("performance: clear %s/%s/%s: %w", org.Slug, period(month), scope, err)
	}
	row, err := q.ComputePerformanceMetrics(ctx, db.ComputePerformanceMetricsParams{
		BrandID: org.BrandID, OrgIds: orgIDs,
		PeriodFrom: ts(from), PeriodTo: ts(to), AsOf: ts(at),
	})
	if err != nil {
		return 0, fmt.Errorf("performance: compute %s %s: %w", org.Slug, period(month), err)
	}
	write := metricWriter{ctx: ctx, q: q, org: org, period: period(month), scope: scope}
	write.value(model.MetricServicesCount, row.ServicesCount, nil, nil, "")
	write.rate(model.MetricWarrantyStartRate, row.WarrantyStarted, row.ServicesCount, true)
	if flags.measurements {
		write.rate(model.MetricMeasurementRate, row.Measurements, row.ServicesCount, true)
	}
	if flags.reviews && positive(row.ReviewCount) {
		write.value(model.MetricReviewAvg, row.ReviewAvg, &row.ReviewCount, nil, "")
	}
	write.stockTurnover(row)
	write.value(model.MetricContractDaysLeft, intNumeric(daysLeft(org.ContractValidUntil, at)), nil, nil, "")
	write.value(model.MetricCariOverdueAmount, row.CariOverdueAmount, nil, nil, org.Currency)
	write.value(model.MetricCariOverdueDays, row.CariOverdueDays, nil, nil, "")
	if flags.certificates {
		write.rate(model.MetricCertificateCoverage, row.CertifiedStaffCount, row.ServiceStaffCount, false)
	}
	if flags.leads {
		write.rate(model.MetricLeadConversionRate, row.WonLeads, row.ClosedLeads, false)
	}
	if flags.efficiency && row.WasteRatio.Valid {
		write.value(model.MetricWasteRatio, row.WasteRatio, nil, nil, "")
	}
	write.value(model.MetricOrderVolume, row.OrderVolume, nil, nil, org.Currency)
	if write.err != nil {
		return 0, write.err
	}
	return write.count, nil
}

type metricWriter struct {
	ctx    context.Context
	q      *db.Queries
	org    db.Organization
	period string
	scope  string
	count  int
	err    error
}

func (w *metricWriter) rate(metric string, numerator, denominator pgtype.Numeric, zeroWhenNoDenominator bool) {
	if w.err != nil {
		return
	}
	den, ok := numericFloat(denominator)
	if !ok || den == 0 {
		if zeroWhenNoDenominator {
			w.value(metric, intNumeric(0), &numerator, &denominator, "")
		}
		return
	}
	num, _ := numericFloat(numerator)
	w.value(metric, floatNumeric(num/den), &numerator, &denominator, "")
}

func (w *metricWriter) stockTurnover(row db.ComputePerformanceMetricsRow) {
	consumedQty, _ := numericFloat(row.StockConsumedQty)
	consumedMeters, _ := numericFloat(row.StockConsumedMeters)
	onHandQty, _ := numericFloat(row.StockOnHandQty)
	onHandMeters, _ := numericFloat(row.StockOnHandMeters)
	consumed := consumedQty + consumedMeters
	onHand := onHandQty + onHandMeters
	if onHand == 0 {
		return
	}
	num, den := floatNumeric(consumed), floatNumeric(onHand)
	w.value(model.MetricStockTurnover, floatNumeric(consumed/onHand), &num, &den, "")
}

func (w *metricWriter) value(metric string, value pgtype.Numeric, numerator, denominator *pgtype.Numeric, currency string) {
	if w.err != nil || !value.Valid {
		return
	}
	arg := db.UpsertPerformanceMetricParams{
		OrganizationID: w.org.ID, BrandID: w.org.BrandID, Period: w.period, Scope: w.scope,
		Metric: metric, Value: value,
	}
	if numerator != nil && numerator.Valid {
		arg.Numerator = *numerator
	}
	if denominator != nil && denominator.Valid {
		arg.Denominator = *denominator
	}
	if currency != "" {
		arg.Currency = pgtype.Text{String: currency, Valid: true}
	}
	if _, err := w.q.UpsertPerformanceMetric(w.ctx, arg); err != nil {
		w.err = fmt.Errorf("performance: upsert %s/%s/%s: %w", w.period, w.scope, metric, err)
		return
	}
	w.count++
}

func (s *Service) featureOn(ctx context.Context, orgID int64, key string) (bool, error) {
	if s.features == nil {
		return false, nil
	}
	on, err := s.features.Enabled(ctx, orgID, key)
	if err != nil {
		return false, fmt.Errorf("performance: feature %s org %d: %w", key, orgID, err)
	}
	return on, nil
}

func computedEvent(org db.Organization, month time.Time, count int) events.Event {
	payload := map[string]any{
		"organization_id": org.ID,
		"brand_id":        org.BrandID,
		"period":          period(month),
		"metrics_count":   count,
	}
	id, u := org.ID, org.Uuid
	return events.New(EventPerformanceComputed).WithTenant(org.ID).WithEntity("organization", &id, &u).WithPayload(payload)
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func period(t time.Time) string {
	return t.UTC().Format("2006-01")
}

func loadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func localHour(at time.Time, tz string) int {
	return at.In(loadLocation(tz)).Hour()
}

func daysLeft(validUntil pgtype.Date, at time.Time) int64 {
	if !validUntil.Valid {
		return 0
	}
	day := time.Date(at.UTC().Year(), at.UTC().Month(), at.UTC().Day(), 0, 0, 0, 0, time.UTC)
	until := time.Date(validUntil.Time.Year(), validUntil.Time.Month(), validUntil.Time.Day(), 0, 0, 0, 0, time.UTC)
	return int64(until.Sub(day).Hours() / 24)
}

func positive(n pgtype.Numeric) bool {
	v, ok := numericFloat(n)
	return ok && v > 0
}

func numericFloat(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return 0, false
	}
	return v.Float64, true
}

func floatNumeric(v float64) pgtype.Numeric {
	if math.Abs(v) < 0.0000001 {
		v = 0
	}
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.4f", v))
	return n
}

func intNumeric(v int64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%d", v))
	return n
}
