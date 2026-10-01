package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeStore implements the few Store methods a test needs; any other call
// panics through the nil embedded interface.
type fakeStore struct {
	Store
	categories map[uuid.UUID]db.ProductCategory
	created    []db.CreateProductParams
	bulk       []db.SetProductsActiveByUUIDsParams
}

func (f *fakeStore) GetProductCategoryByUUID(_ context.Context, arg db.GetProductCategoryByUUIDParams) (db.ProductCategory, error) {
	c, ok := f.categories[arg.Uuid]
	if !ok || c.BrandID != arg.BrandID {
		return db.ProductCategory{}, pgx.ErrNoRows
	}
	return c, nil
}

func (f *fakeStore) CreateProduct(_ context.Context, arg db.CreateProductParams) (db.Product, error) {
	f.created = append(f.created, arg)
	return db.Product{
		ID: 1, Uuid: uuid.New(), OrganizationID: arg.OrganizationID, BrandID: arg.BrandID,
		CategoryID: arg.CategoryID, Sku: arg.Sku, Name: arg.Name, Images: arg.Images,
		UnitType: arg.UnitType, Active: arg.Active, WarrantyDurationMonths: arg.WarrantyDurationMonths,
		MicronThickness: arg.MicronThickness,
	}, nil
}

func (f *fakeStore) SetProductsActiveByUUIDs(_ context.Context, arg db.SetProductsActiveByUUIDsParams) ([]uuid.UUID, error) {
	f.bulk = append(f.bulk, arg)
	return arg.Uuids, nil
}

type fakeIndexer struct{ upserts, deletes []string }

func (f *fakeIndexer) EnqueueUpsert(_ context.Context, _, id string) {
	f.upserts = append(f.upserts, id)
}
func (f *fakeIndexer) EnqueueDelete(_ context.Context, _, id string) {
	f.deletes = append(f.deletes, id)
}

var (
	center      = orgctx.Scope{InternalID: 10, OrgType: "center", BrandID: 1}
	distributor = orgctx.Scope{InternalID: 20, OrgType: "distributor", BrandID: 1}
	dealer      = orgctx.Scope{InternalID: 30, OrgType: "dealer", BrandID: 1}
)

func ptr[T any](v T) *T { return &v }

// K4: every write is refused outside the brand center, before any store call.
func TestWritesAreCenterOnly(t *testing.T) {
	svc := New(&fakeStore{}, nil)
	ctx := context.Background()
	id := uuid.New()
	for name, org := range map[string]orgctx.Scope{"distributor": distributor, "dealer": dealer, "none": {}} {
		calls := map[string]error{}
		_, calls["create category"] = svc.CreateCategory(ctx, org, model.CategoryInput{Name: ptr("x")})
		_, calls["update category"] = svc.UpdateCategory(ctx, org, id, model.CategoryInput{})
		calls["delete category"] = svc.DeleteCategory(ctx, org, id)
		_, calls["create product"] = svc.CreateProduct(ctx, org, model.ProductInput{})
		_, calls["update product"] = svc.UpdateProduct(ctx, org, id, model.ProductInput{})
		calls["delete product"] = svc.DeleteProduct(ctx, org, id)
		_, calls["bulk active"] = svc.SetProductsActive(ctx, org, []uuid.UUID{id}, false)
		for op, err := range calls {
			if !errors.Is(err, ErrCenterOnly) {
				t.Errorf("%s %s: err = %v, want ErrCenterOnly", name, op, err)
			}
		}
	}
}

