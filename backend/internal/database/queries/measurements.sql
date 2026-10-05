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

-- TEC-293 (F3-02a): normalized readings, tires, device registry and the
-- before/after service link. Every query is bounded by the organization.

-- name: InsertMeasurementValue :one
INSERT INTO measurement_values (
    organization_id, brand_id, result_id, place_id, part_type, is_inside,
    position, value_um, interpretation, substrate_type, measured_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(result_id), sqlc.arg(place_id),
    sqlc.arg(part_type), sqlc.arg(is_inside), sqlc.narg(position), sqlc.narg(value_um),
    sqlc.narg(interpretation), sqlc.narg(substrate_type), sqlc.narg(measured_at)
)
RETURNING *;

-- name: ListMeasurementValues :many
SELECT * FROM measurement_values
WHERE result_id = sqlc.arg(result_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY is_inside, place_id, part_type, position NULLS LAST, id;

-- name: DeleteMeasurementValues :exec
DELETE FROM measurement_values
WHERE result_id = sqlc.arg(result_id) AND organization_id = sqlc.arg(organization_id);

-- name: InsertMeasurementTire :one
INSERT INTO measurement_tires (
    organization_id, brand_id, result_id, section, width, profile, diameter,
    maker, season, tread_depth_1_mm, tread_depth_2_mm
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(result_id), sqlc.narg(section),
    sqlc.narg(width), sqlc.narg(profile), sqlc.narg(diameter), sqlc.narg(maker),
    sqlc.narg(season), sqlc.narg(tread_depth_1_mm), sqlc.narg(tread_depth_2_mm)
)
RETURNING *;

-- name: ListMeasurementTires :many
SELECT * FROM measurement_tires
WHERE result_id = sqlc.arg(result_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY id;

-- name: DeleteMeasurementTires :exec
DELETE FROM measurement_tires
WHERE result_id = sqlc.arg(result_id) AND organization_id = sqlc.arg(organization_id);

-- Marks a result normalized and fills the fields parsed from raw.
-- name: MarkMeasurementResultParsed :exec
UPDATE measurement_results
SET parsed_at = NOW(),
    measured_at = sqlc.narg(measured_at),
    device_id = sqlc.narg(device_id),
    body_type = sqlc.narg(body_type)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: SetMeasurementResultPDFKey :exec
UPDATE measurement_results
SET pdf_key = sqlc.narg(pdf_key)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- name: GetMeasurementResultByUUID :one
SELECT * FROM measurement_results
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListMeasurementResultsPanel :many
-- org_ids NULL means the whole brand (brand/all scopes); an empty set
-- (customer scope) matches nothing.
SELECT
    mr.*,
    md.uuid AS device_uuid,
    md.serial AS registry_device_serial,
    md.label AS device_label,
    md.model AS device_model,
    md.is_active AS device_is_active,
    sm.phase AS service_phase,
    sm.link_source AS service_link_source,
    sm.confirmed_at AS service_confirmed_at,
    s.uuid AS service_uuid,
    s.service_no AS service_no,
    o.uuid AS organization_uuid,
    o.name AS organization_name,
    count(*) OVER() AS total_count
FROM measurement_results mr
LEFT JOIN measurement_devices md ON md.id = mr.device_id
LEFT JOIN service_measurements sm ON sm.measurement_result_id = mr.id
LEFT JOIN services s ON s.id = sm.service_id
LEFT JOIN organizations o ON o.id = mr.organization_id
WHERE mr.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR mr.organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(vin)::varchar IS NULL OR mr.vin = sqlc.narg(vin)::varchar)
  AND (sqlc.narg(device_uuid)::uuid IS NULL OR md.uuid = sqlc.narg(device_uuid)::uuid)
  AND (sqlc.narg(status)::varchar IS NULL OR mr.status = sqlc.narg(status)::varchar)
  AND (sqlc.narg(linked)::boolean IS NULL OR (sm.id IS NOT NULL) = sqlc.narg(linked)::boolean)
  AND (sqlc.narg(measured_from)::timestamptz IS NULL OR COALESCE(mr.measured_at, mr.created_at) >= sqlc.narg(measured_from)::timestamptz)
  AND (sqlc.narg(measured_to)::timestamptz IS NULL OR COALESCE(mr.measured_at, mr.created_at) < sqlc.narg(measured_to)::timestamptz)
ORDER BY COALESCE(mr.measured_at, mr.created_at) DESC, mr.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetMeasurementResultPanel :one
SELECT
    mr.*,
    md.uuid AS device_uuid,
    md.serial AS registry_device_serial,
    md.label AS device_label,
    md.model AS device_model,
    md.is_active AS device_is_active,
    sm.phase AS service_phase,
    sm.link_source AS service_link_source,
    sm.confirmed_at AS service_confirmed_at,
    s.uuid AS service_uuid,
    s.service_no AS service_no,
    o.uuid AS organization_uuid,
    o.name AS organization_name
FROM measurement_results mr
LEFT JOIN measurement_devices md ON md.id = mr.device_id
LEFT JOIN service_measurements sm ON sm.measurement_result_id = mr.id
LEFT JOIN services s ON s.id = sm.service_id
LEFT JOIN organizations o ON o.id = mr.organization_id
WHERE mr.uuid = sqlc.arg(uuid)
  AND mr.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR mr.organization_id = ANY(sqlc.narg(org_ids)::bigint[]));

