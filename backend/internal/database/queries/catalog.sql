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
SELECT * FROM product_categories
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY sort ASC, name ASC, id ASC
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
SELECT * FROM products
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(category_id)::bigint IS NULL OR category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(unit_type)::text IS NULL OR unit_type = sqlc.narg(unit_type)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR sku ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY name ASC, id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountProducts :one
SELECT COUNT(*)::bigint FROM products
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(category_id)::bigint IS NULL OR category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(active)::bool IS NULL OR active = sqlc.narg(active)::bool)
  AND (sqlc.narg(unit_type)::text IS NULL OR unit_type = sqlc.narg(unit_type)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- TEC-145: catalog API helpers.

-- name: GetProductCategoryByName :one
SELECT * FROM product_categories
WHERE brand_id = sqlc.arg(brand_id) AND name = sqlc.arg(name);

-- name: SetProductsActiveByUUIDs :many
-- Bulk activate/deactivate by public id within one brand. Returns the rows
-- that changed so the caller can reindex them.
UPDATE products
SET active = sqlc.arg(active)
WHERE brand_id = sqlc.arg(brand_id)
  AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND active IS DISTINCT FROM sqlc.arg(active)
RETURNING uuid;

-- name: GetProductByUUIDForIndex :one
-- Search indexer only. Every document carries its brand_id and the search
-- query filters on it (K1/K20).
SELECT * FROM products WHERE uuid = sqlc.arg(uuid);

-- name: ListProductsForIndex :many
-- Search indexer only (full reindex across brands).
SELECT * FROM products ORDER BY id;
