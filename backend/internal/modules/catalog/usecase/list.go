package usecase

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// List contract of the catalog (TEC-369, docs/list-contract.md). The list
// endpoint and the products export read the same parameters with these
// parsers, so both select the same rows.

// ParseCategoryFilter reads q, active and sort of the category list. Errors
// are *apiquery.ValidationError (400).
func ParseCategoryFilter(values url.Values) (model.CategoryFilter, error) {
	q := apiquery.Parse(values)
	f := model.CategoryFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		return f, err
	}
	if f.Sort, err = apiquery.ResolveSort(q.Sort, model.CategorySort); err != nil {
		return f, err
	}
	return f, nil
}

// ParseProductFilter reads the product list parameters: q, sort,
// category_uuid (CSV), active, unit_type (CSV), uses_fixed_barcode,
// warranty_duration_months_min/_max, micron_thickness_min/_max and
// created_from/_to. Errors are *apiquery.ValidationError (400).
func ParseProductFilter(values url.Values) (model.ProductFilter, error) {
	q := apiquery.Parse(values)
	f := model.ProductFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		return f, err
	}
	if f.UsesFixedBarcode, err = apiquery.Bool(values, "uses_fixed_barcode"); err != nil {
		return f, err
	}
	if f.UnitTypes, err = apiquery.EnumList(values, "unit_type", model.UnitPiece, model.UnitRollMeter); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, "category_uuid") {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "category_uuid", Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		f.CategoryUUIDs = append(f.CategoryUUIDs, id)
	}
	if f.Warranty, err = apiquery.NumRange(values, "warranty_duration_months"); err != nil {
		return f, err
	}
	if f.Micron, err = apiquery.NumRange(values, "micron_thickness"); err != nil {
		return f, err
	}
	if f.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return f, err
	}
	if f.Sort, err = apiquery.ResolveSort(q.Sort, model.ProductSort); err != nil {
		return f, err
	}
	return f, nil
}

// categoryIDs resolves the category filter to row ids of the brand. ok is
// false when every named category is unknown (the list is then empty).
func (s *Service) categoryIDs(ctx context.Context, org orgctx.Scope, ids []uuid.UUID) ([]int64, bool, error) {
	if len(ids) == 0 {
		return nil, true, nil
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		cat, err := s.category(ctx, org, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		out = append(out, cat.ID)
	}
	return out, len(out) > 0, nil
}

func float8Arg(v *float64) pgtype.Float8 {
	if v == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *v, Valid: true}
}

func timeArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func productListParams(brandID int64, categoryIDs []int64, f model.ProductFilter) db.ListProductsParams {
	return db.ListProductsParams{
		BrandID: brandID, CategoryIds: categoryIDs, Active: boolArg(f.Active), UnitTypes: f.UnitTypes,
		UsesFixedBarcode: boolArg(f.UsesFixedBarcode),
		WarrantyMin:      float8Arg(f.Warranty.Min), WarrantyMax: float8Arg(f.Warranty.Max),
		MicronMin: float8Arg(f.Micron.Min), MicronMax: float8Arg(f.Micron.Max),
		CreatedFrom: timeArg(f.Created.From), CreatedBefore: timeArg(f.Created.Before),
		Q:        textArg(f.Q),
		SortDesc: f.Sort.Desc, SortKey: f.Sort.Key,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
}

func productCountParams(p db.ListProductsParams) db.CountProductsParams {
	return db.CountProductsParams{
		BrandID: p.BrandID, CategoryIds: p.CategoryIds, Active: p.Active, UnitTypes: p.UnitTypes,
		UsesFixedBarcode: p.UsesFixedBarcode, WarrantyMin: p.WarrantyMin, WarrantyMax: p.WarrantyMax,
		MicronMin: p.MicronMin, MicronMax: p.MicronMax, CreatedFrom: p.CreatedFrom, CreatedBefore: p.CreatedBefore,
		Q: p.Q,
	}
}

// MaxCategoryOrder caps one reorder request; maxCategoryList bounds the
// full list returned after it.
const (
	MaxCategoryOrder = 1000
	maxCategoryList  = 10000
)

// ReorderCategories sets the display order (sort) of the brand categories
// from a drag-and-drop list (center only). The list may be a subset: those
// categories are rearranged within the positions they already hold, then
// every category is renumbered 10, 20, ... The full list returns in the new
// order. Sort is a local field, so a synced brand may reorder too.
func (s *Service) ReorderCategories(ctx context.Context, org orgctx.Scope, ids []uuid.UUID) ([]model.Category, error) {
	if err := requireCenter(org); err != nil {
		return nil, err
	}
	verr := &ValidationError{}
	seen := make(map[uuid.UUID]bool, len(ids))
	switch {
	case len(ids) == 0:
		verr.add("uuids", "required", "at least one category uuid is required")
	case len(ids) > MaxCategoryOrder:
		verr.add("uuids", "too_many", "too many categories")
	}
	for _, id := range ids {
		if id == uuid.Nil {
			verr.add("uuids", "invalid", "uuids must not be empty")
			break
		}
		if seen[id] {
			verr.add("uuids", "duplicate", "duplicate category "+id.String())
			break
		}
		seen[id] = true
	}
	if err := verr.orNil(); err != nil {
		return nil, err
	}
	rows, err := s.store.ListProductCategoryOrder(ctx, org.BrandID)
	if err != nil {
		return nil, err
	}
	byUUID := make(map[uuid.UUID]int64, len(rows))
	for _, r := range rows {
		byUUID[r.Uuid] = r.ID
	}
	for _, id := range ids {
		if _, ok := byUUID[id]; !ok {
			verr.add("uuids", "not_found", "category "+id.String()+" does not exist in this brand")
			return nil, verr
		}
	}
	// The named categories take the slots they occupy, in the given order.
	order := make([]int64, len(rows))
	next := 0
	for i, r := range rows {
		if seen[r.Uuid] {
			order[i] = byUUID[ids[next]]
			next++
			continue
		}
		order[i] = r.ID
	}
	if _, err := s.store.SetProductCategorySorts(ctx, db.SetProductCategorySortsParams{
		Ids: order, BrandID: org.BrandID,
	}); err != nil {
		return nil, mapDBError(err)
	}
	items, _, err := s.ListCategories(ctx, org, model.CategoryFilter{Limit: maxCategoryList})
	return items, err
}