-- name: ListMeasurementDevices :many
SELECT * FROM measurement_devices
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY is_active DESC, serial;

-- name: GetMeasurementDeviceByUUID :one
SELECT * FROM measurement_devices
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: GetMeasurementDeviceBySerial :one
SELECT * FROM measurement_devices
WHERE organization_id = sqlc.arg(organization_id) AND serial = sqlc.arg(serial);

-- name: CreateMeasurementDevice :one
INSERT INTO measurement_devices (organization_id, brand_id, serial, label, model, is_active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(serial), sqlc.narg(label),
    sqlc.narg(model), sqlc.arg(is_active)
)
RETURNING *;

-- name: UpdateMeasurementDevice :one
UPDATE measurement_devices
SET label = sqlc.narg(label),
    model = sqlc.narg(model),
    is_active = sqlc.arg(is_active)
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: UpsertMeasurementDevice :one
INSERT INTO measurement_devices (organization_id, brand_id, serial, label, model, is_active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(serial), sqlc.narg(label),
    sqlc.narg(model), sqlc.arg(is_active)
)
ON CONFLICT (organization_id, serial) DO UPDATE
SET label = EXCLUDED.label, model = EXCLUDED.model, is_active = EXCLUDED.is_active
RETURNING *;

-- name: SetMeasurementDeviceActive :execrows
UPDATE measurement_devices
SET is_active = sqlc.arg(is_active)
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- LinkServiceMeasurement fails with 23505 when the service already has a
-- measurement in the phase or the measurement is linked to another service.
-- name: LinkServiceMeasurement :one
INSERT INTO service_measurements (
    organization_id, brand_id, service_id, measurement_result_id, phase,
    link_source, confirmed_by, confirmed_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(service_id),
    sqlc.arg(measurement_result_id), sqlc.arg(phase), sqlc.arg(link_source),
    sqlc.narg(confirmed_by)::bigint,
    CASE WHEN sqlc.narg(confirmed_by)::bigint IS NULL THEN NULL ELSE NOW() END
)
RETURNING *;

-- name: ConfirmServiceMeasurement :execrows
UPDATE service_measurements
SET confirmed_by = sqlc.arg(confirmed_by), confirmed_at = NOW()
WHERE service_id = sqlc.arg(service_id) AND phase = sqlc.arg(phase)
  AND organization_id = sqlc.arg(organization_id);

-- name: UnlinkServiceMeasurement :execrows
DELETE FROM service_measurements
WHERE service_id = sqlc.arg(service_id) AND phase = sqlc.arg(phase)
  AND organization_id = sqlc.arg(organization_id);

-- name: ListServiceMeasurements :many
SELECT * FROM service_measurements
WHERE service_id = sqlc.arg(service_id) AND organization_id = sqlc.arg(organization_id)
ORDER BY phase DESC;

-- name: GetServiceMeasurementByResult :one
SELECT * FROM service_measurements
WHERE measurement_result_id = sqlc.arg(measurement_result_id)
  AND organization_id = sqlc.arg(organization_id);

-- name: SetServiceMeasurementCheck :exec
UPDATE services
SET measurement_check_required = sqlc.arg(measurement_check_required),
    measurement_checked_at = sqlc.narg(measurement_checked_at)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- TEC-297 (F3-02e): part based before/after micron difference table.

