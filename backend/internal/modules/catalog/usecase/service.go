// Package usecase implements the product catalog of TEC-145: categories and
// products of one brand. Every read and write is filtered by the brand of the
// active organization (K1/K20); only the center organization of the brand
// writes (K4), distributors and dealers read.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Meilisearch spec id of products.
const SearchSpec = "products"

// OrgTypeCenter is the only organization type that writes the catalog (K4).
const OrgTypeCenter = "center"

var (
	// ErrNotFound: no such record in the active brand.
	ErrNotFound = errors.New("catalog: not found")
	// ErrCenterOnly: the active organization is not the brand center (K4).
	ErrCenterOnly = errors.New("catalog: only the center organization writes the catalog")
	// ErrInUse: the record is still referenced (category with products).
	ErrInUse = errors.New("catalog: record is still in use")
)

// LockedError: the write touches fields a remote hub owns (TEC-268, Glorian
// catalog pull, K2). Fields are the API field names. HTTP 409 CONFLICT with
// one "locked" detail per field.
type LockedError struct{ Fields []string }

func (e *LockedError) Error() string {
	return "catalog: fields are managed by the integration sync: " + strings.Join(e.Fields, ", ")
}

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
		return "catalog: invalid input"
	}
	return "catalog: invalid " + e.Fields[0].Field + ": " + e.Fields[0].Message
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

// ConflictError is a unique violation (duplicate SKU or category name).
type ConflictError struct{ Field string }

func (e *ConflictError) Error() string { return "catalog: duplicate " + e.Field }

// Store is the subset of db.Queries the catalog uses.
type Store interface {
	CreateProductCategory(ctx context.Context, arg db.CreateProductCategoryParams) (db.ProductCategory, error)
	GetProductCategory(ctx context.Context, arg db.GetProductCategoryParams) (db.ProductCategory, error)
	GetProductCategoryByUUID(ctx context.Context, arg db.GetProductCategoryByUUIDParams) (db.ProductCategory, error)
	GetProductCategoryByName(ctx context.Context, arg db.GetProductCategoryByNameParams) (db.ProductCategory, error)
	UpdateProductCategory(ctx context.Context, arg db.UpdateProductCategoryParams) (db.ProductCategory, error)
	DeleteProductCategory(ctx context.Context, arg db.DeleteProductCategoryParams) (int64, error)
	ListProductCategories(ctx context.Context, arg db.ListProductCategoriesParams) ([]db.ProductCategory, error)
	CountProductCategories(ctx context.Context, arg db.CountProductCategoriesParams) (int64, error)
	// TEC-369: category display order.
	ListProductCategoryOrder(ctx context.Context, brandID int64) ([]db.ListProductCategoryOrderRow, error)
	SetProductCategorySorts(ctx context.Context, arg db.SetProductCategorySortsParams) (int64, error)

	CreateProduct(ctx context.Context, arg db.CreateProductParams) (db.Product, error)
	GetProductByUUID(ctx context.Context, arg db.GetProductByUUIDParams) (db.Product, error)
	GetProductBySKU(ctx context.Context, arg db.GetProductBySKUParams) (db.Product, error)
	UpdateProduct(ctx context.Context, arg db.UpdateProductParams) (db.Product, error)
	DeleteProduct(ctx context.Context, arg db.DeleteProductParams) (int64, error)
	ListProducts(ctx context.Context, arg db.ListProductsParams) ([]db.Product, error)
	CountProducts(ctx context.Context, arg db.CountProductsParams) (int64, error)
	SetProductsActiveByUUIDs(ctx context.Context, arg db.SetProductsActiveByUUIDsParams) ([]uuid.UUID, error)
	// TEC-268: a brand with an integration connection takes its categories
	// from the hub.
	BrandHasIntegrationConnection(ctx context.Context, brandID int64) (bool, error)

	AppendProductImage(ctx context.Context, arg db.AppendProductImageParams) (db.Product, error)
	ReplaceProductImages(ctx context.Context, arg db.ReplaceProductImagesParams) (db.Product, error)
	GetActiveProductUUIDByImageKey(ctx context.Context, key string) (uuid.UUID, error)
}

// Indexer receives search index mutations (searchengine.Indexer).
type Indexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service is the catalog use case.
type Service struct {
	store   Store
	indexer Indexer
}

// New creates the service. indexer may be nil.
func New(store Store, indexer Indexer) *Service {
	return &Service{store: store, indexer: indexer}
}

