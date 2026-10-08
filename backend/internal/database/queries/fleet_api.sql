-- TEC-473 (F5-02b): fleet management API. Fleet users, the dealer's access
-- to a fleet (its active links), the fleet card aggregates and the fleet
-- statement (the fleet cari in the dealer's ledger).

-- name: CreateFleetUser :one
INSERT INTO fleet_users (fleet_org_id, brand_id, user_id, is_primary, invited_by_org_id, invited_by_user_id)
VALUES (
    sqlc.arg(fleet_org_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(is_primary),
    sqlc.narg(invited_by_org_id), sqlc.narg(invited_by_user_id)
)
RETURNING *;

-- name: CountFleetUsersOfFleet :one
-- Every user row of the fleet (disabled included): the first one is the
-- primary user.
SELECT COUNT(*)::bigint FROM fleet_users WHERE fleet_org_id = sqlc.arg(fleet_org_id);

-- name: GetFleetUserByUserID :one
-- The fleet of a signed-in fleet user (portal).
SELECT fu.*, o.uuid AS fleet_uuid
FROM fleet_users fu
JOIN organizations o ON o.id = fu.fleet_org_id AND o.deleted_at IS NULL
WHERE fu.user_id = sqlc.arg(user_id) AND fu.status = 'active';

-- name: GetFleetUserByUUID :one
SELECT * FROM fleet_users
WHERE uuid = sqlc.arg(uuid) AND fleet_org_id = sqlc.arg(fleet_org_id);

-- name: DisableFleetUser :one
UPDATE fleet_users
SET status = 'disabled', disabled_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'active'
RETURNING *;

-- name: ListActiveFleetUserIDs :many
-- Recipients of fleet notifications (link requests).
SELECT fu.user_id
FROM fleet_users fu
JOIN users u ON u.id = fu.user_id AND u.deleted_at IS NULL AND u.status = 'active'
WHERE fu.fleet_org_id = sqlc.arg(fleet_org_id) AND fu.status = 'active'
ORDER BY fu.id;

-- name: ListFleetUsers :many
-- Users of a fleet. Sort (docs/list-contract.md): name | email | status |
-- created_at, default created_at, id tiebreak. Filters: q (name, surname,
-- e-mail), multi-value status.
SELECT fu.id, fu.uuid, fu.is_primary, fu.status, fu.created_at, fu.disabled_at,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.last_login_at
FROM fleet_users fu
JOIN users u ON u.id = fu.user_id
WHERE fu.fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR fu.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN u.name END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'name' THEN u.name END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'email' THEN u.email END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'email' THEN u.email END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN fu.status END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN fu.status END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fu.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN fu.created_at END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN fu.id END DESC,
  fu.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetUsers :one
-- Same filter block as ListFleetUsers.
SELECT COUNT(*)::bigint
FROM fleet_users fu
JOIN users u ON u.id = fu.user_id
WHERE fu.fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR fu.status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListActiveFleetLinksInScope :many
-- The active links of a fleet to the organizations in scope (org_ids NULL:
-- every organization of the brand). A dealer reaches a fleet only through
-- an active link.
SELECT * FROM fleet_dealer_links
WHERE fleet_org_id = sqlc.arg(fleet_org_id) AND status = 'active'
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR dealer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
ORDER BY id;

-- name: CountFleetActiveWarranties :one
-- Active warranties on the fleet's vehicles; service_org_ids limits them
-- to the warranties of those organizations (NULL: all).
SELECT COUNT(*)::bigint
FROM warranties w
JOIN vehicles v ON v.id = w.vehicle_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND v.deleted_at IS NULL
  AND w.status = 'active' AND w.end_at > NOW()
  AND (sqlc.narg(service_org_ids)::bigint[] IS NULL
       OR w.organization_id = ANY (sqlc.narg(service_org_ids)::bigint[]));

-- name: ListFleetRecentServices :many
-- The latest services on the fleet's vehicles, limited to service_org_ids
-- (NULL: all organizations).
SELECT s.uuid, s.service_no, s.status, s.created_at, s.completed_at, s.plate,
       v.uuid AS vehicle_uuid, o.uuid AS organization_uuid, o.name AS organization_name
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
JOIN organizations o ON o.id = s.organization_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND (sqlc.narg(service_org_ids)::bigint[] IS NULL
       OR s.organization_id = ANY (sqlc.narg(service_org_ids)::bigint[]))
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count);

-- name: CountFleetServices :one
SELECT COUNT(*)::bigint
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND (sqlc.narg(service_org_ids)::bigint[] IS NULL
       OR s.organization_id = ANY (sqlc.narg(service_org_ids)::bigint[]));

-- name: ListFleetStatementEntries :many
-- The ledger rows of one fleet cari in [period_from, period_to): service
-- income with its service and vehicle, collections and the other cari
-- movements. Reversals stay as their own (negative) rows.
SELECT fe.uuid, fe.direction, fe.category, fe.amount, fe.currency, fe.source_type, fe.source_uuid,
       fe.description, fe.created_at, (fe.reversal_of_id IS NOT NULL)::bool AS is_reversal,
       s.uuid AS service_uuid, s.service_no, s.plate, s.completed_at AS service_completed_at,
       v.uuid AS vehicle_uuid
FROM finance_entries fe
LEFT JOIN services s
    ON fe.source_type = 'service_income' AND s.uuid = fe.source_uuid AND s.organization_id = fe.organization_id
LEFT JOIN vehicles v ON v.id = s.vehicle_id
WHERE fe.cari_id = sqlc.arg(cari_id)::bigint AND fe.organization_id = sqlc.arg(organization_id)
  AND fe.created_at >= sqlc.arg(period_from) AND fe.created_at < sqlc.arg(period_to)
ORDER BY fe.created_at, fe.id;

-- name: FleetCariBalanceBefore :one
-- The cari balance (income/charge/payment add, expense/collection
-- subtract, like cari_account_balances) before a point in time.
SELECT COALESCE(SUM(CASE WHEN direction IN ('income', 'charge', 'payment') THEN amount ELSE -amount END), 0)::numeric(18,2)
FROM finance_entries
WHERE cari_id = sqlc.arg(cari_id)::bigint AND organization_id = sqlc.arg(organization_id)
  AND created_at < sqlc.arg(before);

-- name: GetFleetVehicleForUpdate :one
-- A vehicle of the fleet, locked (PATCH, removal).
SELECT * FROM vehicles
WHERE uuid = sqlc.arg(uuid) AND fleet_org_id = sqlc.arg(fleet_org_id)::bigint AND deleted_at IS NULL
FOR UPDATE;

-- name: GetVehicleByIDForUpdate :one
SELECT * FROM vehicles WHERE id = sqlc.arg(id) AND deleted_at IS NULL FOR UPDATE;

-- name: VehicleHasServices :one
SELECT EXISTS (SELECT 1 FROM services WHERE vehicle_id = sqlc.arg(vehicle_id))::bool;

-- name: FindCarBrandByName :one
-- Import: a car brand of the catalog by name (case insensitive).
SELECT * FROM car_brands WHERE lower(name) = lower(btrim(sqlc.arg(name)::text)) AND active
ORDER BY id LIMIT 1;

-- name: FindCarModelByName :one
SELECT * FROM car_models
WHERE car_brand_id = sqlc.arg(car_brand_id) AND lower(name) = lower(btrim(sqlc.arg(name)::text)) AND active
ORDER BY id LIMIT 1;

-- name: GetFleetCarModelByID :one
SELECT * FROM car_models WHERE id = sqlc.arg(id);

-- TEC-475 (F5-02d): fleet service plans.

-- name: CreateFleetServicePlan :one
INSERT INTO fleet_service_plans (
    organization_id, brand_id, fleet_org_id, fleet_link_id, title, service_type,
    note, start_date, daily_vehicle_limit, preferred_times, idempotency_key,
    created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(fleet_org_id),
    sqlc.arg(fleet_link_id), sqlc.arg(title), sqlc.arg(service_type),
    sqlc.arg(note), sqlc.arg(start_date), sqlc.arg(daily_vehicle_limit),
    sqlc.arg(preferred_times), sqlc.narg(idempotency_key), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetFleetServicePlanByIdempotency :one
SELECT * FROM fleet_service_plans
WHERE organization_id = sqlc.arg(organization_id)::bigint
  AND idempotency_key = sqlc.arg(idempotency_key)::text;

-- name: GetFleetServicePlanByUUID :one
SELECT * FROM fleet_service_plans
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)::bigint;

-- name: CancelFleetServicePlan :one
UPDATE fleet_service_plans
SET status = 'cancelled',
    cancel_reason = sqlc.narg(cancel_reason),
    cancelled_by_user_id = sqlc.narg(cancelled_by_user_id),
    cancelled_at = NOW()
WHERE id = sqlc.arg(id)::bigint
  AND organization_id = sqlc.arg(organization_id)::bigint
  AND status = 'scheduled'
RETURNING *;

-- name: ListFleetPlanVehicles :many
SELECT v.*, u.uuid AS customer_uuid
FROM vehicles v
JOIN fleet_users fu ON fu.user_id = v.user_id AND fu.fleet_org_id = v.fleet_org_id AND fu.status = 'active'
JOIN users u ON u.id = v.user_id AND u.deleted_at IS NULL
WHERE v.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND v.uuid = ANY(sqlc.arg(vehicle_uuids)::uuid[])
  AND v.deleted_at IS NULL
ORDER BY array_position(sqlc.arg(vehicle_uuids)::uuid[], v.uuid);

-- TEC-477 (F5-02f): the panel reads its plans of a fleet.

-- name: ListFleetServicePlansOfOrg :many
-- Plans of one fleet made by the organization. Sort created_at (default
-- -created_at) or start_date; id tiebreak.
SELECT p.*,
    (SELECT COUNT(*) FROM appointments a
     WHERE a.plan_id = p.id AND a.organization_id = p.organization_id AND a.deleted_at IS NULL)::bigint AS appointment_count,
    (SELECT COUNT(*) FROM appointments a
     WHERE a.plan_id = p.id AND a.organization_id = p.organization_id AND a.deleted_at IS NULL
       AND a.service_id IS NOT NULL)::bigint AS intake_count
FROM fleet_service_plans p
WHERE p.organization_id = sqlc.arg(organization_id)::bigint
  AND p.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR p.status = ANY (sqlc.narg(statuses)::text[])
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::bool THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::bool THEN p.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND NOT sqlc.arg(sort_desc)::bool THEN p.start_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'start_date' AND sqlc.arg(sort_desc)::bool THEN p.start_date END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN p.id END DESC,
  p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFleetServicePlansOfOrg :one
SELECT COUNT(*)::bigint FROM fleet_service_plans p
WHERE p.organization_id = sqlc.arg(organization_id)::bigint
  AND p.fleet_org_id = sqlc.arg(fleet_org_id)::bigint
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR p.status = ANY (sqlc.narg(statuses)::text[])
  );

