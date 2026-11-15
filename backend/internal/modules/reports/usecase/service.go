// Package usecase is the /v1/reports contract (TEC-495, F5-05f): the
// redesign of the 22 legacy mobile report endpoints (olexfilms
// MobileReports) as one catalog of reports with a shared envelope, read by
// the mobile app and the panel widgets. Access is the legacy
// ReportAccessResolver rebuilt on RBAC + scope: every report needs the read
// permission of its domain (scope narrows the records: dealer = own
// organization, distributor = subtree, center = brand) and its module; a
// report whose module is off answers 403 FEATURE_DISABLED and is missing
// from the catalog.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	perfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrNotFound: unknown report key.
	ErrNotFound = errors.New("reports: report not found")
	// ErrForbidden: the caller lacks the report's permission or reach.
	ErrForbidden = errors.New("reports: forbidden")
	// ErrFeatureDisabled: the report's module is off for the organization.
	ErrFeatureDisabled = errors.New("reports: feature disabled")
)

// ValidationError is a 400 VALIDATION_ERROR of one field.
type ValidationError struct {
	Field   string
	Message string
	Code    string
}

func (e *ValidationError) Error() string { return "reports: invalid " + e.Field + ": " + e.Message }

func invalid(field, code, msg string) error {
	return &ValidationError{Field: field, Code: code, Message: msg}
}

// Report kinds: how a client draws the series / rows.
const (
	KindSummary      = "summary"
	KindTimeseries   = "timeseries"
	KindDistribution = "distribution"
	KindRanking      = "ranking"
	KindTable        = "table"
)

// Report keys.
const (
	ReportOverview                   = "overview"
	ReportServicesTrend              = "services.trend"
	ReportServicesStatusDistribution = "services.status_distribution"
	ReportServicesTopBrands          = "services.top_brands"
	ReportServicesTopModels          = "services.top_models"
	ReportServicesTopProducts        = "services.top_products"
	ReportOrdersTrend                = "orders.trend"
	ReportOrdersStatusDistribution   = "orders.status_distribution"
	// ReportOrdersPlotterCut: monthly plotter-cut shipments (TEC-1058).
	ReportOrdersPlotterCut     = "orders.plotter_cut"
	ReportCustomersTrend       = "customers.trend"
	ReportStockSummary         = "stock.summary"
	ReportWarrantiesSummary    = "warranties.summary"
	ReportMeasurementsSummary  = "measurements.summary"
	ReportDealersPerformance   = "dealers.performance"
	ReportDealersTopByWarranty = "dealers.top_by_warranty"
	ReportActivitiesRecent     = "activities.recent"
	// TEC-1090: network rankings by organization.
	ReportOrdersByBuyer       = "orders.by_buyer"
	ReportOrdersInactiveBuyer = "orders.inactive_buyers"
	ReportServicesByOrg       = "services.by_org"
	// TEC-1210: role reports of envelope v2.
	ReportOrdersFulfillment = "orders.fulfillment"
	ReportNetworkSummary    = "network.summary"
	ReportStockMovement     = "stock.movement"
)

// Definition is one catalog entry.
type Definition struct {
	Key  string
	Kind string
	// Module is the feature key that must be on ("" = gated per card,
	// overview only).
	Module string
	// Permission is the read permission whose scope narrows the records.
	Permission string
	// Periodic reports accept period / range_from / range_to.
	Periodic bool
	// Granular reports accept granularity (time series).
	Granular bool
	// Limited reports accept limit (rankings and tables).
	Limited bool
	// Network reports compare organizations: they need a reach beyond one
	// organization (subtree, brand, all) outside a dealer.
	Network bool
	// UserScoped reports support own / assigned grants (created_by).
	UserScoped bool
	// Seller reports look at the buyers below the active organization
	// (TEC-1090): closed to a dealer like Network ones, but a managed
	// reach (a distributor's own sales) is enough.
	Seller bool
	// AllTime reports accept all_time=true instead of a period.
	AllTime bool
}