// requireCenter enforces K4 on every write, on top of catalog.write.
func requireCenter(org orgctx.Scope) error {
	if org.OrgType != OrgTypeCenter || org.BrandID == 0 {
		return ErrCenterOnly
	}
	return nil
}

// --- Categories -------------------------------------------------------------

// ListCategories lists the categories of the active brand.
func (s *Service) ListCategories(ctx context.Context, org orgctx.Scope, f model.CategoryFilter) ([]model.Category, int64, error) {
	active := boolArg(f.Active)
	q := textArg(f.Q)
	if f.Sort.Key == "" {
		f.Sort = apiquery.ResolvedSort{Key: model.CategorySort.Default.Field}
	}
	rows, err := s.store.ListProductCategories(ctx, db.ListProductCategoriesParams{
		BrandID: org.BrandID, Active: active, Q: q, SortKey: f.Sort.Key, SortDesc: f.Sort.Desc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountProductCategories(ctx, db.CountProductCategoriesParams{
		BrandID: org.BrandID, Active: active, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Category, 0, len(rows))
	for _, r := range rows {
		out = append(out, categoryView(r))
	}
	return out, total, nil
}

// GetCategory returns one category of the active brand.
func (s *Service) GetCategory(ctx context.Context, org orgctx.Scope, id uuid.UUID) (model.Category, error) {
	row, err := s.category(ctx, org, id)
	if err != nil {
		return model.Category{}, err
	}
	return categoryView(row), nil
}

// CreateCategory adds a category (center only).
func (s *Service) CreateCategory(ctx context.Context, org orgctx.Scope, in model.CategoryInput) (model.Category, error) {
	if err := requireCenter(org); err != nil {
		return model.Category{}, err
	}
	var name string
	parts := []string{}
	var sort int32
	active := true
	if in.Name != nil {
		name = *in.Name
	}
	if in.AvailableParts != nil {
		parts = *in.AvailableParts
	}
	if in.Sort != nil {
		sort = *in.Sort
	}
	if in.Active != nil {
		active = *in.Active
	}
	name, parts, err := validateCategory(name, parts)
	if err != nil {
		return model.Category{}, err
	}
	partsJSON, _ := json.Marshal(parts)
	row, err := s.store.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: org.InternalID, BrandID: org.BrandID, Name: name,
		AvailableParts: partsJSON, Sort: sort, Active: active,
	})
	if err != nil {
		return model.Category{}, mapDBError(err)
	}
	return categoryView(row), nil
}

// UpdateCategory patches a category (center only).
func (s *Service) UpdateCategory(ctx context.Context, org orgctx.Scope, id uuid.UUID, in model.CategoryInput) (model.Category, error) {
	if err := requireCenter(org); err != nil {
		return model.Category{}, err
	}
	cur, err := s.category(ctx, org, id)
	if err != nil {
		return model.Category{}, err
	}
	name := cur.Name
	parts := decodeParts(cur.AvailableParts)
	sort, active := cur.Sort, cur.Active
	if in.Name != nil {
		name = *in.Name
	}
	if in.AvailableParts != nil {
		parts = *in.AvailableParts
	}
	if in.Sort != nil {
		sort = *in.Sort
	}
	if in.Active != nil {
		active = *in.Active
	}
	name, parts, err = validateCategory(name, parts)
	if err != nil {
		return model.Category{}, err
	}
	var changed []string
	if name != cur.Name {
		changed = append(changed, model.FieldName)
	}
	if !slices.Equal(parts, decodeParts(cur.AvailableParts)) {
		changed = append(changed, model.FieldAvailableParts)
	}
	if active != cur.Active {
		changed = append(changed, model.FieldActive)
	}
	if err := s.guardSyncedCategory(ctx, org.BrandID, changed); err != nil {
		return model.Category{}, err
	}
	partsJSON, _ := json.Marshal(parts)
	row, err := s.store.UpdateProductCategory(ctx, db.UpdateProductCategoryParams{
		ID: cur.ID, BrandID: org.BrandID, Name: name, AvailableParts: partsJSON, Sort: sort, Active: active,
	})
	if err != nil {
		return model.Category{}, mapDBError(err)
	}
	return categoryView(row), nil
}

// DeleteCategory removes a category without products (center only).
func (s *Service) DeleteCategory(ctx context.Context, org orgctx.Scope, id uuid.UUID) error {
	if err := requireCenter(org); err != nil {
		return err
	}
	cur, err := s.category(ctx, org, id)
	if err != nil {
		return err
	}
	if err := s.guardSyncedCategory(ctx, org.BrandID, model.SyncedCategoryLockedFields); err != nil {
		return err
	}
	n, err := s.store.DeleteProductCategory(ctx, db.DeleteProductCategoryParams{ID: cur.ID, BrandID: org.BrandID})
	if err != nil {
		return mapDBError(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// guardSyncedCategory refuses a change of the remote-sourced category fields
// in a brand whose categories come from an integration connection (TEC-268).
func (s *Service) guardSyncedCategory(ctx context.Context, brandID int64, changed []string) error {
	if len(changed) == 0 {
		return nil
	}
	synced, err := s.store.BrandHasIntegrationConnection(ctx, brandID)
	if err != nil {
		return err
	}
	if synced {
		return &LockedError{Fields: changed}
	}
	return nil
}

func (s *Service) category(ctx context.Context, org orgctx.Scope, id uuid.UUID) (db.ProductCategory, error) {
	row, err := s.store.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: id, BrandID: org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ProductCategory{}, ErrNotFound
	}
	return row, err
}

// --- Products ---------------------------------------------------------------

// ListProducts lists the products of the active brand.
func (s *Service) ListProducts(ctx context.Context, org orgctx.Scope, f model.ProductFilter) ([]model.Product, int64, error) {
	categoryIDs, ok, err := s.categoryIDs(ctx, org, f.CategoryUUIDs)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return []model.Product{}, 0, nil
	}
	if f.Sort.Key == "" {
		f.Sort = apiquery.ResolvedSort{Key: model.ProductSort.Default.Field, Desc: model.ProductSort.Default.Desc}
	}
	params := productListParams(org.BrandID, categoryIDs, f)
	rows, err := s.store.ListProducts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountProducts(ctx, productCountParams(params))
	if err != nil {
		return nil, 0, err
	}
	out, err := s.productViews(ctx, org.BrandID, rows)
	return out, total, err
}

// GetProduct returns one product of the active brand.
func (s *Service) GetProduct(ctx context.Context, org orgctx.Scope, id uuid.UUID) (model.Product, error) {
	row, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return model.Product{}, err
	}
	return s.productView(ctx, row)
}

// CreateProduct adds a product (center only).
func (s *Service) CreateProduct(ctx context.Context, org orgctx.Scope, in model.ProductInput) (model.Product, error) {
	if err := requireCenter(org); err != nil {
		return model.Product{}, err
	}
	if err := checkUploadedKeys(nil, in.Images); err != nil {
		return model.Product{}, err
	}
	p := productFields{UnitType: model.UnitPiece, Active: true, Images: []model.Image{}}
	p.apply(in)
	cat, err := s.validateProduct(ctx, org.BrandID, &p, in.CategoryUUID, true)
	if err != nil {
		return model.Product{}, err
	}
	images, _ := json.Marshal(p.Images)
	row, err := s.store.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: org.InternalID, BrandID: org.BrandID, CategoryID: cat.ID,
		Sku: p.SKU, Name: p.Name, DescriptionMd: p.DescriptionMD,
		WarrantyDurationMonths: int4Arg(p.Warranty), MicronThickness: numericArg(p.Micron),
		Images: images, UnitType: p.UnitType, UsesFixedBarcode: p.UsesFixedBarcode, Active: p.Active,
	})
	if err != nil {
		return model.Product{}, mapDBError(err)
	}
	s.reindex(ctx, row.Uuid)
	return productView(row, cat), nil
}