-- name: ListFleetPlanAppointmentRefs :many
-- Vehicle and draft service references of a plan's appointments (plan
-- detail).
SELECT a.uuid AS appointment_uuid, v.uuid AS vehicle_uuid, v.plate,
    cb.name AS car_brand_name, cm.name AS car_model_name, s.uuid AS service_uuid
FROM appointments a
LEFT JOIN vehicles v ON v.id = a.vehicle_id
LEFT JOIN car_brands cb ON cb.id = v.car_brand_id
LEFT JOIN car_models cm ON cm.id = v.car_model_id
LEFT JOIN services s ON s.id = a.service_id
WHERE a.plan_id = sqlc.arg(plan_id)::bigint
  AND a.organization_id = sqlc.arg(organization_id)::bigint
  AND a.deleted_at IS NULL;

-- name: ListFleetReportsSorted :many
-- Reports of the fleet for the panel (every status). Sort period_start,
-- default -period_start; id tiebreak.
SELECT * FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
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

-- name: CountFleetReportsFiltered :one
SELECT COUNT(*)::bigint FROM fleet_reports
WHERE fleet_org_id = sqlc.arg(fleet_org_id)
  AND (
    COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0
    OR status = ANY (sqlc.narg(statuses)::text[])
  )
  AND (
    COALESCE(cardinality(sqlc.narg(period_kinds)::text[]), 0) = 0
    OR period_kind = ANY (sqlc.narg(period_kinds)::text[])
  );
