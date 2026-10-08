package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusInsufficientData = "insufficient_data"
	StatusOK               = "ok"
	StatusWarning          = "warning"
	StatusCritical         = "critical"
	StatusNoConsumption    = "no_consumption"

	EventStockForecastComputed = events.StockForecastComputed

	DefaultBatchLimit = 500
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

type Settings interface {
	ForecastMinDays(ctx context.Context) int
	ForecastDefaultWarningDays(ctx context.Context) int
	ForecastCriticalDays(ctx context.Context) int
	ForecastDefaultCoverDays(ctx context.Context) int
}

type Service struct {
	pool     TxBeginner
	q        *db.Queries
	out      outbox.Enqueuer
	features FeatureChecker
	settings Settings
	log      *slog.Logger
	now      func() time.Time
}

type RunResult struct {
	Organizations int
	Snapshots     int
	Notifications int
	NetworkRows   int
}

type ProductResult struct {
	Snapshot    db.StockForecast
	Notified    bool
	Computed    bool
	Previous    string
	Consumption float64
}

func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, featureChecker FeatureChecker, settings Settings, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, out: out, features: featureChecker, settings: settings, log: log, now: time.Now}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) DailyTask(ctx context.Context, organizationID int64, computedOn time.Time) error {
	if organizationID == 0 && computedOn.IsZero() {
		return s.HourlyTask(ctx)
	}
	if computedOn.IsZero() {
		computedOn = s.now()
	}
	_, err := s.Run(ctx, organizationID, computedOn)
	return err
}