// UpdateProduct patches a product (center only).
func (s *Service) UpdateProduct(ctx context.Context, org orgctx.Scope, id uuid.UUID, in model.ProductInput) (model.Product, error) {
	if err := requireCenter(org); err != nil {
		return model.Product{}, err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return model.Product{}, err
	}
	if err := checkUploadedKeys(decodeImages(cur.Images), in.Images); err != nil {
		return model.Product{}, err
	}
	p := fieldsOf(cur)
	p.apply(in)
	cat, err := s.validateProduct(ctx, org.BrandID, &p, in.CategoryUUID, false)
	if err != nil {
		return model.Product{}, err
	}
	if in.CategoryUUID == nil {
		cat, err = s.store.GetProductCategory(ctx, db.GetProductCategoryParams{ID: cur.CategoryID, BrandID: org.BrandID})
		if err != nil {
			return model.Product{}, err
		}
	}
	if locked := lockedChanges(cur, p, cat.ID); len(locked) > 0 {
		return model.Product{}, &LockedError{Fields: locked}
	}
	row, err := s.store.UpdateProduct(ctx, p.updateParams(cur.ID, org.BrandID, cat.ID))
	if err != nil {
		return model.Product{}, mapDBError(err)
	}
	s.reindex(ctx, row.Uuid)
	return productView(row, cat), nil
}

