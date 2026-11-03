-- TEC-1043: usage counts of the lookup popularity engine
-- (internal/platform/popularity, legacy LookupPopularityService parity).
-- One domain brand only (K20). org_id NULL counts every organization of the
-- brand (the "general" count); org_id set counts that organization;
-- created_by_user_id narrows it further for own / assigned grants.
--
-- A car brand / model is used by a service (every status except cancelled,
-- like the earlier frequent-vehicles query) and by a measurement report through the
-- vehicle it is linked to (measurement_results has no car columns; legacy
-- counted nexptg_reports.car_brand_id / car_model_id). Reports without a
-- vehicle do not count. Like the legacy service, a report linked to a
-- service counts next to the service.
-- Service side reads idx_services_brand_car / idx_services_org_status.

-- name: PopularityCarBrandCounts :many
WITH used AS (
    SELECT s.car_brand_id AS car_brand_id
    FROM services s
    WHERE s.brand_id = sqlc.arg(brand_id)
      AND s.status <> 'cancelled'
      AND (sqlc.narg(org_id)::bigint IS NULL OR s.organization_id = sqlc.narg(org_id)::bigint)
      AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id)::bigint)
    UNION ALL
    SELECT v.car_brand_id AS car_brand_id
    FROM measurement_results mr
    JOIN vehicles v ON v.id = mr.vehicle_id
    WHERE mr.brand_id = sqlc.arg(brand_id)
      AND v.car_brand_id IS NOT NULL
      AND (sqlc.narg(org_id)::bigint IS NULL OR mr.organization_id = sqlc.narg(org_id)::bigint)
      AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR mr.created_by = sqlc.narg(created_by_user_id)::bigint)
)
SELECT u.car_brand_id::bigint AS id, COUNT(*)::bigint AS use_count
FROM used u
GROUP BY u.car_brand_id;

-- name: PopularityCarModelCounts :many
WITH used AS (
    SELECT s.car_model_id AS car_model_id
    FROM services s
    WHERE s.brand_id = sqlc.arg(brand_id)
      AND s.car_brand_id = sqlc.arg(car_brand_id)
      AND s.status <> 'cancelled'
      AND (sqlc.narg(org_id)::bigint IS NULL OR s.organization_id = sqlc.narg(org_id)::bigint)
      AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id)::bigint)
    UNION ALL
    SELECT v.car_model_id AS car_model_id
    FROM measurement_results mr
    JOIN vehicles v ON v.id = mr.vehicle_id
    WHERE mr.brand_id = sqlc.arg(brand_id)
      AND v.car_brand_id = sqlc.arg(car_brand_id)
      AND v.car_model_id IS NOT NULL
      AND (sqlc.narg(org_id)::bigint IS NULL OR mr.organization_id = sqlc.narg(org_id)::bigint)
      AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR mr.created_by = sqlc.narg(created_by_user_id)::bigint)
)
SELECT u.car_model_id::bigint AS id, COUNT(*)::bigint AS use_count
FROM used u
GROUP BY u.car_model_id;

-- Catalog rows of the ids the engine ranked, for GET
-- /v1/stats/frequent-vehicles (active rows only, like the catalog pickers).

-- name: ActiveCarBrandsByIDs :many
SELECT cb.id, cb.uuid, cb.name, cb.logo_object_key
FROM car_brands cb
WHERE cb.id = ANY (sqlc.arg(ids)::bigint[])
  AND cb.active;

-- name: ActiveCarModelsByIDs :many
SELECT cm.id, cm.uuid, cm.name
FROM car_models cm
WHERE cm.id = ANY (sqlc.arg(ids)::bigint[])
  AND cm.car_brand_id = sqlc.arg(car_brand_id)
  AND cm.active;
