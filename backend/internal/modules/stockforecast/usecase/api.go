package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound         = errors.New("stockforecast: not found")
	ErrForbidden        = errors.New("stockforecast: forbidden")
	ErrInsufficientData = errors.New("stockforecast: insufficient data")
)

const CodeInsufficientData = "STOCK_FORECAST_INSUFFICIENT_DATA"

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

func (c Caller) orderCaller() ordersuc.Caller {
	return ordersuc.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter}
}

type ProductRef struct {
	UUID         uuid.UUID `json:"uuid"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	UnitType     string    `json:"unit_type"`
	CategoryUUID uuid.UUID `json:"category_uuid,omitempty"`
	CategoryName string    `json:"category_name,omitempty"`
}

type ForecastView struct {
	UUID                uuid.UUID  `json:"uuid"`
	Product             ProductRef `json:"product"`
	ComputedOn          string     `json:"computed_on"`
	OnHandQty           int32      `json:"on_hand_qty"`
	OnHandMeters        string     `json:"on_hand_meters"`
	AvgDaily30          string     `json:"avg_daily_30"`
	AvgDaily90          string     `json:"avg_daily_90"`
	SeasonalityFactor   string     `json:"seasonality_factor"`
	AvgMetersPerVehicle *string    `json:"avg_meters_per_vehicle,omitempty"`
	VehiclesLeft        *string    `json:"vehicles_left,omitempty"`
	DaysLeft            *string    `json:"days_left,omitempty"`
	DepletionDate       *string    `json:"depletion_date,omitempty"`
	DataDays            int32      `json:"data_days"`
	Status              string     `json:"status"`
	SuggestedQty        *int32     `json:"suggested_qty,omitempty"`
	SuggestedMeters     *string    `json:"suggested_meters,omitempty"`
}

type ConsumptionPoint struct {
	Date   string `json:"date"`
	Qty    int32  `json:"qty"`
	Meters string `json:"meters"`
}

type ProjectionPoint struct {
	Date        string `json:"date"`
	StockQty    int32  `json:"stock_qty"`
	StockMeters string `json:"stock_meters"`
}

type ProductDetail struct {
	Forecast    ForecastView       `json:"forecast"`
	Consumption []ConsumptionPoint `json:"consumption"`
	Projection  []ProjectionPoint  `json:"projection"`
	Thresholds  ThresholdConfig    `json:"thresholds"`
	Parameters  ForecastParameters `json:"parameters"`
}

type ForecastParameters struct {
	MinDataDays  int `json:"min_data_days"`
	WarningDays  int `json:"warning_days"`
	CriticalDays int `json:"critical_days"`
	CoverDays    int `json:"cover_days"`
	HorizonDays  int `json:"horizon_days"`
}

type ThresholdConfig struct {
	WarningDays int32           `json:"warning_days"`
	CoverDays   int32           `json:"cover_days"`
	Overrides   []ThresholdView `json:"overrides,omitempty"`
}

type ThresholdView struct {
	UUID        uuid.UUID   `json:"uuid"`
	Product     *ProductRef `json:"product,omitempty"`
	WarningDays int32       `json:"warning_days"`
	CoverDays   int32       `json:"cover_days"`
}

type ListFilter struct {
	Q           string
	Statuses    []string
	DaysLeftMin *float64
	DaysLeftMax *float64
	Limit       int32
	Offset      int32
	SortKey     string
	SortDesc    bool
}

type NetworkFilter struct {
	Q      string
	From   time.Time
	Before time.Time
	Limit  int32
	Offset int32
}

type SubtreeFilter struct {
	Q      string
	Limit  int32
	Offset int32
}

type ThresholdInput struct {
	WarningDays int32
	CoverDays   int32
	Products    []ProductThresholdInput
}

type ProductThresholdInput struct {
	ProductUUID uuid.UUID
	WarningDays int32
	CoverDays   int32
}

type OrderDraftInput struct {
	Items []OrderDraftItem
	Note  *string
}

type OrderDraftItem struct {
	ProductUUID uuid.UUID
	Quantity    *int64
	Meters      *string
}

type NetworkDemandView struct {
	UUID                      uuid.UUID  `json:"uuid"`
	Product                   ProductRef `json:"product"`
	ForecastMonth             string     `json:"forecast_month"`
	ExpectedQty               int32      `json:"expected_qty"`
	ExpectedMeters            string     `json:"expected_meters"`
	NetworkOnHandQty          int32      `json:"network_on_hand_qty"`
	NetworkOnHandMeters       string     `json:"network_on_hand_meters"`
	OpenOrderQty              int32      `json:"open_order_qty"`
	OpenOrderMeters           string     `json:"open_order_meters"`
	SuggestedProductionQty    int32      `json:"suggested_production_qty"`
	SuggestedProductionMeters string     `json:"suggested_production_meters"`
	ComputedAt                *time.Time `json:"computed_at,omitempty"`
}

type SubtreeSummary struct {
	OrganizationUUID      uuid.UUID `json:"organization_uuid"`
	Slug                  string    `json:"slug"`
	Name                  string    `json:"name"`
	CriticalCount         int64     `json:"critical_count"`
	WarningCount          int64     `json:"warning_count"`
	InsufficientDataCount int64     `json:"insufficient_data_count"`
	TotalCount            int64     `json:"total_count"`
}

var ForecastSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"days_left": "days_left", "depletion_date": "depletion_date", "product_name": "product_name",
		"avg_daily_30": "avg_daily_30", "status": "status",
	},
	Default: apiquery.SortField{Field: "days_left"},
}

func ParseListFilter(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", StatusInsufficientData, StatusOK, StatusWarning, StatusCritical, StatusNoConsumption); err != nil {
		return f, err
	}
	r, err := apiquery.NumRange(values, "days_left")
	if err != nil {
		return f, err
	}
	f.DaysLeftMin, f.DaysLeftMax = r.Min, r.Max
	sort, err := apiquery.ResolveSort(q.Sort, ForecastSort)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	return f, nil
}

func ParseNetworkFilter(values url.Values, now time.Time) (NetworkFilter, error) {
	q := apiquery.Parse(values)
	f := NetworkFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	from := firstOfMonth(now.UTC())
	before := from.AddDate(0, 3, 0)
	if raw := strings.TrimSpace(values.Get("from_month")); raw != "" {
		t, err := time.Parse(time.DateOnly, raw+"-01")
		if len(raw) == len(time.DateOnly) {
			t, err = time.Parse(time.DateOnly, raw)
		}
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "from_month", Message: "must be YYYY-MM or YYYY-MM-DD", Code: "invalid"}}}
		}
		from = firstOfMonth(t)
		before = from.AddDate(0, 3, 0)
	}
	if raw := strings.TrimSpace(values.Get("months")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 12 {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "months", Message: "must be between 1 and 12", Code: "invalid"}}}
		}
		before = from.AddDate(0, n, 0)
	}
	f.From, f.Before = from, before
	return f, nil
}

func ParseSubtreeFilter(values url.Values) SubtreeFilter {
	q := apiquery.Parse(values)
	return SubtreeFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
}

func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]ForecastView, int64, error) {
	rows, err := s.q.ListStockForecasts(ctx, db.ListStockForecastsParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Statuses: f.Statuses,
		DaysLeftMin: numericArg(f.DaysLeftMin), DaysLeftMax: numericArg(f.DaysLeftMax), Q: textArg(f.Q),
		SortKey: f.SortKey, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: list: %w", err)
	}
	total, err := s.q.CountStockForecasts(ctx, db.CountStockForecastsParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Statuses: f.Statuses,
		DaysLeftMin: numericArg(f.DaysLeftMin), DaysLeftMax: numericArg(f.DaysLeftMax), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: count: %w", err)
	}
	out := make([]ForecastView, 0, len(rows))
	for _, r := range rows {
		out = append(out, forecastFromList(r))
	}
	return out, total, nil
}

func (s *Service) Detail(ctx context.Context, c Caller, productUUID uuid.UUID) (ProductDetail, error) {
	row, err := s.q.GetLatestStockForecastByProductUUID(ctx, db.GetLatestStockForecastByProductUUIDParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, ProductUuid: productUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductDetail{}, ErrNotFound
	}
	if err != nil {
		return ProductDetail{}, fmt.Errorf("stockforecast: detail: %w", err)
	}
	to := s.now()
	from := to.AddDate(0, 0, -89)
	series, err := s.q.ListProductConsumptionSeries(ctx, db.ListProductConsumptionSeriesParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, ProductID: row.ProductID,
		FromOn: date(from), ToOn: date(to),
	})
	if err != nil {
		return ProductDetail{}, fmt.Errorf("stockforecast: series: %w", err)
	}
	thresholds, err := s.thresholdConfig(ctx, c.Org, row.ProductID)
	if err != nil {
		return ProductDetail{}, err
	}
	view := forecastFromDetail(row)
	return ProductDetail{
		Forecast: view, Consumption: consumptionViews(series), Projection: projection(view, 90),
		Thresholds: thresholds,
		Parameters: ForecastParameters{
			MinDataDays: setting(s.settings, 90, Settings.ForecastMinDays, ctx),
			WarningDays: int(thresholds.WarningDays), CriticalDays: setting(s.settings, 7, Settings.ForecastCriticalDays, ctx),
			CoverDays: int(thresholds.CoverDays), HorizonDays: 90,
		},
	}, nil
}

func (s *Service) Meta(ctx context.Context, c Caller) (map[string]any, error) {
	thresholds, err := s.thresholdConfig(ctx, c.Org, 0)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"default_sort":    ForecastSort.DefaultString(),
		"sortable_fields": ForecastSort.Fields(),
		"statuses":        []string{StatusCritical, StatusWarning, StatusOK, StatusNoConsumption, StatusInsufficientData},
		"thresholds":      thresholds,
		"parameters": ForecastParameters{
			MinDataDays: setting(s.settings, 90, Settings.ForecastMinDays, ctx),
			WarningDays: int(thresholds.WarningDays), CriticalDays: setting(s.settings, 7, Settings.ForecastCriticalDays, ctx),
			CoverDays: int(thresholds.CoverDays), HorizonDays: 90,
		},
	}, nil
}

func (s *Service) UpsertThresholds(ctx context.Context, c Caller, in ThresholdInput) (ThresholdConfig, error) {
	if !c.Principal.Can(rbac.PermStockForecastManage, rbac.ScopeManaged) {
		return ThresholdConfig{}, ErrForbidden
	}
	if in.WarningDays <= 0 || in.CoverDays <= 0 {
		return ThresholdConfig{}, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "warning_days", Message: "warning_days and cover_days must be positive", Code: "invalid"}}}
	}
	if _, err := s.q.UpsertStockForecastThreshold(ctx, db.UpsertStockForecastThresholdParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, WarningDays: in.WarningDays, CoverDays: in.CoverDays,
	}); err != nil {
		return ThresholdConfig{}, fmt.Errorf("stockforecast: threshold: %w", err)
	}
	for i, p := range in.Products {
		if p.WarningDays <= 0 || p.CoverDays <= 0 {
			return ThresholdConfig{}, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: fmt.Sprintf("products[%d]", i), Message: "warning_days and cover_days must be positive", Code: "invalid"}}}
		}
		product, err := s.q.ResolveStockForecastThresholdProduct(ctx, db.ResolveStockForecastThresholdProductParams{
			ProductUuid: p.ProductUUID, BrandID: c.Org.BrandID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ThresholdConfig{}, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: fmt.Sprintf("products[%d].product_uuid", i), Message: "product not found", Code: "invalid"}}}
		}
		if err != nil {
			return ThresholdConfig{}, fmt.Errorf("stockforecast: threshold product: %w", err)
		}
		if _, err := s.q.UpsertStockForecastThreshold(ctx, db.UpsertStockForecastThresholdParams{
			OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, ProductID: pgtype.Int8{Int64: product.ID, Valid: true},
			WarningDays: p.WarningDays, CoverDays: p.CoverDays,
		}); err != nil {
			return ThresholdConfig{}, fmt.Errorf("stockforecast: threshold product upsert: %w", err)
		}
	}
	return s.thresholdConfig(ctx, c.Org, 0)
}

func (s *Service) CreateOrderDraft(ctx context.Context, c Caller, orders interface {
	CreateOrAppendDraft(context.Context, ordersuc.Caller, ordersuc.CreateInput) (ordersuc.DraftResult, error)
}, in OrderDraftInput) (ordersuc.DraftResult, error) {
	if orders == nil {
		return ordersuc.DraftResult{}, fmt.Errorf("stockforecast: orders dependency missing")
	}
	items := make([]ordersuc.ItemInput, 0, len(in.Items))
	for i, it := range in.Items {
		latest, err := s.q.GetLatestStockForecastByProductUUID(ctx, db.GetLatestStockForecastByProductUUIDParams{
			OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, ProductUuid: it.ProductUUID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ordersuc.DraftResult{}, ErrNotFound
		}
		if err != nil {
			return ordersuc.DraftResult{}, fmt.Errorf("stockforecast: order forecast: %w", err)
		}
		if latest.Status == StatusInsufficientData {
			return ordersuc.DraftResult{}, ErrInsufficientData
		}
		if it.Quantity == nil && it.Meters == nil {
			switch latest.UnitType {
			case "roll_meter":
				if !latest.SuggestedMeters.Valid {
					return ordersuc.DraftResult{}, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: fmt.Sprintf("items[%d].meters", i), Message: "meters is required", Code: "invalid"}}}
				}
				m := moneyText(latest.SuggestedMeters, 2)
				it.Meters = &m
			default:
				if !latest.SuggestedQty.Valid {
					return ordersuc.DraftResult{}, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: fmt.Sprintf("items[%d].quantity", i), Message: "quantity is required", Code: "invalid"}}}
				}
				q := int64(latest.SuggestedQty.Int32)
				it.Quantity = &q
			}
		}
		items = append(items, ordersuc.ItemInput{ProductUUID: it.ProductUUID.String(), Quantity: it.Quantity, Meters: it.Meters})
	}
	return orders.CreateOrAppendDraft(ctx, c.orderCaller(), ordersuc.CreateInput{Note: in.Note, Items: items})
}

func (s *Service) Network(ctx context.Context, c Caller, f NetworkFilter) ([]NetworkDemandView, int64, error) {
	if c.Org.OrgType != "center" {
		return nil, 0, ErrForbidden
	}
	arg := db.ListNetworkDemandForecastsParams{
		BrandID: c.Org.BrandID, FromMonth: date(f.From), BeforeMonth: date(f.Before),
		Q: textArg(f.Q), LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListNetworkDemandForecasts(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: network: %w", err)
	}
	total, err := s.q.CountNetworkDemandForecasts(ctx, db.CountNetworkDemandForecastsParams{
		BrandID: c.Org.BrandID, FromMonth: date(f.From), BeforeMonth: date(f.Before), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: network count: %w", err)
	}
	out := make([]NetworkDemandView, 0, len(rows))
	for _, r := range rows {
		out = append(out, networkView(r))
	}
	return out, total, nil
}

func (s *Service) Subtree(ctx context.Context, c Caller, f SubtreeFilter) ([]SubtreeSummary, int64, error) {
	if c.Org.OrgType != "distributor" {
		return nil, 0, ErrForbidden
	}
	arg := db.ListStockForecastSubtreeSummaryParams{
		BrandID: c.Org.BrandID, OrganizationIds: c.Filter.OrgIDsArg(),
		Q: textArg(f.Q), LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListStockForecastSubtreeSummary(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: subtree: %w", err)
	}
	total, err := s.q.CountStockForecastSubtreeSummary(ctx, db.CountStockForecastSubtreeSummaryParams{
		BrandID: c.Org.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stockforecast: subtree count: %w", err)
	}
	out := make([]SubtreeSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, SubtreeSummary{
			OrganizationUUID: r.OrganizationUuid, Slug: r.Slug, Name: r.Name,
			CriticalCount: r.CriticalCount, WarningCount: r.WarningCount,
			InsufficientDataCount: r.InsufficientDataCount, TotalCount: r.TotalCount,
		})
	}
	return out, total, nil
}

func (s *Service) thresholdConfig(ctx context.Context, org orgctx.Scope, productID int64) (ThresholdConfig, error) {
	warn := int32(setting(s.settings, 14, Settings.ForecastDefaultWarningDays, ctx))
	cover := int32(setting(s.settings, 30, Settings.ForecastDefaultCoverDays, ctx))
	if global, err := s.q.GetStockForecastThreshold(ctx, db.GetStockForecastThresholdParams{OrganizationID: org.InternalID}); err == nil {
		warn, cover = global.WarningDays, global.CoverDays
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ThresholdConfig{}, fmt.Errorf("stockforecast: threshold: %w", err)
	}
	if productID > 0 {
		row, err := s.q.GetStockForecastThreshold(ctx, db.GetStockForecastThresholdParams{
			OrganizationID: org.InternalID, ProductID: pgtype.Int8{Int64: productID, Valid: true},
		})
		if err == nil {
			warn, cover = row.WarningDays, row.CoverDays
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ThresholdConfig{}, fmt.Errorf("stockforecast: product threshold: %w", err)
		}
	}
	rows, err := s.q.ListStockForecastThresholds(ctx, org.InternalID)
	if err != nil {
		return ThresholdConfig{}, fmt.Errorf("stockforecast: thresholds: %w", err)
	}
	out := ThresholdConfig{WarningDays: warn, CoverDays: cover}
	for _, r := range rows {
		v := ThresholdView{UUID: r.Uuid, WarningDays: r.WarningDays, CoverDays: r.CoverDays}
		if r.ProductUuid.Valid {
			id, err := uuid.FromBytes(r.ProductUuid.Bytes[:])
			if err != nil {
				return ThresholdConfig{}, fmt.Errorf("stockforecast: threshold uuid: %w", err)
			}
			v.Product = &ProductRef{UUID: id, SKU: r.Sku.String, Name: r.ProductName.String}
		}
		out.Overrides = append(out.Overrides, v)
	}
	return out, nil
}

func forecastFromList(r db.ListStockForecastsRow) ForecastView {
	return ForecastView{
		UUID: r.Uuid, Product: ProductRef{UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, UnitType: r.UnitType, CategoryUUID: r.CategoryUuid, CategoryName: r.CategoryName},
		ComputedOn: dateText(r.ComputedOn), OnHandQty: r.OnHandQty, OnHandMeters: moneyText(r.OnHandMeters, 2),
		AvgDaily30: moneyText(r.AvgDaily30, 4), AvgDaily90: moneyText(r.AvgDaily90, 4),
		SeasonalityFactor: moneyText(r.SeasonalityFactor, 3), AvgMetersPerVehicle: numericPtr(r.AvgMetersPerVehicle, 2),
		VehiclesLeft: numericPtr(r.VehiclesLeft, 2), DaysLeft: numericPtr(r.DaysLeft, 2), DepletionDate: datePtr(r.DepletionDate),
		DataDays: r.DataDays, Status: r.Status, SuggestedQty: intPtr(r.SuggestedQty), SuggestedMeters: numericPtr(r.SuggestedMeters, 2),
	}
}

func forecastFromDetail(r db.GetLatestStockForecastByProductUUIDRow) ForecastView {
	return ForecastView{
		UUID: r.Uuid, Product: ProductRef{UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, UnitType: r.UnitType, CategoryUUID: r.CategoryUuid, CategoryName: r.CategoryName},
		ComputedOn: dateText(r.ComputedOn), OnHandQty: r.OnHandQty, OnHandMeters: moneyText(r.OnHandMeters, 2),
		AvgDaily30: moneyText(r.AvgDaily30, 4), AvgDaily90: moneyText(r.AvgDaily90, 4),
		SeasonalityFactor: moneyText(r.SeasonalityFactor, 3), AvgMetersPerVehicle: numericPtr(r.AvgMetersPerVehicle, 2),
		VehiclesLeft: numericPtr(r.VehiclesLeft, 2), DaysLeft: numericPtr(r.DaysLeft, 2), DepletionDate: datePtr(r.DepletionDate),
		DataDays: r.DataDays, Status: r.Status, SuggestedQty: intPtr(r.SuggestedQty), SuggestedMeters: numericPtr(r.SuggestedMeters, 2),
	}
}

func networkView(r db.ListNetworkDemandForecastsRow) NetworkDemandView {
	return NetworkDemandView{
		UUID: r.Uuid, Product: ProductRef{UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, CategoryUUID: r.CategoryUuid, CategoryName: r.CategoryName},
		ForecastMonth: dateText(r.ForecastMonth), ExpectedQty: r.ExpectedQty, ExpectedMeters: moneyText(r.ExpectedMeters, 2),
		NetworkOnHandQty: r.NetworkOnHandQty, NetworkOnHandMeters: moneyText(r.NetworkOnHandMeters, 2),
		OpenOrderQty: r.OpenOrderQty, OpenOrderMeters: moneyText(r.OpenOrderMeters, 2),
		SuggestedProductionQty: r.SuggestedProductionQty, SuggestedProductionMeters: moneyText(r.SuggestedProductionMeters, 2),
		ComputedAt: timePtr(r.ComputedAt),
	}
}

func consumptionViews(rows []db.ListProductConsumptionSeriesRow) []ConsumptionPoint {
	out := make([]ConsumptionPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, ConsumptionPoint{Date: dateText(r.ConsumedOn), Qty: r.ConsumedQty, Meters: moneyText(r.ConsumedMeters, 2)})
	}
	return out
}

func projection(f ForecastView, days int) []ProjectionPoint {
	out := make([]ProjectionPoint, 0, days)
	qty := float64(f.OnHandQty)
	meters := parseFloat(f.OnHandMeters)
	avgQty := parseFloat(f.AvgDaily30)
	avgMeters := 0.0
	if f.Product.UnitType == "roll_meter" {
		avgMeters, avgQty = avgQty, 0
	}
	start, _ := time.Parse(time.DateOnly, f.ComputedOn)
	for i := 1; i <= days; i++ {
		qty = math.Max(0, qty-avgQty)
		meters = math.Max(0, meters-avgMeters)
		out = append(out, ProjectionPoint{
			Date:     start.AddDate(0, 0, i).Format(time.DateOnly),
			StockQty: int32(math.Ceil(qty)), StockMeters: fmt.Sprintf("%.2f", meters),
		})
	}
	return out
}

func textArg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func numericArg(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	return scanNumeric(strconv.FormatFloat(*v, 'f', -1, 64))
}

func moneyText(n pgtype.Numeric, decimals int) string {
	if !n.Valid {
		return "0"
	}
	if r, ok := n.Float64Value(); ok == nil && r.Valid {
		return strconv.FormatFloat(r.Float64, 'f', decimals, 64)
	}
	return "0"
}

func numericPtr(n pgtype.Numeric, decimals int) *string {
	if !n.Valid {
		return nil
	}
	v := moneyText(n, decimals)
	return &v
}

func intPtr(n pgtype.Int4) *int32 {
	if !n.Valid {
		return nil
	}
	return &n.Int32
}

func dateText(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(time.DateOnly)
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format(time.DateOnly)
	return &v
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
