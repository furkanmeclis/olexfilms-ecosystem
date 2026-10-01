// Package usecase implements the TEC-149 vehicle catalog: car brands and
// models as global reference data (no organization/brand scope). Every
// organization role and the portal roles read; only super_admin writes
// (vehicle_catalog.write, enforced on the routes). Ported from the
// otopoly-go vehiclecatalog module, reshaped to the hub data model
// (external_id, year range, body type, powertrain, logo + hero images).
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Public URL paths. The frontend serves them at a fixed, cacheable,
// unsigned URL and proxies them to the Go /v1/public/* routes.
const (
	BrandLogoPath  = "/brand-logos/"
	BrandHeroPath  = "/vehicle-heroes/brands/"
	ModelHeroPath  = "/vehicle-heroes/models/"
	DefaultHeroURL = "/vehicle-heroes/default"
)

// Validation limits.
const (
	maxBrandName   = 150
	maxModelName   = 200
	maxExternalID  = 64
	maxShortText   = 64
	minLogoHeight  = 8
	maxLogoHeight  = 512
	minYear        = 1900
	maxYear        = 2100
	maxSearchChars = 100
)

var (
	// ErrNotFound: no such brand or model.
	ErrNotFound = errors.New("vehicle catalog: not found")
	// ErrInUse: the brand still has models (or the row is referenced).
	ErrInUse = errors.New("vehicle catalog: record is still in use")
)

// FieldError is one invalid input field.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// ValidationError lists invalid input fields (422).
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "vehicle catalog: invalid input"
	}
	return "vehicle catalog: invalid " + e.Fields[0].Field + ": " + e.Fields[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// ConflictError is a unique violation (duplicate name or external id).
type ConflictError struct{ Field string }

func (e *ConflictError) Error() string { return "vehicle catalog: duplicate " + e.Field }

// Store is the subset of db.Queries the vehicle catalog uses.
type Store interface {
	CreateCarBrand(ctx context.Context, arg db.CreateCarBrandParams) (db.CarBrand, error)
	GetCarBrandByUUID(ctx context.Context, id uuid.UUID) (db.CarBrand, error)
	GetCarBrandByID(ctx context.Context, id int64) (db.CarBrand, error)
	UpdateCarBrand(ctx context.Context, arg db.UpdateCarBrandParams) (db.CarBrand, error)
	SetCarBrandLogo(ctx context.Context, arg db.SetCarBrandLogoParams) (db.CarBrand, error)
	SetCarBrandHero(ctx context.Context, arg db.SetCarBrandHeroParams) (db.CarBrand, error)
	DeleteCarBrand(ctx context.Context, id int64) (int64, error)
	ListCarBrands(ctx context.Context, arg db.ListCarBrandsParams) ([]db.ListCarBrandsRow, error)
	CountCarBrands(ctx context.Context, arg db.CountCarBrandsParams) (int64, error)
	CountCarModelsByBrand(ctx context.Context, carBrandID int64) (int64, error)
	CreateCarModel(ctx context.Context, arg db.CreateCarModelParams) (db.CarModel, error)
	GetCarModelByUUID(ctx context.Context, id uuid.UUID) (db.CarModel, error)
	UpdateCarModel(ctx context.Context, arg db.UpdateCarModelParams) (db.CarModel, error)
	SetCarModelHero(ctx context.Context, arg db.SetCarModelHeroParams) (db.CarModel, error)
	DeleteCarModel(ctx context.Context, id int64) (int64, error)
	ListCarModels(ctx context.Context, arg db.ListCarModelsParams) ([]db.ListCarModelsRow, error)
	CountCarModels(ctx context.Context, arg db.CountCarModelsParams) (int64, error)
}

// Service is the vehicle catalog use case.
type Service struct {
	store Store
}

// New creates the service.
func New(store Store) *Service { return &Service{store: store} }

// --- Image identity ---------------------------------------------------------

// ETag is the strong entity tag of a stored image. Object keys carry a
// per-upload version, so the tag changes exactly when the image changes.
func ETag(objectKey string) string {
	sum := sha256.Sum256([]byte(objectKey))
	return `"` + hex.EncodeToString(sum[:10]) + `"`
}

func version(objectKey string) string {
	sum := sha256.Sum256([]byte(objectKey))
	return hex.EncodeToString(sum[:6])
}

func withVersion(base, key string) string {
	if key == "" {
		return base
	}
	return base + "?v=" + version(key)
}

func text(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return strings.TrimSpace(t.String)
}

// ResolveHero returns the first non-empty object key in the order model →
// brand. An empty result means the default hero image.
func ResolveHero(modelKey, brandKey string) string {
	if modelKey != "" {
		return modelKey
	}
	return brandKey
}

func brandLogoURL(id uuid.UUID, key string) string {
	return withVersion(BrandLogoPath+id.String(), key)
}

func brandHeroURL(id uuid.UUID, key string) string {
	if key == "" {
		return DefaultHeroURL
	}
	return withVersion(BrandHeroPath+id.String(), key)
}

func modelHeroURL(id uuid.UUID, modelKey, brandKey string) string {
	key := ResolveHero(modelKey, brandKey)
	if key == "" {
		return DefaultHeroURL
	}
	return withVersion(ModelHeroPath+id.String(), key)
}

// --- Views ------------------------------------------------------------------

func strPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func int2Ptr(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	n := v.Int16
	return &n
}

func textArg(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *p, Valid: true}
}

