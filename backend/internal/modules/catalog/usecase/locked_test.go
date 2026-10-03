package usecase

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-268: fields owned by the Glorian pull are refused in the panel.

type lockedStore struct {
	Store
	product  db.Product
	category db.ProductCategory
	synced   bool
	updated  int
}

func (s *lockedStore) GetProductByUUID(_ context.Context, _ db.GetProductByUUIDParams) (db.Product, error) {
	return s.product, nil
}

func (s *lockedStore) GetProductCategory(_ context.Context, _ db.GetProductCategoryParams) (db.ProductCategory, error) {
	return s.category, nil
}

func (s *lockedStore) GetProductCategoryByUUID(_ context.Context, _ db.GetProductCategoryByUUIDParams) (db.ProductCategory, error) {
	return s.category, nil
}

func (s *lockedStore) UpdateProduct(_ context.Context, arg db.UpdateProductParams) (db.Product, error) {
	s.updated++
	p := s.product
	p.UsesFixedBarcode = arg.UsesFixedBarcode
	return p, nil
}

func (s *lockedStore) UpdateProductCategory(_ context.Context, arg db.UpdateProductCategoryParams) (db.ProductCategory, error) {
	s.updated++
	c := s.category
	c.Sort = arg.Sort
	return c, nil
}

func (s *lockedStore) BrandHasIntegrationConnection(context.Context, int64) (bool, error) {
	return s.synced, nil
}

func syncedProduct() db.Product {
	return db.Product{
		ID: 1, Uuid: uuid.New(), BrandID: 1, CategoryID: 5, Sku: "HYDRA-150", Name: "HYDRA 150",
		WarrantyDurationMonths: pgtype.Int4{Int32: 24, Valid: true},
		MicronThickness:        pgtype.Numeric{Int: big.NewInt(150), Valid: true},
		Images:                 []byte("[]"), UnitType: model.UnitPiece, Active: true,
		ExternalID:   pgtype.Text{String: "10", Valid: true},
		ConnectionID: pgtype.Int8{Int64: 3, Valid: true},
		LockedFields: model.SyncedProductLockedFields,
	}
}

func TestUpdateProductRefusesLockedFields(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		in   model.ProductInput
		want string
	}{
		"name":     {model.ProductInput{Name: ptr("Panel")}, model.FieldName},
		"sku":      {model.ProductInput{SKU: ptr("X-1")}, model.FieldSKU},
		"active":   {model.ProductInput{Active: ptr(false)}, model.FieldActive},
		"warranty": {model.ProductInput{WarrantySet: true}, model.FieldWarrantyDurationMonths},
		"micron":   {model.ProductInput{MicronThickness: ptr(200.0)}, model.FieldMicronThickness},
		"desc":     {model.ProductInput{DescriptionMD: ptr("yeni")}, model.FieldDescriptionMD},
	}
	for name, tc := range cases {
		st := &lockedStore{product: syncedProduct(), category: db.ProductCategory{ID: 5, BrandID: 1}}
		_, err := New(st, nil).UpdateProduct(ctx, center, st.product.Uuid, tc.in)
		var locked *LockedError
		if !errors.As(err, &locked) || len(locked.Fields) != 1 || locked.Fields[0] != tc.want {
			t.Errorf("%s: err = %v, want LockedError{%s}", name, err, tc.want)
		}
		if st.updated != 0 {
			t.Errorf("%s: store written despite the lock", name)
		}
	}

	// Local fields and unchanged synced values pass.
	st := &lockedStore{product: syncedProduct(), category: db.ProductCategory{ID: 5, BrandID: 1}}
	got, err := New(st, nil).UpdateProduct(ctx, center, st.product.Uuid, model.ProductInput{
		UsesFixedBarcode: ptr(true), Name: ptr(" HYDRA 150 "), MicronThickness: ptr(150.0),
	})
	if err != nil || !got.UsesFixedBarcode || st.updated != 1 {
		t.Fatalf("local update: %+v, %v (updated %d)", got, err, st.updated)
	}

	// A product without locked fields is fully editable.
	st = &lockedStore{product: db.Product{ID: 2, Uuid: uuid.New(), BrandID: 1, CategoryID: 5, Sku: "L", Name: "L", Images: []byte("[]"), UnitType: model.UnitPiece}, category: db.ProductCategory{ID: 5, BrandID: 1}}
	if _, err := New(st, nil).UpdateProduct(ctx, center, st.product.Uuid, model.ProductInput{Name: ptr("Yeni")}); err != nil {
		t.Fatalf("local product: %v", err)
	}
}

func TestDeleteSyncedProductIsLocked(t *testing.T) {
	st := &lockedStore{product: syncedProduct()}
	var locked *LockedError
	if err := New(st, nil).DeleteProduct(context.Background(), center, st.product.Uuid); !errors.As(err, &locked) {
		t.Fatalf("err = %v, want LockedError", err)
	}
}

func TestSyncedBrandCategoryLocks(t *testing.T) {
	ctx := context.Background()
	cat := db.ProductCategory{ID: 5, Uuid: uuid.New(), BrandID: 1, Name: "PPF", AvailableParts: []byte(`["hood"]`), Active: true}

	st := &lockedStore{category: cat, synced: true}
	svc := New(st, nil)
	var locked *LockedError
	if _, err := svc.UpdateCategory(ctx, center, cat.Uuid, model.CategoryInput{Name: ptr("Yeni")}); !errors.As(err, &locked) || locked.Fields[0] != model.FieldName {
		t.Fatalf("rename: err = %v", err)
	}
	if _, err := svc.UpdateCategory(ctx, center, cat.Uuid, model.CategoryInput{AvailableParts: &[]string{"roof"}}); !errors.As(err, &locked) || locked.Fields[0] != model.FieldAvailableParts {
		t.Fatalf("parts: err = %v", err)
	}
	if err := svc.DeleteCategory(ctx, center, cat.Uuid); !errors.As(err, &locked) {
		t.Fatalf("delete: err = %v", err)
	}
	if _, err := svc.UpdateCategory(ctx, center, cat.Uuid, model.CategoryInput{Sort: ptr(int32(3)), Name: ptr("PPF")}); err != nil {
		t.Fatalf("sort is local: %v", err)
	}

	// Without a connection the brand edits its categories freely.
	st = &lockedStore{category: cat}
	if _, err := New(st, nil).UpdateCategory(ctx, center, cat.Uuid, model.CategoryInput{Name: ptr("Yeni")}); err != nil {
		t.Fatalf("unsynced rename: %v", err)
	}
}