-- name: ListServiceMeasurementDiffParts :many
WITH links AS (
    SELECT sm.phase, sm.measurement_result_id
    FROM service_measurements sm
    WHERE sm.service_id = sqlc.arg(service_id)
      AND sm.organization_id = sqlc.arg(organization_id)
      AND sm.phase IN ('before', 'after')
),
readings AS (
    SELECT
        l.phase,
        mv.place_id,
        mv.part_type,
        (CASE UPPER(mv.part_type)
            WHEN 'HOOD' THEN 'body_kaput'
            WHEN 'ROOF' THEN 'body_tavan'
            WHEN 'TRUNK' THEN 'body_bagaj'
            WHEN 'TRUNK_INSIDE' THEN 'body_bagaj'
            WHEN 'LEFT_FRONT_DOOR' THEN 'body_sol_on_kapi'
            WHEN 'LEFT_REAR_DOOR' THEN 'body_sol_arka_kapi'
            WHEN 'RIGHT_FRONT_DOOR' THEN 'body_sag_on_kapi'
            WHEN 'RIGHT_REAR_DOOR' THEN 'body_sag_arka_kapi'
            WHEN 'LEFT_FRONT_FENDER' THEN 'body_sol_on_camurluk'
            WHEN 'LEFT_REAR_FENDER' THEN 'body_sol_arka_camurluk'
            WHEN 'RIGHT_FRONT_FENDER' THEN 'body_sag_on_camurluk'
            WHEN 'RIGHT_REAR_FENDER' THEN 'body_sag_arka_camurluk'
            ELSE lower(mv.part_type)
        END)::text AS service_part_key,
        COUNT(mv.value_um)::int AS value_count,
        AVG(mv.value_um)::numeric(8,2) AS avg_um,
        MIN(mv.value_um)::numeric(8,2) AS min_um,
        MAX(mv.value_um)::numeric(8,2) AS max_um
    FROM links l
    JOIN measurement_values mv ON mv.result_id = l.measurement_result_id
       AND mv.organization_id = sqlc.arg(organization_id)
    WHERE mv.value_um IS NOT NULL
    GROUP BY l.phase, mv.place_id, mv.part_type
),
pairs AS (
    SELECT
        COALESCE(b.place_id, a.place_id) AS place_id,
        COALESCE(b.part_type, a.part_type) AS part_type,
        COALESCE(b.service_part_key, a.service_part_key) AS service_part_key,
        b.avg_um AS before_avg_um,
        b.min_um AS before_min_um,
        b.max_um AS before_max_um,
        COALESCE(b.value_count, 0)::int AS before_count,
        a.avg_um AS after_avg_um,
        a.min_um AS after_min_um,
        a.max_um AS after_max_um,
        COALESCE(a.value_count, 0)::int AS after_count
    FROM (SELECT * FROM readings WHERE phase = 'before') b
    FULL OUTER JOIN (SELECT * FROM readings WHERE phase = 'after') a
      ON a.place_id = b.place_id AND a.part_type = b.part_type
),
expected AS (
    SELECT
        pairs.*,
        (
            SELECT SUM(p.micron_thickness)::numeric(8,2)
            FROM service_items si
            JOIN products p ON p.id = si.product_id AND p.brand_id = si.brand_id
            WHERE si.service_id = sqlc.arg(service_id)
              AND si.organization_id = sqlc.arg(organization_id)
              AND si.applied_parts ? pairs.service_part_key
              AND p.micron_thickness IS NOT NULL
        ) AS expected_um
    FROM pairs
)
SELECT
    place_id,
    part_type,
    service_part_key,
    before_avg_um,
    before_min_um,
    before_max_um,
    before_count,
    after_avg_um,
    after_min_um,
    after_max_um,
    after_count,
    CASE
        WHEN before_avg_um IS NULL OR after_avg_um IS NULL THEN NULL
        ELSE (after_avg_um - before_avg_um)::numeric(8,2)
    END AS diff_um,
    expected_um
FROM expected
ORDER BY service_part_key, place_id, part_type;

-- name: MarkServiceMeasurementChecked :execrows
UPDATE services
SET measurement_checked_at = NOW()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id);

-- TEC-296 (F3-02d): VIN based before/after matching, dealer confirmation
-- and manual selection.

