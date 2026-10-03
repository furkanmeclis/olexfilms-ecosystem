-- TEC-262: migrator step 8 (hub nexptg_api_users -> measurement_devices,
-- nexptg_reports + nexptg_report_measurements + service_nexptg_report ->
-- measurement_results). Written only by cmd/migrator inside a step
-- transaction.

-- name: MigratorServiceForMeasurement :one
SELECT id, organization_id, brand_id, vehicle_id FROM services WHERE uuid = $1;

-- name: MigratorMeasurementDeviceByUUID :one
SELECT id, organization_id, serial, label FROM measurement_devices WHERE uuid = $1;

-- name: MigratorMeasurementDeviceBySerial :one
SELECT id, uuid FROM measurement_devices
WHERE organization_id = sqlc.arg(organization_id)::bigint AND serial = sqlc.arg(serial)::varchar;

-- name: MigratorInsertMeasurementDevice :one
-- A device the organization already holds under the serial is left alone
-- (no row then).
INSERT INTO measurement_devices (uuid, organization_id, brand_id, serial, label, created_at, updated_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint, sqlc.arg(serial)::varchar,
    sqlc.narg(label)::varchar, COALESCE(sqlc.narg(created_at)::timestamptz, NOW()),
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
ON CONFLICT (organization_id, serial) DO NOTHING
RETURNING id;

-- name: MigratorUpdateMeasurementDevice :exec
UPDATE measurement_devices
SET organization_id = sqlc.arg(organization_id)::bigint,
    brand_id        = sqlc.arg(brand_id)::bigint,
    serial          = sqlc.arg(serial)::varchar,
    label           = sqlc.narg(label)::varchar
WHERE id = sqlc.arg(id)::bigint;

-- name: MigratorMeasurementResultIDByUUID :one
SELECT id FROM measurement_results WHERE uuid = $1;

-- name: MigratorInsertMeasurementResult :one
-- Legacy reports carry no idempotency key / client id; the uuid (from
-- migration_map) makes a rerun find the row.
INSERT INTO measurement_results (
    uuid, organization_id, brand_id, service_id, vehicle_id, vin, status, raw, device_serial, source,
    created_by, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint, sqlc.narg(service_id)::bigint,
    sqlc.narg(vehicle_id)::bigint, sqlc.narg(vin)::varchar, sqlc.arg(status)::varchar, sqlc.arg(raw)::jsonb,
    sqlc.narg(device_serial)::varchar, 'legacy_import', sqlc.narg(created_by)::bigint,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
ON CONFLICT (uuid) DO NOTHING
RETURNING id;

-- name: MigratorUpdateMeasurementResult :exec
UPDATE measurement_results
SET organization_id = sqlc.arg(organization_id)::bigint,
    brand_id        = sqlc.arg(brand_id)::bigint,
    service_id      = sqlc.narg(service_id)::bigint,
    vehicle_id      = sqlc.narg(vehicle_id)::bigint,
    vin             = sqlc.narg(vin)::varchar,
    status          = sqlc.arg(status)::varchar,
    raw             = sqlc.arg(raw)::jsonb,
    device_serial   = sqlc.narg(device_serial)::varchar,
    created_by      = sqlc.narg(created_by)::bigint
WHERE id = sqlc.arg(id)::bigint AND source = 'legacy_import';
