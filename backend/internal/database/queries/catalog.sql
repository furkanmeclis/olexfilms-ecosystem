-- TEC-144: product categories and products. Every query is brand-filtered;
-- callers pass the active brand (K1/K20). Writes are center-only (K4) and
-- are checked in the use case layer.

-- name: CreateProductCategory :one
INSERT INTO product_categories (organization_id, brand_id, name, available_parts, sort, active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name),
    sqlc.arg(available_parts), sqlc.arg(sort), sqlc.arg(active)
)
RETURNING *;

-- name: GetProductCategory :one
SELECT * FROM product_categories
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetProductCategoryByUUID :one
SELECT * FROM product_categories
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: UpdateProductCategory :one
-- Full replacement of the editable fields (read-modify-write in the use case).
UPDATE product_categories
SET name = sqlc.arg(name),
    available_parts = sqlc.arg(available_parts),
    sort = sqlc.arg(sort),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeleteProductCategory :execrows
-- Fails with a foreign key violation while products still use the category.
DELETE FROM product_categories
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListProductCategories :many
-- Sort keys from model.CategorySort (docs/list-contract.md); default sort.
SELECT * FROM product_categories
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN name::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN name::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort' THEN sort END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort' THEN sort END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'active' THEN active END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'active' THEN active END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  name ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountProductCategories :one
SELECT COUNT(*)::bigint FROM product_categories
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: CreateProduct :one
INSERT INTO products (
    organization_id, brand_id, category_id, sku, name, description_md,
    warranty_duration_months, micron_thickness, images, unit_type,
    uses_fixed_barcode, active, external_id, connection_id, locked_fields
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(category_id), sqlc.arg(sku),
    sqlc.arg(name), sqlc.arg(description_md), sqlc.narg(warranty_duration_months),
    sqlc.narg(micron_thickness), sqlc.arg(images), sqlc.arg(unit_type),
    sqlc.arg(uses_fixed_barcode), sqlc.arg(active), sqlc.narg(external_id),
    sqlc.narg(connection_id), sqlc.narg(locked_fields)
)
RETURNING *;

-- name: GetProduct :one
SELECT * FROM products
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetProductByUUID :one
SELECT * FROM products
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetProductBySKU :one
SELECT * FROM products
WHERE brand_id = sqlc.arg(brand_id) AND sku = sqlc.arg(sku);

-- name: UpdateProduct :one
-- Full replacement of the editable fields (read-modify-write in the use case).
-- The F2 sync columns are not touched here.
UPDATE products
SET category_id = sqlc.arg(category_id),
    sku = sqlc.arg(sku),
    name = sqlc.arg(name),
    description_md = sqlc.arg(description_md),
    warranty_duration_months = sqlc.narg(warranty_duration_months),
    micron_thickness = sqlc.narg(micron_thickness),
    images = sqlc.arg(images),
    unit_type = sqlc.arg(unit_type),
    uses_fixed_barcode = sqlc.arg(uses_fixed_barcode),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: SetProductsActive :execrows
-- Bulk activate/deactivate within one brand.
UPDATE products
SET active = sqlc.arg(active)
WHERE brand_id = sqlc.arg(brand_id) AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: DeleteProduct :execrows
DELETE FROM products
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: ListProducts :many
-- TEC-369: sort keys from model.ProductSort (docs/list-contract.md);
-- default name. category sorts by the category name.
SELECT p.* FROM products p
WHERE p.brand_id = sqlc.arg(brand_id)
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (sqlc.narg(active)::bool IS NULL OR p.active = sqlc.narg(active)::bool)
  AND (
    COALESCE(cardinality(sqlc.narg(unit_types)::text[]), 0) = 0
    OR p.unit_type = ANY (sqlc.narg(unit_types)::text[])
  )
  AND (sqlc.narg(uses_fixed_barcode)::bool IS NULL OR p.uses_fixed_barcode = sqlc.narg(uses_fixed_barcode)::bool)
  AND (sqlc.narg(warranty_min)::float8 IS NULL OR p.warranty_duration_months >= sqlc.narg(warranty_min)::float8)
  AND (sqlc.narg(warranty_max)::float8 IS NULL OR p.warranty_duration_months <= sqlc.narg(warranty_max)::float8)
  AND (sqlc.narg(micron_min)::float8 IS NULL OR p.micron_thickness >= sqlc.narg(micron_min)::float8)
  AND (sqlc.narg(micron_max)::float8 IS NULL OR p.micron_thickness <= sqlc.narg(micron_max)::float8)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR p.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR p.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'sku' THEN p.sku::text
      WHEN 'name' THEN p.name::text
      WHEN 'unit_type' THEN p.unit_type::text
      WHEN 'category' THEN (SELECT c.name::text FROM product_categories c WHERE c.id = p.category_id)
    END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text
      WHEN 'sku' THEN p.sku::text
      WHEN 'name' THEN p.name::text
      WHEN 'unit_type' THEN p.unit_type::text
      WHEN 'category' THEN (SELECT c.name::text FROM product_categories c WHERE c.id = p.category_id)
    END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'warranty_duration_months' THEN p.warranty_duration_months END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'warranty_duration_months' THEN p.warranty_duration_months END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'micron_thickness' THEN p.micron_thickness END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'micron_thickness' THEN p.micron_thickness END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'active' THEN p.active END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'active' THEN p.active END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN p.created_at WHEN 'updated_at' THEN p.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN p.created_at WHEN 'updated_at' THEN p.updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN p.id END DESC,
  p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountProducts :one
