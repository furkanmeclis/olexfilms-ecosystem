-- TEC-476 (F5-02e): periodic fleet reports. The scheduler (worker-core)
-- walks the fleets with a report frequency, creates one fleet_reports row
-- per (fleet, period) and the docs worker renders it. The report content
-- reads the services, warranties and parts of the dealers that have the
-- fleet module (dealer_ids).

-- name: ListFleetsDueForReport :many
-- Fleets with a report frequency (monthly / quarterly), keyset by
-- organization id.
SELECT o.id, o.uuid, o.brand_id, o.timezone, o.created_at,
       fp.report_frequency, fp.report_locale
FROM fleet_profiles fp
JOIN organizations o ON o.id = fp.organization_id
WHERE fp.report_frequency <> 'off'
  AND o.type = 'fleet' AND o.deleted_at IS NULL
  AND o.id > sqlc.arg(after_id)
ORDER BY o.id
LIMIT sqlc.arg(limit_count);

-- name: CreateFleetReportIfAbsent :one
-- The scheduled report of a period: no row when the period already has one
-- (whatever its status), so two ticks create a single report.
INSERT INTO fleet_reports (fleet_org_id, brand_id, period_kind, period_start, period_end, locale)
VALUES (
    sqlc.arg(fleet_org_id), sqlc.arg(brand_id), sqlc.arg(period_kind),
    sqlc.arg(period_start), sqlc.arg(period_end), sqlc.arg(locale)
)
ON CONFLICT (fleet_org_id, period_kind, period_start) DO NOTHING
RETURNING *;

-- name: GetFleetReportByID :one
SELECT * FROM fleet_reports WHERE id = sqlc.arg(id);

-- name: ListStalePendingFleetReports :many
-- Pending reports whose generation task may have been lost (enqueue
-- failure): the scheduler enqueues them again (task id dedupe). Database
-- clock: older than 15 minutes.
SELECT id FROM fleet_reports
WHERE status = 'pending' AND created_at < NOW() - INTERVAL '15 minutes'
ORDER BY created_at, id
LIMIT sqlc.arg(limit_count);

-- name: ListActiveFleetUserEmails :many
-- E-mail addresses of the fleet's active users (report recipients).
SELECT u.email::text AS email
FROM fleet_users fu
JOIN users u ON u.id = fu.user_id AND u.deleted_at IS NULL AND u.status = 'active'
  AND u.email IS NOT NULL
WHERE fu.fleet_org_id = sqlc.arg(fleet_org_id) AND fu.status = 'active'
ORDER BY fu.id;

-- name: CountFleetReportVehicles :one
SELECT COUNT(*)::bigint FROM vehicles
WHERE fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND deleted_at IS NULL;

-- name: FleetReportServicesByDealer :many
-- Services completed in [period_from, period_to) on the fleet's vehicles,
-- per dealer.
SELECT o.id AS organization_id, o.name AS organization_name, COUNT(*)::bigint AS service_count
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN organizations o ON o.id = s.organization_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status = 'completed'
  AND s.completed_at >= sqlc.arg(period_from) AND s.completed_at < sqlc.arg(period_to)
GROUP BY o.id, o.name
ORDER BY service_count DESC, o.name, o.id;

-- name: FleetReportServicesByVehicle :many
-- Same services per vehicle (plate, car brand / model).
SELECT v.id AS vehicle_id, COALESCE(v.plate, '')::text AS plate,
       COALESCE(cb.name, '')::text AS car_brand_name, COALESCE(cm.name, '')::text AS car_model_name,
       COUNT(*)::bigint AS service_count
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status = 'completed'
  AND s.completed_at >= sqlc.arg(period_from) AND s.completed_at < sqlc.arg(period_to)
GROUP BY v.id, v.plate, cb.name, cm.name
ORDER BY service_count DESC, v.plate, v.id;

-- name: FleetReportParts :many
-- Part distribution: applied_parts keys of the period's service items,
-- counted per key.
SELECT part.key::text AS part_key, COUNT(*)::bigint AS part_count
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN service_items si ON si.service_id = s.id
CROSS JOIN LATERAL jsonb_array_elements_text(si.applied_parts) AS part(key)
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status = 'completed'
  AND s.completed_at >= sqlc.arg(period_from) AND s.completed_at < sqlc.arg(period_to)
GROUP BY part.key
ORDER BY part_count DESC, part_key;

-- name: FleetReportProducts :many
-- Products used: items, pieces and meters per product.
SELECT p.id AS product_id, p.name AS product_name, p.sku AS product_sku,
       COUNT(*)::bigint AS item_count,
       COALESCE(SUM(si.quantity), 0)::bigint AS quantity,
       COALESCE(SUM(si.meters), 0)::numeric(18,2) AS meters
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN service_items si ON si.service_id = s.id
JOIN products p ON p.id = si.product_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND s.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND s.status = 'completed'
  AND s.completed_at >= sqlc.arg(period_from) AND s.completed_at < sqlc.arg(period_to)
GROUP BY p.id, p.name, p.sku
ORDER BY item_count DESC, p.name, p.id;

-- name: FleetReportWarrantyCounts :one
-- Warranty state at the end of the period (period_to): active (started,
-- not void, not past end_at), expired (ended by period_to, not void) and
-- started within the period.
SELECT
  COUNT(*) FILTER (WHERE w.status <> 'void' AND w.start_at < sqlc.arg(period_to) AND w.end_at > sqlc.arg(period_to))::bigint AS active_count,
  COUNT(*) FILTER (WHERE w.status <> 'void' AND w.end_at <= sqlc.arg(period_to))::bigint AS expired_count,
  COUNT(*) FILTER (WHERE w.start_at >= sqlc.arg(period_from) AND w.start_at < sqlc.arg(period_to))::bigint AS started_count
FROM warranties w
JOIN vehicles v ON v.id = w.vehicle_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[]);

-- name: FleetReportUpcomingExpirations :many
-- Warranties still running at period_to that end before until.
SELECT w.public_code, w.end_at, COALESCE(v.plate, '')::text AS plate, p.name AS product_name
FROM warranties w
JOIN vehicles v ON v.id = w.vehicle_id
JOIN products p ON p.id = w.product_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND w.organization_id = ANY (sqlc.arg(dealer_ids)::bigint[])
  AND w.status <> 'void'
  AND w.end_at > sqlc.arg(period_to) AND w.end_at <= sqlc.arg(until)
ORDER BY w.end_at, w.id
LIMIT sqlc.arg(limit_count);
