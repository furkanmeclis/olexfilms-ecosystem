-- TEC-240: dealer coordinates and the public "nearby dealers" lookup.

-- name: UpdateOrganizationCoordinates :one
-- Sets or clears (both NULL) the map position of an organization.
UPDATE organizations
SET latitude = sqlc.narg(latitude)::numeric,
    longitude = sqlc.narg(longitude)::numeric
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: ListNearbyDealers :many
-- Active, serving (access window open) dealers and distributors of a brand
-- with coordinates, within radius_km of (lat, lng). Distance is the
-- haversine great-circle distance in km (mean Earth radius 6371.0088).
SELECT n.slug, n.name, n.city, n.district, n.latitude, n.longitude, n.phone, n.distance_km, n.accepts_appointments
FROM (
    SELECT o.slug,
           o.name,
           COALESCE(NULLIF(btrim(o.city), ''), p.name, '')::text AS city,
           COALESCE(NULLIF(btrim(o.district), ''), d.name, '')::text AS district,
           o.latitude::float8 AS latitude,
           o.longitude::float8 AS longitude,
           o.phone,
           (COALESCE(s.portal_appointments_enabled, FALSE) AND
            CASE
                WHEN sys.enabled = FALSE THEN FALSE
                WHEN o.type = 'dealer' AND parent.type = 'distributor' THEN
                    CASE
                        WHEN own.source = 'admin' THEN own.enabled
                        WHEN COALESCE(parent_own.enabled, m.default_enabled) = FALSE THEN FALSE
                        WHEN own.id IS NOT NULL THEN own.enabled
                        WHEN dealer_std.id IS NOT NULL THEN dealer_std.enabled
                        ELSE m.default_enabled
                    END
                ELSE COALESCE(own.enabled, m.default_enabled)
            END)::boolean AS accepts_appointments,
           (6371.0088 * 2 * asin(sqrt(LEAST(1.0,
               power(sin(radians(o.latitude::float8 - sqlc.arg(lat)::float8) / 2), 2)
               + cos(radians(sqlc.arg(lat)::float8)) * cos(radians(o.latitude::float8))
                 * power(sin(radians(o.longitude::float8 - sqlc.arg(lng)::float8) / 2), 2)
           ))))::float8 AS distance_km
    FROM organizations o
    LEFT JOIN appointment_settings s ON s.organization_id = o.id
    JOIN modules m ON m.key = 'appointments'
    LEFT JOIN organizations parent ON parent.id = o.parent_id
    LEFT JOIN module_flags sys ON sys.scope = 'system' AND sys.module_key = m.key
    LEFT JOIN module_flags own ON own.scope = 'org' AND own.organization_id = o.id AND own.module_key = m.key
    LEFT JOIN module_flags parent_own ON parent_own.scope = 'org' AND parent_own.organization_id = parent.id AND parent_own.module_key = m.key
    LEFT JOIN module_flags dealer_std ON dealer_std.scope = 'dealer_standard' AND dealer_std.organization_id = parent.id AND dealer_std.module_key = m.key
    LEFT JOIN provinces p ON p.id = o.province_id
    LEFT JOIN districts d ON d.id = o.district_id
    WHERE o.brand_id = sqlc.arg(brand_id)
      AND o.deleted_at IS NULL
      AND o.status = 'active'
      AND o.type IN ('dealer', 'distributor')
      AND o.latitude IS NOT NULL
      AND o.longitude IS NOT NULL
      AND o.access_starts_at <= NOW()
      AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
) n
WHERE n.distance_km <= sqlc.arg(radius_km)::float8
ORDER BY n.distance_km ASC, n.slug ASC
LIMIT sqlc.arg(limit_count);

-- name: GetPublicDealerBySlug :one
-- TEC-250: the public showcase of one active, serving (access window open)
-- dealer or distributor of a brand. Only the showcase columns: no tax id,
-- account, members or settings.
SELECT o.uuid,
       o.slug,
       o.name,
       o.logo_object_key,
       o.address,
       COALESCE(NULLIF(btrim(o.city), ''), p.name, '')::text AS city,
       COALESCE(NULLIF(btrim(o.district), ''), d.name, '')::text AS district,
       o.latitude,
       o.longitude,
       o.phone
FROM organizations o
LEFT JOIN provinces p ON p.id = o.province_id
LEFT JOIN districts d ON d.id = o.district_id
WHERE o.slug = sqlc.arg(slug)
  AND o.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type IN ('dealer', 'distributor')
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW());

-- name: ListPublicDealerCodes :many
-- TEC-251: codes (slugs) of the brand's active, serving dealers and
-- distributors for the public sitemap. Same filters as
-- GetPublicDealerBySlug; code and last change only.
SELECT o.slug,
       o.updated_at
FROM organizations o
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type IN ('dealer', 'distributor')
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
ORDER BY o.slug
LIMIT sqlc.arg(row_limit)::int;
