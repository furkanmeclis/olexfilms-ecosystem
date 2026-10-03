package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// CatalogStep imports the hub product categories and products into the Olex
// catalog (brand olex, owned by the Olex center) and matches the warehouse
// products to them (TEC-256).
//
// A warehouse product of the Olex brand is matched to a catalog product by
// SKU, else by a unique name; a match maps the warehouse row to the same
// product, so the two legacy systems end up as one record. Unmatched Olex
// products are reported, not created. Glorian products are skipped (the
// Glorian profile is disabled, K2) and counted.
type CatalogStep struct {
	// System and WHSystem are the migration_map source systems of the hub
	// and warehouse rows; empty means SourceHub / SourceWH.
	System   string
	WHSystem string
}

// Name implements Step.
func (CatalogStep) Name() string { return "catalog" }

func (s CatalogStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

func (s CatalogStep) whSystem() string {
	if s.WHSystem == "" {
		return SourceWH
	}
	return s.WHSystem
}

const productCategoriesQuery = `SELECT id, name, available_parts, is_active, deleted_at, created_at, updated_at
FROM product_categories`

const productsQuery = `SELECT id, category_id, name, sku, COALESCE(description, ''), warranty_duration,
	micron_thickness, is_active, deleted_at, created_at, updated_at
FROM products`

const whProductsQuery = `SELECT CAST(p.id AS CHAR(36)), p.name, p.sku, COALESCE(b.name, ''), p.deleted_at, p.created_at, p.updated_at
FROM products p
LEFT JOIN brands b ON b.id = p.brand_id`

// Column limits of product_categories / products (000039).
const (
	categoryNameMax = 200
	productSKUMax   = 64
	productNameMax  = 200
)

type legacyCategory struct {
	ID                              int64
	Name                            string
	Parts                           []byte
	Active                          bool
	DeletedAt, CreatedAt, UpdatedAt sql.NullTime
}

type legacyProduct struct {
	ID, CategoryID                  int64
	Name, SKU, Description          string
	Warranty, Micron                sql.NullInt64
	Active                          bool
	DeletedAt, CreatedAt, UpdatedAt sql.NullTime
}

type whProduct struct {
	ID, Name, SKU, Brand            string
	DeletedAt, CreatedAt, UpdatedAt sql.NullTime
}

// catalogOwner is the brand and center organization catalog rows belong to.
type catalogOwner struct{ BrandID, CenterID int64 }

// Run implements Step.
func (s CatalogStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	wh, err := src.Get(SourceWH)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	center, err := dst.Q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		return StepResult{}, fmt.Errorf("olex center (run the organizations step first): %w", err)
	}
	owner := catalogOwner{BrandID: brand.ID, CenterID: center.ID}
	where, args := deltaFilter(dst)
	var watermark time.Time
	seen := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}

	cats, err := readCategories(ctx, hub, where, args)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	for _, cat := range cats {
		c.inc("categories_read")
		seen(cat.CreatedAt, cat.UpdatedAt, cat.DeletedAt)
		if err := s.importCategory(ctx, dst.Q, m, owner, cat, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("product category %d: %w", cat.ID, err)
		}
	}

	products, err := readProducts(ctx, hub, where, args)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	categoryIDs := map[int64]int64{}
	for _, p := range products {
		c.inc("products_read")
		seen(p.CreatedAt, p.UpdatedAt, p.DeletedAt)
		if err := s.importProduct(ctx, dst.Q, m, owner, categoryIDs, p, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("product %d: %w", p.ID, err)
		}
	}

	whWhere, _ := deltaFilterOn(dst, "p.")
	whRows, err := readWHProducts(ctx, wh, whWhere, args)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	for _, p := range whRows {
		c.inc("wh_products_read")
		seen(p.CreatedAt, p.UpdatedAt, p.DeletedAt)
		if err := s.matchWHProduct(ctx, dst.Q, m, owner, p, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warehouse product %s: %w", p.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readCategories(ctx context.Context, hub source.LegacySource, where string, args []any) ([]legacyCategory, error) {
	rows, err := hub.Query(ctx, productCategoriesQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyCategory
	for rows.Next() {
		var cat legacyCategory
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Parts, &cat.Active, &cat.DeletedAt, &cat.CreatedAt, &cat.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan product category: %w", err)
		}
		out = append(out, cat)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read product categories: %w", err)
	}
	return out, nil
}

func readProducts(ctx context.Context, hub source.LegacySource, where string, args []any) ([]legacyProduct, error) {
	rows, err := hub.Query(ctx, productsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyProduct
	for rows.Next() {
		var p legacyProduct
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.SKU, &p.Description, &p.Warranty, &p.Micron,
			&p.Active, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read products: %w", err)
	}
	return out, nil
}

func readWHProducts(ctx context.Context, wh source.LegacySource, where string, args []any) ([]whProduct, error) {
	rows, err := wh.Query(ctx, whProductsQuery+where+" ORDER BY p.id", args...)
	if err != nil {
		return nil, err
	}
	var out []whProduct
	for rows.Next() {
		var p whProduct
		if err := rows.Scan(&p.ID, &p.Name, &p.SKU, &p.Brand, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warehouse product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warehouse products: %w", err)
	}
	return out, nil
}

// availableParts keeps a legacy JSON array of part keys; anything else
// becomes an empty list.
func availableParts(raw []byte) ([]byte, bool) {
	var parts []string
	if err := json.Unmarshal(raw, &parts); err != nil {
		return []byte("[]"), len(strings.TrimSpace(string(raw))) == 0
	}
	if parts == nil {
		parts = []string{}
	}
	out, err := json.Marshal(parts)
	if err != nil {
		return []byte("[]"), false
	}
	return out, true
}

func (s CatalogStep) importCategory(ctx context.Context, q *db.Queries, m *Mapper, owner catalogOwner, cat legacyCategory, c counts) error {
	id := strconv.FormatInt(cat.ID, 10)
	name := truncate(strings.TrimSpace(cat.Name), categoryNameMax)
	if name == "" {
		c.inc("category_skipped_no_name:" + id)
		return nil
	}
	parts, ok := availableParts(cat.Parts)
	if !ok {
		c.inc("category_parts_invalid:" + id)
	}
	active := cat.Active && !cat.DeletedAt.Valid

	key := Key{System: s.system(), Table: "product_categories", ID: id, TargetTable: "product_categories"}
	sum := Checksum(cat.Name, string(cat.Parts), cat.Active, cat.DeletedAt.Valid)

	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		existing, err := q.MigratorFindCategory(ctx, db.MigratorFindCategoryParams{BrandID: owner.BrandID, Name: name})
		switch {
		case err == nil:
			linked, err := m.Link(ctx, key, existing, sum)
			if err != nil {
				return err
			}
			if linked {
				c.inc("categories_linked_existing")
				return nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find category: %w", err)
		}
	}

	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	catID, err := q.MigratorCategoryIDByUUID(ctx, db.MigratorCategoryIDByUUIDParams{Uuid: res.UUID, BrandID: owner.BrandID})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read category: %w", err)
	}
	if !exists {
		if _, err := q.MigratorInsertCategory(ctx, db.MigratorInsertCategoryParams{
			Uuid: res.UUID, OrganizationID: owner.CenterID, BrandID: owner.BrandID, Name: name,
			AvailableParts: parts, Active: active, CreatedAt: pgTime(cat.CreatedAt),
		}); err != nil {
			return fmt.Errorf("insert category: %w", err)
		}
		c.inc("categories_created")
		return nil
	}
	if !res.Changed {
		c.inc("categories_unchanged")
		return nil
	}
	if err := q.MigratorUpdateCategory(ctx, db.MigratorUpdateCategoryParams{
		ID: catID, BrandID: owner.BrandID, Name: name, AvailableParts: parts, Active: active,
	}); err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	c.inc("categories_updated")
	return nil
}

func (s CatalogStep) importProduct(ctx context.Context, q *db.Queries, m *Mapper, owner catalogOwner,
	categoryIDs map[int64]int64, p legacyProduct, c counts,
) error {
	id := strconv.FormatInt(p.ID, 10)
	sku := strings.TrimSpace(p.SKU)
	name := truncate(strings.TrimSpace(p.Name), productNameMax)
	if sku == "" || name == "" {
		c.inc("product_skipped_no_sku_or_name:" + id)
		return nil
	}
	if len([]rune(sku)) > productSKUMax {
		// A cut SKU could collide with another product: report, do not guess.
		c.inc("product_skipped_sku_too_long:" + id)
		return nil
	}
	categoryID, ok := categoryIDs[p.CategoryID]
	if !ok {
		target, found, err := m.Lookup(ctx, s.system(), "product_categories", strconv.FormatInt(p.CategoryID, 10))
		if err != nil {
			return err
		}
		if !found {
			c.inc("product_skipped_category_unmapped:" + id)
			return nil
		}
		categoryID, err = q.MigratorCategoryIDByUUID(ctx, db.MigratorCategoryIDByUUIDParams{Uuid: target, BrandID: owner.BrandID})
		if err != nil {
			return fmt.Errorf("read category: %w", err)
		}
		categoryIDs[p.CategoryID] = categoryID
	}
	warranty := pgtype.Int4{}
	if p.Warranty.Valid && p.Warranty.Int64 >= 0 && p.Warranty.Int64 <= 1200 {
		warranty = pgtype.Int4{Int32: int32(p.Warranty.Int64), Valid: true}
	}
	micron := pgtype.Numeric{}
	if p.Micron.Valid && p.Micron.Int64 > 0 && p.Micron.Int64 < 1_000_000 {
		if err := micron.Scan(strconv.FormatInt(p.Micron.Int64, 10)); err != nil {
			return fmt.Errorf("micron: %w", err)
		}
	}
	active := p.Active && !p.DeletedAt.Valid

	key := Key{System: s.system(), Table: "products", ID: id, TargetTable: "products"}
	sum := Checksum(p.CategoryID, p.Name, p.SKU, p.Description, p.Warranty.Int64, p.Warranty.Valid,
		p.Micron.Int64, p.Micron.Valid, p.Active, p.DeletedAt.Valid)

	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		existing, err := q.MigratorFindProductBySKU(ctx, db.MigratorFindProductBySKUParams{BrandID: owner.BrandID, Sku: sku})
		switch {
		case err == nil:
			linked, err := m.Link(ctx, key, existing, sum)
			if err != nil {
				return err
			}
			if linked {
				c.inc("products_linked_existing")
				return nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find product: %w", err)
		}
	}

	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	productID, err := q.MigratorProductIDByUUID(ctx, db.MigratorProductIDByUUIDParams{Uuid: res.UUID, BrandID: owner.BrandID})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read product: %w", err)
	}
	if !exists {
		if _, err := q.MigratorInsertProduct(ctx, db.MigratorInsertProductParams{
			Uuid: res.UUID, OrganizationID: owner.CenterID, BrandID: owner.BrandID, CategoryID: categoryID,
			Sku: sku, Name: name, DescriptionMd: p.Description, WarrantyDurationMonths: warranty,
			MicronThickness: micron, Active: active, CreatedAt: pgTime(p.CreatedAt),
		}); err != nil {
			return fmt.Errorf("insert product: %w", err)
		}
		c.inc("products_created")
		return nil
	}
	if !res.Changed {
		c.inc("products_unchanged")
		return nil
	}
	if err := q.MigratorUpdateProduct(ctx, db.MigratorUpdateProductParams{
		ID: productID, BrandID: owner.BrandID, CategoryID: categoryID, Sku: sku, Name: name,
		DescriptionMd: p.Description, WarrantyDurationMonths: warranty, MicronThickness: micron, Active: active,
	}); err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	c.inc("products_updated")
	return nil
}

// whBrand classifies a warehouse brand name: "olex", "glorian" or "" for
// one the migrator does not know. A product without a brand is treated as
// Olex (the warehouse only held the two brands).
func whBrand(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return OlexBrandSlug
	case strings.Contains(n, "glorian"):
		return "glorian"
	case strings.Contains(n, "olex"):
		return OlexBrandSlug
	}
	return ""
}