// DeleteProduct removes a product (center only). Prices cascade (000040).
func (s *Service) DeleteProduct(ctx context.Context, org orgctx.Scope, id uuid.UUID) error {
	if err := requireCenter(org); err != nil {
		return err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return err
	}
	if len(cur.LockedFields) > 0 {
		// A synced product is removed on the hub, not here (TEC-268).
		return &LockedError{Fields: slices.Clone(cur.LockedFields)}
	}
	n, err := s.store.DeleteProduct(ctx, db.DeleteProductParams{ID: cur.ID, BrandID: org.BrandID})
	if err != nil {
		return mapDBError(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, cur.Uuid.String())
	}
	return nil
}

// MaxBulk caps one bulk activate/deactivate request.
const MaxBulk = 500

// SetProductsActive activates or deactivates products of the active brand
// (center only). UUIDs of other brands are ignored.
func (s *Service) SetProductsActive(ctx context.Context, org orgctx.Scope, ids []uuid.UUID, active bool) (model.BulkActiveResult, error) {
	if err := requireCenter(org); err != nil {
		return model.BulkActiveResult{}, err
	}
	uniq := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	verr := &ValidationError{}
	switch {
	case len(uniq) == 0:
		verr.add("uuids", "required", "at least one product uuid is required")
	case len(uniq) > MaxBulk:
		verr.add("uuids", "too_many", fmt.Sprintf("at most %d products per request", MaxBulk))
	}
	if err := verr.orNil(); err != nil {
		return model.BulkActiveResult{}, err
	}
	changed, err := s.store.SetProductsActiveByUUIDs(ctx, db.SetProductsActiveByUUIDsParams{
		Active: active, BrandID: org.BrandID, Uuids: uniq,
	})
	if err != nil {
		return model.BulkActiveResult{}, err
	}
	for _, id := range changed {
		s.reindex(ctx, id)
	}
	if changed == nil {
		changed = []uuid.UUID{}
	}
	return model.BulkActiveResult{Requested: len(uniq), Updated: len(changed), UUIDs: changed}, nil
}

func (s *Service) product(ctx context.Context, brandID int64, id uuid.UUID) (db.Product, error) {
	row, err := s.store.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Product{}, ErrNotFound
	}
	return row, err
}

func (s *Service) reindex(ctx context.Context, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, id.String())
	}
}

func (s *Service) productView(ctx context.Context, row db.Product) (model.Product, error) {
	cat, err := s.store.GetProductCategory(ctx, db.GetProductCategoryParams{ID: row.CategoryID, BrandID: row.BrandID})
	if err != nil {
		return model.Product{}, err
	}
	return productView(row, cat), nil
}

func (s *Service) productViews(ctx context.Context, brandID int64, rows []db.Product) ([]model.Product, error) {
	cats := map[int64]db.ProductCategory{}
	out := make([]model.Product, 0, len(rows))
	for _, r := range rows {
		cat, ok := cats[r.CategoryID]
		if !ok {
			c, err := s.store.GetProductCategory(ctx, db.GetProductCategoryParams{ID: r.CategoryID, BrandID: brandID})
			if err != nil {
				return nil, err
			}
			cats[r.CategoryID], cat = c, c
		}
		out = append(out, productView(r, cat))
	}
	return out, nil
}

// --- Validation -------------------------------------------------------------

const (
	maxNameLen        = 200
	maxSKULen         = 64
	maxDescriptionLen = 20000
	maxPartLen        = 100
	maxParts          = 100
	maxImageKeyLen    = 512
	maxWarrantyMonths = 600
	// NUMERIC(8,2)
	maxMicron = 999999.99
)

