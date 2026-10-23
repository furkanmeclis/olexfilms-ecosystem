package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	vcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-151 (F1-09c): top-10 car brands / models by completed services for
// the center dashboard. Brand scoped (K20): a center sees its own domain
// brand, super_admin the brand of the organization it selected. Dealers and
// distributors (organization / subtree scope) get ErrForbidden.

// Statistic periods (query parameter period).
const (
	StatsPeriod30d = "30d"
	StatsPeriod90d = "90d"
	StatsPeriod12m = "12m"
	StatsPeriodAll = "all"
)

// Statistic groupings (query parameter group).
const (
	StatsGroupBrand = "brand"
	StatsGroupModel = "model"
)

// TopVehicleLimit is the number of rows of the top list.
const TopVehicleLimit = 10

// StatsCarBrand is the car brand of a top-list row; UUID is also the logo id
// (/brand-logos/{uuid}).
type StatsCarBrand struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	HasLogo bool      `json:"has_logo"`
	LogoURL string    `json:"logo_url"`
}

// StatsCarModel is the car model of a top-list row (group=model only).
type StatsCarModel struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// TopVehicle is one row of the top list.
type TopVehicle struct {
	ServiceCount int64          `json:"service_count"`
	CarBrand     StatsCarBrand  `json:"car_brand"`
	CarModel     *StatsCarModel `json:"car_model"`
}

// TopVehicles is the response of GET /v1/stats/top-vehicle-models.
type TopVehicles struct {
	Period string       `json:"period"`
	Group  string       `json:"group"`
	Since  *time.Time   `json:"since"`
	Items  []TopVehicle `json:"items"`
}

// StatsSince returns the lower completed_at bound of period at now (nil for
// all time). 12m is twelve calendar months.
func StatsSince(period string, now time.Time) (*time.Time, error) {
	var t time.Time
	switch period {
	case StatsPeriod30d:
		t = now.AddDate(0, 0, -30)
	case StatsPeriod90d:
		t = now.AddDate(0, 0, -90)
	case StatsPeriod12m:
		t = now.AddDate(0, -12, 0)
	case StatsPeriodAll:
		return nil, nil
	default:
		return nil, invalid("period", "must be one of 30d, 90d, 12m, all")
	}
	return &t, nil
}

// statsBrand returns the domain brand the caller may read statistics of:
// the center with a brand-wide services.read grant, or a platform operator
// (scope all) for the brand of the selected organization.
func statsBrand(c Caller) (int64, error) {
	scope, ok := c.Principal.ScopeFor(rbac.PermServicesRead)
	if !ok || c.Org.BrandID == 0 {
		return 0, ErrForbidden
	}
	switch scope {
	case rbac.ScopeAll:
		return c.Org.BrandID, nil
	case rbac.ScopeBrand:
		if c.isCenter() {
			return c.Org.BrandID, nil
		}
	}
	return 0, ErrForbidden
}