func (s *Service) HourlyTask(ctx context.Context) error {
	now := s.now()
	orgs, err := s.q.ListStockForecastOrganizations(ctx, pgtype.Int8{})
	if err != nil {
		return fmt.Errorf("stockforecast: list organizations: %w", err)
	}
	for _, org := range orgs {
		loc := loadLocation(org.Timezone)
		if now.In(loc).Hour() != 3 {
			continue
		}
		if _, err := s.Run(ctx, org.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context, organizationID int64, at time.Time) (RunResult, error) {
	var orgArg pgtype.Int8
	if organizationID > 0 {
		orgArg = pgtype.Int8{Int64: organizationID, Valid: true}
	}
	orgs, err := s.q.ListStockForecastOrganizations(ctx, orgArg)
	if err != nil {
		return RunResult{}, fmt.Errorf("stockforecast: list organizations: %w", err)
	}
	var res RunResult
	for _, org := range orgs {
		on, err := s.featureOn(ctx, org.ID)
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
		res.Snapshots += one.Snapshots
		res.Notifications += one.Notifications
		res.NetworkRows += one.NetworkRows
	}
	return res, nil
}

func (s *Service) RunOrganization(ctx context.Context, org db.Organization, at time.Time) (RunResult, error) {
	day := localDay(at, org.Timezone)
	products, err := s.q.ListStockForecastProductsForOrg(ctx, db.ListStockForecastProductsForOrgParams{
		BrandID: org.BrandID, OrganizationID: org.ID,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("stockforecast: list products: %w", err)
	}
	var res RunResult
	for _, product := range products {
		got, err := s.ComputeProduct(ctx, org, product, day)
		if err != nil {
			return res, err
		}
		if got.Computed {
			res.Snapshots++
		}
		if got.Notified {
			res.Notifications++
		}
	}
	if org.Type == "center" {
		n, err := s.computeNetworkDemand(ctx, org, day)
		if err != nil {
			return res, err
		}
		res.NetworkRows = n
	}
	return res, nil
}

func (s *Service) ComputeProduct(ctx context.Context, org db.Organization, product db.Product, day time.Time) (ProductResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ProductResult{}, fmt.Errorf("stockforecast: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	input, err := s.inputs(ctx, q, org, product, day)
	if err != nil {
		return ProductResult{}, err
	}
	prevStatus := ""
	prev, err := q.GetLatestStockForecast(ctx, db.GetLatestStockForecastParams{OrganizationID: org.ID, ProductID: product.ID})
	if err == nil {
		prevStatus = prev.Status
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ProductResult{}, fmt.Errorf("stockforecast: latest: %w", err)
	}
	snap := calculate(input)
	row, err := q.UpsertStockForecastSnapshot(ctx, snap)
	if err != nil {
		return ProductResult{}, fmt.Errorf("stockforecast: upsert snapshot: %w", err)
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, computedEvent(org, product, row)); err != nil {
			return ProductResult{}, fmt.Errorf("stockforecast: computed event: %w", err)
		}
	}
	notified := false
	if shouldNotify(prevStatus, row.Status) {
		recipients, err := q.ListStockForecastNotifyUserIDs(ctx, org.ID)
		if err != nil {
			return ProductResult{}, fmt.Errorf("stockforecast: recipients: %w", err)
		}
		if len(recipients) > 0 && s.out != nil {
			if err := s.out.Enqueue(ctx, tx, lowStockEvent(org, product, row, recipients)); err != nil {
				return ProductResult{}, fmt.Errorf("stockforecast: low event: %w", err)
			}
			notified = true
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ProductResult{}, fmt.Errorf("stockforecast: commit: %w", err)
	}
	return ProductResult{Snapshot: row, Notified: notified, Computed: true, Previous: prevStatus, Consumption: input.Avg30 * 30}, nil
}

type calcInput struct {
	OrganizationID  int64
	BrandID         int64
	ProductID       int64
	ComputedOn      time.Time
	ProductName     string
	UnitType        string
	OnHandQty       int32
	OnHandMeters    float64
	OpenOrderQty    int32
	OpenOrderMeters float64
	Avg30           float64
	Avg90           float64
	Seasonality     float64
	DataDays        int
	MinDays         int
	WarningDays     int
	CriticalDays    int
	CoverDays       int
	PartialMeters90 float64
	Services90      int64
}

func (s *Service) inputs(ctx context.Context, q *db.Queries, org db.Organization, product db.Product, day time.Time) (calcInput, error) {
	to := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, time.UTC)
	from30 := to.AddDate(0, 0, -30)
	from90 := to.AddDate(0, 0, -90)
	includeOrderOut := org.Type == "center" || org.Type == "distributor"
	total30, err := q.GetStockForecastConsumptionTotals(ctx, db.GetStockForecastConsumptionTotalsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID,
		FromAt: ts(from30), ToAt: ts(to), IncludeOrderOut: includeOrderOut,
	})
	if err != nil {
		return calcInput{}, fmt.Errorf("stockforecast: 30d totals: %w", err)
	}
	total90, err := q.GetStockForecastConsumptionTotals(ctx, db.GetStockForecastConsumptionTotalsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID,
		FromAt: ts(from90), ToAt: ts(to), IncludeOrderOut: includeOrderOut,
	})
	if err != nil {
		return calcInput{}, fmt.Errorf("stockforecast: 90d totals: %w", err)
	}
	first, err := q.GetStockForecastFirstMovementDate(ctx, db.GetStockForecastFirstMovementDateParams{OrganizationID: org.ID, BrandID: org.BrandID})
	if err != nil {
		return calcInput{}, fmt.Errorf("stockforecast: first movement: %w", err)
	}
	dataDays := 0
	if first.Valid {
		dataDays = int(day.Sub(first.Time)/(24*time.Hour)) + 1
		if dataDays < 0 {
			dataDays = 0
		}
	}
	onHand, err := q.GetStockForecastOnHand(ctx, db.GetStockForecastOnHandParams{OrganizationID: org.ID, ProductID: product.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return calcInput{}, fmt.Errorf("stockforecast: on hand: %w", err)
	}
	open, err := q.GetStockForecastOpenIncomingOrders(ctx, db.GetStockForecastOpenIncomingOrdersParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID,
	})
	if err != nil {
		return calcInput{}, fmt.Errorf("stockforecast: open orders: %w", err)
	}
	partial, err := q.GetStockForecastPartialMetersAndServices(ctx, db.GetStockForecastPartialMetersAndServicesParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID, FromAt: ts(from90), ToAt: ts(to),
	})
	if err != nil {
		return calcInput{}, fmt.Errorf("stockforecast: partial meters: %w", err)
	}
	seasonality := 1.0
	if dataDays >= 365 {
		lastYearStart := time.Date(day.Year()-1, time.January, 1, 0, 0, 0, 0, time.UTC)
		lastYearEnd := lastYearStart.AddDate(1, 0, 0)
		sameMonth := time.Date(day.Year()-1, day.Month(), 1, 0, 0, 0, 0, time.UTC)
		season, err := q.GetStockForecastSeasonalityTotals(ctx, db.GetStockForecastSeasonalityTotalsParams{
			OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID,
			FromAt: ts(lastYearStart), ToAt: ts(lastYearEnd), SameMonth: date(sameMonth), IncludeOrderOut: includeOrderOut,
		})
		if err != nil {
			return calcInput{}, fmt.Errorf("stockforecast: seasonality: %w", err)
		}
		yearTotal := numeric(season.YearTotal)
		if yearTotal > 0 {
			seasonality = clamp(numeric(season.SameMonth)/(yearTotal/12), 0.5, 2.0)
		}
	}
	return calcInput{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: product.ID,
		ComputedOn: day, ProductName: product.Name, UnitType: product.UnitType,
		OnHandQty: onHand.Quantity, OnHandMeters: numeric(onHand.Meters),
		OpenOrderQty: open.Qty, OpenOrderMeters: numeric(open.Meters),
		Avg30:       math.Max(numeric(total30.Qty)+numeric(total30.Meters), 0) / 30,
		Avg90:       math.Max(numeric(total90.Qty)+numeric(total90.Meters), 0) / 90,
		Seasonality: seasonality, DataDays: dataDays,
		MinDays:         setting(s.settings, 90, func(st Settings, ctx context.Context) int { return st.ForecastMinDays(ctx) }, ctx),
		WarningDays:     setting(s.settings, 14, func(st Settings, ctx context.Context) int { return st.ForecastDefaultWarningDays(ctx) }, ctx),
		CriticalDays:    setting(s.settings, 7, func(st Settings, ctx context.Context) int { return st.ForecastCriticalDays(ctx) }, ctx),
		CoverDays:       setting(s.settings, 30, func(st Settings, ctx context.Context) int { return st.ForecastDefaultCoverDays(ctx) }, ctx),
		PartialMeters90: numeric(partial.Meters), Services90: partial.Services,
	}, nil
}