func TestCreateProductValidatesAndUsesActiveBrand(t *testing.T) {
	olexCat := uuid.New()
	glorianCat := uuid.New()
	store := &fakeStore{categories: map[uuid.UUID]db.ProductCategory{
		olexCat:    {ID: 5, Uuid: olexCat, BrandID: 1, Name: "PPF"},
		glorianCat: {ID: 6, Uuid: glorianCat, BrandID: 2, Name: "Glorian PPF"},
	}}
	idx := &fakeIndexer{}
	svc := New(store, idx)
	ctx := context.Background()

	_, err := svc.CreateProduct(ctx, center, model.ProductInput{
		CategoryUUID: &glorianCat, SKU: ptr(" "), Name: ptr("x"), UnitType: ptr("box"),
		WarrantyDurationMonths: ptr(int32(-1)), MicronThickness: ptr(0.0),
	})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want validation", err)
	}
	fields := map[string]bool{}
	for _, f := range verr.Fields {
		fields[f.Field] = true
	}
	for _, want := range []string{"sku", "unit_type", "warranty_duration_months", "micron_thickness", "category_uuid"} {
		if !fields[want] {
			t.Errorf("missing validation error for %s (got %v)", want, verr.Fields)
		}
	}
	if len(store.created) != 0 {
		t.Fatal("invalid product reached the store")
	}

	p, err := svc.CreateProduct(ctx, center, model.ProductInput{
		CategoryUUID: &olexCat, SKU: ptr(" OLX-1 "), Name: ptr(" Film "), MicronThickness: ptr(190.5),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := store.created[0]
	if got.BrandID != 1 || got.OrganizationID != center.InternalID || got.CategoryID != 5 {
		t.Fatalf("create params = %+v", got)
	}
	if got.Sku != "OLX-1" || got.Name != "Film" || got.UnitType != model.UnitPiece || !got.Active {
		t.Fatalf("defaults/trim = %+v", got)
	}
	if p.MicronThickness == nil || *p.MicronThickness != 190.5 || p.Category.Name != "PPF" {
		t.Fatalf("view = %+v", p)
	}
	if len(idx.upserts) != 1 || idx.upserts[0] != p.UUID.String() {
		t.Fatalf("indexer upserts = %v", idx.upserts)
	}
}

func TestSetProductsActive(t *testing.T) {
	store := &fakeStore{}
	idx := &fakeIndexer{}
	svc := New(store, idx)
	ctx := context.Background()
	var verr *ValidationError
	if _, err := svc.SetProductsActive(ctx, center, nil, true); !errors.As(err, &verr) {
		t.Fatalf("empty: %v", err)
	}
	a := uuid.New()
	res, err := svc.SetProductsActive(ctx, center, []uuid.UUID{a, a, uuid.Nil}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Requested != 1 || res.Updated != 1 || store.bulk[0].BrandID != 1 || store.bulk[0].Active {
		t.Fatalf("res = %+v params = %+v", res, store.bulk[0])
	}
	if len(idx.upserts) != 1 {
		t.Fatalf("reindexed %v", idx.upserts)
	}
}

func TestValidateCategory(t *testing.T) {
	name, parts, err := validateCategory("  PPF  ", []string{" hood", "hood", "", "roof"})
	if err != nil || name != "PPF" || len(parts) != 2 || parts[0] != "hood" || parts[1] != "roof" {
		t.Fatalf("got %q %v %v", name, parts, err)
	}
	if _, _, err := validateCategory(" ", nil); err == nil {
		t.Fatal("empty name accepted")
	}
}

func TestImportRowNeedsCenter(t *testing.T) {
	a := NewIOAdapter(New(&fakeStore{}, nil), nil)
	row := map[string]any{"sku": "A", "name": "B", "category": "C"}
	for _, ctx := range []context.Context{
		context.Background(),
		orgctx.WithScope(context.Background(), distributor),
	} {
		res, err := a.ApplyRow(ctx, row, nil)
		if err != nil || res.OK {
			t.Fatalf("non-center import row = %+v, %v", res, err)
		}
	}
}

func TestImportInput(t *testing.T) {
	cat := uuid.New()
	row := map[string]string{
		"sku": "A", "name": "B", "warranty_duration_months": "24", "micron_thickness": "190,5",
		"active": "pasif", "uses_fixed_barcode": "evet", "unit_type": "roll_meter",
	}
	in, perr := importInput(func(k string) string { return row[k] }, cat)
	if perr != "" {
		t.Fatal(perr)
	}
	if *in.WarrantyDurationMonths != 24 || *in.MicronThickness != 190.5 || *in.Active || !*in.UsesFixedBarcode || *in.UnitType != "roll_meter" {
		t.Fatalf("in = %+v", in)
	}
	row["micron_thickness"] = "thick"
	if _, perr := importInput(func(k string) string { return row[k] }, cat); perr == "" {
		t.Fatal("bad micron accepted")
	}
}

func TestProductDocumentCarriesBrand(t *testing.T) {
	doc := productDocument(db.Product{Uuid: uuid.New(), BrandID: 2, Sku: "S", Name: "N", Active: true},
		db.ProductCategory{Name: "C"})
	if doc.BrandID != 2 || doc.Spec != SearchSpec || doc.Title != "N" {
		t.Fatalf("doc = %+v", doc)
	}
}

// PostgreSQL 18 reports ON DELETE RESTRICT as 23001, older versions as 23503.
func TestMapDBErrorInUse(t *testing.T) {
	for _, code := range []string{"23503", "23001"} {
		if err := mapDBError(&pgconn.PgError{Code: code}); !errors.Is(err, ErrInUse) {
			t.Errorf("%s -> %v, want ErrInUse", code, err)
		}
	}
}

func TestNumericRoundTrip(t *testing.T) {
	if numericPtr(pgtype.Numeric{}) != nil || numericArg(nil).Valid {
		t.Fatal("nil numeric")
	}
	if v := numericPtr(numericArg(ptr(12.34))); v == nil || *v != 12.34 {
		t.Fatalf("round trip = %v", v)
	}
}