// TopVehicleModels lists the top car brands (group=brand) or models
// (group=model) by completed services of the caller's brand in period.
func (s *Service) TopVehicleModels(ctx context.Context, c Caller, period, group string, now time.Time) (TopVehicles, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = StatsPeriod30d
	}
	group = strings.TrimSpace(group)
	if group == "" {
		group = StatsGroupModel
	}
	if group != StatsGroupBrand && group != StatsGroupModel {
		return TopVehicles{}, invalid("group", "must be brand or model")
	}
	since, err := StatsSince(period, now)
	if err != nil {
		return TopVehicles{}, err
	}
	brandID, err := statsBrand(c)
	if err != nil {
		return TopVehicles{}, err
	}
	sinceArg := pgtype.Timestamptz{}
	if since != nil {
		sinceArg = pgtype.Timestamptz{Time: *since, Valid: true}
	}
	out := TopVehicles{Period: period, Group: group, Since: since, Items: []TopVehicle{}}
	brand := func(id uuid.UUID, name string, key pgtype.Text) StatsCarBrand {
		k := ""
		if key.Valid {
			k = key.String
		}
		return StatsCarBrand{UUID: id, Name: name, HasLogo: k != "", LogoURL: vcuc.BrandLogoURL(id, k)}
	}
	if group == StatsGroupBrand {
		rows, err := s.q.TopServicedCarBrands(ctx, db.TopServicedCarBrandsParams{BrandID: brandID, Since: sinceArg})
		if err != nil {
			return TopVehicles{}, fmt.Errorf("services: top car brands: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, TopVehicle{
				ServiceCount: r.ServiceCount,
				CarBrand:     brand(r.CarBrandUuid, r.CarBrandName, r.CarBrandLogoKey),
			})
		}
		return out, nil
	}
	rows, err := s.q.TopServicedCarModels(ctx, db.TopServicedCarModelsParams{BrandID: brandID, Since: sinceArg})
	if err != nil {
		return TopVehicles{}, fmt.Errorf("services: top car models: %w", err)
	}
	for _, r := range rows {
		out.Items = append(out.Items, TopVehicle{
			ServiceCount: r.ServiceCount,
			CarBrand:     brand(r.CarBrandUuid, r.CarBrandName, r.CarBrandLogoKey),
			CarModel:     &StatsCarModel{UUID: r.CarModelUuid, Name: r.CarModelName},
		})
	}
	return out, nil
}

// TEC-385 (F4-01c): period activity summary for the AI assistant, parity
// with the legacy chatbot dealer/services/count, brand-breakdown and
// products/top. Unlike TopVehicleModels it is open to every services.read
// grant: the caller's own scope (organization, subtree, brand, own)
// applies, always inside the domain brand (K20).

// ActivityLimit caps the brand and product breakdowns.
const ActivityLimit = 10

// ActivityCarBrand is one car brand row of the breakdown.
type ActivityCarBrand struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	ServiceCount int64     `json:"service_count"`
}