func int2Arg(p *int16) pgtype.Int2 {
	if p == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: *p, Valid: true}
}

func brandView(b db.CarBrand, modelCount int64) model.Brand {
	logo, hero := text(b.LogoObjectKey), text(b.HeroObjectKey)
	return model.Brand{
		UUID: b.Uuid, ExternalID: strPtr(b.ExternalID), Name: b.Name, ShowName: b.ShowName,
		LogoHeight: int2Ptr(b.LogoHeight), Active: b.Active,
		HasLogo: logo != "", LogoURL: brandLogoURL(b.Uuid, logo),
		HasHero: hero != "", HeroURL: brandHeroURL(b.Uuid, hero),
		ModelCount: modelCount, CreatedAt: b.CreatedAt.Time, UpdatedAt: b.UpdatedAt.Time,
	}
}

func modelView(m db.CarModel, brand db.CarBrand) model.Model {
	key := text(m.HeroObjectKey)
	return model.Model{
		UUID: m.Uuid, Brand: model.BrandRef{UUID: brand.Uuid, Name: brand.Name},
		ExternalID: strPtr(m.ExternalID), Name: m.Name,
		BodyType: strPtr(m.BodyType), Powertrain: strPtr(m.Powertrain),
		YearStart: int2Ptr(m.YearStart), YearStop: int2Ptr(m.YearStop), Active: m.Active,
		HasHero: key != "", HeroURL: modelHeroURL(m.Uuid, key, text(brand.HeroObjectKey)),
		CreatedAt: m.CreatedAt.Time, UpdatedAt: m.UpdatedAt.Time,
	}
}

// --- Brands -----------------------------------------------------------------

func searchArg(q string) pgtype.Text {
	q = strings.TrimSpace(q)
	if q == "" {
		return pgtype.Text{}
	}
	if utf8.RuneCountInString(q) > maxSearchChars {
		q = string([]rune(q)[:maxSearchChars])
	}
	return pgtype.Text{String: q, Valid: true}
}