// Definitions is the catalog in display order.
var Definitions = []Definition{
	{Key: ReportOverview, Kind: KindSummary, Periodic: true},
	{Key: ReportServicesTrend, Kind: KindTimeseries, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, Granular: true, UserScoped: true},
	{Key: ReportServicesStatusDistribution, Kind: KindDistribution, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, UserScoped: true},
	{Key: ReportServicesTopBrands, Kind: KindRanking, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, Limited: true, UserScoped: true},
	{Key: ReportServicesTopModels, Kind: KindRanking, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, Limited: true, UserScoped: true},
	{Key: ReportServicesTopProducts, Kind: KindRanking, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, Limited: true, UserScoped: true},
	{Key: ReportOrdersTrend, Kind: KindTimeseries, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true, Granular: true, UserScoped: true},
	{Key: ReportOrdersStatusDistribution, Kind: KindDistribution, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true, UserScoped: true},
	{Key: ReportOrdersPlotterCut, Kind: KindTable, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true},
	{Key: ReportCustomersTrend, Kind: KindTimeseries, Module: features.ModuleCustomers, Permission: rbac.PermCustomersRead, Periodic: true, Granular: true},
	{Key: ReportStockSummary, Kind: KindSummary, Module: features.ModuleStock, Permission: rbac.PermStockRead},
	{Key: ReportWarrantiesSummary, Kind: KindSummary, Module: features.ModuleServices, Permission: rbac.PermWarrantiesRead, Periodic: true, Granular: true, UserScoped: true},
	{Key: ReportMeasurementsSummary, Kind: KindSummary, Module: features.ModuleMeasurements, Permission: rbac.PermMeasurementsRead, Periodic: true, Granular: true, UserScoped: true},
	{Key: ReportDealersPerformance, Kind: KindTable, Module: features.ModulePerformance, Permission: rbac.PermPerformanceRead, Periodic: true, Limited: true},
	{Key: ReportDealersTopByWarranty, Kind: KindRanking, Module: features.ModuleServices, Permission: rbac.PermWarrantiesRead, Periodic: true, Limited: true, Network: true},
	{Key: ReportActivitiesRecent, Kind: KindTable, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Limited: true, UserScoped: true},
	{Key: ReportOrdersByBuyer, Kind: KindRanking, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true, Limited: true, Seller: true},
	{Key: ReportOrdersInactiveBuyer, Kind: KindTable, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true, Limited: true, Seller: true, AllTime: true},
	{Key: ReportServicesByOrg, Kind: KindRanking, Module: features.ModuleServices, Permission: rbac.PermServicesRead, Periodic: true, Limited: true, Network: true},
	{Key: ReportOrdersFulfillment, Kind: KindSummary, Module: features.ModuleOrders, Permission: rbac.PermOrdersRead, Periodic: true, Granular: true},
	{Key: ReportNetworkSummary, Kind: KindSummary, Module: features.ModuleOrganizations, Permission: rbac.PermOrganizationsRead, Periodic: true, Granular: true, Limited: true, Network: true},
	{Key: ReportStockMovement, Kind: KindTimeseries, Module: features.ModuleStock, Permission: rbac.PermStockRead, Periodic: true, Granular: true, Limited: true},
}

// networkDef gates the overview's dealer count cards.
var networkDef = Definition{Key: "network", Module: features.ModuleOrganizations, Permission: rbac.PermOrganizationsRead, Network: true}

// DefinitionByKey looks a report up.
func DefinitionByKey(key string) (Definition, bool) {
	for _, d := range Definitions {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}

// FeatureChecker reports whether a module is on for an organization.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Caller is the authenticated member and the active organization.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
}

// Service builds reports and stores layouts.
type Service struct {
	q        *db.Queries
	features FeatureChecker
	perf     *perfuc.Service
	now      func() time.Time
}

// New wires the service; features nil means every module is on.
func New(q *db.Queries, checker FeatureChecker) *Service {
	return &Service{q: q, features: checker, perf: perfuc.New(nil, q, nil), now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
		s.perf.WithClock(now)
	}
	return s
}