func (s CatalogStep) matchWHProduct(ctx context.Context, q *db.Queries, m *Mapper, owner catalogOwner, p whProduct, c counts) error {
	switch whBrand(p.Brand) {
	case "glorian":
		c.inc("wh_glorian_skipped")
		return nil
	case "":
		c.inc("wh_brand_unknown")
		c.inc("wh_brand_unknown:" + strings.TrimSpace(p.Brand))
		c.inc("wh_product_skipped_brand_unknown:" + p.ID)
		return nil
	}
	key := Key{System: s.whSystem(), Table: "products", ID: p.ID, TargetTable: "products"}
	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if mapped {
		c.inc("wh_unchanged")
		return nil
	}

	sku := strings.TrimSpace(p.SKU)
	target, err := q.MigratorFindProductBySKU(ctx, db.MigratorFindProductBySKUParams{BrandID: owner.BrandID, Sku: sku})
	how := "wh_matched_sku"
	if errors.Is(err, pgx.ErrNoRows) || sku == "" {
		err = nil
		how = ""
		byName, nerr := q.MigratorFindProductsByName(ctx, db.MigratorFindProductsByNameParams{BrandID: owner.BrandID, Name: p.Name})
		if nerr != nil {
			return fmt.Errorf("find product by name: %w", nerr)
		}
		if len(byName) == 1 {
			target, how = byName[0], "wh_matched_name"
		}
	}
	if err != nil {
		return fmt.Errorf("find product by sku: %w", err)
	}
	if how == "" {
		c.inc("wh_unmatched")
		c.inc("wh_unmatched:" + sku)
		c.inc("wh_product_skipped_unmatched:" + p.ID)
		return nil
	}
	linked, err := m.Link(ctx, key, target, Checksum(p.SKU, p.Name, p.Brand))
	if err != nil {
		return err
	}
	if !linked {
		c.inc("wh_unchanged")
		return nil
	}
	c.inc(how)
	return nil
}
