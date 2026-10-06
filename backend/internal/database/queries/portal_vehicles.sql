-- TEC-238 (F2-03a): customer portal "my vehicles" reads.
--
-- Every query is bound to the signed-in user (vehicles.user_id,
-- warranties.holder_user_id; a service is the user's when the user is
-- its customer OR holds one of its warranties, the TEC-239 rule, so a new
-- owner who received a transferred warranty sees the service) and to the domain
-- brand (K20). Glorian rows never come back (K1/K2: service, warranty and
-- customer are closed to Glorian), even on a Glorian host. Draft services
-- are dealer-internal and stay out. No measurement column is selected.

-- name: ListPortalVehicles :many
SELECT v.uuid, v.model_year, v.plate, v.plate_country, v.vin, v.created_at,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name,
       (SELECT COUNT(*) FROM services s
        WHERE s.vehicle_id = v.id AND (s.customer_user_id = v.user_id
               OR EXISTS (SELECT 1 FROM warranties hw WHERE hw.service_id = s.id AND hw.holder_user_id = v.user_id))
          AND s.brand_id = v.brand_id AND s.status <> 'draft')::bigint AS service_count,
       (SELECT MAX(COALESCE(s.completed_at, s.created_at)) FROM services s
        WHERE s.vehicle_id = v.id AND (s.customer_user_id = v.user_id
               OR EXISTS (SELECT 1 FROM warranties hw WHERE hw.service_id = s.id AND hw.holder_user_id = v.user_id))
          AND s.brand_id = v.brand_id AND s.status <> 'draft')::timestamptz AS last_service_at,
       (SELECT COUNT(*) FROM warranties w
        WHERE w.vehicle_id = v.id AND w.holder_user_id = v.user_id
          AND w.brand_id = v.brand_id AND w.status = 'active'
          AND w.end_at > sqlc.arg(now)::timestamptz)::bigint AS active_warranty_count
FROM vehicles v
JOIN brands b ON b.id = v.brand_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.user_id = sqlc.arg(user_id)::bigint
  AND v.brand_id = sqlc.arg(brand_id)::bigint
  AND v.deleted_at IS NULL
  AND b.slug <> 'glorian'
ORDER BY v.created_at DESC, v.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountPortalVehicles :one
SELECT COUNT(*)::bigint
FROM vehicles v
JOIN brands b ON b.id = v.brand_id
WHERE v.user_id = sqlc.arg(user_id)::bigint
  AND v.brand_id = sqlc.arg(brand_id)::bigint
  AND v.deleted_at IS NULL
  AND b.slug <> 'glorian';

-- name: GetPortalVehicle :one
SELECT v.id, v.uuid, v.model_year, v.plate, v.plate_country, v.vin, v.created_at,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       cm.uuid AS car_model_uuid, cm.name AS car_model_name
FROM vehicles v
JOIN brands b ON b.id = v.brand_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.uuid = sqlc.arg(uuid)
  AND v.user_id = sqlc.arg(user_id)::bigint
  AND v.brand_id = sqlc.arg(brand_id)::bigint
  AND v.deleted_at IS NULL
  AND b.slug <> 'glorian';

-- Services of the user across every organization of the brand (one list,
-- newest first by default). vehicle_id narrows to one vehicle (vehicle
-- detail). TEC-377 (DT-BE-7): statuses / organization_uuids (multi-value),
-- created window, q (service number or plate; the caller escapes LIKE
-- wildcards) and the sort keys of portalvehicles usecase.ServiceSort.
-- name: ListPortalServices :many
SELECT s.uuid, s.service_no, s.status, s.package, s.plate, s.plate_country, s.model_year,
       s.completed_at, s.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       v.uuid AS vehicle_uuid,
       cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN brands b ON b.id = s.brand_id
