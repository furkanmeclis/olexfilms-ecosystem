package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeStore implements the Store methods a test needs; any other call
// panics through the nil embedded interface.
type fakeStore struct {
	Store
	brands  map[uuid.UUID]db.CarBrand
	models  map[uuid.UUID]db.CarModel
	updated []db.UpdateCarBrandParams
}

func (f *fakeStore) GetCarBrandByUUID(_ context.Context, id uuid.UUID) (db.CarBrand, error) {
	b, ok := f.brands[id]
	if !ok {
		return db.CarBrand{}, pgx.ErrNoRows
	}
	return b, nil
}

func (f *fakeStore) GetCarBrandByID(_ context.Context, id int64) (db.CarBrand, error) {
	for _, b := range f.brands {
		if b.ID == id {
			return b, nil
		}
	}
	return db.CarBrand{}, pgx.ErrNoRows
}

func (f *fakeStore) GetCarModelByUUID(_ context.Context, id uuid.UUID) (db.CarModel, error) {
	m, ok := f.models[id]
	if !ok {
		return db.CarModel{}, pgx.ErrNoRows
	}
	return m, nil
}

func (f *fakeStore) UpdateCarBrand(_ context.Context, arg db.UpdateCarBrandParams) (db.CarBrand, error) {
	f.updated = append(f.updated, arg)
	return db.CarBrand{ID: arg.ID, Name: arg.Name, ExternalID: arg.ExternalID, LogoHeight: arg.LogoHeight, Active: arg.Active}, nil
}

func (f *fakeStore) CountCarModelsByBrand(context.Context, int64) (int64, error) { return 0, nil }

func ptr[T any](v T) *T { return &v }

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// Hero order: model → brand → default.
func TestHeroFallbackOrder(t *testing.T) {
	brandID, modelID := uuid.New(), uuid.New()
	cases := []struct {
		name, modelKey, brandKey, wantKey, wantURLPrefix string
	}{
		{"model wins", "vehicle-models/m/hero-1.png", "vehicle-brands/b/hero-1.png", "vehicle-models/m/hero-1.png", ModelHeroPath},
		{"brand next", "", "vehicle-brands/b/hero-1.png", "vehicle-brands/b/hero-1.png", ModelHeroPath},
		{"default last", "", "", "", DefaultHeroURL},
	}
	for _, c := range cases {
		svc := New(&fakeStore{
			brands: map[uuid.UUID]db.CarBrand{brandID: {ID: 1, Uuid: brandID, Name: "BMW", HeroObjectKey: txt(c.brandKey)}},
			models: map[uuid.UUID]db.CarModel{modelID: {ID: 2, Uuid: modelID, CarBrandID: 1, Name: "X5", HeroObjectKey: txt(c.modelKey)}},
		})
		key, err := svc.ModelHeroKey(context.Background(), modelID)
		if err != nil || key != c.wantKey {
			t.Fatalf("%s: key = %q, %v; want %q", c.name, key, err, c.wantKey)
		}
		m, err := svc.GetModel(context.Background(), modelID)
		if err != nil || !strings.HasPrefix(m.HeroURL, c.wantURLPrefix) {
			t.Fatalf("%s: hero_url = %q, %v", c.name, m.HeroURL, err)
		}
		if m.Brand.UUID != brandID || m.HasHero != (c.modelKey != "") {
			t.Fatalf("%s: model view = %+v", c.name, m)
		}
	}
	if _, err := New(&fakeStore{}).ModelHeroKey(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown model: %v", err)
	}
}

// The logo URL is stable (/brand-logos/{uuid}) and its ?v= and the ETag
// change with the object key.
func TestLogoURLAndETag(t *testing.T) {
	id := uuid.New()
	if got := brandLogoURL(id, ""); got != BrandLogoPath+id.String() {
		t.Fatalf("placeholder url = %q", got)
	}
	a, b := "vehicle-brands/x/logo-a.png", "vehicle-brands/x/logo-b.png"
	if brandLogoURL(id, a) == brandLogoURL(id, b) || ETag(a) == ETag(b) {
		t.Fatal("a new logo must change the url version and the etag")
	}
	if !strings.HasPrefix(ETag(a), `"`) || !strings.HasSuffix(ETag(a), `"`) {
		t.Fatalf("etag must be quoted: %s", ETag(a))
	}
}

