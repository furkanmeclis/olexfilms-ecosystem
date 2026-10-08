package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ResourceSummaryExport = "efficiency.summary"
	ResourceRollsExport   = "efficiency.rolls"
	ResourceExpectations  = "platform.part_consumption_expectations"

	DefaultNetworkWindowDays = 180
	DefaultNetworkMinSamples = 20
	// DefaultWarningWasteRatio mirrors the sysconfig default of
	// efficiency.warning_waste_ratio.
	DefaultWarningWasteRatio = "0.15"
)

var (
	ErrForbidden = errors.New("efficiency: forbidden")
	ErrNotFound  = errors.New("efficiency: not found")
)

var SummarySort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"waste_ratio": "waste_ratio", "meters": "meters", "services": "services"},
	Default: apiquery.SortField{Field: "waste_ratio", Desc: true},
}

var RollSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"waste_ratio": "waste_ratio", "consumed_meters": "consumed_meters",
		"last_used_at": "last_used_at", "remaining_meters": "remaining_meters",
	},
	Default: apiquery.SortField{Field: "waste_ratio", Desc: true},
}

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

type Service struct {
	q *db.Queries
}

func New(q *db.Queries) *Service { return &Service{q: q} }

type AnalyticsFilter struct {
	From   time.Time
	To     time.Time
	Limit  int32
	Offset int32
	Sort   []apiquery.SortField
	// WasteRatioMin/Max filter summary rows on their average waste ratio
	// (TEC-489).
	WasteRatioMin *float64
	WasteRatioMax *float64
}

type RollFilter struct {
	Limit         int32
	Offset        int32
	Q             string
	ProductIDs    []int64
	WasteRatioMin *float64
	WasteRatioMax *float64
	LastUsedFrom  *time.Time
	LastUsedTo    *time.Time
	Sort          []apiquery.SortField
}

type SummaryRow struct {
	DimensionKey   string  `json:"dimension_key"`
	DimensionLabel string  `json:"dimension_label"`
	Services       int64   `json:"services"`
	Meters         string  `json:"meters"`
	ExpectedMeters *string `json:"expected_meters"`
	WasteRatio     *string `json:"waste_ratio"`
}

type TrendRow struct {
	Month          string  `json:"month"`
	Services       int64   `json:"services"`
	Meters         string  `json:"meters"`
	ExpectedMeters *string `json:"expected_meters"`
	WasteRatio     *string `json:"waste_ratio"`
}

type CompareRow struct {
	Bucket         string  `json:"bucket"`
	Services       int64   `json:"services"`
	Meters         string  `json:"meters"`
	ExpectedMeters *string `json:"expected_meters"`
	WasteRatio     *string `json:"waste_ratio"`
}

// RollService is one service that consumed a roll (roll detail, TEC-489).
type RollService struct {
	UUID           uuid.UUID `json:"uuid"`
	ServiceNo      string    `json:"service_no"`
	ServiceDate    string    `json:"service_date"`
	DealerName     string    `json:"dealer_name"`
	ActualMeters   string    `json:"actual_meters"`
	ExpectedMeters *string   `json:"expected_meters"`
	WasteRatio     *string   `json:"waste_ratio"`
}

// RollDetail is a roll row plus the services that used it.
type RollDetail struct {
	RollRow
	Services []RollService `json:"services"`
}

type RollRow struct {
	UUID            uuid.UUID `json:"uuid"`
	UnitID          int64     `json:"-"`
	Barcode         string    `json:"barcode"`
	ProductName     string    `json:"product_name"`
	InitialMeters   string    `json:"initial_meters"`
	ConsumedMeters  string    `json:"consumed_meters"`
	ExpectedMeters  string    `json:"expected_meters"`
	WasteMeters     string    `json:"waste_meters"`
	RemainingMeters string    `json:"remaining_meters"`
	ServiceCount    int32     `json:"service_count"`
	LastUsedAt      *string   `json:"last_used_at"`
	WasteRatio      *string   `json:"waste_ratio"`
}

