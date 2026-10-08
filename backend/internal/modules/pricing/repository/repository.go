// Package repository is the recommended price data adapter (TEC-505,
// F5-09a): version history, the current projection and price discipline
// snapshots.
package repository

import (
	"context"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

// VersionSort is the list-contract whitelist of the version history.
var VersionSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"effective_from": "effective_from",
		"published_at":   "published_at",
		"price":          "price",
	},
	Default: apiquery.SortField{Field: "effective_from", Desc: true},
}

// CurrentSort is the list-contract whitelist of the current price list.
var CurrentSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"product_name":   "product_name",
		"price":          "price",
		"effective_from": "effective_from",
	},
	Default: apiquery.SortField{Field: "product_name"},
}

// DisciplineSort is the list-contract whitelist of the deviation list.
var DisciplineSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"deviation_pct": "deviation_pct",
		"org_name":      "org_name",
		"product_name":  "product_name",
	},
	Default: apiquery.SortField{Field: "deviation_pct", Desc: true},
}

// Store is the thin recommended price data adapter. Authorization and
// scoping stay in the usecase layer; the database keeps versions
// append-only and the projection consistent.
type Store struct {
	q *db.Queries
}

func New(conn db.DBTX) *Store {
	return &Store{q: db.New(conn)}
}

func FromQueries(q *db.Queries) *Store {
	return &Store{q: q}
}

func (s *Store) Queries() *db.Queries { return s.q }

// Publish appends a version: a live version of the same key and day is
// superseded first, and the new one becomes current when its day is not
// after asOf. Run it inside a transaction.
func (s *Store) Publish(ctx context.Context, arg db.InsertRecommendedPriceVersionParams, asOf pgtype.Date) (db.RecommendedPriceVersion, error) {
	if _, err := s.q.SupersedeLiveRecommendedPriceVersionsOn(ctx, db.SupersedeLiveRecommendedPriceVersionsOnParams{
		ProductID: arg.ProductID, CountryID: arg.CountryID, Currency: arg.Currency, EffectiveFrom: arg.EffectiveFrom,
	}); err != nil {
		return db.RecommendedPriceVersion{}, err
	}
	v, err := s.q.InsertRecommendedPriceVersion(ctx, arg)
	if err != nil {
		return db.RecommendedPriceVersion{}, err
	}
	if !v.EffectiveFrom.Time.After(asOf.Time) {
		if err := s.q.MakeRecommendedPriceCurrent(ctx, v.ID); err != nil {
			return db.RecommendedPriceVersion{}, err
		}
	}
	return v, nil
}

// ListVersions resolves the sort against VersionSort.
func (s *Store) ListVersions(ctx context.Context, arg db.ListRecommendedPriceVersionsParams, sort []apiquery.SortField) ([]db.ListRecommendedPriceVersionsRow, error) {
	resolved, err := apiquery.ResolveSort(sort, VersionSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey, arg.SortDesc = resolved.Key, resolved.Desc
	arg.RowLimit = defaultLimit(arg.RowLimit)
	arg.Q = trimQ(arg.Q)
	return s.q.ListRecommendedPriceVersions(ctx, arg)
}

// ListCurrent resolves the sort against CurrentSort.
func (s *Store) ListCurrent(ctx context.Context, arg db.ListRecommendedPricesCurrentParams, sort []apiquery.SortField) ([]db.ListRecommendedPricesCurrentRow, error) {
	resolved, err := apiquery.ResolveSort(sort, CurrentSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey, arg.SortDesc = resolved.Key, resolved.Desc
	arg.RowLimit = defaultLimit(arg.RowLimit)
	arg.Q = trimQ(arg.Q)
	return s.q.ListRecommendedPricesCurrent(ctx, arg)
}

// ListDiscipline resolves the sort against DisciplineSort.
func (s *Store) ListDiscipline(ctx context.Context, arg db.ListPriceDisciplineSnapshotsParams, sort []apiquery.SortField) ([]db.ListPriceDisciplineSnapshotsRow, error) {
	resolved, err := apiquery.ResolveSort(sort, DisciplineSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey, arg.SortDesc = resolved.Key, resolved.Desc
	arg.RowLimit = defaultLimit(arg.RowLimit)
	arg.Q = trimQ(arg.Q)
	return s.q.ListPriceDisciplineSnapshots(ctx, arg)
}

func defaultLimit(n int32) int32 {
	if n == 0 {
		return apiquery.DefaultLimit
	}
	return n
}

func trimQ(q pgtype.Text) pgtype.Text {
	if q.Valid {
		q.String = strings.TrimSpace(q.String)
		q.Valid = q.String != ""
	}
	return q
}
