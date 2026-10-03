-- TEC-260: migrator step 7b (hub warranties -> warranties, legacy numbers ->
-- public codes / aliases, hub service_customer_transfers -> completed
-- vehicle_transfers). Written only by cmd/migrator inside a step
-- transaction. Nothing here writes the outbox: no warranty event, no
-- reminder and no transfer message is sent for a historical record.

-- name: MigratorWarrantyServiceItem :one
-- The service item a legacy warranty covers, with the service state the
-- warranty copies (vehicle, customer) and the organization's time zone.
SELECT si.id, si.service_id, si.organization_id, si.brand_id, si.product_id, si.unit_id, si.kind,
       s.status AS service_status, s.vehicle_id, s.customer_user_id, o.timezone,
       EXISTS (SELECT 1 FROM service_item_corrections c WHERE c.service_item_id = si.id)::boolean AS corrected
FROM service_items si
JOIN services s ON s.id = si.service_id
JOIN organizations o ON o.id = si.organization_id
WHERE si.uuid = $1;

-- name: MigratorWarrantyByUUID :one
SELECT id, uuid, public_code, service_id, organization_id, brand_id, status, end_at
FROM warranties WHERE uuid = $1;

-- name: MigratorWarrantyByServiceItem :one
SELECT id, uuid FROM warranties WHERE service_item_id = $1;

-- name: MigratorActiveFullWarranty :one
-- The active full-unit warranty of a vehicle and unit
-- (uq_warranties_active_full_unit).
SELECT id, uuid FROM warranties
WHERE vehicle_id = sqlc.arg(vehicle_id)::bigint AND unit_id = sqlc.arg(unit_id)::bigint
  AND status = 'active' AND item_kind = 'full';

-- name: MigratorPublicCodeOwner :one
-- The service of the warranty a code resolves to (its public_code or an
-- alias); no row means the code is free.
SELECT w.service_id FROM warranties w WHERE w.public_code = sqlc.arg(code)::text
UNION ALL
SELECT w.service_id FROM warranty_public_code_aliases a JOIN warranties w ON w.id = a.warranty_id
WHERE a.code = sqlc.arg(code)::text
LIMIT 1;

-- name: MigratorInsertWarranty :one
-- A NULL public_code takes a random 22 character code (the column default).
INSERT INTO warranties (
    uuid, public_code, organization_id, brand_id, service_id, service_item_id, product_id, unit_id, item_kind,
    vehicle_id, holder_user_id, start_at, end_at, status, expired_at, voided_at, void_reason,
    notified_30_at, notified_7_at, created_at, updated_at
)
VALUES (
    sqlc.arg(uuid),
    COALESCE(sqlc.narg(public_code)::varchar,
             rtrim(translate(encode(uuid_send(gen_random_uuid()), 'base64'), '+/', '-_'), '=')),
    sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint, sqlc.arg(service_id)::bigint,
    sqlc.arg(service_item_id)::bigint, sqlc.arg(product_id)::bigint, sqlc.arg(unit_id)::bigint,
    sqlc.arg(item_kind)::varchar, sqlc.arg(vehicle_id)::bigint, sqlc.arg(holder_user_id)::bigint,
    sqlc.arg(start_at)::timestamptz, sqlc.arg(end_at)::timestamptz, sqlc.arg(status)::varchar,
    sqlc.narg(expired_at)::timestamptz, sqlc.narg(voided_at)::timestamptz, sqlc.narg(void_reason)::text,
    sqlc.narg(notified_30_at)::timestamptz, sqlc.narg(notified_7_at)::timestamptz,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW()),
    COALESCE(sqlc.narg(updated_at)::timestamptz, sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id, public_code;

-- name: MigratorUpdateActiveWarranty :execrows
-- A changed legacy warranty that is still active here: the end (extension)
-- and the status may change; the period start and the item never do.
UPDATE warranties
SET end_at = sqlc.arg(end_at)::timestamptz, status = sqlc.arg(status)::varchar,
    expired_at = sqlc.narg(expired_at)::timestamptz, voided_at = sqlc.narg(voided_at)::timestamptz,
    void_reason = sqlc.narg(void_reason)::text,
    notified_30_at = COALESCE(notified_30_at, sqlc.narg(notified_30_at)::timestamptz),
    notified_7_at = COALESCE(notified_7_at, sqlc.narg(notified_7_at)::timestamptz)
WHERE id = sqlc.arg(id)::bigint AND status = 'active';

-- name: MigratorWarrantyAliasByCode :one
SELECT id, uuid, warranty_id FROM warranty_public_code_aliases WHERE code = sqlc.arg(code)::text;

-- name: MigratorInsertWarrantyAlias :one
INSERT INTO warranty_public_code_aliases (uuid, code, warranty_id, organization_id, brand_id, reason, created_at)
SELECT sqlc.arg(uuid), sqlc.arg(code)::varchar, w.id, w.organization_id, w.brand_id, sqlc.arg(reason)::varchar,
       COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
FROM warranties w WHERE w.id = sqlc.arg(warranty_id)::bigint
RETURNING id;

-- name: MigratorTransferService :one
-- The migrated service of a legacy transfer, with its vehicle's owner now.
SELECT s.id, s.organization_id, s.brand_id, s.vehicle_id, v.user_id AS vehicle_user_id
FROM services s
JOIN vehicles v ON v.id = s.vehicle_id
WHERE s.uuid = $1;

-- name: MigratorUserPhone :one
SELECT phone_e164 FROM users WHERE id = $1;

-- name: MigratorVehicleTransferByUUID :one
SELECT id FROM vehicle_transfers WHERE uuid = $1;

-- name: MigratorInsertCompletedVehicleTransfer :one
-- A historical, already completed transfer. The codes were sent and
-- verified in the old hub; they are not carried over, the hashes are a
-- marker no code ever matches (only a pending transfer is verified).
INSERT INTO vehicle_transfers (
    uuid, organization_id, brand_id, vehicle_id, from_user_id, to_user_id, to_phone,
    from_code_hash, to_code_hash, from_verified_at, to_verified_at, attempts, expires_at, status,
    initiated_by_user_id, completed_at, created_at, updated_at
)
VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id)::bigint, sqlc.arg(brand_id)::bigint, sqlc.arg(vehicle_id)::bigint,
    sqlc.arg(from_user_id)::bigint, sqlc.arg(to_user_id)::bigint, sqlc.arg(to_phone)::varchar,
    sqlc.arg(code_hash)::varchar, sqlc.arg(code_hash)::varchar,
    sqlc.arg(completed_at)::timestamptz, sqlc.arg(completed_at)::timestamptz, 0,
    sqlc.arg(expires_at)::timestamptz, 'completed', sqlc.narg(initiated_by_user_id)::bigint,
    sqlc.arg(completed_at)::timestamptz, sqlc.arg(created_at)::timestamptz, sqlc.arg(completed_at)::timestamptz
)
RETURNING id;

-- name: MigratorMoveVehicleOwner :execrows
UPDATE vehicles SET user_id = sqlc.arg(new_user_id)::bigint
WHERE id = sqlc.arg(id)::bigint AND user_id = sqlc.arg(old_user_id)::bigint;

-- name: MigratorSetServiceWarrantyHolder :execrows
-- The active warranties of a transferred service follow the new owner.
UPDATE warranties SET holder_user_id = sqlc.arg(holder_user_id)::bigint
WHERE service_id = sqlc.arg(service_id)::bigint AND status = 'active'
  AND holder_user_id IS DISTINCT FROM sqlc.arg(holder_user_id)::bigint;