func validateCategory(name string, parts []string) (string, []string, error) {
	verr := &ValidationError{}
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		verr.add("name", "required", "name is required")
	case utf8.RuneCountInString(name) > maxNameLen:
		verr.add("name", "too_long", fmt.Sprintf("name is longer than %d characters", maxNameLen))
	}
	clean := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if utf8.RuneCountInString(p) > maxPartLen {
			verr.add("available_parts", "too_long", fmt.Sprintf("a part is longer than %d characters", maxPartLen))
			break
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		clean = append(clean, p)
	}
	if len(clean) > maxParts {
		verr.add("available_parts", "too_many", fmt.Sprintf("at most %d parts", maxParts))
	}
	return name, clean, verr.orNil()
}

// productFields is the editable state of a product.
type productFields struct {
	SKU, Name, DescriptionMD string
	Warranty                 *int32
	Micron                   *float64
	Images                   []model.Image
	UnitType                 string
	UsesFixedBarcode, Active bool
}

func fieldsOf(row db.Product) productFields {
	return productFields{
		SKU: row.Sku, Name: row.Name, DescriptionMD: row.DescriptionMd,
		Warranty: int4Ptr(row.WarrantyDurationMonths), Micron: numericPtr(row.MicronThickness),
		Images: decodeImages(row.Images), UnitType: row.UnitType,
		UsesFixedBarcode: row.UsesFixedBarcode, Active: row.Active,
	}
}

func (p *productFields) apply(in model.ProductInput) {
	if in.SKU != nil {
		p.SKU = *in.SKU
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.DescriptionMD != nil {
		p.DescriptionMD = *in.DescriptionMD
	}
	if in.WarrantySet || in.WarrantyDurationMonths != nil {
		p.Warranty = in.WarrantyDurationMonths
	}
	if in.MicronSet || in.MicronThickness != nil {
		p.Micron = in.MicronThickness
	}
	if in.Images != nil {
		p.Images = *in.Images
	}
	if in.UnitType != nil {
		p.UnitType = *in.UnitType
	}
	if in.UsesFixedBarcode != nil {
		p.UsesFixedBarcode = *in.UsesFixedBarcode
	}
	if in.Active != nil {
		p.Active = *in.Active
	}
}

func (p productFields) updateParams(id, brandID, categoryID int64) db.UpdateProductParams {
	images, _ := json.Marshal(p.Images)
	return db.UpdateProductParams{
		ID: id, BrandID: brandID, CategoryID: categoryID,
		Sku: p.SKU, Name: p.Name, DescriptionMd: p.DescriptionMD,
		WarrantyDurationMonths: int4Arg(p.Warranty), MicronThickness: numericArg(p.Micron),
		Images: images, UnitType: p.UnitType, UsesFixedBarcode: p.UsesFixedBarcode, Active: p.Active,
	}
}

// lockedChanges lists the locked fields of cur that next (normalized) would
// change; categoryID is the category next points at (TEC-268).
func lockedChanges(cur db.Product, next productFields, categoryID int64) []string {
	if len(cur.LockedFields) == 0 {
		return nil
	}
	prev := fieldsOf(cur)
	changed := map[string]bool{
		model.FieldCategoryUUID:           categoryID != cur.CategoryID,
		model.FieldSKU:                    next.SKU != prev.SKU,
		model.FieldName:                   next.Name != prev.Name,
		model.FieldDescriptionMD:          next.DescriptionMD != prev.DescriptionMD,
		model.FieldWarrantyDurationMonths: !ptrEqual(next.Warranty, prev.Warranty),
		model.FieldMicronThickness:        !ptrEqual(next.Micron, prev.Micron),
		model.FieldActive:                 next.Active != prev.Active,
	}
	var out []string
	for _, f := range cur.LockedFields {
		if changed[f] {
			out = append(out, f)
		}
	}
	return out
}

func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// validateProduct normalizes p in place and resolves the category when
// categoryUUID is given (requireCategory reports a missing one).
func (s *Service) validateProduct(ctx context.Context, brandID int64, p *productFields, categoryUUID *uuid.UUID, requireCategory bool) (db.ProductCategory, error) {
	verr := &ValidationError{}
	p.SKU = strings.TrimSpace(p.SKU)
	p.Name = strings.TrimSpace(p.Name)
	p.UnitType = strings.TrimSpace(p.UnitType)
	switch {
	case p.SKU == "":
		verr.add("sku", "required", "sku is required")
	case utf8.RuneCountInString(p.SKU) > maxSKULen:
		verr.add("sku", "too_long", fmt.Sprintf("sku is longer than %d characters", maxSKULen))
	}
	switch {
	case p.Name == "":
		verr.add("name", "required", "name is required")
	case utf8.RuneCountInString(p.Name) > maxNameLen:
		verr.add("name", "too_long", fmt.Sprintf("name is longer than %d characters", maxNameLen))
	}
	if utf8.RuneCountInString(p.DescriptionMD) > maxDescriptionLen {
		verr.add("description_md", "too_long", fmt.Sprintf("description is longer than %d characters", maxDescriptionLen))
	}
	if p.UnitType != model.UnitPiece && p.UnitType != model.UnitRollMeter {
		verr.add("unit_type", "invalid", "unit_type must be piece or roll_meter")
	}
	if p.Warranty != nil && (*p.Warranty < 0 || *p.Warranty > maxWarrantyMonths) {
		verr.add("warranty_duration_months", "out_of_range", fmt.Sprintf("warranty must be between 0 and %d months", maxWarrantyMonths))
	}
	if p.Micron != nil && (*p.Micron <= 0 || *p.Micron > maxMicron || math.IsNaN(*p.Micron)) {
		verr.add("micron_thickness", "out_of_range", "micron_thickness must be greater than 0")
	}
	if len(p.Images) > MaxProductImages {
		verr.add("images", "too_many", fmt.Sprintf("at most %d images", MaxProductImages))
	}
	for i := range p.Images {
		p.Images[i].Key = strings.TrimSpace(p.Images[i].Key)
		if p.Images[i].Key == "" || len(p.Images[i].Key) > maxImageKeyLen {
			verr.add("images", "invalid", "every image needs a storage key")
			break
		}
	}
	if p.Images == nil {
		p.Images = []model.Image{}
	}
	var cat db.ProductCategory
	if categoryUUID == nil {
		if requireCategory {
			verr.add("category_uuid", "required", "category_uuid is required")
		}
	} else {
		c, err := s.store.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: *categoryUUID, BrandID: brandID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			verr.add("category_uuid", "not_found", "category does not exist in this brand")
		case err != nil:
			return db.ProductCategory{}, err
		default:
			cat = c
		}
	}
	return cat, verr.orNil()
}

