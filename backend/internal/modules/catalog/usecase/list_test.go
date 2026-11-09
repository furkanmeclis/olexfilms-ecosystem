package usecase

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

func TestParseProductFilter(t *testing.T) {
	cat := uuid.New()
	f, err := ParseProductFilter(url.Values{
		"category_uuid": {cat.String() + "," + cat.String()}, "unit_type": {"piece,roll_meter"},
		"uses_fixed_barcode": {"true"}, "warranty_duration_months_min": {"12"},
		"micron_thickness_max": {"200.5"}, "created_from": {"2026-01-01"}, "sort": {"-micron_thickness"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.CategoryUUIDs) != 1 || f.CategoryUUIDs[0] != cat {
		t.Fatalf("category_uuid = %v", f.CategoryUUIDs)
	}
	if !slices.Equal(f.UnitTypes, []string{"piece", "roll_meter"}) || f.UsesFixedBarcode == nil || !*f.UsesFixedBarcode {
		t.Fatalf("unit_type / barcode = %v %v", f.UnitTypes, f.UsesFixedBarcode)
	}
	if f.Warranty.Min == nil || *f.Warranty.Min != 12 || f.Micron.Max == nil || *f.Micron.Max != 200.5 || f.Created.From == nil {
		t.Fatalf("ranges = %+v %+v %+v", f.Warranty, f.Micron, f.Created)
	}
	if f.Sort != (apiquery.ResolvedSort{Key: "micron_thickness", Desc: true}) {
		t.Fatalf("sort = %+v", f.Sort)
	}

	def, err := ParseProductFilter(url.Values{})
	if err != nil || def.Sort.Key != "name" || def.Sort.Desc {
		t.Fatalf("default sort = %+v %v", def.Sort, err)
	}

	for name, v := range map[string]url.Values{
		"sort":          {"sort": {"description_md"}},
		"unit_type":     {"unit_type": {"box"}},
		"category_uuid": {"category_uuid": {"nope"}},
		"barcode":       {"uses_fixed_barcode": {"yes"}},
		"range":         {"warranty_duration_months_min": {"24"}, "warranty_duration_months_max": {"12"}},
		"created":       {"created_from": {"01.01.2026"}},
	} {
		var verr *apiquery.ValidationError
		if _, err := ParseProductFilter(v); !errors.As(err, &verr) {
			t.Fatalf("%s: err = %v, want ValidationError", name, err)
		}
	}
	var verr *apiquery.ValidationError
	if _, err := ParseCategoryFilter(url.Values{"sort": {"available_parts"}}); !errors.As(err, &verr) {
		t.Fatalf("category sort err = %v", err)
	}
}

type orderStore struct {
	fakeStore
	rows []db.ListProductCategoryOrderRow
	set  []int64
}

func (o *orderStore) ListProductCategoryOrder(_ context.Context, _ int64) ([]db.ListProductCategoryOrderRow, error) {
	return o.rows, nil
}

func (o *orderStore) SetProductCategorySorts(_ context.Context, arg db.SetProductCategorySortsParams) (int64, error) {
	o.set = arg.Ids
	return int64(len(arg.Ids)), nil
}

func (o *orderStore) ListProductCategories(_ context.Context, _ db.ListProductCategoriesParams) ([]db.ProductCategory, error) {
	return []db.ProductCategory{}, nil
}

func (o *orderStore) CountProductCategories(_ context.Context, _ db.CountProductCategoriesParams) (int64, error) {
	return 0, nil
}

// A subset is rearranged inside the slots it holds; the others keep theirs.
func TestReorderCategoriesSubset(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &orderStore{rows: []db.ListProductCategoryOrderRow{
		{ID: 1, Uuid: a, Sort: 0}, {ID: 2, Uuid: b, Sort: 5}, {ID: 3, Uuid: c, Sort: 5}, {ID: 4, Uuid: d, Sort: 9},
	}}
	svc := New(store, nil)
	ctx := context.Background()
	if _, err := svc.ReorderCategories(ctx, center, []uuid.UUID{d, b}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.set, []int64{1, 4, 3, 2}) {
		t.Fatalf("order = %v, want [1 4 3 2]", store.set)
	}

	var verr *ValidationError
	for name, ids := range map[string][]uuid.UUID{
		"empty": nil, "duplicate": {a, a}, "unknown": {a, uuid.New()}, "nil": {uuid.Nil},
	} {
		if _, err := svc.ReorderCategories(ctx, center, ids); !errors.As(err, &verr) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if _, err := svc.ReorderCategories(ctx, distributor, []uuid.UUID{a}); !errors.Is(err, ErrCenterOnly) {
		t.Fatalf("distributor reorder err = %v", err)
	}
}

// listStore records the list / count parameters.
type listStore struct {
	fakeStore
	list  []db.ListProductsParams
	count []db.CountProductsParams
}

func (l *listStore) ListProducts(_ context.Context, arg db.ListProductsParams) ([]db.Product, error) {
	l.list = append(l.list, arg)
	return []db.Product{}, nil
}

func (l *listStore) CountProducts(_ context.Context, arg db.CountProductsParams) (int64, error) {
	l.count = append(l.count, arg)
	return 0, nil
}

// TEC-1061: a non-center organization only lists products open to dealer
// orders; asking for closed ones gives nothing. The center filters freely.
func TestListProductsDealerOrderableScope(t *testing.T) {
	f, err := ParseProductFilter(url.Values{"dealer_orderable": {"false"}})
	if err != nil || f.DealerOrderable == nil || *f.DealerOrderable {
		t.Fatalf("parse dealer_orderable = %v %v", f.DealerOrderable, err)
	}
	if _, err := ParseProductFilter(url.Values{"dealer_orderable": {"maybe"}}); err == nil {
		t.Fatal("bad dealer_orderable must fail")
	}

	st := &listStore{}
	s := New(st, nil)
	if _, _, err := s.ListProducts(context.Background(), center, f); err != nil {
		t.Fatal(err)
	}
	if got := st.list[0].DealerOrderable; !got.Valid || got.Bool {
		t.Fatalf("center filter = %+v", got)
	}
	if _, _, err := s.ListProducts(context.Background(), center, model.ProductFilter{}); err != nil {
		t.Fatal(err)
	}
	if got := st.list[1].DealerOrderable; got.Valid {
		t.Fatalf("center without filter = %+v", got)
	}

	for name, org := range map[string]orgctx.Scope{"distributor": distributor, "dealer": dealer} {
		st := &listStore{}
		s := New(st, nil)
		if _, _, err := s.ListProducts(context.Background(), org, model.ProductFilter{}); err != nil {
			t.Fatal(err)
		}
		if got := st.list[0].DealerOrderable; !got.Valid || !got.Bool || !st.count[0].DealerOrderable.Bool {
			t.Fatalf("%s default = %+v", name, got)
		}
		items, total, err := s.ListProducts(context.Background(), org, f)
		if err != nil || len(items) != 0 || total != 0 || len(st.list) != 1 {
			t.Fatalf("%s asking closed = %v %d %v (queries %d)", name, items, total, err, len(st.list))
		}
	}
}
