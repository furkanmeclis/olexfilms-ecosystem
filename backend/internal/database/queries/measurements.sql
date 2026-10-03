-- InsertMeasurementResult skips the insert when the idempotency key or the
-- client_measurement_id was already used in the organization (no row then).
-- name: InsertMeasurementResult :one
INSERT INTO measurement_results (
    organization_id, brand_id, service_id, vehicle_id, vin, status, raw,
    client_measurement_id, idempotency_key, device_serial, source, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(service_id), sqlc.narg(vehicle_id),
    sqlc.narg(vin), sqlc.arg(status), sqlc.arg(raw),
    sqlc.narg(client_measurement_id), sqlc.narg(idempotency_key), sqlc.narg(device_serial),
    sqlc.arg(source), sqlc.narg(created_by)
)
ON CONFLICT DO NOTHING
RETURNING *;

-- The first upload of a repeated idempotency key or client_measurement_id.
-- name: FindMeasurementResultByKeys :one
SELECT * FROM measurement_results
WHERE organization_id = sqlc.arg(organization_id)
  AND ((sqlc.narg(idempotency_key)::varchar IS NOT NULL AND idempotency_key = sqlc.narg(idempotency_key)::varchar)
    OR (sqlc.narg(client_measurement_id)::varchar IS NOT NULL AND client_measurement_id = sqlc.narg(client_measurement_id)::varchar))
ORDER BY id
LIMIT 1;

-- The service a measurement is attached to, bounded by the active
-- organization (a service of another organization is not found).
-- name: GetServiceForMeasurement :one
SELECT id, vehicle_id FROM services
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id) AND brand_id = sqlc.arg(brand_id);