func calculate(in calcInput) db.UpsertStockForecastSnapshotParams {
	speed := (0.6*in.Avg30 + 0.4*in.Avg90) * in.Seasonality
	arg := db.UpsertStockForecastSnapshotParams{
		OrganizationID: in.OrganizationID, BrandID: in.BrandID, ProductID: in.ProductID,
		ComputedOn: date(in.ComputedOn), IsLatest: true,
		OnHandQty: in.OnHandQty, OnHandMeters: num(in.OnHandMeters),
		AvgDaily30: num4(in.Avg30), AvgDaily90: num4(in.Avg90), SeasonalityFactor: num3(in.Seasonality),
		DataDays: int32(in.DataDays), Status: StatusOK,
	}
	if in.DataDays < in.MinDays {
		arg.Status = StatusInsufficientData
		return arg
	}
	onHand := float64(in.OnHandQty)
	openOrder := float64(in.OpenOrderQty)
	if in.UnitType == "roll_meter" {
		onHand = in.OnHandMeters
		openOrder = in.OpenOrderMeters
		if in.Services90 > 0 && in.PartialMeters90 > 0 {
			avgMeters := in.PartialMeters90 / float64(in.Services90)
			arg.AvgMetersPerVehicle = num4(avgMeters)
			if avgMeters > 0 {
				arg.VehiclesLeft = num2(in.OnHandMeters / avgMeters)
			}
		}
	}
	if speed <= 0 {
		arg.Status = StatusNoConsumption
		return arg
	}
	daysLeft := onHand / speed
	arg.DaysLeft = num2(daysLeft)
	arg.DepletionDate = date(in.ComputedOn.AddDate(0, 0, int(math.Ceil(daysLeft))))
	switch {
	case daysLeft <= float64(in.CriticalDays):
		arg.Status = StatusCritical
	case daysLeft <= float64(in.WarningDays):
		arg.Status = StatusWarning
	default:
		arg.Status = StatusOK
	}
	suggested := math.Max(0, speed*float64(in.CoverDays)-onHand-openOrder)
	if in.UnitType == "roll_meter" {
		arg.SuggestedMeters = num2(suggested)
	} else {
		arg.SuggestedQty = pgtype.Int4{Int32: int32(math.Ceil(suggested - 1e-9)), Valid: true}
	}
	return arg
}

func shouldNotify(prev, next string) bool {
	return (prev == StatusOK && next == StatusWarning) || (prev == StatusWarning && next == StatusCritical)
}

func computedEvent(org db.Organization, product db.Product, row db.StockForecast) events.Event {
	id, u := row.ID, row.Uuid
	return events.New(events.StockForecastComputed).
		WithTenant(org.ID).
		WithEntity("stock_forecast", &id, &u).
		WithPayload(map[string]any{
			"forecast_uuid": row.Uuid.String(), "organization_id": org.ID,
			"brand_id": org.BrandID, "product_id": product.ID, "product_uuid": product.Uuid.String(),
			"product_name": product.Name, "status": row.Status, "computed_on": row.ComputedOn.Time.Format("2006-01-02"),
		})
}

