-- TEC-149: vehicle catalog (car brands and models). Global reference data:
-- no organization/brand filter; only super_admin writes (use case + route).

-- name: CreateCarBrand :one
INSERT INTO car_brands (external_id, name, show_name, logo_height, active)
VALUES (sqlc.narg(external_id), sqlc.arg(name), sqlc.arg(show_name), sqlc.narg(logo_height), sqlc.arg(active))
RETURNING *;

-- name: GetCarBrandByUUID :one
SELECT * FROM car_brands WHERE uuid = sqlc.arg(uuid);

-- name: GetCarBrandByID :one
SELECT * FROM car_brands WHERE id = sqlc.arg(id);

-- name: UpdateCarBrand :one
-- Full replacement of the editable fields (read-modify-write in the use case).
UPDATE car_brands
SET external_id = sqlc.narg(external_id),
    name = sqlc.arg(name),
    show_name = sqlc.arg(show_name),
    logo_height = sqlc.narg(logo_height),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetCarBrandLogo :one
UPDATE car_brands SET logo_object_key = sqlc.narg(logo_object_key)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetCarBrandHero :one
UPDATE car_brands SET hero_object_key = sqlc.narg(hero_object_key)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCarBrand :execrows
-- Fails with a restrict/foreign key violation while models still use the brand.
DELETE FROM car_brands WHERE id = sqlc.arg(id);

-- name: ListCarBrands :many
SELECT b.*,
       (SELECT COUNT(*) FROM car_models m WHERE m.car_brand_id = b.id)::bigint AS model_count
FROM car_brands b
WHERE (sqlc.narg(active)::bool IS NULL OR b.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR b.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR b.external_id = sqlc.narg(q)::text)
ORDER BY lower(b.name) ASC, b.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCarBrands :one
SELECT COUNT(*)::bigint FROM car_brands b
WHERE (sqlc.narg(active)::bool IS NULL OR b.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR b.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR b.external_id = sqlc.narg(q)::text);

-- name: CountCarModelsByBrand :one
SELECT COUNT(*)::bigint FROM car_models WHERE car_brand_id = sqlc.arg(car_brand_id);

-- name: CreateCarModel :one
INSERT INTO car_models (car_brand_id, external_id, name, body_type, powertrain, year_start, year_stop, active)
VALUES (
    sqlc.arg(car_brand_id), sqlc.narg(external_id), sqlc.arg(name), sqlc.narg(body_type),
    sqlc.narg(powertrain), sqlc.narg(year_start), sqlc.narg(year_stop), sqlc.arg(active)
)
RETURNING *;

-- name: GetCarModelByUUID :one
SELECT * FROM car_models WHERE uuid = sqlc.arg(uuid);

-- name: UpdateCarModel :one
UPDATE car_models
SET external_id = sqlc.narg(external_id),
    name = sqlc.arg(name),
    body_type = sqlc.narg(body_type),
    powertrain = sqlc.narg(powertrain),
    year_start = sqlc.narg(year_start),
    year_stop = sqlc.narg(year_stop),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetCarModelHero :one
UPDATE car_models SET hero_object_key = sqlc.narg(hero_object_key)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCarModel :execrows
DELETE FROM car_models WHERE id = sqlc.arg(id);

-- name: ListCarModels :many
-- Search matches the model name, "brand model" and the external id.
SELECT m.*, b.uuid AS brand_uuid, b.name AS brand_name, b.hero_object_key AS brand_hero_object_key
FROM car_models m
JOIN car_brands b ON b.id = m.car_brand_id
WHERE (sqlc.narg(car_brand_id)::bigint IS NULL OR m.car_brand_id = sqlc.narg(car_brand_id)::bigint)
  AND (sqlc.narg(active)::bool IS NULL OR m.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(brand_active)::bool IS NULL OR b.active = sqlc.narg(brand_active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR (b.name || ' ' || m.name) ILIKE '%' || sqlc.narg(q)::text || '%'
       OR m.external_id = sqlc.narg(q)::text)
ORDER BY lower(b.name) ASC, lower(m.name) ASC, m.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCarModels :one
SELECT COUNT(*)::bigint
FROM car_models m
JOIN car_brands b ON b.id = m.car_brand_id
WHERE (sqlc.narg(car_brand_id)::bigint IS NULL OR m.car_brand_id = sqlc.narg(car_brand_id)::bigint)
  AND (sqlc.narg(active)::bool IS NULL OR m.active = sqlc.narg(active)::bool)
  AND (sqlc.narg(brand_active)::bool IS NULL OR b.active = sqlc.narg(brand_active)::bool)
  AND (sqlc.narg(q)::text IS NULL OR (b.name || ' ' || m.name) ILIKE '%' || sqlc.narg(q)::text || '%'
       OR m.external_id = sqlc.narg(q)::text);
