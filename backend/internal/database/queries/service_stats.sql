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