func TestValidateYearRange(t *testing.T) {
	cases := []struct {
		start, stop *int16
		codes       []string
	}{
		{nil, nil, nil},
		{ptr[int16](2010), nil, nil},
		{ptr[int16](2010), ptr[int16](2018), nil},
		{ptr[int16](2018), ptr[int16](2018), nil},
		{ptr[int16](2018), ptr[int16](2010), []string{"before_start"}},
		{ptr[int16](1800), ptr[int16](2200), []string{"out_of_range", "out_of_range"}},
	}
	for _, c := range cases {
		got := ValidateYearRange(c.start, c.stop)
		if len(got) != len(c.codes) {
			t.Fatalf("ValidateYearRange(%v,%v) = %+v, want %v", c.start, c.stop, got, c.codes)
		}
		for i, f := range got {
			if f.Code != c.codes[i] {
				t.Fatalf("code %d = %s, want %s", i, f.Code, c.codes[i])
			}
		}
	}
}

// Invalid input never reaches the store (the fake panics on any write).
func TestValidationBeforeStore(t *testing.T) {
	svc := New(&fakeStore{})
	ctx := context.Background()
	var verr *ValidationError
	if _, err := svc.CreateBrand(ctx, model.BrandInput{Name: ptr("  ")}); !errors.As(err, &verr) || verr.Fields[0].Field != "name" {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := svc.CreateBrand(ctx, model.BrandInput{Name: ptr("BMW"), LogoHeight: ptr[int16](2)}); !errors.As(err, &verr) || verr.Fields[0].Field != "logo_height" {
		t.Fatalf("logo height: %v", err)
	}
	if _, err := svc.CreateModel(ctx, model.ModelInput{Name: ptr("X5")}); !errors.As(err, &verr) || verr.Fields[0].Field != "brand_uuid" {
		t.Fatalf("missing brand: %v", err)
	}
	if _, err := svc.CreateModel(ctx, model.ModelInput{
		BrandUUID: ptr(uuid.New()), Name: ptr("X5"), YearStart: ptr[int16](2020), YearStop: ptr[int16](2010),
	}); !errors.As(err, &verr) || verr.Fields[0].Code != "before_start" {
		t.Fatalf("year range: %v", err)
	}
}

// PATCH: an absent nullable field is kept, an explicit null clears it.
func TestUpdateBrandNullHandling(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{brands: map[uuid.UUID]db.CarBrand{id: {
		ID: 7, Uuid: id, Name: "BMW", ExternalID: txt("12"), LogoHeight: pgtype.Int2{Int16: 40, Valid: true}, Active: true,
	}}}
	svc := New(fs)
	ctx := context.Background()
	if _, err := svc.UpdateBrand(ctx, id, model.BrandInput{Name: ptr("BMW AG")}); err != nil {
		t.Fatal(err)
	}
	if got := fs.updated[0]; got.ExternalID.String != "12" || got.LogoHeight.Int16 != 40 || got.Name != "BMW AG" {
		t.Fatalf("absent fields changed: %+v", got)
	}
	if _, err := svc.UpdateBrand(ctx, id, model.BrandInput{Present: map[string]bool{"external_id": true, "logo_height": true}}); err != nil {
		t.Fatal(err)
	}
	if got := fs.updated[1]; got.ExternalID.Valid || got.LogoHeight.Valid {
		t.Fatalf("explicit null did not clear: %+v", got)
	}
}

// PostgreSQL 18 reports ON DELETE RESTRICT as 23001; both it and 23503 are
// "in use" (409).
func TestMapDBError(t *testing.T) {
	for _, code := range []string{"23001", "23503"} {
		if err := mapDBError(&pgconn.PgError{Code: code}); !errors.Is(err, ErrInUse) {
			t.Fatalf("%s -> %v", code, err)
		}
	}
	var conflict *ConflictError
	if err := mapDBError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_car_brands_name"}); !errors.As(err, &conflict) || conflict.Field != "name" {
		t.Fatalf("unique name -> %v", err)
	}
	if err := mapDBError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_car_models_external_id"}); !errors.As(err, &conflict) || conflict.Field != "external_id" {
		t.Fatalf("unique external id -> %v", err)
	}
}
