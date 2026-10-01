-- TEC-84: countries > provinces > districts, territories, plate formats.

-- name: ListCountries :many
SELECT c.*,
       EXISTS (SELECT 1 FROM provinces p WHERE p.country_id = c.id) AS has_provinces
FROM countries c
WHERE (NOT sqlc.arg(active_only)::bool OR c.is_active)
ORDER BY c.name_en ASC;

-- name: GetCountryByISO2 :one
SELECT * FROM countries WHERE iso2 = sqlc.arg(iso2)::text;

-- name: GetCountryByID :one
SELECT * FROM countries WHERE id = $1;

-- name: SetCountryActive :one
UPDATE countries SET is_active = sqlc.arg(is_active)
WHERE iso2 = sqlc.arg(iso2)::text
RETURNING *;

-- name: ListProvincesByCountry :many
SELECT p.*,
       EXISTS (SELECT 1 FROM districts d WHERE d.province_id = p.id) AS has_districts
FROM provinces p
WHERE p.country_id = $1
ORDER BY p.code ASC;

-- name: GetProvinceByID :one
SELECT * FROM provinces WHERE id = $1;

-- name: CreateProvince :one
INSERT INTO provinces (country_id, code, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteProvince :execrows
DELETE FROM provinces WHERE id = $1;

-- name: ListDistrictsByProvince :many
SELECT * FROM districts
WHERE province_id = $1
ORDER BY name ASC;

-- name: GetDistrictByID :one
SELECT * FROM districts WHERE id = $1;

-- name: CreateDistrict :one
INSERT INTO districts (province_id, code, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteDistrict :execrows
DELETE FROM districts WHERE id = $1;

-- name: LockTerritoryArea :exec
-- Serializes territory writes of one (brand, country) inside a transaction.
SELECT pg_advisory_xact_lock(
    hashtextextended('territory:' || sqlc.arg(brand_id)::bigint::text || ':' || sqlc.arg(country_id)::bigint::text, 0)
);

-- name: ListOverlappingTerritories :many
-- Territories of the brand that overlap an area: the same area, an ancestor
-- (the country or the province of a district) or a descendant.
SELECT t.id, t.uuid, t.level, t.country_id, t.province_id, t.district_id,
       o.uuid AS organization_uuid, o.name AS organization_name
FROM territories t
JOIN organizations o ON o.id = t.organization_id
WHERE t.brand_id = sqlc.arg(brand_id)
  AND t.country_id = sqlc.arg(country_id)
  AND (
    t.province_id IS NULL
    OR sqlc.narg(province_id)::bigint IS NULL
    OR (
      t.province_id = sqlc.narg(province_id)::bigint
      AND (
        t.district_id IS NULL
        OR sqlc.narg(district_id)::bigint IS NULL
        OR t.district_id = sqlc.narg(district_id)::bigint
      )
    )
  )
ORDER BY t.id;

-- name: CreateTerritory :one
INSERT INTO territories (brand_id, organization_id, country_id, province_id, district_id, created_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteTerritory :execrows
DELETE FROM territories
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: ListTerritories :many
SELECT t.id, t.uuid, t.level, t.country_id, t.province_id, t.district_id, t.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name,
       c.iso2 AS country_iso2, c.name_en AS country_name_en, c.name_tr AS country_name_tr,
       p.name AS province_name, d.name AS district_name
FROM territories t
JOIN organizations o ON o.id = t.organization_id AND o.deleted_at IS NULL
JOIN countries c ON c.id = t.country_id
LEFT JOIN provinces p ON p.id = t.province_id
LEFT JOIN districts d ON d.id = t.district_id
WHERE t.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(organization_id)::bigint IS NULL OR t.organization_id = sqlc.narg(organization_id)::bigint)
ORDER BY c.name_en, p.name NULLS FIRST, d.name NULLS FIRST;

-- name: ResolveTerritory :one
-- The most specific territory covering an address (district > province >
-- country) whose distributor is live.
SELECT t.id, t.uuid, t.level, t.organization_id,
       o.uuid AS organization_uuid, o.name AS organization_name
FROM territories t
JOIN organizations o ON o.id = t.organization_id AND o.deleted_at IS NULL AND o.type = 'distributor'
WHERE t.brand_id = sqlc.arg(brand_id)
  AND t.country_id = sqlc.arg(country_id)
  AND (t.province_id IS NULL OR t.province_id = sqlc.narg(province_id)::bigint)
  AND (t.district_id IS NULL OR t.district_id = sqlc.narg(district_id)::bigint)
ORDER BY (t.district_id IS NOT NULL) DESC, (t.province_id IS NOT NULL) DESC
LIMIT 1;

-- name: ListPlateFormats :many
SELECT f.*, c.iso2 AS country_iso2, c.name_en AS country_name_en, c.name_tr AS country_name_tr
FROM plate_formats f
JOIN countries c ON c.id = f.country_id
WHERE (NOT sqlc.arg(active_only)::bool OR f.is_active)
ORDER BY f.sort_order ASC, c.name_en ASC;

-- name: GetPlateFormatByCountry :one
SELECT f.*, c.iso2 AS country_iso2, c.name_en AS country_name_en, c.name_tr AS country_name_tr
FROM plate_formats f
JOIN countries c ON c.id = f.country_id
WHERE c.iso2 = sqlc.arg(iso2)::text;

-- name: CreatePlateFormat :one
INSERT INTO plate_formats (
    country_id, regex, input_mask, example, country_label,
    strip_color, background_color, text_color, is_active, sort_order
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: UpdatePlateFormat :one
UPDATE plate_formats
SET regex = COALESCE(sqlc.narg(regex), regex),
    input_mask = COALESCE(sqlc.narg(input_mask), input_mask),
    example = COALESCE(sqlc.narg(example), example),
    country_label = COALESCE(sqlc.narg(country_label), country_label),
    strip_color = COALESCE(sqlc.narg(strip_color), strip_color),
    background_color = COALESCE(sqlc.narg(background_color), background_color),
    text_color = COALESCE(sqlc.narg(text_color), text_color),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    sort_order = COALESCE(sqlc.narg(sort_order), sort_order)
WHERE country_id = sqlc.arg(country_id)
RETURNING *;

-- name: DeletePlateFormat :execrows
DELETE FROM plate_formats WHERE country_id = $1;