SELECT COUNT(*)::bigint FROM products p
WHERE p.brand_id = sqlc.arg(brand_id)
  AND (
    COALESCE(cardinality(sqlc.narg(category_ids)::bigint[]), 0) = 0
    OR p.category_id = ANY (sqlc.narg(category_ids)::bigint[])
  )
  AND (sqlc.narg(active)::bool IS NULL OR p.active = sqlc.narg(active)::bool)
  AND (
    COALESCE(cardinality(sqlc.narg(unit_types)::text[]), 0) = 0
    OR p.unit_type = ANY (sqlc.narg(unit_types)::text[])
  )
  AND (sqlc.narg(uses_fixed_barcode)::bool IS NULL OR p.uses_fixed_barcode = sqlc.narg(uses_fixed_barcode)::bool)
  AND (sqlc.narg(warranty_min)::float8 IS NULL OR p.warranty_duration_months >= sqlc.narg(warranty_min)::float8)
  AND (sqlc.narg(warranty_max)::float8 IS NULL OR p.warranty_duration_months <= sqlc.narg(warranty_max)::float8)
  AND (sqlc.narg(micron_min)::float8 IS NULL OR p.micron_thickness >= sqlc.narg(micron_min)::float8)
  AND (sqlc.narg(micron_max)::float8 IS NULL OR p.micron_thickness <= sqlc.narg(micron_max)::float8)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR p.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR p.created_at < sqlc.narg(created_before))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- TEC-145: catalog API helpers.

-- name: GetProductCategoryByName :one
SELECT * FROM product_categories
WHERE brand_id = sqlc.arg(brand_id) AND name = sqlc.arg(name);

-- name: SetProductsActiveByUUIDs :many
-- Bulk activate/deactivate by public id within one brand. Returns the rows
-- that changed so the caller can reindex them. A product whose active flag
-- is locked by the integration sync is skipped (TEC-268).
UPDATE products
SET active = sqlc.arg(active)
WHERE brand_id = sqlc.arg(brand_id)
  AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND active IS DISTINCT FROM sqlc.arg(active)
  AND NOT ('active' = ANY(COALESCE(locked_fields, '{}'::text[])))
RETURNING uuid;

-- name: GetProductByUUIDForIndex :one
-- Search indexer only. Every document carries its brand_id and the search
-- query filters on it (K1/K20).
SELECT * FROM products WHERE uuid = sqlc.arg(uuid);

-- name: ListProductsForIndex :many
-- Search indexer only (full reindex across brands).
SELECT * FROM products ORDER BY id;

-- TEC-152: product image uploads.

-- name: AppendProductImage :one
-- Atomically appends one image while the product holds fewer than
-- max_images; no row means the product is gone or already full.
UPDATE products
SET images = images || jsonb_build_array(sqlc.arg(image)::jsonb)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
  AND jsonb_array_length(images) < sqlc.arg(max_images)::int
RETURNING *;

-- name: ReplaceProductImages :one
-- Optimistic replacement of the image list: no row when another request
-- changed the list since it was read (expected).
UPDATE products
SET images = sqlc.arg(images)::jsonb
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
  AND images = sqlc.arg(expected)::jsonb
RETURNING *;

-- name: GetActiveProductUUIDByImageKey :one
-- Public image route: the active product (any brand) that lists the key.
SELECT uuid FROM products
WHERE active
  AND images @> jsonb_build_array(jsonb_build_object('key', sqlc.arg(key)::text))
LIMIT 1;

-- TEC-212: bulk engine adapter (one product, logged + undoable).
-- name: SetProductActiveByUUID :one
UPDATE products
SET active = sqlc.arg(active)
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- TEC-369: category order and bulk actions.

-- name: ListProductCategoryOrder :many
-- Every category of the brand in display order (reorder input).
SELECT id, uuid, sort FROM product_categories
WHERE brand_id = sqlc.arg(brand_id)
ORDER BY sort ASC, name ASC, id ASC;

-- name: SetProductCategorySorts :execrows
-- Renumbers the categories in the given id order (10, 20, ...) in one
-- statement; rows whose value does not change are skipped.
UPDATE product_categories c
SET sort = (v.pos * 10)::int
FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS v(id, pos)
WHERE c.id = v.id AND c.brand_id = sqlc.arg(brand_id) AND c.sort <> (v.pos * 10)::int;

-- name: SetProductCategoryActiveByUUID :one
UPDATE product_categories
SET active = sqlc.arg(active)
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: DeleteUnusedProductCategory :execrows
-- Bulk delete: no row while products still use the category (the bulk run
-- shares one transaction, so a foreign key error must not happen).
DELETE FROM product_categories c
WHERE c.id = sqlc.arg(id) AND c.brand_id = sqlc.arg(brand_id)
  AND NOT EXISTS (SELECT 1 FROM products p WHERE p.category_id = c.id);

-- name: SetProductCategoryByUUID :one
-- Bulk set_category of one product.
UPDATE products
SET category_id = sqlc.arg(category_id)
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
RETURNING *;
