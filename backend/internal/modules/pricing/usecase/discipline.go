package usecase

// TEC-506 (F5-09b): price discipline reads. The center reads the whole
// brand (pricing.discipline.read brand), a distributor its subtree; both
// read one snapshot day (default: the latest).

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DisciplineScope is the reach of a discipline read: the brand and, below
// brand scope, the organizations (subtree) it may see.
type DisciplineScope struct {
	BrandID int64
	OrgIDs  []int64 // nil: whole brand
}

// DisciplineSort is the sort contract of GET /v1/pricing/discipline.
var DisciplineSort = repository.DisciplineSort

// DisciplineFilter narrows the deviation list (docs/list-contract.md).
type DisciplineFilter struct {
	Date            *time.Time
	Countries       []string // ISO2
	ProductUUIDs    []uuid.UUID
	Currencies      []string
	DistributorUUID []uuid.UUID
	DeviationMin    *string
	DeviationMax    *string
	OverThreshold   *bool
	Q               string
	Sort            []apiquery.SortField
	Limit           int32
	Offset          int32
}

// DisciplineRow is one organization x product of a snapshot day.
type DisciplineRow struct {
	SnapshotDate     string    `json:"snapshot_date"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
	OrganizationType string    `json:"organization_type"`
	ProductUUID      uuid.UUID `json:"product_uuid"`
	ProductSKU       string    `json:"product_sku"`
	ProductName      string    `json:"product_name"`
	CountryISO2      string    `json:"country_iso2"`
	Currency         string    `json:"currency"`
	RecommendedPrice string    `json:"recommended_price"`
	ListPrice        string    `json:"list_price"`
	DeviationPct     *string   `json:"deviation_pct"`
	AvgSalePrice     *string   `json:"avg_sale_price"`
	SalesQuantity    string    `json:"sales_quantity"`
	OverThreshold    bool      `json:"over_threshold"`
}

// snapshotDay resolves the day to read: the requested one or the latest of
// the brand (nil when there is no snapshot yet).
func (s *Recommended) snapshotDay(ctx context.Context, brandID int64, day *time.Time) (*time.Time, error) {
	if day != nil {
		d := *day
		return &d, nil
	}
	latest, err := s.q.GetLatestPriceDisciplineSnapshotDate(ctx, brandID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !latest.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pricing: latest snapshot: %w", err)
	}
	d := latest.Time
	return &d, nil
}

func numericText2(raw *string, field string) (pgtype.Numeric, error) {
	if raw == nil {
		return pgtype.Numeric{}, nil
	}
	var n pgtype.Numeric
	if err := n.Scan(strings.TrimSpace(*raw)); err != nil {
		return pgtype.Numeric{}, invalid(field, "must be a number")
	}
	return n, nil
}

// ListDiscipline returns the deviation rows of a snapshot day in scope.
// The second value is the total, then the day ("" without snapshot) and
// the threshold in percent.
func (s *Recommended) ListDiscipline(ctx context.Context, scope DisciplineScope, f DisciplineFilter) ([]DisciplineRow, int64, string, int, error) {
	threshold := s.threshold(ctx)
	day, err := s.snapshotDay(ctx, scope.BrandID, f.Date)
	if err != nil || day == nil {
		return []DisciplineRow{}, 0, "", threshold, err
	}
	products, err := s.productIDs(ctx, scope.BrandID, f.ProductUUIDs)
	if err != nil {
		return nil, 0, "", 0, err
	}
	var countries []int64
	if len(f.Countries) > 0 {
		countries = []int64{-1}
		for _, iso := range f.Countries {
			c, err := s.q.GetCountryByISO2(ctx, strings.ToUpper(strings.TrimSpace(iso)))
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, 0, "", 0, fmt.Errorf("pricing: country: %w", err)
			}
			countries = append(countries, c.ID)
		}
	}
	var distributors []int64
	if len(f.DistributorUUID) > 0 {
		distributors = []int64{-1}
		for _, id := range f.DistributorUUID {
			o, err := s.q.GetOrganizationByUUID(ctx, id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, 0, "", 0, fmt.Errorf("pricing: distributor: %w", err)
			}
			if o.BrandID == scope.BrandID && o.Type == OrgDistributor {
				distributors = append(distributors, o.ID)
			}
		}
	}
	minDev, err := numericText2(f.DeviationMin, "deviation_pct_min")
	if err != nil {
		return nil, 0, "", 0, err
	}
	maxDev, err := numericText2(f.DeviationMax, "deviation_pct_max")
	if err != nil {
		return nil, 0, "", 0, err
	}
	arg := db.ListPriceDisciplineSnapshotsParams{
		BrandID: scope.BrandID, SnapshotDate: pgDate(*day), OrgIds: scope.OrgIDs,
		DistributorIds: distributors, CountryIds: countries, ProductIds: products, Currencies: f.Currencies,
		DeviationPctMin: minDev, DeviationPctMax: maxDev, ThresholdPct: numericInt(threshold),
		Q: pgtype.Text{String: f.Q, Valid: f.Q != ""}, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if f.OverThreshold != nil {
		arg.OverThreshold = pgtype.Bool{Bool: *f.OverThreshold, Valid: true}
	}
	rows, err := repository.FromQueries(s.q).ListDiscipline(ctx, arg, f.Sort)
	if err != nil {
		return nil, 0, "", 0, err
	}
	orgUUIDs := map[int64]uuid.UUID{}
	productUUIDs := map[int64]uuid.UUID{}
	out := make([]DisciplineRow, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		ou, ok := orgUUIDs[r.OrganizationID]
		if !ok {
			o, err := s.q.GetOrganizationByID(ctx, r.OrganizationID)
			if err != nil {
				return nil, 0, "", 0, fmt.Errorf("pricing: organization: %w", err)
			}
			ou = o.Uuid
			orgUUIDs[r.OrganizationID] = ou
		}
		pu, ok := productUUIDs[r.ProductID]
		if !ok {
			p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: r.ProductID, BrandID: scope.BrandID})
			if err != nil {
				return nil, 0, "", 0, fmt.Errorf("pricing: product: %w", err)
			}
			pu = p.Uuid
			productUUIDs[r.ProductID] = pu
		}
		dev := fixed2(r.DeviationPct)
		out = append(out, DisciplineRow{
			SnapshotDate: dateText(r.SnapshotDate), OrganizationUUID: ou, OrganizationName: r.OrgName,
			OrganizationType: r.OrgType, ProductUUID: pu, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			CountryISO2: r.CountryIso2, Currency: strings.TrimSpace(r.Currency),
			RecommendedPrice: money(r.RecommendedPrice), ListPrice: money(r.ListPrice), DeviationPct: dev,
			AvgSalePrice: fixed2(r.AvgSalePrice), SalesQuantity: money(r.SalesQuantity),
			OverThreshold: overThreshold(dev, threshold),
		})
	}
	return out, total, day.Format(time.DateOnly), threshold, nil
}

// overThreshold reports |dev| >= threshold (the list filter's rule).
func overThreshold(dev *string, threshold int) bool {
	if dev == nil {
		return false
	}
	d, ok := new(big.Rat).SetString(strings.TrimSpace(*dev))
	if !ok {
		return false
	}
	return d.Abs(d).Cmp(big.NewRat(int64(threshold), 1)) >= 0
}

// DisciplineCountry is the summary of one country x currency.
type DisciplineCountry struct {
	CountryISO2           string  `json:"country_iso2"`
	CountryNameEN         string  `json:"country_name_en"`
	CountryNameTR         string  `json:"country_name_tr"`
	Currency              string  `json:"currency"`
	OrgCount              int64   `json:"org_count"`
	RowCount              int64   `json:"row_count"`
	AvgDeviationPct       *string `json:"avg_deviation_pct"`
	MedianDeviationPct    *string `json:"median_deviation_pct"`
	OverThresholdOrgCount int64   `json:"over_threshold_org_count"`
}

// DisciplineProduct is the summary of one country x product.
type DisciplineProduct struct {
	CountryISO2           string    `json:"country_iso2"`
	Currency              string    `json:"currency"`
	ProductUUID           uuid.UUID `json:"product_uuid"`
	ProductSKU            string    `json:"product_sku"`
	ProductName           string    `json:"product_name"`
	RecommendedPrice      string    `json:"recommended_price"`
	OrgCount              int64     `json:"org_count"`
	AvgDeviationPct       *string   `json:"avg_deviation_pct"`
	MedianDeviationPct    *string   `json:"median_deviation_pct"`
	OverThresholdOrgCount int64     `json:"over_threshold_org_count"`
}

// DisciplineSummary is the country level summary of a snapshot day.
type DisciplineSummary struct {
	SnapshotDate *string             `json:"snapshot_date"`
	ThresholdPct int                 `json:"threshold_pct"`
	Countries    []DisciplineCountry `json:"countries"`
	Products     []DisciplineProduct `json:"products"`
}

// Summary returns the country and country x product summaries.
func (s *Recommended) Summary(ctx context.Context, scope DisciplineScope, date *time.Time) (DisciplineSummary, error) {
	out := DisciplineSummary{ThresholdPct: s.threshold(ctx), Countries: []DisciplineCountry{}, Products: []DisciplineProduct{}}
	day, err := s.snapshotDay(ctx, scope.BrandID, date)
	if err != nil || day == nil {
		return out, err
	}
	d := day.Format(time.DateOnly)
	out.SnapshotDate = &d
	th := numericInt(out.ThresholdPct)
	countries, err := s.q.PriceDisciplineSummaryByCountry(ctx, db.PriceDisciplineSummaryByCountryParams{
		ThresholdPct: th, BrandID: scope.BrandID, SnapshotDate: pgDate(*day), OrgIds: scope.OrgIDs,
	})
	if err != nil {
		return out, fmt.Errorf("pricing: summary: %w", err)
	}
	for _, c := range countries {
		out.Countries = append(out.Countries, DisciplineCountry{
			CountryISO2: c.CountryIso2, CountryNameEN: c.CountryNameEn, CountryNameTR: c.CountryNameTr,
			Currency: strings.TrimSpace(c.Currency), OrgCount: c.OrgCount, RowCount: c.RowCount,
			AvgDeviationPct: fixed2(c.AvgDeviationPct), MedianDeviationPct: fixed2(c.MedianDeviationPct),
			OverThresholdOrgCount: c.OverThresholdOrgCount,
		})
	}
	products, err := s.q.PriceDisciplineSummaryByProduct(ctx, db.PriceDisciplineSummaryByProductParams{
		ThresholdPct: th, BrandID: scope.BrandID, SnapshotDate: pgDate(*day), OrgIds: scope.OrgIDs,
	})
	if err != nil {
		return out, fmt.Errorf("pricing: summary by product: %w", err)
	}
	for _, p := range products {
		out.Products = append(out.Products, DisciplineProduct{
			CountryISO2: p.CountryIso2, Currency: strings.TrimSpace(p.Currency), ProductUUID: p.ProductUuid,
			ProductSKU: p.ProductSku, ProductName: p.ProductName, RecommendedPrice: money(p.RecommendedPrice),
			OrgCount: p.OrgCount, AvgDeviationPct: fixed2(p.AvgDeviationPct),
			MedianDeviationPct: fixed2(p.MedianDeviationPct), OverThresholdOrgCount: p.OverThresholdOrgCount,
		})
	}
	return out, nil
}
