-- TEC-256: migrator steps 3-4 (vehicle catalog, product catalog and the
-- warehouse product match). Written only by cmd/migrator inside a step
-- transaction.

-- name: MigratorCarBrandByUUID :one
SELECT id, uuid, logo_object_key FROM car_brands WHERE uuid = $1;

-- name: MigratorFindCarBrand :one
-- An existing brand with the legacy external id, else with the same name
-- (case insensitive, like uq_car_brands_name).
SELECT id, uuid, logo_object_key FROM car_brands
WHERE (sqlc.narg(external_id)::text IS NOT NULL AND external_id = sqlc.narg(external_id)::text)
   OR lower(name) = lower(btrim(sqlc.arg(name)::text))
ORDER BY (external_id IS NOT DISTINCT FROM sqlc.narg(external_id)::text) DESC, id
LIMIT 1;

-- name: MigratorInsertCarBrand :one
INSERT INTO car_brands (uuid, external_id, name, show_name, logo_height, active, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.narg(external_id), sqlc.arg(name), sqlc.arg(show_name),
    sqlc.narg(logo_height), sqlc.arg(active), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateCarBrand :exec
-- Legacy-sourced fields of a brand the migrator created; the logo is set
-- separately.
UPDATE car_brands
SET name = sqlc.arg(name), show_name = sqlc.arg(show_name),
    logo_height = sqlc.narg(logo_height), active = sqlc.arg(active)
WHERE id = sqlc.arg(id);

-- name: MigratorSetCarBrandLogo :exec
UPDATE car_brands SET logo_object_key = sqlc.arg(logo_object_key)::text WHERE id = sqlc.arg(id);

-- name: MigratorCarModelIDByUUID :one
SELECT id FROM car_models WHERE uuid = $1;

-- name: MigratorFindCarModel :one
-- An existing model with the legacy external id, else with the same name
-- under the brand (case insensitive, like uq_car_models_brand_name).
SELECT uuid FROM car_models
WHERE (sqlc.narg(external_id)::text IS NOT NULL AND external_id = sqlc.narg(external_id)::text)
   OR (car_brand_id = sqlc.arg(car_brand_id)::bigint AND lower(name) = lower(btrim(sqlc.arg(name)::text)))
ORDER BY (external_id IS NOT DISTINCT FROM sqlc.narg(external_id)::text) DESC, id
LIMIT 1;

-- name: MigratorInsertCarModel :one
INSERT INTO car_models (uuid, car_brand_id, external_id, name, body_type, powertrain, year_start, year_stop, active, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(car_brand_id), sqlc.narg(external_id), sqlc.arg(name), sqlc.narg(body_type),
    sqlc.narg(powertrain), sqlc.narg(year_start), sqlc.narg(year_stop), sqlc.arg(active),
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateCarModel :exec
UPDATE car_models
SET car_brand_id = sqlc.arg(car_brand_id), name = sqlc.arg(name), body_type = sqlc.narg(body_type),
    powertrain = sqlc.narg(powertrain), year_start = sqlc.narg(year_start), year_stop = sqlc.narg(year_stop),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id);

-- name: MigratorCategoryIDByUUID :one
SELECT id FROM product_categories WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorFindCategory :one
SELECT uuid FROM product_categories
WHERE brand_id = sqlc.arg(brand_id) AND lower(name) = lower(btrim(sqlc.arg(name)::text))
ORDER BY id
LIMIT 1;

-- name: MigratorInsertCategory :one
INSERT INTO product_categories (uuid, organization_id, brand_id, name, available_parts, active, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(name),
    sqlc.arg(available_parts), sqlc.arg(active), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateCategory :exec
UPDATE product_categories
SET name = sqlc.arg(name), available_parts = sqlc.arg(available_parts), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorProductIDByUUID :one
SELECT id FROM products WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorFindProductBySKU :one
SELECT uuid FROM products
WHERE brand_id = sqlc.arg(brand_id) AND lower(sku) = lower(btrim(sqlc.arg(sku)::text))
ORDER BY id
LIMIT 1;

-- name: MigratorFindProductsByName :many
-- Products of the brand with the name (case insensitive); the warehouse
-- match uses it only when exactly one row comes back.
SELECT uuid FROM products
WHERE brand_id = sqlc.arg(brand_id) AND lower(name) = lower(btrim(sqlc.arg(name)::text))
ORDER BY id
LIMIT 2;

-- name: MigratorInsertProduct :one
INSERT INTO products (
    uuid, organization_id, brand_id, category_id, sku, name, description_md,
    warranty_duration_months, micron_thickness, active, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(category_id), sqlc.arg(sku),
    sqlc.arg(name), sqlc.arg(description_md), sqlc.narg(warranty_duration_months),
    sqlc.narg(micron_thickness)::numeric, sqlc.arg(active), COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateProduct :exec
-- Legacy-sourced fields only; images, unit type and the sync columns stay.
UPDATE products
SET category_id = sqlc.arg(category_id), sku = sqlc.arg(sku), name = sqlc.arg(name),
    description_md = sqlc.arg(description_md),
    warranty_duration_months = sqlc.narg(warranty_duration_months),
    micron_thickness = sqlc.narg(micron_thickness)::numeric, active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);
