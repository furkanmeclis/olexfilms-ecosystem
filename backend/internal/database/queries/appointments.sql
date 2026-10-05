-- TEC-322 (F3-04a): appointment schema (migration 000090). Reads are
-- bounded by resolved organization ids; the API layer owns scope resolution.

-- name: UpsertAppointmentSettings :one
INSERT INTO appointment_settings (
    organization_id, brand_id, daily_vehicle_capacity, default_estimated_minutes,
    slot_interval_minutes, working_hours, portal_appointments_enabled
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(daily_vehicle_capacity),
    sqlc.arg(default_estimated_minutes), sqlc.arg(slot_interval_minutes),
    sqlc.arg(working_hours), sqlc.arg(portal_appointments_enabled)
)
ON CONFLICT (organization_id) DO UPDATE
SET daily_vehicle_capacity = EXCLUDED.daily_vehicle_capacity,
    default_estimated_minutes = EXCLUDED.default_estimated_minutes,
    slot_interval_minutes = EXCLUDED.slot_interval_minutes,
    working_hours = EXCLUDED.working_hours,
    portal_appointments_enabled = EXCLUDED.portal_appointments_enabled
RETURNING *;

-- name: GetAppointmentSettings :one
SELECT * FROM appointment_settings
WHERE organization_id = sqlc.arg(organization_id);

-- name: GetPortalAppointmentDealer :one
-- Portal booking accepts only active dealers of the request brand whose
-- appointment settings explicitly allow portal bookings.
SELECT o.*, s.portal_appointments_enabled
FROM organizations o
JOIN appointment_settings s ON s.organization_id = o.id
WHERE o.uuid = sqlc.arg(uuid)
  AND o.brand_id = sqlc.arg(brand_id)::bigint
  AND o.deleted_at IS NULL
  AND o.status = 'active'
  AND o.type = 'dealer'
  AND o.access_starts_at <= NOW()
  AND (o.access_ends_at IS NULL OR o.access_ends_at > NOW())
  AND s.portal_appointments_enabled = TRUE;

-- TEC-323: serializes bookings of one organization; the capacity count and
-- the insert run under this row lock so concurrent bookings cannot overfill.
-- name: LockAppointmentSettings :one
SELECT * FROM appointment_settings
WHERE organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: ListAppointmentSettingsByOrganizations :many
SELECT * FROM appointment_settings
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
ORDER BY organization_id;

-- name: CreateAppointmentClosure :one
INSERT INTO appointment_closures (organization_id, brand_id, closed_on, reason)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(closed_on), sqlc.arg(reason))
RETURNING *;

-- name: ListAppointmentClosures :many
SELECT * FROM appointment_closures
WHERE organization_id = sqlc.arg(organization_id)
  AND closed_on >= sqlc.arg(from_date)::date
  AND closed_on <= sqlc.arg(to_date)::date
ORDER BY closed_on, id;

-- name: AppointmentClosureExists :one
SELECT EXISTS (
    SELECT 1 FROM appointment_closures
    WHERE organization_id = sqlc.arg(organization_id) AND closed_on = sqlc.arg(closed_on)::date
)::boolean AS closed;

-- name: DeleteAppointmentClosure :execrows
DELETE FROM appointment_closures
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: DeleteAppointmentClosureByUUID :execrows
DELETE FROM appointment_closures
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: CreateAppointment :one
INSERT INTO appointments (
    organization_id, brand_id, customer_user_id, vehicle_id, starts_at, ends_at,
    estimated_minutes, source, status, cancel_reason, lead_id, service_id, note,
    created_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(customer_user_id),
    sqlc.narg(vehicle_id), sqlc.arg(starts_at), sqlc.arg(ends_at),
    sqlc.arg(estimated_minutes), sqlc.arg(source), sqlc.arg(status),
    sqlc.narg(cancel_reason), sqlc.narg(lead_id), sqlc.narg(service_id),
    sqlc.arg(note), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetAppointmentByUUID :one
SELECT * FROM appointments
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id) AND deleted_at IS NULL;

-- name: GetPortalAppointmentByUUID :one
SELECT * FROM appointments
WHERE uuid = sqlc.arg(uuid)
  AND customer_user_id = sqlc.arg(customer_user_id)::bigint
  AND brand_id = sqlc.arg(brand_id)::bigint
  AND deleted_at IS NULL;

