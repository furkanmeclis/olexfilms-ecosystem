-- TEC-474 (F5-02c): fleet portal reads. Every query is bounded by the
-- signed-in user's fleet (fleet_org_id) and by dealer_ids: the dealers the
-- fleet worked with (an active or ended link) whose fleet module is still
-- on. A dealer that turns the module off drops out of dealer_ids, so its
-- services, warranties, appointments and cari disappear from the portal
-- (nothing is deleted).

-- name: ListFleetPortalVehicles :many
-- Vehicles of the fleet with the last service, the service count, the
-- active warranty count and the latest active warranty end of the visible
-- dealers. Sort keys plate | last_service_at | warranty_until, default plate.
WITH fv AS (
    SELECT v.id, v.uuid, v.plate, v.plate_country, v.vin, v.model_year, v.created_at,
           cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
           cm.uuid AS car_model_uuid, cm.name AS car_model_name,
           (SELECT MAX(s.completed_at) FROM services s
            WHERE s.vehicle_id = v.id AND s.status = 'completed'
              AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]))::timestamptz AS last_service_at,
           (SELECT COUNT(*) FROM services s
            WHERE s.vehicle_id = v.id AND s.status <> 'draft'
              AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]))::bigint AS service_count,
           (SELECT COUNT(*) FROM warranties w
            WHERE w.vehicle_id = v.id AND w.status = 'active' AND w.end_at > NOW()
              AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]))::bigint AS active_warranty_count,
           (SELECT MAX(w.end_at) FROM warranties w
            WHERE w.vehicle_id = v.id AND w.status = 'active' AND w.end_at > NOW()
              AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]))::timestamptz AS warranty_until
    FROM vehicles v
    LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
    LEFT JOIN car_models cm ON cm.id = v.car_model_id
    WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND v.deleted_at IS NULL
      AND (sqlc.narg(vehicle_uuid)::uuid IS NULL OR v.uuid = sqlc.narg(vehicle_uuid)::uuid)
      AND (
        sqlc.narg(q)::text IS NULL
        OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
        OR v.plate_normalized LIKE sqlc.narg(q_plate)::text || '%'
        OR v.vin ILIKE sqlc.narg(q) || '%'
      )
      AND (
        COALESCE(cardinality(sqlc.narg(car_brand_uuids)::uuid[]), 0) = 0
        OR cb.uuid = ANY (sqlc.narg(car_brand_uuids)::uuid[])
      )
)
SELECT * FROM fv
WHERE sqlc.narg(has_active_warranty)::bool IS NULL
   OR (fv.active_warranty_count > 0) = sqlc.narg(has_active_warranty)::bool
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN fv.plate END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'plate' THEN fv.plate END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fv.last_service_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_service_at' THEN fv.last_service_at END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'warranty_until' THEN fv.warranty_until END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'warranty_until' THEN fv.warranty_until END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN fv.id END DESC,
  fv.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetPortalVehicles :one
-- Same filter block as ListFleetPortalVehicles.
SELECT COUNT(*)::bigint
FROM vehicles v
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND v.deleted_at IS NULL
  AND (
    sqlc.narg(q)::text IS NULL
    OR v.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR v.plate_normalized LIKE sqlc.narg(q_plate)::text || '%'
    OR v.vin ILIKE sqlc.narg(q) || '%'
  )
  AND (
    COALESCE(cardinality(sqlc.narg(car_brand_uuids)::uuid[]), 0) = 0
    OR cb.uuid = ANY (sqlc.narg(car_brand_uuids)::uuid[])
  )
  AND (
    sqlc.narg(has_active_warranty)::bool IS NULL
    OR EXISTS (SELECT 1 FROM warranties w
               WHERE w.vehicle_id = v.id AND w.status = 'active' AND w.end_at > NOW()
                 AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]))
       = sqlc.narg(has_active_warranty)::bool
  );