func (s *Service) Summary(ctx context.Context, c Caller, dimension string, f AnalyticsFilter) ([]SummaryRow, int64, error) {
	if !validDimension(dimension, c) {
		return nil, 0, invalid("dimension", "must be dealer, staff, product, body_type or part")
	}
	sort, err := apiquery.ResolveSort(f.Sort, SummarySort)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.EfficiencySummary(ctx, db.EfficiencySummaryParams{
		Dimension: dimension, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
		DateFrom: date(f.From), DateTo: date(f.To), SortKey: sort.Key, SortDesc: sort.Desc,
		WasteRatioMin: numPtr(f.WasteRatioMin), WasteRatioMax: numPtr(f.WasteRatioMax),
		RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("efficiency summary: %w", err)
	}
	out := make([]SummaryRow, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		out = append(out, SummaryRow{
			DimensionKey: fmt.Sprint(r.DimensionKey), DimensionLabel: fmt.Sprint(r.DimensionLabel),
			Services: r.ServiceCount, Meters: numeric(r.ActualMeters),
			ExpectedMeters: numericPtr(r.ExpectedMeters), WasteRatio: numericPtr(r.AvgWasteRatio),
		})
	}
	return out, total, nil
}

func (s *Service) Trend(ctx context.Context, c Caller, f AnalyticsFilter) ([]TrendRow, error) {
	rows, err := s.q.EfficiencyMonthlyTrend(ctx, db.EfficiencyMonthlyTrendParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), DateFrom: date(f.From), DateTo: date(f.To),
	})
	if err != nil {
		return nil, fmt.Errorf("efficiency trend: %w", err)
	}
	out := make([]TrendRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrendRow{Month: r.Month.Time.Format("2006-01"), Services: r.ServiceCount,
			Meters: numeric(r.ActualMeters), ExpectedMeters: numericPtr(r.ExpectedMeters), WasteRatio: numericPtr(r.AvgWasteRatio)})
	}
	return out, nil
}

func (s *Service) Compare(ctx context.Context, c Caller, f AnalyticsFilter) ([]CompareRow, error) {
	subtree, err := s.compareSubtree(ctx, c)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.EfficiencyComparison(ctx, db.EfficiencyComparisonParams{
		OrgID: c.Org.InternalID, SubtreeOrgIds: subtree, BrandID: c.Org.BrandID, DateFrom: date(f.From), DateTo: date(f.To),
	})
	if err != nil {
		return nil, fmt.Errorf("efficiency compare: %w", err)
	}
	out := make([]CompareRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompareRow{Bucket: r.Bucket, Services: r.ServiceCount, Meters: numeric(r.ActualMeters),
			ExpectedMeters: numericPtr(r.ExpectedMeters), WasteRatio: numericPtr(r.AvgWasteRatio)})
	}
	return out, nil
}

// compareSubtree is the middle comparison bucket: a dealer is compared with
// its supplier's network (the distributor and its dealers, TEC-489); other
// organizations with their own scope.
func (s *Service) compareSubtree(ctx context.Context, c Caller) ([]int64, error) {
	root := c.Org.InternalID
	if c.Org.OrgType == rbac.OrgTypeDealer {
		parent, err := s.q.SupplierOf(ctx, c.Org.InternalID)
		switch {
		case err == nil:
			root = parent.ID
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("efficiency compare supplier: %w", err)
		}
	} else if ids := c.Filter.OrgIDsArg(); ids != nil {
		return ids, nil
	}
	orgs, err := s.q.Descendants(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("efficiency compare descendants: %w", err)
	}
	subtree := []int64{root}
	for _, o := range orgs {
		subtree = append(subtree, o.ID)
	}
	return subtree, nil
}

// Roll is one roll with the services that consumed it.
func (s *Service) Roll(ctx context.Context, c Caller, unit uuid.UUID) (RollDetail, error) {
	rows, _, err := s.Rolls(ctx, c, RollFilter{Limit: 1}, &unit)
	if err != nil {
		return RollDetail{}, err
	}
	services, err := s.q.ListRollEfficiencyServices(ctx, db.ListRollEfficiencyServicesParams{
		UnitID: rows[0].UnitID, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
	})
	if err != nil {
		return RollDetail{}, fmt.Errorf("efficiency roll services: %w", err)
	}
	out := RollDetail{RollRow: rows[0], Services: make([]RollService, 0, len(services))}
	for _, r := range services {
		out.Services = append(out.Services, RollService{
			UUID: r.Uuid, ServiceNo: r.ServiceNo, ServiceDate: r.ServiceDate.Time.Format(time.DateOnly),
			DealerName: r.DealerName, ActualMeters: numeric(r.ActualMeters),
			ExpectedMeters: numericPtr(r.ExpectedMeters), WasteRatio: numericPtr(r.AvgWasteRatio),
		})
	}
	return out, nil
}

func (s *Service) Rolls(ctx context.Context, c Caller, f RollFilter, unit *uuid.UUID) ([]RollRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, RollSort)
	if err != nil {
		return nil, 0, err
	}
	arg := db.ListRollEfficiencyParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Q: text(f.Q), ProductIds: f.ProductIDs,
		WasteRatioMin: numPtr(f.WasteRatioMin), WasteRatioMax: numPtr(f.WasteRatioMax),
		LastUsedFrom: tsPtr(f.LastUsedFrom), LastUsedTo: tsPtr(f.LastUsedTo),
		SortKey: sort.Key, SortDesc: sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if unit != nil {
		arg.UnitUuid = pgtype.UUID{Bytes: *unit, Valid: true}
		arg.RowLimit = 1
		arg.RowOffset = 0
	}
	rows, err := s.q.ListRollEfficiency(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("efficiency rolls: %w", err)
	}
	if unit != nil && len(rows) == 0 {
		return nil, 0, ErrNotFound
	}
	out := make([]RollRow, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		out = append(out, rollView(r))
	}
	return out, total, nil
}