// access resolves the caller's reach for one definition.
func (s *Service) access(ctx context.Context, c Caller, d Definition) (scopefilter.Filter, error) {
	switch c.Org.OrgType {
	case "center", "distributor", "dealer":
	default:
		return scopefilter.Filter{}, ErrForbidden
	}
	if d.Module != "" && s.features != nil {
		on, err := s.features.Enabled(ctx, c.Org.InternalID, d.Module)
		if err != nil {
			return scopefilter.Filter{}, fmt.Errorf("reports: feature check: %w", err)
		}
		if !on {
			return scopefilter.Filter{}, ErrFeatureDisabled
		}
	}
	org := c.Org
	f, err := scopefilter.Resolve(ctx, s.q, c.Principal, &org, d.Permission)
	if errors.Is(err, scopefilter.ErrForbidden) || errors.Is(err, scopefilter.ErrOrganizationRequired) {
		return scopefilter.Filter{}, ErrForbidden
	}
	if err != nil {
		return scopefilter.Filter{}, err
	}
	if f.Scope == rbac.ScopeCustomer || (f.UserOnly() && !d.UserScoped) {
		return scopefilter.Filter{}, ErrForbidden
	}
	if d.Seller && c.Org.OrgType == "dealer" {
		return scopefilter.Filter{}, ErrForbidden
	}
	if d.Network {
		if c.Org.OrgType == "dealer" {
			return scopefilter.Filter{}, ErrForbidden
		}
		switch f.Scope {
		case rbac.ScopeSubtree, rbac.ScopeBrand, rbac.ScopeAll:
		default:
			return scopefilter.Filter{}, ErrForbidden
		}
	}
	return f, nil
}

// accessible reports whether a definition is open to the caller (overview:
// at least one of its card domains).
func (s *Service) accessible(ctx context.Context, c Caller, d Definition) (bool, error) {
	if d.Key == ReportOverview {
		cards, err := s.overviewDomains(ctx, c)
		return len(cards) > 0, err
	}
	_, err := s.access(ctx, c, d)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrFeatureDisabled):
		return false, nil
	default:
		return false, err
	}
}

// CatalogItem is one report the caller may open.
type CatalogItem struct {
	Key           string   `json:"key"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Module        *string  `json:"module"`
	Periodic      bool     `json:"periodic"`
	Periods       []string `json:"periods"`
	DefaultPeriod *string  `json:"default_period"`
	Granularities []string `json:"granularities"`
	Limited       bool     `json:"limited"`
	// AllTime: the report accepts all_time=true (TEC-1090).
	AllTime bool `json:"all_time"`
	// Compares are the compare values the report takes (TEC-1210): none,
	// previous, year for a periodic report, only none otherwise.
	Compares []string `json:"compares"`
	// Exportable: GET /v1/reports/{report}/export answers (TEC-1210).
	Exportable bool `json:"exportable"`
}

// Catalog lists the reports open to the caller: a report whose module is
// off or whose permission / reach is missing is left out.
func (s *Service) Catalog(ctx context.Context, c Caller, locale i18n.Locale) ([]CatalogItem, error) {
	out := make([]CatalogItem, 0, len(Definitions))
	for _, d := range Definitions {
		ok, err := s.accessible(ctx, c, d)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		item := CatalogItem{
			Key: d.Key, Kind: d.Kind, Title: translate(locale, "title."+d.Key), Periodic: d.Periodic,
			Periods: []string{}, Granularities: []string{}, Limited: d.Limited, AllTime: d.AllTime,
			Compares: []string{CompareNone}, Exportable: true,
		}
		if d.Module != "" {
			m := d.Module
			item.Module = &m
		}
		if d.Periodic {
			item.Periods = append(item.Periods, Periods...)
			p := DefaultPeriod
			item.DefaultPeriod = &p
			item.Compares = append(item.Compares, ComparePrevious, CompareYear)
		}
		if d.Granular {
			item.Granularities = append(item.Granularities, Granularities...)
		}
		out = append(out, item)
	}
	return out, nil
}

// Envelope is the shared shape of every report response: the same keys in
// every report (null / empty when a report does not use them).
type Envelope struct {
	Report      string  `json:"report"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Locale      string  `json:"locale"`
	Timezone    string  `json:"timezone"`
	Scope       string  `json:"scope"`
	Period      *string `json:"period"`
	RangeFrom   *string `json:"range_from"`
	RangeTo     *string `json:"range_to"`
	Granularity *string `json:"granularity"`
	// AllTime: the report was asked with all_time=true (no period).
	AllTime     bool             `json:"all_time"`
	GeneratedAt time.Time        `json:"generated_at"`
	Series      []Series         `json:"series"`
	Columns     []Column         `json:"columns"`
	Rows        []map[string]any `json:"rows"`
	// PeriodInfo is the resolved period with the compared range (TEC-1210;
	// null for a report without a period or asked with all_time). The
	// contract names it "period"; that key is the legacy period string.
	PeriodInfo *PeriodInfo `json:"period_info"`
	// KPIs are the headline numbers of the report (TEC-1210): summary
	// reports and the overview put their cards here (the cards series stays
	// for older clients).
	KPIs []KPI `json:"kpis"`
}