-- The service a measurement is matched to; FOR UPDATE serializes the
-- matching of one service (auto rule, confirmation, manual selection).
-- name: GetServiceForMeasurementMatch :one
SELECT id, uuid, organization_id, brand_id, vin, has_measurement, status, created_at, completed_at
FROM services
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: GetServiceForMeasurementMatchByUUID :one
SELECT id, uuid, organization_id, brand_id, vin, has_measurement, status, created_at, completed_at
FROM services
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- Unlinked accepted measurements of the organization with the VIN; the
-- matching rule (usecase.Match) decides the phase from measured_at (device
-- time, falling back to the upload time).
-- name: ListMeasurementMatchCandidates :many
SELECT mr.id, mr.uuid, mr.vin, mr.status, mr.source, mr.device_serial, mr.measured_at, mr.created_at
FROM measurement_results mr
WHERE mr.organization_id = sqlc.arg(organization_id)
  AND mr.vin = sqlc.arg(vin)
  AND mr.status = 'accepted'
  AND NOT EXISTS (SELECT 1 FROM service_measurements sm WHERE sm.measurement_result_id = mr.id)
ORDER BY COALESCE(mr.measured_at, mr.created_at), mr.id;

-- The before/after links of a service with the linked measurement and the
-- confirming user.
-- name: ListServiceMeasurementLinks :many
SELECT
    sm.phase, sm.link_source, sm.confirmed_at, sm.created_at AS linked_at,
    mr.id AS measurement_id, mr.uuid AS measurement_uuid, mr.vin, mr.status, mr.source,
    mr.device_serial, mr.measured_at, mr.created_at AS measurement_created_at,
    u.uuid AS confirmed_by_uuid, u.name AS confirmed_by_name, u.surname AS confirmed_by_surname
FROM service_measurements sm
JOIN measurement_results mr ON mr.id = sm.measurement_result_id
LEFT JOIN users u ON u.id = sm.confirmed_by
WHERE sm.service_id = sqlc.arg(service_id) AND sm.organization_id = sqlc.arg(organization_id)
ORDER BY sm.phase DESC;

-- The services of the organization a newly accepted measurement may belong
-- to (measurement expected, same VIN, not cancelled).
-- name: ListServicesForMeasurementMatch :many
SELECT id FROM services
WHERE organization_id = sqlc.arg(organization_id)
  AND vin = sqlc.arg(vin)
  AND has_measurement
  AND status <> 'cancelled'
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: GetMeasurementResultForLink :one
SELECT id, uuid, organization_id, vin, status FROM measurement_results
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- Replaces an unconfirmed link of the phase with a manually chosen one.
-- name: ReplaceServiceMeasurement :execrows
UPDATE service_measurements
SET measurement_result_id = sqlc.arg(measurement_result_id),
    link_source = 'manual',
    confirmed_by = sqlc.arg(confirmed_by),
    confirmed_at = NOW()
WHERE service_id = sqlc.arg(service_id) AND phase = sqlc.arg(phase)
  AND organization_id = sqlc.arg(organization_id);

-- TEC-294 (F3-02b): NexPTG normalization, device auto-registration, VIN
-- completion and the reparse backfill.

-- A device first seen in an upload is registered to the organization; a
-- concurrent upload of the same serial wins the insert (no row then).
-- name: InsertMeasurementDeviceIfAbsent :one
INSERT INTO measurement_devices (organization_id, brand_id, serial, model)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(serial), sqlc.narg(model))
ON CONFLICT (organization_id, serial) DO NOTHING
RETURNING *;

-- The next page of results still waiting for normalization (keyset by id).
-- name: ListUnparsedMeasurementResultIDs :many
SELECT id FROM measurement_results
WHERE parsed_at IS NULL AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(limit_count);

-- Locks one unparsed result for normalization; no row when another run
-- normalized it meanwhile.
-- name: LockUnparsedMeasurementResult :one
SELECT * FROM measurement_results
WHERE id = sqlc.arg(id) AND parsed_at IS NULL
FOR UPDATE;

-- Completes the VIN of a vin_pending result (status accepted).
-- name: CompleteMeasurementResultVIN :execrows
UPDATE measurement_results
SET vin = sqlc.arg(vin), status = 'accepted'
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'vin_pending';
