// Package repository is the performance data adapter (TEC-490, F5-05a).
package repository

import (
	"context"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// RankingSort is the list-contract whitelist of the ranking list: every
// metric key and the organization name; default -services_count.
var RankingSort = func() apiquery.SortSpec {
	cols := apiquery.SortColumns{"name": "name"}
	for _, m := range model.Metrics {
		cols[m] = m
	}
	return apiquery.SortSpec{
		Columns: cols,
		Default: apiquery.SortField{Field: model.MetricServicesCount, Desc: true},
	}
}()

// TargetSort is the list-contract whitelist of performance targets.
var TargetSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"target_name":     "target_name",
		"metric":          "metric",
		"period_kind":     "period_kind",
		"period_start":    "period_start",
		"value":           "value",
		"achievement_pct": "achievement_pct",
		"created_at":      "created_at",
	},
	Default: apiquery.SortField{Field: "period_start", Desc: true},
}

// Store is the thin performance data adapter. Business authorization stays
// in the usecase/API layer; SQL enforces tenant consistency and list
// contracts.
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

// ListRanking resolves the sort against RankingSort and lists one month.
func (s *Store) ListRanking(ctx context.Context, arg db.ListPerformanceRankingParams, sort []apiquery.SortField) ([]db.ListPerformanceRankingRow, error) {
	resolved, err := apiquery.ResolveSort(sort, RankingSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey = resolved.Key
	arg.SortDesc = resolved.Desc
	if arg.Scope == "" {
		arg.Scope = "org"
	}
	if arg.RowLimit == 0 {
		arg.RowLimit = int32(apiquery.DefaultLimit)
	}
	if arg.Q.Valid {
		arg.Q.String = strings.TrimSpace(arg.Q.String)
		arg.Q.Valid = arg.Q.String != ""
	}
	return s.q.ListPerformanceRanking(ctx, arg)
}

// ListTargets resolves the sort against TargetSort and lists targets with
// their achievement.
func (s *Store) ListTargets(ctx context.Context, arg db.ListPerformanceTargetsParams, sort []apiquery.SortField) ([]db.ListPerformanceTargetsRow, error) {
	resolved, err := apiquery.ResolveSort(sort, TargetSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey = resolved.Key
	arg.SortDesc = resolved.Desc
	if arg.RowLimit == 0 {
		arg.RowLimit = int32(apiquery.DefaultLimit)
	}
	return s.q.ListPerformanceTargets(ctx, arg)
}