func (s *Service) RefreshService(ctx context.Context, serviceID int64) error {
	rows, err := s.q.ListServiceItems(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("efficiency service items: %w", err)
	}
	seen := map[int64]struct{}{}
	for _, item := range rows {
		if item.Kind != "partial" {
			continue
		}
		if _, err := s.q.RefreshEfficiencyFactsForServiceItem(ctx, item.ID); err != nil {
			return fmt.Errorf("efficiency refresh item %d: %w", item.ID, err)
		}
		seen[item.UnitID] = struct{}{}
	}
	for unitID := range seen {
		id := unitID
		if _, err := s.q.RebuildRollEfficiency(ctx, pgtype.Int8{Int64: id, Valid: true}); err != nil {
			return fmt.Errorf("efficiency rebuild roll %d: %w", id, err)
		}
	}
	return nil
}

func (s *Service) RefreshItem(ctx context.Context, itemID int64) error {
	if _, err := s.q.RefreshEfficiencyFactsForServiceItem(ctx, itemID); err != nil {
		return fmt.Errorf("efficiency refresh item %d: %w", itemID, err)
	}
	unitID, err := s.q.GetEfficiencyServiceItemUnit(ctx, itemID)
	if err != nil {
		return fmt.Errorf("efficiency item unit %d: %w", itemID, err)
	}
	_, err = s.q.RebuildRollEfficiency(ctx, pgtype.Int8{Int64: unitID, Valid: true})
	return err
}

func (s *Service) Backfill(ctx context.Context, limit int32) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	var after pgtype.Int8
	total := 0
	for {
		ids, err := s.q.ListEfficiencyServiceItems(ctx, db.ListEfficiencyServiceItemsParams{AfterID: after, RowLimit: limit})
		if err != nil {
			return total, fmt.Errorf("efficiency backfill list: %w", err)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if err := s.RefreshItem(ctx, id); err != nil {
				return total, err
			}
			after = pgtype.Int8{Int64: id, Valid: true}
			total++
		}
	}
	if _, err := s.q.RebuildRollEfficiency(ctx, pgtype.Int8{}); err != nil {
		return total, fmt.Errorf("efficiency rebuild rolls: %w", err)
	}
	return total, nil
}

type Settings interface {
	EfficiencyNetworkWindowDays(ctx context.Context) int
	EfficiencyNetworkMinSamples(ctx context.Context) int
}

func (s *Service) RefreshNetwork(ctx context.Context, brandID int64, at time.Time, settings Settings) (int64, error) {
	window := DefaultNetworkWindowDays
	minSamples := DefaultNetworkMinSamples
	if settings != nil {
		if v := settings.EfficiencyNetworkWindowDays(ctx); v > 0 {
			window = v
		}
		if v := settings.EfficiencyNetworkMinSamples(ctx); v > 0 {
			minSamples = v
		}
	}
	to := time.Date(at.UTC().Year(), at.UTC().Month(), at.UTC().Day()+1, 0, 0, 0, 0, time.UTC)
	n, err := s.q.RefreshNetworkPartExpectations(ctx, db.RefreshNetworkPartExpectationsParams{
		BrandID: brandID, FromDate: date(to.AddDate(0, 0, -window)), ToDate: date(to), MinSamples: int32(minSamples),
	})
	if err != nil {
		return 0, fmt.Errorf("efficiency network refresh: %w", err)
	}
	return n, nil
}

func validDimension(d string, c Caller) bool {
	switch d {
	case "dealer", "product", "body_type", "part":
		return true
	case "staff":
		return c.Filter.Scope == rbac.ScopeBrand || c.Filter.Scope == rbac.ScopeAll || c.Org.OrgType != rbac.OrgTypeDealer
	default:
		return false
	}
}

func invalid(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func date(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func text(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}

func numPtr(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.6f", *v))
	return n
}

func numeric(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp > 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)))
	} else if n.Exp < 0 {
		r.Quo(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)))
	}
	return r.FloatString(2)
}

func numericPtr(n pgtype.Numeric) *string {
	if !n.Valid || n.Int == nil {
		return nil
	}
	v := numeric(n)
	return &v
}

func timePtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC().Format(time.RFC3339)
	return &v
}

func rollView(r db.ListRollEfficiencyRow) RollRow {
	return RollRow{
		UUID: r.Uuid, UnitID: r.UnitID, Barcode: r.Barcode, ProductName: r.ProductName,
		InitialMeters: numeric(r.InitialMeters), ConsumedMeters: numeric(r.ConsumedMeters),
		ExpectedMeters: numeric(r.ExpectedMeters), WasteMeters: numeric(r.WasteMeters),
		RemainingMeters: numeric(r.RemainingMeters), ServiceCount: r.ServiceCount,
		LastUsedAt: timePtr(r.LastUsedAt), WasteRatio: numericPtr(r.WasteRatio),
	}
}