func boolArg(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// ListBrands lists brands by name (paged, searchable).
func (s *Service) ListBrands(ctx context.Context, f model.BrandFilter) ([]model.Brand, int64, error) {
	q, active := searchArg(f.Q), boolArg(f.Active)
	rows, err := s.store.ListCarBrands(ctx, db.ListCarBrandsParams{
		Active: active, Q: q, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountCarBrands(ctx, db.CountCarBrandsParams{Active: active, Q: q})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Brand, 0, len(rows))
	for _, r := range rows {
		out = append(out, brandView(db.CarBrand{
			ID: r.ID, Uuid: r.Uuid, ExternalID: r.ExternalID, Name: r.Name,
			LogoObjectKey: r.LogoObjectKey, HeroObjectKey: r.HeroObjectKey, ShowName: r.ShowName,
			LogoHeight: r.LogoHeight, Active: r.Active, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, r.ModelCount))
	}
	return out, total, nil
}

func (s *Service) brand(ctx context.Context, id uuid.UUID) (db.CarBrand, error) {
	row, err := s.store.GetCarBrandByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CarBrand{}, ErrNotFound
	}
	return row, err
}

func (s *Service) brandWithCount(ctx context.Context, b db.CarBrand) (model.Brand, error) {
	n, err := s.store.CountCarModelsByBrand(ctx, b.ID)
	if err != nil {
		return model.Brand{}, err
	}
	return brandView(b, n), nil
}

// GetBrand returns one brand.
func (s *Service) GetBrand(ctx context.Context, id uuid.UUID) (model.Brand, error) {
	b, err := s.brand(ctx, id)
	if err != nil {
		return model.Brand{}, err
	}
	return s.brandWithCount(ctx, b)
}

type brandFields struct {
	externalID *string
	name       string
	showName   bool
	logoHeight *int16
	active     bool
}

func applyBrand(cur brandFields, in model.BrandInput, create bool) brandFields {
	if in.Name != nil {
		cur.name = *in.Name
	}
	if in.ShowName != nil {
		cur.showName = *in.ShowName
	}
	if in.Active != nil {
		cur.active = *in.Active
	}
	if create || in.Present["external_id"] || in.ExternalID != nil {
		cur.externalID = in.ExternalID
	}
	if create || in.Present["logo_height"] || in.LogoHeight != nil {
		cur.logoHeight = in.LogoHeight
	}
	return cur
}

func validateBrand(f brandFields) (brandFields, error) {
	verr := &ValidationError{}
	f.name = strings.TrimSpace(f.name)
	switch {
	case f.name == "":
		verr.add("name", "required", "name is required")
	case utf8.RuneCountInString(f.name) > maxBrandName:
		verr.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", maxBrandName))
	}
	f.externalID = cleanOptional(verr, "external_id", f.externalID, maxExternalID)
	if f.logoHeight != nil && (*f.logoHeight < minLogoHeight || *f.logoHeight > maxLogoHeight) {
		verr.add("logo_height", "out_of_range", fmt.Sprintf("logo_height must be between %d and %d", minLogoHeight, maxLogoHeight))
	}
	return f, verr.orNil()
}

// cleanOptional trims an optional text; empty becomes NULL.
func cleanOptional(verr *ValidationError, field string, v *string, max int) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	if utf8.RuneCountInString(t) > max {
		verr.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
	}
	return &t
}

// CreateBrand creates a brand.
func (s *Service) CreateBrand(ctx context.Context, in model.BrandInput) (model.Brand, error) {
	f, err := validateBrand(applyBrand(brandFields{showName: true, active: true}, in, true))
	if err != nil {
		return model.Brand{}, err
	}
	row, err := s.store.CreateCarBrand(ctx, db.CreateCarBrandParams{
		ExternalID: textArg(f.externalID), Name: f.name, ShowName: f.showName,
		LogoHeight: int2Arg(f.logoHeight), Active: f.active,
	})
	if err != nil {
		return model.Brand{}, mapDBError(err)
	}
	return brandView(row, 0), nil
}

// UpdateBrand patches a brand.
func (s *Service) UpdateBrand(ctx context.Context, id uuid.UUID, in model.BrandInput) (model.Brand, error) {
	cur, err := s.brand(ctx, id)
	if err != nil {
		return model.Brand{}, err
	}
	f, err := validateBrand(applyBrand(brandFields{
		externalID: strPtr(cur.ExternalID), name: cur.Name, showName: cur.ShowName,
		logoHeight: int2Ptr(cur.LogoHeight), active: cur.Active,
	}, in, false))
	if err != nil {
		return model.Brand{}, err
	}
	row, err := s.store.UpdateCarBrand(ctx, db.UpdateCarBrandParams{
		ID: cur.ID, ExternalID: textArg(f.externalID), Name: f.name, ShowName: f.showName,
		LogoHeight: int2Arg(f.logoHeight), Active: f.active,
	})
	if err != nil {
		return model.Brand{}, mapDBError(err)
	}
	return s.brandWithCount(ctx, row)
}

// DeleteBrand deletes a brand without models (409 otherwise). It returns
// the object keys of the brand images so the caller can remove them.
func (s *Service) DeleteBrand(ctx context.Context, id uuid.UUID) ([]string, error) {
	cur, err := s.brand(ctx, id)
	if err != nil {
		return nil, err
	}
	n, err := s.store.DeleteCarBrand(ctx, cur.ID)
	if err != nil {
		return nil, mapDBError(err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return nonEmpty(text(cur.LogoObjectKey), text(cur.HeroObjectKey)), nil
}

func nonEmpty(keys ...string) []string {
	out := keys[:0]
	for _, k := range keys {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// Image kinds of SetBrandImage.
const (
	ImageLogo = "logo"
	ImageHero = "hero"
)

// SetBrandImage stores (key != "") or clears (key == "") the brand logo or
// hero and returns the brand plus the previous object key to delete.
func (s *Service) SetBrandImage(ctx context.Context, id uuid.UUID, kind, key string) (model.Brand, string, error) {
	cur, err := s.brand(ctx, id)
	if err != nil {
		return model.Brand{}, "", err
	}
	arg := pgtype.Text{String: key, Valid: key != ""}
	var row db.CarBrand
	var old string
	switch kind {
	case ImageLogo:
		old = text(cur.LogoObjectKey)
		row, err = s.store.SetCarBrandLogo(ctx, db.SetCarBrandLogoParams{ID: cur.ID, LogoObjectKey: arg})
	case ImageHero:
		old = text(cur.HeroObjectKey)
		row, err = s.store.SetCarBrandHero(ctx, db.SetCarBrandHeroParams{ID: cur.ID, HeroObjectKey: arg})
	default:
		return model.Brand{}, "", fmt.Errorf("vehicle catalog: unknown image kind %q", kind)
	}
	if err != nil {
		return model.Brand{}, "", mapDBError(err)
	}
	b, err := s.brandWithCount(ctx, row)
	if old == key {
		old = ""
	}
	return b, old, err
}

// BrandLogoKey returns the logo object key of a brand ("" = no logo, the
// caller serves the placeholder).
func (s *Service) BrandLogoKey(ctx context.Context, id uuid.UUID) (string, error) {
	b, err := s.brand(ctx, id)
	if err != nil {
		return "", err
	}
	return text(b.LogoObjectKey), nil
}

// BrandHeroKey returns the brand hero object key ("" = default image).
func (s *Service) BrandHeroKey(ctx context.Context, id uuid.UUID) (string, error) {
	b, err := s.brand(ctx, id)
	if err != nil {
		return "", err
	}
	return text(b.HeroObjectKey), nil
}

// ModelHeroKey resolves the hero of a model: model → brand ("" = default).
func (s *Service) ModelHeroKey(ctx context.Context, id uuid.UUID) (string, error) {
	m, b, err := s.model(ctx, id)
	if err != nil {
		return "", err
	}
	return ResolveHero(text(m.HeroObjectKey), text(b.HeroObjectKey)), nil
}

// --- Models -----------------------------------------------------------------

func (s *Service) model(ctx context.Context, id uuid.UUID) (db.CarModel, db.CarBrand, error) {
	m, err := s.store.GetCarModelByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CarModel{}, db.CarBrand{}, ErrNotFound
	}
	if err != nil {
		return db.CarModel{}, db.CarBrand{}, err
	}
	b, err := s.store.GetCarBrandByID(ctx, m.CarBrandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CarModel{}, db.CarBrand{}, ErrNotFound
	}
	return m, b, err
}

// ListModels lists or searches models ("brand model" text, external id).
func (s *Service) ListModels(ctx context.Context, f model.ModelFilter) ([]model.Model, int64, error) {
	var brandID pgtype.Int8
	if f.BrandUUID != nil {
		b, err := s.brand(ctx, *f.BrandUUID)
		if err != nil {
			return nil, 0, err
		}
		brandID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	var brandActive pgtype.Bool
	if f.OnlyActiveBrands {
		brandActive = pgtype.Bool{Bool: true, Valid: true}
	}
	q, active := searchArg(f.Q), boolArg(f.Active)
	rows, err := s.store.ListCarModels(ctx, db.ListCarModelsParams{
		CarBrandID: brandID, Active: active, BrandActive: brandActive, Q: q,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountCarModels(ctx, db.CountCarModelsParams{
		CarBrandID: brandID, Active: active, BrandActive: brandActive, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Model, 0, len(rows))
	for _, r := range rows {
		out = append(out, modelView(db.CarModel{
			ID: r.ID, Uuid: r.Uuid, CarBrandID: r.CarBrandID, ExternalID: r.ExternalID, Name: r.Name,
			BodyType: r.BodyType, Powertrain: r.Powertrain, YearStart: r.YearStart, YearStop: r.YearStop,
			HeroObjectKey: r.HeroObjectKey, Active: r.Active, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, db.CarBrand{Uuid: r.BrandUuid, Name: r.BrandName, HeroObjectKey: r.BrandHeroObjectKey}))
	}
	return out, total, nil
}

// GetModel returns one model.
func (s *Service) GetModel(ctx context.Context, id uuid.UUID) (model.Model, error) {
	m, b, err := s.model(ctx, id)
	if err != nil {
		return model.Model{}, err
	}
	return modelView(m, b), nil
}

type modelFields struct {
	externalID, bodyType, powertrain *string
	name                             string
	yearStart, yearStop              *int16
	active                           bool
}

func applyModel(cur modelFields, in model.ModelInput, create bool) modelFields {
	if in.Name != nil {
		cur.name = *in.Name
	}
	if in.Active != nil {
		cur.active = *in.Active
	}
	set := func(field string, dst **string, v *string) {
		if create || in.Present[field] || v != nil {
			*dst = v
		}
	}
	set("external_id", &cur.externalID, in.ExternalID)
	set("body_type", &cur.bodyType, in.BodyType)
	set("powertrain", &cur.powertrain, in.Powertrain)
	if create || in.Present["year_start"] || in.YearStart != nil {
		cur.yearStart = in.YearStart
	}
	if create || in.Present["year_stop"] || in.YearStop != nil {
		cur.yearStop = in.YearStop
	}
	return cur
}

// ValidateYearRange checks year_start/year_stop (each optional, 1900–2100,
// stop ≥ start).
func ValidateYearRange(start, stop *int16) []FieldError {
	verr := &ValidationError{}
	for _, y := range []struct {
		field string
		v     *int16
	}{{"year_start", start}, {"year_stop", stop}} {
		if y.v != nil && (*y.v < minYear || *y.v > maxYear) {
			verr.add(y.field, "out_of_range", fmt.Sprintf("%s must be between %d and %d", y.field, minYear, maxYear))
		}
	}
	if len(verr.Fields) == 0 && start != nil && stop != nil && *stop < *start {
		verr.add("year_stop", "before_start", "year_stop must not be before year_start")
	}
	return verr.Fields
}

func validateModel(f modelFields) (modelFields, error) {
	verr := &ValidationError{}
	f.name = strings.TrimSpace(f.name)
	switch {
	case f.name == "":
		verr.add("name", "required", "name is required")
	case utf8.RuneCountInString(f.name) > maxModelName:
		verr.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", maxModelName))
	}
	f.externalID = cleanOptional(verr, "external_id", f.externalID, maxExternalID)
	f.bodyType = cleanOptional(verr, "body_type", f.bodyType, maxShortText)
	f.powertrain = cleanOptional(verr, "powertrain", f.powertrain, maxShortText)
	verr.Fields = append(verr.Fields, ValidateYearRange(f.yearStart, f.yearStop)...)
	return f, verr.orNil()
}

// CreateModel creates a model under in.BrandUUID.
func (s *Service) CreateModel(ctx context.Context, in model.ModelInput) (model.Model, error) {
	if in.BrandUUID == nil {
		return model.Model{}, &ValidationError{Fields: []FieldError{{Field: "brand_uuid", Code: "required", Message: "brand_uuid is required"}}}
	}
	f, err := validateModel(applyModel(modelFields{active: true}, in, true))
	if err != nil {
		return model.Model{}, err
	}
	b, err := s.brand(ctx, *in.BrandUUID)
	if errors.Is(err, ErrNotFound) {
		return model.Model{}, &ValidationError{Fields: []FieldError{{Field: "brand_uuid", Code: "not_found", Message: "brand not found"}}}
	}
	if err != nil {
		return model.Model{}, err
	}
	row, err := s.store.CreateCarModel(ctx, db.CreateCarModelParams{
		CarBrandID: b.ID, ExternalID: textArg(f.externalID), Name: f.name,
		BodyType: textArg(f.bodyType), Powertrain: textArg(f.powertrain),
		YearStart: int2Arg(f.yearStart), YearStop: int2Arg(f.yearStop), Active: f.active,
	})
	if err != nil {
		return model.Model{}, mapDBError(err)
	}
	return modelView(row, b), nil
}

// UpdateModel patches a model. The brand of a model is fixed.
func (s *Service) UpdateModel(ctx context.Context, id uuid.UUID, in model.ModelInput) (model.Model, error) {
	if in.BrandUUID != nil {
		return model.Model{}, &ValidationError{Fields: []FieldError{{Field: "brand_uuid", Code: "immutable", Message: "the brand of a model cannot change"}}}
	}
	cur, b, err := s.model(ctx, id)
	if err != nil {
		return model.Model{}, err
	}
	f, err := validateModel(applyModel(modelFields{
		externalID: strPtr(cur.ExternalID), bodyType: strPtr(cur.BodyType), powertrain: strPtr(cur.Powertrain),
		name: cur.Name, yearStart: int2Ptr(cur.YearStart), yearStop: int2Ptr(cur.YearStop), active: cur.Active,
	}, in, false))
	if err != nil {
		return model.Model{}, err
	}
	row, err := s.store.UpdateCarModel(ctx, db.UpdateCarModelParams{
		ID: cur.ID, ExternalID: textArg(f.externalID), Name: f.name,
		BodyType: textArg(f.bodyType), Powertrain: textArg(f.powertrain),
		YearStart: int2Arg(f.yearStart), YearStop: int2Arg(f.yearStop), Active: f.active,
	})
	if err != nil {
		return model.Model{}, mapDBError(err)
	}
	return modelView(row, b), nil
}

// DeleteModel deletes a model and returns its hero object key ("" = none).
func (s *Service) DeleteModel(ctx context.Context, id uuid.UUID) (string, error) {
	cur, _, err := s.model(ctx, id)
	if err != nil {
		return "", err
	}
	n, err := s.store.DeleteCarModel(ctx, cur.ID)
	if err != nil {
		return "", mapDBError(err)
	}
	if n == 0 {
		return "", ErrNotFound
	}
	return text(cur.HeroObjectKey), nil
}

// SetModelHero stores or clears (key == "") the model hero image and
// returns the model plus the previous object key to delete.
func (s *Service) SetModelHero(ctx context.Context, id uuid.UUID, key string) (model.Model, string, error) {
	cur, b, err := s.model(ctx, id)
	if err != nil {
		return model.Model{}, "", err
	}
	row, err := s.store.SetCarModelHero(ctx, db.SetCarModelHeroParams{
		ID: cur.ID, HeroObjectKey: pgtype.Text{String: key, Valid: key != ""},
	})
	if err != nil {
		return model.Model{}, "", mapDBError(err)
	}
	old := text(cur.HeroObjectKey)
	if old == key {
		old = ""
	}
	return modelView(row, b), old, nil
}

// mapDBError maps constraint violations like the product catalog
// (TEC-145): unique → 409 conflict; foreign key / restrict (PostgreSQL 18
// reports ON DELETE RESTRICT as 23001) → in use; check → validation.
func mapDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		switch pgErr.ConstraintName {
		case "uq_car_brands_external_id", "uq_car_models_external_id":
			return &ConflictError{Field: "external_id"}
		case "uq_car_brands_name", "uq_car_models_brand_name":
			return &ConflictError{Field: "name"}
		}
		return &ConflictError{Field: "record"}
	case "23503", "23001":
		return ErrInUse
	case "23514":
		return &ValidationError{Fields: []FieldError{{Field: pgErr.ConstraintName, Code: "invalid", Message: pgErr.Message}}}
	}
	return err
}
