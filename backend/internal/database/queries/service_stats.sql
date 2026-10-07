-- TEC-151 (F1-09c): top-10 car brands / models by completed services of one
-- domain brand. Both queries read idx_services_brand_car
-- (brand_id, car_brand_id, car_model_id, completed_at). completed_at is set
-- exactly when status = 'completed' (chk_services_completed), so
-- "completed_at IS NOT NULL" is the completed filter and stays inside the
-- index. since NULL means all time. Ties sort by name.

-- name: TopServicedCarBrands :many
WITH counts AS (
    SELECT s.car_brand_id, COUNT(*)::bigint AS service_count
    FROM services s
    WHERE s.brand_id = sqlc.arg(brand_id)
      AND s.completed_at IS NOT NULL
      AND (sqlc.narg(since)::timestamptz IS NULL OR s.completed_at >= sqlc.narg(since)::timestamptz)
    GROUP BY s.car_brand_id
)
SELECT c.service_count,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cb.logo_object_key AS car_brand_logo_key
FROM counts c
JOIN car_brands cb ON cb.id = c.car_brand_id
ORDER BY c.service_count DESC, lower(cb.name), cb.id
LIMIT 10;

-- name: TopServicedCarModels :many
WITH counts AS (
    SELECT s.car_brand_id, s.car_model_id, COUNT(*)::bigint AS service_count
    FROM services s
    WHERE s.brand_id = sqlc.arg(brand_id)
      AND s.completed_at IS NOT NULL
      AND (sqlc.narg(since)::timestamptz IS NULL OR s.completed_at >= sqlc.narg(since)::timestamptz)
    GROUP BY s.car_brand_id, s.car_model_id
)
SELECT c.service_count,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cb.logo_object_key AS car_brand_logo_key,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name
FROM counts c
JOIN car_brands cb ON cb.id = c.car_brand_id
JOIN car_models cm ON cm.id = c.car_model_id
ORDER BY c.service_count DESC, lower(cb.name), lower(cm.name), cm.id
LIMIT 10;

-- TEC-385 (F4-01c): the AI assistant's activity summary of one period, in
-- the caller's services.read scope (org_ids NULL = whole brand,
-- created_by_user_id for own / assigned grants), parity with the legacy
-- chatbot dealer/services/count, brand-breakdown and products/top. Created
-- counts services opened in [from, to), completed those completed in it;
-- the breakdowns read completed services only.

-- name: ServiceActivityCounts :one
SELECT
    COUNT(*) FILTER (WHERE s.created_at >= sqlc.arg(period_from)::timestamptz
                       AND s.created_at < sqlc.arg(period_to)::timestamptz)::bigint AS created_count,
    COUNT(*) FILTER (WHERE s.completed_at >= sqlc.arg(period_from)::timestamptz
                       AND s.completed_at < sqlc.arg(period_to)::timestamptz)::bigint AS completed_count
FROM services s
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND (
    (s.created_at >= sqlc.arg(period_from)::timestamptz AND s.created_at < sqlc.arg(period_to)::timestamptz)
    OR (s.completed_at >= sqlc.arg(period_from)::timestamptz AND s.completed_at < sqlc.arg(period_to)::timestamptz)
  );

-- name: ServiceActivityCarBrands :many
SELECT cb.uuid AS car_brand_uuid, cb.name AS car_brand_name, COUNT(*)::bigint AS service_count
FROM services s
JOIN car_brands cb ON cb.id = s.car_brand_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.completed_at >= sqlc.arg(period_from)::timestamptz
  AND s.completed_at < sqlc.arg(period_to)::timestamptz
GROUP BY cb.id, cb.uuid, cb.name
ORDER BY service_count DESC, lower(cb.name), cb.id
LIMIT sqlc.arg(row_limit);

-- name: ServiceActivityTopProducts :many
SELECT p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       COUNT(DISTINCT si.service_id)::bigint AS service_count,
       COALESCE(SUM(si.quantity), 0)::bigint AS quantity,
       COALESCE(SUM(si.meters), 0)::numeric(14,2)::text AS meters
FROM service_items si
JOIN services s ON s.id = si.service_id
JOIN products p ON p.id = si.product_id
WHERE s.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(created_by_user_id)::bigint IS NULL OR s.created_by_user_id = sqlc.narg(created_by_user_id))
  AND s.completed_at >= sqlc.arg(period_from)::timestamptz
  AND s.completed_at < sqlc.arg(period_to)::timestamptz
GROUP BY p.id, p.uuid, p.sku, p.name, p.unit_type
ORDER BY service_count DESC, lower(p.name), p.id
LIMIT sqlc.arg(row_limit);