-- name: LockAppointmentByID :one
SELECT * FROM appointments
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: GetAppointmentByID :one
SELECT * FROM appointments
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL;

-- name: ListAppointmentsByOrganizations :many
SELECT * FROM appointments
WHERE (sqlc.narg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND starts_at >= sqlc.arg(from_time)::timestamptz
  AND starts_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar)
ORDER BY starts_at, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListPortalAppointments :many
SELECT a.*
FROM appointments a
JOIN organizations o ON o.id = a.organization_id
WHERE a.customer_user_id = sqlc.arg(customer_user_id)::bigint
  AND a.brand_id = sqlc.arg(brand_id)::bigint
  AND a.deleted_at IS NULL
  AND (sqlc.arg(upcoming)::boolean = FALSE OR a.starts_at >= sqlc.arg(now)::timestamptz)
  AND (sqlc.arg(past)::boolean = FALSE OR a.starts_at < sqlc.arg(now)::timestamptz)
ORDER BY
  CASE WHEN sqlc.arg(upcoming)::boolean THEN a.starts_at END ASC,
  CASE WHEN sqlc.arg(past)::boolean THEN a.starts_at END DESC,
  a.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPortalAppointments :one
SELECT COUNT(*)::bigint
FROM appointments a
WHERE a.customer_user_id = sqlc.arg(customer_user_id)::bigint
  AND a.brand_id = sqlc.arg(brand_id)::bigint
  AND a.deleted_at IS NULL
  AND (sqlc.arg(upcoming)::boolean = FALSE OR a.starts_at >= sqlc.arg(now)::timestamptz)
  AND (sqlc.arg(past)::boolean = FALSE OR a.starts_at < sqlc.arg(now)::timestamptz);

-- name: CountAppointmentsByOrganizations :one
SELECT COUNT(*) FROM appointments
WHERE (sqlc.narg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND starts_at >= sqlc.arg(from_time)::timestamptz
  AND starts_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(status)::varchar IS NULL OR status = sqlc.narg(status)::varchar);

-- name: CountActiveAppointmentsForOrganization :one
SELECT COUNT(*) FROM appointments
WHERE organization_id = sqlc.arg(organization_id)
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND status IN ('scheduled', 'confirmed', 'arrived')
  AND starts_at >= sqlc.arg(from_time)::timestamptz
  AND starts_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id)::bigint);

-- name: CountActiveAppointmentsByOrganization :many
SELECT organization_id, COUNT(*)::bigint AS active_count
FROM appointments
WHERE organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND brand_id = sqlc.arg(brand_id)
  AND deleted_at IS NULL
  AND status IN ('scheduled', 'confirmed', 'arrived')
  AND starts_at >= sqlc.arg(from_time)::timestamptz
  AND starts_at < sqlc.arg(to_time)::timestamptz
GROUP BY organization_id
ORDER BY organization_id;

-- name: UpdateAppointment :one
UPDATE appointments
SET customer_user_id = sqlc.arg(customer_user_id),
    vehicle_id = sqlc.narg(vehicle_id),
    starts_at = sqlc.arg(starts_at),
    ends_at = sqlc.arg(ends_at),
    estimated_minutes = sqlc.arg(estimated_minutes),
    source = sqlc.arg(source),
    lead_id = sqlc.narg(lead_id),
    service_id = sqlc.narg(service_id),
    note = sqlc.arg(note)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: SetAppointmentStatus :one
UPDATE appointments
SET status = sqlc.arg(status),
    cancel_reason = sqlc.narg(cancel_reason)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: MarkAppointmentReminder24h :one
UPDATE appointments
SET reminded_24h_at = COALESCE(reminded_24h_at, sqlc.arg(sent_at)::timestamptz)
WHERE id = sqlc.arg(id) AND reminded_24h_at IS NULL
RETURNING *;

-- name: MarkAppointmentReminder2h :one
UPDATE appointments
SET reminded_2h_at = COALESCE(reminded_2h_at, sqlc.arg(sent_at)::timestamptz)
WHERE id = sqlc.arg(id) AND reminded_2h_at IS NULL
RETURNING *;

-- name: SoftDeleteAppointment :execrows
UPDATE appointments
SET deleted_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL;