func lowStockEvent(org db.Organization, product db.Product, row db.StockForecast, recipients []int64) events.Event {
	id, u := row.ID, row.Uuid
	days := ""
	if row.DaysLeft.Valid {
		days = fmt.Sprintf("%.0f", math.Floor(numeric(row.DaysLeft)+0.5))
	}
	return events.New(events.StockForecastLow).
		WithTenant(org.ID).
		WithEntity("stock_forecast", &id, &u).
		WithPayload(map[string]any{
			"forecast_uuid": row.Uuid.String(), "organization_id": org.ID, "organization_name": org.Name,
			"brand_id": org.BrandID, "product_id": product.ID, "product_uuid": product.Uuid.String(),
			"product_name": product.Name, "status": row.Status, "days_left": days, "notify_user_ids": recipients,
		})
}

func (s *Service) computeNetworkDemand(ctx context.Context, org db.Organization, day time.Time) (int, error) {
	rows, err := s.q.ListStockForecastNetworkInputs(ctx, db.ListStockForecastNetworkInputsParams{
		ComputedOn:     date(day),
		BrandID:        org.BrandID,
		OrganizationID: org.ID,
	})
	if err != nil {
		return 0, fmt.Errorf("stockforecast: network inputs: %w", err)
	}
	for _, row := range rows {
		expectedQty := row.ExpectedQty
		expectedMeters := numeric(row.ExpectedMeters)
		onHandQty := row.NetworkOnHandQty
		onHandMeters := numeric(row.NetworkOnHandMeters)
		openQty := row.OpenOrderQty
		openMeters := numeric(row.OpenOrderMeters)
		suggestQty := int32(math.Max(0, float64(expectedQty-onHandQty-openQty)))
		suggestMeters := math.Max(0, expectedMeters-onHandMeters-openMeters)
		if _, err := s.q.UpsertNetworkDemandForecast(ctx, db.UpsertNetworkDemandForecastParams{
			OrganizationID:            org.ID,
			BrandID:                   org.BrandID,
			ProductID:                 row.ProductID,
			ForecastMonth:             row.ForecastMonth,
			ExpectedQty:               expectedQty,
			ExpectedMeters:            row.ExpectedMeters,
			NetworkOnHandQty:          onHandQty,
			NetworkOnHandMeters:       row.NetworkOnHandMeters,
			OpenOrderQty:              openQty,
			OpenOrderMeters:           row.OpenOrderMeters,
			SuggestedProductionQty:    suggestQty,
			SuggestedProductionMeters: num2(suggestMeters),
			ComputedAt:                ts(s.now()),
		}); err != nil {
			return 0, fmt.Errorf("stockforecast: upsert network demand: %w", err)
		}
	}
	return len(rows), nil
}

func (s *Service) featureOn(ctx context.Context, organizationID int64) (bool, error) {
	if s.features == nil {
		return true, nil
	}
	on, err := s.features.Enabled(ctx, organizationID, features.ModuleStockForecast)
	if err != nil {
		return false, fmt.Errorf("stockforecast: feature check: %w", err)
	}
	return on, nil
}

func setting(settings Settings, fallback int, read func(Settings, context.Context) int, ctx context.Context) int {
	if settings == nil {
		return fallback
	}
	if v := read(settings, ctx); v > 0 {
		return v
	}
	return fallback
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func date(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func localDay(t time.Time, zone string) time.Time {
	loc := loadLocation(zone)
	v := t.In(loc)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
}

func loadLocation(zone string) *time.Location {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		return time.UTC
	}
	return loc
}

func numeric(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return 0
	}
	return v.Float64
}

func num(v float64) pgtype.Numeric  { return scanNumeric(fmt.Sprintf("%.2f", math.Max(v, 0))) }
func num2(v float64) pgtype.Numeric { return scanNumeric(fmt.Sprintf("%.2f", math.Max(v, 0))) }
func num3(v float64) pgtype.Numeric { return scanNumeric(fmt.Sprintf("%.3f", math.Max(v, 0))) }
func num4(v float64) pgtype.Numeric { return scanNumeric(fmt.Sprintf("%.4f", math.Max(v, 0))) }

func scanNumeric(s string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
