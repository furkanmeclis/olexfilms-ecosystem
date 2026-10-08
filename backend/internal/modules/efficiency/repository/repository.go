package repository

import (
	"context"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExpectationSort is the list-contract whitelist for expected consumption
// definitions.
var ExpectationSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"part_key":        "part_key",
		"product":         "product",
		"category":        "category",
		"body_type":       "body_type",
		"expected_meters": "expected_meters",
		"source":          "source",
		"sample_size":     "sample_size",
		"updated_at":      "updated_at",
	},
	Default: apiquery.SortField{Field: "updated_at", Desc: true},
}

// RollSort is the list-contract whitelist for roll efficiency rows.
var RollSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"waste_ratio":      "waste_ratio",
		"consumed_meters":  "consumed_meters",
		"last_used_at":     "last_used_at",
		"remaining_meters": "remaining_meters",
	},
	Default: apiquery.SortField{Field: "waste_ratio", Desc: true},
}

// Store is the thin efficiency data adapter. Business authorization stays in
// the usecase/API layer; SQL enforces tenant consistency and list contracts.
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

func (s *Store) BestExpectation(ctx context.Context, arg db.GetBestPartExpectationParams) (db.PartConsumptionExpectation, error) {
	arg.PartKey = strings.TrimSpace(arg.PartKey)
	return s.q.GetBestPartExpectation(ctx, arg)
}

func (s *Store) ListExpectations(ctx context.Context, arg db.ListPartConsumptionExpectationsParams, sort []apiquery.SortField) ([]db.ListPartConsumptionExpectationsRow, error) {
	resolved, err := apiquery.ResolveSort(sort, ExpectationSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey = resolved.Key
	arg.SortDesc = resolved.Desc
	if arg.RowLimit == 0 {
		arg.RowLimit = int32(apiquery.DefaultLimit)
	}
	return s.q.ListPartConsumptionExpectations(ctx, arg)
}

func (s *Store) ListRolls(ctx context.Context, arg db.ListRollEfficiencyParams, sort []apiquery.SortField) ([]db.ListRollEfficiencyRow, error) {
	resolved, err := apiquery.ResolveSort(sort, RollSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey = resolved.Key
	arg.SortDesc = resolved.Desc
	if arg.RowLimit == 0 {
		arg.RowLimit = int32(apiquery.DefaultLimit)
	}
	return s.q.ListRollEfficiency(ctx, arg)
}

func (s *Store) RefreshServiceItem(ctx context.Context, serviceItemID int64) error {
	_, err := s.q.RefreshEfficiencyFactsForServiceItem(ctx, serviceItemID)
	return err
}

func (s *Store) RebuildRoll(ctx context.Context, unitID *int64) error {
	_, err := s.q.RebuildRollEfficiency(ctx, int8Narg(unitID))
	return err
}

func int8Narg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
