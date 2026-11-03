package popularity

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// List is a picker list the engine ranks.
type List string

// Lists with a usage source in the new schema. Plate country, address
// country / city / district and body type have no mobile picker yet
// (TEC-1043 report), so they are not ranked.
const (
	CarBrands List = "car_brands"
	CarModels List = "car_models"
)

// Scope is who asks: the domain brand (always), the active organization
// (0 = none, general counts only) and, for own / assigned grants, the user
// whose records count as the organization's (0 = the whole organization).
type Scope struct {
	BrandID int64
	OrgID   int64
	UserID  int64
}

// HasOrg reports whether the organization part of the score applies.
func (s Scope) HasOrg() bool { return s.OrgID != 0 }

// Key names one cached count map. The brand is always part of it, the
// organization (and user) when the counts are the organization's.
type Key struct {
	List       List
	BrandID    int64
	OrgID      int64
	UserID     int64
	CarBrandID int64
}

// String is the cache key: v1:<list>:b<brand>:o<org|all>[:u<user>][:cb<car brand>].
func (k Key) String() string {
	org := "all"
	if k.OrgID != 0 {
		org = fmt.Sprint(k.OrgID)
	}
	s := fmt.Sprintf("v1:%s:b%d:o%s", k.List, k.BrandID, org)
	if k.UserID != 0 {
		s += fmt.Sprintf(":u%d", k.UserID)
	}
	if k.CarBrandID != 0 {
		s += fmt.Sprintf(":cb%d", k.CarBrandID)
	}
	return s
}

// Querier is the subset of db.Queries the engine reads.
type Querier interface {
	PopularityCarBrandCounts(ctx context.Context, arg db.PopularityCarBrandCountsParams) ([]db.PopularityCarBrandCountsRow, error)
	PopularityCarModelCounts(ctx context.Context, arg db.PopularityCarModelCountsParams) ([]db.PopularityCarModelCountsRow, error)
}

// Service loads (cached) usage counts and scores them.
type Service struct {
	q     Querier
	cache Cache
}

// New creates the engine. cache may be nil (no caching).
func New(q Querier, cache Cache) *Service {
	if cache == nil {
		cache = NoCache{}
	}
	return &Service{q: q, cache: cache}
}

// Result is a scored list: Scores for ordering and OrgCounts, the caller
// organization's own counts (never another organization's), for responses
// that show a count.
type Result struct {
	Scores    map[int64]float64
	OrgCounts Counts
}

// CarBrands scores the car brands for scope.
func (s *Service) CarBrands(ctx context.Context, scope Scope) (Result, error) {
	return s.score(ctx, CarBrands, scope, 0)
}

// CarModels scores the models of one car brand for scope.
func (s *Service) CarModels(ctx context.Context, scope Scope, carBrandID int64) (Result, error) {
	return s.score(ctx, CarModels, scope, carBrandID)
}

func (s *Service) score(ctx context.Context, list List, scope Scope, carBrandID int64) (Result, error) {
	if scope.BrandID == 0 {
		return Result{}, fmt.Errorf("popularity: %s without a brand", list)
	}
	global, err := s.counts(ctx, Key{List: list, BrandID: scope.BrandID, CarBrandID: carBrandID})
	if err != nil {
		return Result{}, err
	}
	org := Counts{}
	if scope.HasOrg() {
		if org, err = s.counts(ctx, Key{List: list, BrandID: scope.BrandID, OrgID: scope.OrgID, UserID: scope.UserID, CarBrandID: carBrandID}); err != nil {
			return Result{}, err
		}
	}
	return Result{Scores: Scores(org, global, scope.HasOrg()), OrgCounts: org}, nil
}

// counts reads one count map through the cache.
func (s *Service) counts(ctx context.Context, k Key) (Counts, error) {
	key := k.String()
	if c, ok := s.cache.Get(ctx, key); ok {
		return c, nil
	}
	c, err := s.load(ctx, k)
	if err != nil {
		return nil, err
	}
	s.cache.Set(ctx, key, c)
	return c, nil
}

func int8Arg(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func (s *Service) load(ctx context.Context, k Key) (Counts, error) {
	out := Counts{}
	switch k.List {
	case CarBrands:
		rows, err := s.q.PopularityCarBrandCounts(ctx, db.PopularityCarBrandCountsParams{
			BrandID: k.BrandID, OrgID: int8Arg(k.OrgID), CreatedByUserID: int8Arg(k.UserID),
		})
		if err != nil {
			return nil, fmt.Errorf("popularity: car brand counts: %w", err)
		}
		for _, r := range rows {
			out[r.ID] = r.UseCount
		}
	case CarModels:
		rows, err := s.q.PopularityCarModelCounts(ctx, db.PopularityCarModelCountsParams{
			BrandID: k.BrandID, CarBrandID: k.CarBrandID, OrgID: int8Arg(k.OrgID), CreatedByUserID: int8Arg(k.UserID),
		})
		if err != nil {
			return nil, fmt.Errorf("popularity: car model counts: %w", err)
		}
		for _, r := range rows {
			out[r.ID] = r.UseCount
		}
	default:
		return nil, fmt.Errorf("popularity: unknown list %q", k.List)
	}
	return out, nil
}
