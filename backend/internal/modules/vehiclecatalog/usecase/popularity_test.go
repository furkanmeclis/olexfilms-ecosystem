package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/popularity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// listStore records the list arguments.
type listStore struct {
	fakeStore
	brandArgs []db.ListCarBrandsParams
	modelArgs []db.ListCarModelsParams
}

func (l *listStore) ListCarBrands(_ context.Context, arg db.ListCarBrandsParams) ([]db.ListCarBrandsRow, error) {
	l.brandArgs = append(l.brandArgs, arg)
	return nil, nil
}

func (l *listStore) CountCarBrands(context.Context, db.CountCarBrandsParams) (int64, error) {
	return 0, nil
}

func (l *listStore) ListCarModels(_ context.Context, arg db.ListCarModelsParams) ([]db.ListCarModelsRow, error) {
	l.modelArgs = append(l.modelArgs, arg)
	return nil, nil
}

func (l *listStore) CountCarModels(context.Context, db.CountCarModelsParams) (int64, error) {
	return 0, nil
}

// fakePop records the scopes it scored.
type fakePop struct {
	scopes   []popularity.Scope
	carBrand []int64
	res      popularity.Result
}

func (f *fakePop) CarBrands(_ context.Context, s popularity.Scope) (popularity.Result, error) {
	f.scopes = append(f.scopes, s)
	return f.res, nil
}

func (f *fakePop) CarModels(_ context.Context, s popularity.Scope, carBrandID int64) (popularity.Result, error) {
	f.scopes = append(f.scopes, s)
	f.carBrand = append(f.carBrand, carBrandID)
	return f.res, nil
}

var popSort = apiquery.ResolvedSort{Key: model.SortPopularity}

// sort=popularity hands the scores to the list query as id/score arrays;
// any other sort never asks the engine and sends no arrays (default order
// unchanged).
func TestListBrandsPopularity(t *testing.T) {
	st := &listStore{}
	pop := &fakePop{res: popularity.Result{Scores: map[int64]float64{5: 0.2, 2: 0.9}, OrgCounts: popularity.Counts{2: 1}}}
	svc := New(st).WithPopularity(pop)
	scope := popularity.Scope{BrandID: 1, OrgID: 10}
	if _, _, err := svc.ListBrands(context.Background(), model.BrandFilter{Sort: popSort, Popularity: scope, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	a := st.brandArgs[0]
	if a.SortKey != "popularity" || !reflect.DeepEqual(a.PopIds, []int64{2, 5}) || !reflect.DeepEqual(a.PopScores, []float64{0.9, 0.2}) {
		t.Fatalf("args = %+v", a)
	}
	if !reflect.DeepEqual(pop.scopes, []popularity.Scope{scope}) {
		t.Fatalf("scopes = %+v", pop.scopes)
	}

	if _, _, err := svc.ListBrands(context.Background(), model.BrandFilter{Popularity: scope, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if b := st.brandArgs[1]; b.SortKey != "name" || b.PopIds != nil || b.PopScores != nil || len(pop.scopes) != 1 {
		t.Fatalf("default sort args = %+v (engine calls %d)", b, len(pop.scopes))
	}
}

// Model popularity needs one car brand and is scored inside it.
func TestListModelsPopularity(t *testing.T) {
	brand := uuid.New()
	st := &listStore{fakeStore: fakeStore{brands: map[uuid.UUID]db.CarBrand{brand: {ID: 42, Uuid: brand, Name: "BMW"}}}}
	pop := &fakePop{res: popularity.Result{Scores: map[int64]float64{7: 1}}}
	svc := New(st).WithPopularity(pop)
	scope := popularity.Scope{BrandID: 1}

	if _, _, err := svc.ListModels(context.Background(), model.ModelFilter{Sort: popSort, Popularity: scope}); !errors.Is(err, ErrPopularityUnavailable) {
		t.Fatalf("without brand err = %v", err)
	}
	if _, _, err := svc.ListModels(context.Background(), model.ModelFilter{BrandUUID: &brand, Sort: popSort, Popularity: scope}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pop.carBrand, []int64{42}) || !reflect.DeepEqual(st.modelArgs[0].PopIds, []int64{7}) {
		t.Fatalf("car brand = %v, args = %+v", pop.carBrand, st.modelArgs[0])
	}
}

// No engine or no brand: the popularity sort is refused, never silently
// unordered.
func TestPopularityUnavailable(t *testing.T) {
	ctx := context.Background()
	if _, _, err := New(&listStore{}).ListBrands(ctx, model.BrandFilter{Sort: popSort, Popularity: popularity.Scope{BrandID: 1}}); !errors.Is(err, ErrPopularityUnavailable) {
		t.Fatalf("no engine err = %v", err)
	}
	if _, _, err := New(&listStore{}).WithPopularity(&fakePop{}).ListBrands(ctx, model.BrandFilter{Sort: popSort}); !errors.Is(err, ErrPopularityUnavailable) {
		t.Fatalf("no brand err = %v", err)
	}
}

// Portal and /meta keep the old whitelist; only the panel specs add
// popularity, with the same default.
func TestPopularitySortSpecs(t *testing.T) {
	if _, ok := model.BrandSort.Columns[model.SortPopularity]; ok {
		t.Fatal("BrandSort (portal) has popularity")
	}
	if _, ok := model.ModelSort.Columns[model.SortPopularity]; ok {
		t.Fatal("ModelSort (portal) has popularity")
	}
	if model.BrandPanelSort.Columns[model.SortPopularity] != model.SortPopularity || model.BrandPanelSort.Default != model.BrandSort.Default {
		t.Fatalf("panel brand spec = %+v", model.BrandPanelSort)
	}
	if model.ModelPanelSort.Columns[model.SortPopularity] != model.SortPopularity || model.ModelPanelSort.Default != model.ModelSort.Default {
		t.Fatalf("panel model spec = %+v", model.ModelPanelSort)
	}
}