-- name: ListFleetPortalServices :many
-- Services on the fleet's vehicles at the visible dealers (drafts stay
-- dealer internal). Filters: vehicle, status, dealer (organization uuid),
-- created_at range, q (service no / plate). Sort keys created_at |
-- completed_at | service_no | status, default -created_at.
SELECT s.uuid, s.service_no, s.status, s.package, s.plate, s.plate_country, s.model_year,
       s.completed_at, s.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       v.uuid AS vehicle_uuid,
       cb.name AS car_brand_name, cm.name AS car_model_name
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN organizations o ON o.id = s.organization_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status <> 'draft'
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR s.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(dealer_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(dealer_uuids)::uuid[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR s.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR s.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.plate ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'service_no' THEN s.service_no END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'service_no' THEN s.service_no END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN s.status END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN s.status END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'completed_at' THEN s.completed_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'completed_at' THEN s.completed_at END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN s.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN s.id END DESC,
  s.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetPortalServices :one
-- Same filter block as ListFleetPortalServices.
SELECT COUNT(*)::bigint
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN organizations o ON o.id = s.organization_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status <> 'draft'
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR s.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR s.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(dealer_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(dealer_uuids)::uuid[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR s.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR s.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR s.service_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR s.plate ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: ListFleetPortalWarranties :many
-- Warranties on the fleet's vehicles issued by the visible dealers. state
-- is the effective status: active (active and not past end_at), expired
-- (expired, or active past end_at) or void. Sort keys end_at | start_at,
-- default end_at.
WITH fw AS (
    SELECT w.id, w.uuid, w.public_code, w.start_at, w.end_at,
           (CASE WHEN w.status = 'void' THEN 'void'
                 WHEN w.status = 'active' AND w.end_at > NOW() THEN 'active'
                 ELSE 'expired' END)::text AS state,
           p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
           s.uuid AS service_uuid, s.service_no,
           o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
           v.uuid AS vehicle_uuid, v.plate
    FROM warranties w
    JOIN vehicles v ON v.id = w.vehicle_id
    JOIN products p ON p.id = w.product_id
    JOIN services s ON s.id = w.service_id
    JOIN organizations o ON o.id = w.organization_id
    WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
      AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
      AND (sqlc.narg(vehicle_id)::bigint IS NULL OR w.vehicle_id = sqlc.narg(vehicle_id)::bigint)
      AND (COALESCE(cardinality(sqlc.narg(dealer_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(dealer_uuids)::uuid[]))
      AND (sqlc.narg(q)::text IS NULL
           OR v.plate ILIKE '%' || sqlc.narg(q)::text || '%'
           OR w.public_code ILIKE '%' || sqlc.narg(q)::text || '%'
           OR p.name ILIKE '%' || sqlc.narg(q)::text || '%')
)
SELECT * FROM fw
WHERE COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0 OR fw.state = ANY (sqlc.narg(states)::text[])
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'end_at' THEN fw.end_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'end_at' THEN fw.end_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'start_at' THEN fw.start_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'start_at' THEN fw.start_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN fw.id END DESC,
  fw.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetPortalWarranties :one
-- Same filter block as ListFleetPortalWarranties.
SELECT COUNT(*)::bigint
FROM warranties w
JOIN vehicles v ON v.id = w.vehicle_id
JOIN products p ON p.id = w.product_id
JOIN organizations o ON o.id = w.organization_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR w.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(dealer_uuids)::uuid[]), 0) = 0 OR o.uuid = ANY (sqlc.narg(dealer_uuids)::uuid[]))
  AND (sqlc.narg(q)::text IS NULL
       OR v.plate ILIKE '%' || sqlc.narg(q)::text || '%'
       OR w.public_code ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (
    COALESCE(cardinality(sqlc.narg(states)::text[]), 0) = 0
    OR (CASE WHEN w.status = 'void' THEN 'void'
             WHEN w.status = 'active' AND w.end_at > NOW() THEN 'active'
             ELSE 'expired' END) = ANY (sqlc.narg(states)::text[])
  );

-- name: ListFleetPortalUpcomingAppointments :many
-- Scheduled / confirmed appointments of the fleet's vehicles at the visible
-- dealers from now on.
SELECT a.uuid, a.starts_at, a.ends_at, a.status,
       o.uuid AS organization_uuid, o.name AS organization_name,
       v.uuid AS vehicle_uuid, v.plate
FROM appointments a
JOIN vehicles v ON v.id = a.vehicle_id
JOIN organizations o ON o.id = a.organization_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND a.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND a.deleted_at IS NULL
  AND a.status IN ('scheduled', 'confirmed')
  AND a.starts_at >= sqlc.arg(after)::timestamptz
ORDER BY a.starts_at, a.id
LIMIT sqlc.arg(limit_count);

-- name: CountFleetPortalUpcomingAppointments :one
SELECT COUNT(*)::bigint
FROM appointments a
JOIN vehicles v ON v.id = a.vehicle_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND a.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND a.deleted_at IS NULL
  AND a.status IN ('scheduled', 'confirmed')
  AND a.starts_at >= sqlc.arg(after)::timestamptz;

-- name: ListFleetPortalCariEntries :many
-- The fleet's view of its cari in one dealer's ledger: only service income
-- (source service_income) and collections; the dealer's other cari
-- movements never leave the dealer. Reversals stay as their own rows.
SELECT fe.uuid, fe.direction, fe.amount, fe.currency, fe.created_at,
       (fe.reversal_of_id IS NOT NULL)::bool AS is_reversal,
       s.uuid AS service_uuid, s.service_no, s.plate, s.completed_at AS service_completed_at,
       v.uuid AS vehicle_uuid
FROM finance_entries fe
LEFT JOIN services s
    ON fe.source_type = 'service_income' AND s.uuid = fe.source_uuid AND s.organization_id = fe.organization_id
LEFT JOIN vehicles v ON v.id = s.vehicle_id
WHERE fe.cari_id = sqlc.arg(cari_id)::bigint AND fe.organization_id = sqlc.arg(organization_id)
  AND (fe.source_type = 'service_income' OR fe.direction = 'collection')
  AND fe.created_at >= sqlc.arg(period_from) AND fe.created_at < sqlc.arg(period_to)
ORDER BY fe.created_at, fe.id;

-- name: FleetPortalCariBalanceBefore :one
-- The balance of the same rows (service income adds, collections subtract)
-- before a point in time.
SELECT COALESCE(SUM(CASE WHEN direction = 'collection' THEN -amount ELSE amount END), 0)::numeric(18,2)
FROM finance_entries
WHERE cari_id = sqlc.arg(cari_id)::bigint AND organization_id = sqlc.arg(organization_id)
  AND (source_type = 'service_income' OR direction = 'collection')
  AND created_at < sqlc.arg(before);

-- name: ListFleetPortalReports :many
-- Ready reports of the fleet (the portal downloads them). Sort
-- period_start, default -period_start.
SELECT * FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id) AND status = 'ready'
  AND (
    COALESCE(cardinality(sqlc.narg(period_kinds)::text[]), 0) = 0
    OR period_kind = ANY (sqlc.narg(period_kinds)::text[])
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN period_start END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN period_start END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetPortalReports :one
SELECT COUNT(*)::bigint FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id) AND status = 'ready'
  AND (
    COALESCE(cardinality(sqlc.narg(period_kinds)::text[]), 0) = 0
    OR period_kind = ANY (sqlc.narg(period_kinds)::text[])
  );