// --- Mapping ----------------------------------------------------------------

func categoryView(r db.ProductCategory) model.Category {
	return model.Category{
		UUID: r.Uuid, Name: r.Name, AvailableParts: decodeParts(r.AvailableParts),
		Sort: r.Sort, Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func productView(r db.Product, cat db.ProductCategory) model.Product {
	var ext *string
	if r.ExternalID.Valid {
		v := r.ExternalID.String
		ext = &v
	}
	locked := r.LockedFields
	if locked == nil {
		locked = []string{}
	}
	return model.Product{
		UUID: r.Uuid, Category: model.CategoryRef{UUID: cat.Uuid, Name: cat.Name},
		SKU: r.Sku, Name: r.Name, DescriptionMD: r.DescriptionMd,
		WarrantyDurationMonths: int4Ptr(r.WarrantyDurationMonths), MicronThickness: numericPtr(r.MicronThickness),
		Images: decodeImages(r.Images), UnitType: r.UnitType, UsesFixedBarcode: r.UsesFixedBarcode,
		Active: r.Active, ExternalID: ext, LockedFields: locked,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func decodeParts(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func decodeImages(raw []byte) []model.Image {
	out := []model.Image{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = []model.Image{}
	}
	return out
}

func mapDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		switch pgErr.ConstraintName {
		case "uq_products_brand_sku":
			return &ConflictError{Field: "sku"}
		case "uq_product_categories_brand_name":
			return &ConflictError{Field: "name"}
		}
		return &ConflictError{Field: "record"}
	case "23503", "23001":
		// foreign_key_violation; PostgreSQL 18 reports ON DELETE RESTRICT
		// as restrict_violation (23001).
		return ErrInUse
	case "23514":
		// catalog_check_center_org trigger (K4) or a CHECK constraint.
		if strings.Contains(pgErr.Message, "center organization") {
			return ErrCenterOnly
		}
		return &ValidationError{Fields: []FieldError{{Field: pgErr.ConstraintName, Code: "invalid", Message: pgErr.Message}}}
	}
	return err
}

func boolArg(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func textArg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func int4Arg(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func numericArg(v *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if v == nil {
		return n
	}
	if err := n.Scan(strconv.FormatFloat(*v, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func numericPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}