// PeriodInfo is the period a report was built for: an inclusive local
// date range, the bucket size and the compared range (null with
// compare=none).
type PeriodInfo struct {
	From         string  `json:"from"`
	To           string  `json:"to"`
	Granularity  string  `json:"granularity"`
	Compare      string  `json:"compare"`
	PreviousFrom *string `json:"previous_from"`
	PreviousTo   *string `json:"previous_to"`
}

// Series is one labelled list of points (a line, the slices of a donut,
// the bars of a ranking or the cards of a summary).
type Series struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Unit   string  `json:"unit"`
	Points []Point `json:"points"`
	// Total is the sum of the points (null for the cards series, whose
	// points are unlike numbers); PreviousTotal the same of the compared
	// range (null with compare=none); DeltaPct the change in percent
	// (null when there is no previous total or it is 0).
	Total         *float64 `json:"total"`
	PreviousTotal *float64 `json:"previous_total"`
	DeltaPct      *float64 `json:"delta_pct"`
	// timeline series are compared bucket by bucket (same index), the
	// others point by point (same key).
	timeline bool
}

// Point is one value: a time bucket (key = bucket start date), a status, a
// ranked item (key = its uuid) or a card.
type Point struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	// Previous is the same bucket / key of the compared range (null with
	// compare=none or when it has none).
	Previous *float64 `json:"previous"`
}

// Column describes one key of the table rows.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Type: text | number | money | percent | date | datetime | duration |
	// status | org | link (TEC-1210).
	Type string `json:"type"`
	// Align is left or right (numbers, money, percent, duration).
	Align string `json:"align"`
	// Aggregate is the footer of the column: sum, avg or none.
	Aggregate string `json:"aggregate"`
	// Currency of a money column when every row shares it (null: the row's
	// own currency field).
	Currency *string `json:"currency"`
}

// Report builds one report (overview included).
func (s *Service) Report(ctx context.Context, c Caller, key string, in Input) (Envelope, error) {
	d, ok := DefinitionByKey(key)
	if !ok {
		return Envelope{}, ErrNotFound
	}
	q, err := s.resolveQuery(d, in)
	if err != nil {
		return Envelope{}, err
	}
	if d.Key == ReportOverview {
		env, err := s.overview(ctx, c, d, q)
		if err != nil {
			return Envelope{}, err
		}
		if q.compare != CompareNone {
			prev, err := s.overview(ctx, c, d, q.previousQuery(d))
			if err != nil {
				return Envelope{}, err
			}
			mergePrevious(&env, prev)
		}
		finalize(&env)
		return env, nil
	}
	f, err := s.access(ctx, c, d)
	if err != nil {
		return Envelope{}, err
	}
	env := s.envelope(d, q, f)
	a := args{brandID: c.Org.BrandID, orgIDs: f.OrgIDsArg()}
	if f.UserOnly() {
		a.createdBy = pgtype.Int8{Int64: f.UserID, Valid: true}
	}
	if err := s.build(ctx, c, f, d, q, a, &env); err != nil {
		return Envelope{}, err
	}
	if q.compare != CompareNone {
		pq := q.previousQuery(d)
		prev := s.envelope(d, pq, f)
		if err := s.build(ctx, c, f, d, pq, a, &prev); err != nil {
			return Envelope{}, err
		}
		mergePrevious(&env, prev)
	}
	finalize(&env)
	return env, nil
}

func (s *Service) envelope(d Definition, q query, f scopefilter.Filter) Envelope {
	env := Envelope{
		Report: d.Key, Kind: d.Kind, Title: translate(q.locale, "title."+d.Key),
		Locale: string(q.locale), Timezone: q.tz.String(), Scope: string(f.Scope),
		GeneratedAt: s.now().UTC(), Series: []Series{}, Columns: []Column{}, Rows: []map[string]any{},
		KPIs: []KPI{},
	}
	env.AllTime = q.allTime
	if d.Periodic && !q.allTime {
		period, from, to := q.period, q.fromDate.Format(dateLayout), q.toDate.Format(dateLayout)
		env.Period, env.RangeFrom, env.RangeTo = &period, &from, &to
		env.PeriodInfo = q.periodInfo()
	}
	if d.Granular {
		g := q.granularity
		env.Granularity = &g
	}
	return env
}

// args are the sqlc scope arguments.
type args struct {
	brandID   int64
	orgIDs    []int64
	createdBy pgtype.Int8
}
