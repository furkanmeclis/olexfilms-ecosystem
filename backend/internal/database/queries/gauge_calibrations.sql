-- TEC-939: SmartPTG zero calibrations (append-only ledger). Every query is
-- bounded by the active organization.

-- InsertGaugeCalibration skips the insert when the client_id was already
-- used in the organization (no row then: read it with FindGaugeCalibrationByClientID).
-- name: InsertGaugeCalibration :one
INSERT INTO gauge_calibrations (
    organization_id, brand_id, serial, fe_offset_hz, al_offset_hz, target_um,
    samples, client_id, calibrated_at, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(serial), sqlc.narg(fe_offset_hz),
    sqlc.narg(al_offset_hz), sqlc.arg(target_um), sqlc.arg(samples), sqlc.narg(client_id),
    sqlc.arg(calibrated_at), sqlc.arg(created_by)
)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: FindGaugeCalibrationByClientID :one
SELECT id FROM gauge_calibrations
WHERE organization_id = sqlc.arg(organization_id) AND client_id = sqlc.arg(client_id);

-- name: GetGaugeCalibration :one
SELECT gc.uuid, gc.serial, gc.fe_offset_hz, gc.al_offset_hz, gc.target_um, gc.samples,
       gc.calibrated_at, gc.created_at, u.uuid AS created_by_uuid, u.name AS created_by_name,
       u.surname AS created_by_surname
FROM gauge_calibrations gc
JOIN users u ON u.id = gc.created_by
WHERE gc.id = sqlc.arg(id) AND gc.organization_id = sqlc.arg(organization_id);

-- The current calibration of a serial: the latest one of the organization.
-- name: GetLatestGaugeCalibration :one
SELECT gc.uuid, gc.serial, gc.fe_offset_hz, gc.al_offset_hz, gc.target_um, gc.samples,
       gc.calibrated_at, gc.created_at, u.uuid AS created_by_uuid, u.name AS created_by_name,
       u.surname AS created_by_surname
FROM gauge_calibrations gc
JOIN users u ON u.id = gc.created_by
WHERE gc.organization_id = sqlc.arg(organization_id) AND gc.serial = sqlc.arg(serial)
ORDER BY gc.calibrated_at DESC, gc.id DESC
LIMIT 1;