JOIN organizations o ON o.id = s.organization_id
JOIN vehicles v ON v.id = s.vehicle_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND s.brand_id = sqlc.arg(brand_id)::bigint
  AND s.status <> 'draft'
  AND b.slug <> 'glorian'
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR s.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR s.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR s.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR s.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.plate ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'service_no' THEN s.service_no::text WHEN 'organization' THEN o.name::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'service_no' THEN s.service_no::text WHEN 'organization' THEN o.name::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE s.status WHEN 'draft' THEN 0 WHEN 'pending' THEN 1 WHEN 'processing' THEN 2
      WHEN 'ready' THEN 3 WHEN 'completed' THEN 4 WHEN 'cancelled' THEN 5 ELSE 6 END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE s.status WHEN 'draft' THEN 0 WHEN 'pending' THEN 1 WHEN 'processing' THEN 2
      WHEN 'ready' THEN 3 WHEN 'completed' THEN 4 WHEN 'cancelled' THEN 5 ELSE 6 END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'completed_at' THEN s.completed_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'completed_at' THEN s.completed_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountPortalServices :one
SELECT COUNT(*)::bigint
FROM services s
JOIN brands b ON b.id = s.brand_id
WHERE (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND s.brand_id = sqlc.arg(brand_id)::bigint
  AND s.status <> 'draft'
  AND b.slug <> 'glorian'
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR s.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR s.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR s.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR s.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.plate ILIKE '%' || sqlc.narg(q)::text || '%');

-- Active warranties of one vehicle held by the user (soonest end first).
-- name: ListPortalVehicleActiveWarranties :many
SELECT w.uuid, w.public_code, w.start_at, w.end_at,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       s.uuid AS service_uuid, s.service_no,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type
FROM warranties w
JOIN brands b ON b.id = w.brand_id
JOIN products p ON p.id = w.product_id
JOIN services s ON s.id = w.service_id
JOIN organizations o ON o.id = w.organization_id
WHERE w.vehicle_id = sqlc.arg(vehicle_id)::bigint
  AND w.holder_user_id = sqlc.arg(user_id)::bigint
  AND w.brand_id = sqlc.arg(brand_id)::bigint
  AND w.status = 'active'
  AND w.end_at > sqlc.arg(now)::timestamptz
  AND b.slug <> 'glorian'
ORDER BY w.end_at ASC, w.id ASC;

-- Service summary of one vehicle across every organization of the brand.
-- name: GetPortalVehicleServiceSummary :one
SELECT COUNT(*)::bigint AS total,
       (COUNT(*) FILTER (WHERE s.status = 'completed'))::bigint AS completed,
       (COUNT(DISTINCT s.organization_id))::bigint AS organization_count,
       MAX(COALESCE(s.completed_at, s.created_at))::timestamptz AS last_service_at
FROM services s
JOIN brands b ON b.id = s.brand_id
WHERE s.vehicle_id = sqlc.arg(vehicle_id)::bigint
  AND (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND s.brand_id = sqlc.arg(brand_id)::bigint
  AND s.status <> 'draft'
  AND b.slug <> 'glorian';

-- TEC-288 (F3-01d): the user's executed vehicle intake contracts. The
-- ownership rule stays identical to portal services: the service customer or
-- warranty holder sees it, within the domain brand, excluding Glorian.
-- name: ListPortalContracts :many
SELECT s.uuid, s.service_no, s.status, s.plate, s.plate_country, s.model_year, s.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       v.uuid AS vehicle_uuid,
       cb.name AS car_brand_name, cm.name AS car_model_name,
       ci.uuid AS contract_uuid, ci.contract_no, ci.executed_at,
       (ci.pdf_key IS NOT NULL)::boolean AS pdf_ready
FROM contract_instances ci
JOIN services s ON s.id = ci.subject_service_id
JOIN brands b ON b.id = s.brand_id
JOIN organizations o ON o.id = s.organization_id
JOIN vehicles v ON v.id = s.vehicle_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE ci.status = 'executed'
  AND (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND ci.brand_id = sqlc.arg(brand_id)::bigint
  AND b.slug <> 'glorian'
ORDER BY ci.executed_at DESC, ci.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountPortalContracts :one
SELECT COUNT(*)::bigint
FROM contract_instances ci
JOIN services s ON s.id = ci.subject_service_id
JOIN brands b ON b.id = ci.brand_id
WHERE ci.status = 'executed'
  AND (s.customer_user_id = sqlc.arg(user_id)::bigint
       OR EXISTS (SELECT 1 FROM warranties hw
                  WHERE hw.service_id = s.id AND hw.holder_user_id = sqlc.arg(user_id)::bigint))
  AND ci.brand_id = sqlc.arg(brand_id)::bigint
  AND b.slug <> 'glorian';