// ActivityProduct is one product row of the top list.
type ActivityProduct struct {
	UUID         uuid.UUID `json:"uuid"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	UnitType     string    `json:"unit_type"`
	ServiceCount int64     `json:"service_count"`
	Quantity     int64     `json:"quantity"`
	Meters       string    `json:"meters"`
}

// Activity is the summary of [From, To).
type Activity struct {
	From           time.Time          `json:"from"`
	To             time.Time          `json:"to"`
	CreatedCount   int64              `json:"created_count"`
	CompletedCount int64              `json:"completed_count"`
	CarBrands      []ActivityCarBrand `json:"car_brands"`
	TopProducts    []ActivityProduct  `json:"top_products"`
}

// ActivitySummary counts the services opened and completed in [from, to)
// within the caller's services.read scope, with the completed services'
// car brand breakdown and most used products.
func (s *Service) ActivitySummary(ctx context.Context, c Caller, from, to time.Time) (Activity, error) {
	if !to.After(from) {
		return Activity{}, invalid("to", "must be after from")
	}
	if c.Org.BrandID == 0 || !c.Principal.HasPermission(rbac.PermServicesRead) {
		return Activity{}, ErrForbidden
	}
	orgIDs := c.Filter.OrgIDsArg()
	var createdBy pgtype.Int8
	if c.Filter.UserOnly() {
		createdBy = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	fromArg := pgtype.Timestamptz{Time: from, Valid: true}
	toArg := pgtype.Timestamptz{Time: to, Valid: true}
	counts, err := s.q.ServiceActivityCounts(ctx, db.ServiceActivityCountsParams{
		PeriodFrom: fromArg, PeriodTo: toArg, BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
	})
	if err != nil {
		return Activity{}, fmt.Errorf("services: activity counts: %w", err)
	}
	brands, err := s.q.ServiceActivityCarBrands(ctx, db.ServiceActivityCarBrandsParams{
		BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
		PeriodFrom: fromArg, PeriodTo: toArg, RowLimit: ActivityLimit,
	})
	if err != nil {
		return Activity{}, fmt.Errorf("services: activity car brands: %w", err)
	}
	products, err := s.q.ServiceActivityTopProducts(ctx, db.ServiceActivityTopProductsParams{
		BrandID: c.Org.BrandID, OrgIds: orgIDs, CreatedByUserID: createdBy,
		PeriodFrom: fromArg, PeriodTo: toArg, RowLimit: ActivityLimit,
	})
	if err != nil {
		return Activity{}, fmt.Errorf("services: activity products: %w", err)
	}
	out := Activity{
		From: from, To: to, CreatedCount: counts.CreatedCount, CompletedCount: counts.CompletedCount,
		CarBrands: make([]ActivityCarBrand, 0, len(brands)), TopProducts: make([]ActivityProduct, 0, len(products)),
	}
	for _, b := range brands {
		out.CarBrands = append(out.CarBrands, ActivityCarBrand{UUID: b.CarBrandUuid, Name: b.CarBrandName, ServiceCount: b.ServiceCount})
	}
	for _, p := range products {
		out.TopProducts = append(out.TopProducts, ActivityProduct{
			UUID: p.ProductUuid, SKU: p.Sku, Name: p.ProductName, UnitType: p.UnitType,
			ServiceCount: p.ServiceCount, Quantity: p.Quantity, Meters: p.Meters,
		})
	}
	return out, nil
}

// TEC-868: home overview of the active organization (dealer home "Analiz"):
// most serviced car models, most used products and the latest completed
// services. Only the active organization's own services count, inside the
// domain brand (K20), whatever wider reach the grant has; own / assigned
// grants see the services they created.

// OverviewTopLimit caps the two ranked lists; OverviewRecentLimit the
// latest completed services.
const (
	OverviewTopLimit    = 5
	OverviewRecentLimit = 10
)

// OverviewVehicle is one row of the most serviced car models.
type OverviewVehicle struct {
	ServiceCount int64         `json:"service_count"`
	CarBrand     StatsCarBrand `json:"car_brand"`
	CarModel     StatsCarModel `json:"car_model"`
}

// OverviewRecentService is one latest completed service.
type OverviewRecentService struct {
	UUID         uuid.UUID     `json:"uuid"`
	ServiceNo    string        `json:"service_no"`
	Plate        *string       `json:"plate"`
	PlateCountry *string       `json:"plate_country"`
	ModelYear    *int16        `json:"model_year"`
	Package      *string       `json:"package"`
	Products     []string      `json:"products"`
	CompletedAt  time.Time     `json:"completed_at"`
	VehicleUUID  uuid.UUID     `json:"vehicle_uuid"`
	CarBrand     StatsCarBrand `json:"car_brand"`
	CarModel     StatsCarModel `json:"car_model"`
}

// Overview is the response of GET /v1/stats/overview.
type Overview struct {
	Period         string                  `json:"period"`
	Since          *time.Time              `json:"since"`
	TopVehicles    []OverviewVehicle       `json:"top_vehicles"`
	TopProducts    []ActivityProduct       `json:"top_products"`
	RecentServices []OverviewRecentService `json:"recent_services"`
}

// overviewOrg returns the organization whose services the overview reads:
// the active organization, when the services.read grant reaches it.
func overviewOrg(c Caller) (int64, error) {
	if c.Org.InternalID == 0 || c.Org.BrandID == 0 || !c.Principal.HasPermission(rbac.PermServicesRead) {
		return 0, ErrForbidden
	}
	if !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return 0, ErrForbidden
	}
	return c.Org.InternalID, nil
}

// HomeOverview builds the overview of period (default 90d) for the active
// organization of c.
func (s *Service) HomeOverview(ctx context.Context, c Caller, period string, now time.Time) (Overview, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		period = StatsPeriod90d
	}
	since, err := StatsSince(period, now)
	if err != nil {
		return Overview{}, err
	}
	orgID, err := overviewOrg(c)
	if err != nil {
		return Overview{}, err
	}
	var createdBy pgtype.Int8
	if c.Filter.UserOnly() {
		createdBy = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	sinceArg := pgtype.Timestamptz{}
	if since != nil {
		sinceArg = pgtype.Timestamptz{Time: *since, Valid: true}
	}
	brand := func(id uuid.UUID, name string, key pgtype.Text) StatsCarBrand {
		k := ""
		if key.Valid {
			k = key.String
		}
		return StatsCarBrand{UUID: id, Name: name, HasLogo: k != "", LogoURL: vcuc.BrandLogoURL(id, k)}
	}

	vehicles, err := s.q.OverviewTopCarModels(ctx, db.OverviewTopCarModelsParams{
		BrandID: c.Org.BrandID, OrgID: orgID, CreatedByUserID: createdBy, PeriodFrom: sinceArg, RowLimit: OverviewTopLimit,
	})
	if err != nil {
		return Overview{}, fmt.Errorf("services: overview car models: %w", err)
	}
	products, err := s.q.OverviewTopProducts(ctx, db.OverviewTopProductsParams{
		BrandID: c.Org.BrandID, OrgID: orgID, CreatedByUserID: createdBy, PeriodFrom: sinceArg, RowLimit: OverviewTopLimit,
	})
	if err != nil {
		return Overview{}, fmt.Errorf("services: overview products: %w", err)
	}
	recent, err := s.q.OverviewRecentCompletedServices(ctx, db.OverviewRecentCompletedServicesParams{
		BrandID: c.Org.BrandID, OrgID: orgID, CreatedByUserID: createdBy, RowLimit: OverviewRecentLimit,
	})
	if err != nil {
		return Overview{}, fmt.Errorf("services: overview recent: %w", err)
	}

	out := Overview{
		Period: period, Since: since,
		TopVehicles:    make([]OverviewVehicle, 0, len(vehicles)),
		TopProducts:    make([]ActivityProduct, 0, len(products)),
		RecentServices: make([]OverviewRecentService, 0, len(recent)),
	}
	for _, v := range vehicles {
		out.TopVehicles = append(out.TopVehicles, OverviewVehicle{
			ServiceCount: v.ServiceCount,
			CarBrand:     brand(v.CarBrandUuid, v.CarBrandName, v.CarBrandLogoKey),
			CarModel:     StatsCarModel{UUID: v.CarModelUuid, Name: v.CarModelName},
		})
	}
	for _, p := range products {
		out.TopProducts = append(out.TopProducts, ActivityProduct{
			UUID: p.ProductUuid, SKU: p.Sku, Name: p.ProductName, UnitType: p.UnitType,
			ServiceCount: p.ServiceCount, Quantity: p.Quantity, Meters: p.Meters,
		})
	}
	for _, r := range recent {
		row := OverviewRecentService{
			UUID: r.Uuid, ServiceNo: r.ServiceNo,
			Plate: textPtr(r.Plate), PlateCountry: textPtr(r.PlateCountry), Package: textPtr(r.Package),
			Products: r.ProductNames, CompletedAt: r.CompletedAt.Time, VehicleUUID: r.VehicleUuid,
			CarBrand: brand(r.CarBrandUuid, r.CarBrandName, r.CarBrandLogoKey),
			CarModel: StatsCarModel{UUID: r.CarModelUuid, Name: r.CarModelName},
		}
		if row.Products == nil {
			row.Products = []string{}
		}
		if r.ModelYear.Valid {
			y := r.ModelYear.Int16
			row.ModelYear = &y
		}
		out.RecentServices = append(out.RecentServices, row)
	}
	return out, nil
}
