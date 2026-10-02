-- TEC-185 (F1-06a): warranties and vehicle ownership transfers (migration
-- 000051). Panel reads are brand-bound (K20); the public lookup by
-- public_code is brand-bound too (a warranty of another brand is 404).

-- ---------------------------------------------------------------------------
-- Warranties.

-- Idempotent creation from service.completed (decision 3): the item, its
-- service, organization, brand, product, unit and kind come from the
-- service item; a second run returns no row (ON CONFLICT DO NOTHING).
-- name: CreateWarrantyForServiceItem :one
INSERT INTO warranties (
    organization_id, brand_id, service_id, service_item_id, product_id, unit_id,
    item_kind, vehicle_id, holder_user_id, start_at, end_at
)
SELECT si.organization_id, si.brand_id, si.service_id, si.id, si.product_id, si.unit_id,
       si.kind, s.vehicle_id, sqlc.arg(holder_user_id), sqlc.arg(start_at), sqlc.arg(end_at)
FROM service_items si
JOIN services s ON s.id = si.service_id
WHERE si.id = sqlc.arg(service_item_id)
ON CONFLICT (service_item_id) DO NOTHING
RETURNING *;

-- name: GetWarranty :one
SELECT * FROM warranties
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetWarrantyByUUID :one
SELECT * FROM warranties
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetWarrantyByServiceItem :one
SELECT * FROM warranties
WHERE service_item_id = sqlc.arg(service_item_id);

-- Public page /garanti/{public_code} (decision 1).
-- name: GetWarrantyByPublicCode :one
SELECT * FROM warranties
WHERE public_code = sqlc.arg(public_code) AND brand_id = sqlc.arg(brand_id);

-- name: LockWarranty :one
SELECT * FROM warranties
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- Full-unit duplicate guard before creation (decision 3); the partial
-- unique index uq_warranties_active_full_unit is the final barrier.
-- name: GetActiveFullWarrantyByVehicleUnit :one
SELECT * FROM warranties
WHERE vehicle_id = sqlc.arg(vehicle_id) AND unit_id = sqlc.arg(unit_id)
  AND status = 'active' AND item_kind = 'full';

-- All warranties of a service (one PDF per service, decision 2).
-- name: ListWarrantiesByService :many
SELECT * FROM warranties
WHERE service_id = sqlc.arg(service_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY id;

-- name: ListWarrantiesByVehicle :many
SELECT * FROM warranties
WHERE vehicle_id = sqlc.arg(vehicle_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY start_at DESC, id DESC;

-- Scope list: org_ids NULL = whole brand (brand/all scope);
-- holder_user_id for scope customer (portal).
-- name: ListWarrantiesInScope :many
SELECT * FROM warranties
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(holder_user_id)::bigint IS NULL OR holder_user_id = sqlc.narg(holder_user_id))
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY end_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountWarrantiesInScope :one
SELECT COUNT(*) FROM warranties
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(holder_user_id)::bigint IS NULL OR holder_user_id = sqlc.narg(holder_user_id))
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- Center void (warranties.void). Expired warranties may be voided too.
-- name: VoidWarranty :one
UPDATE warranties
SET status = 'void',
    voided_at = NOW(),
    voided_by_user_id = sqlc.narg(actor_user_id),
    void_reason = sqlc.narg(void_reason)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id) AND status IN ('active', 'expired')
RETURNING *;

-- Vehicle transfer (decision 6): the active warranties of the vehicle move
-- to the new owner in the transfer transaction.
-- name: ChangeWarrantyHolderByVehicle :many
UPDATE warranties
SET holder_user_id = sqlc.arg(new_holder_user_id)
WHERE vehicle_id = sqlc.arg(vehicle_id) AND status = 'active'
  AND holder_user_id IS DISTINCT FROM sqlc.arg(new_holder_user_id)
RETURNING *;

-- Daily cron (decision 4/5): end_at is the end of the last covered day in
-- the organization's time zone, so expiry is a plain comparison.
-- name: ExpireDueWarranties :many
UPDATE warranties
SET status = 'expired', expired_at = sqlc.arg(now)
WHERE status = 'active' AND end_at <= sqlc.arg(now)
RETURNING *;

-- Reminder scan, 30 days: active warranties ending within 30 days that were
-- not notified yet (a missed day is caught up by the <= condition).
-- name: ListWarrantiesDue30DayNotice :many
SELECT * FROM warranties
WHERE status = 'active' AND notified_30_at IS NULL
  AND end_at > sqlc.arg(now)
  AND end_at <= sqlc.arg(now)::timestamptz + INTERVAL '30 days'
ORDER BY end_at, id
LIMIT sqlc.arg(row_limit);

-- name: ListWarrantiesDue7DayNotice :many
SELECT * FROM warranties
WHERE status = 'active' AND notified_7_at IS NULL
  AND end_at > sqlc.arg(now)
  AND end_at <= sqlc.arg(now)::timestamptz + INTERVAL '7 days'
ORDER BY end_at, id
LIMIT sqlc.arg(row_limit);

-- Stamped in the notification transaction; a second run is a no-op.
-- name: MarkWarrantyNotified30 :execrows
UPDATE warranties
SET notified_30_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'active' AND notified_30_at IS NULL;

-- name: MarkWarrantyNotified7 :execrows
UPDATE warranties
SET notified_7_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND status = 'active' AND notified_7_at IS NULL;

-- ---------------------------------------------------------------------------
-- Vehicle transfers.

-- name: CreateVehicleTransfer :one
INSERT INTO vehicle_transfers (
    organization_id, brand_id, vehicle_id, from_user_id, to_user_id, to_phone,
    from_code_hash, to_code_hash, expires_at, initiated_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(vehicle_id), sqlc.arg(from_user_id),
    sqlc.narg(to_user_id), sqlc.arg(to_phone), sqlc.arg(from_code_hash), sqlc.arg(to_code_hash),
    sqlc.arg(expires_at), sqlc.narg(initiated_by_user_id)
)
RETURNING *;

-- name: GetVehicleTransfer :one
SELECT * FROM vehicle_transfers
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetVehicleTransferByUUID :one
SELECT * FROM vehicle_transfers
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockVehicleTransferByUUID :one
SELECT * FROM vehicle_transfers
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: GetPendingVehicleTransfer :one
SELECT * FROM vehicle_transfers
WHERE vehicle_id = sqlc.arg(vehicle_id) AND status = 'pending';

-- name: ListVehicleTransfersByVehicle :many
SELECT * FROM vehicle_transfers
WHERE vehicle_id = sqlc.arg(vehicle_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY created_at DESC, id DESC;

-- A wrong code: one more attempt (the use case cancels at the limit).
-- name: IncrementVehicleTransferAttempts :one
UPDATE vehicle_transfers
SET attempts = attempts + 1
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: SetVehicleTransferVerified :one
UPDATE vehicle_transfers
SET from_verified_at = CASE WHEN sqlc.arg(from_side)::boolean THEN COALESCE(from_verified_at, NOW()) ELSE from_verified_at END,
    to_verified_at = CASE WHEN sqlc.arg(to_side)::boolean THEN COALESCE(to_verified_at, NOW()) ELSE to_verified_at END
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: CompleteVehicleTransfer :one
UPDATE vehicle_transfers
SET status = 'completed', completed_at = NOW(), to_user_id = sqlc.arg(to_user_id)
WHERE id = sqlc.arg(id) AND status = 'pending'
  AND from_verified_at IS NOT NULL AND to_verified_at IS NOT NULL
RETURNING *;

-- name: CancelVehicleTransfer :one
UPDATE vehicle_transfers
SET status = 'cancelled', cancelled_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: ExpireDueVehicleTransfers :many
UPDATE vehicle_transfers
SET status = 'expired'
WHERE status = 'pending' AND expires_at <= sqlc.arg(now)
RETURNING *;

-- Vehicle owner change in the transfer transaction (services keep their
-- customer snapshot, 000050).
-- name: SetVehicleOwner :one
UPDATE vehicles
SET user_id = sqlc.arg(new_user_id)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(old_user_id) AND deleted_at IS NULL
RETURNING *;

-- ---------------------------------------------------------------------------
-- Organization Google Business link (decision 7).

-- name: SetOrganizationGoogleBusinessURL :one
UPDATE organizations
SET google_business_url = sqlc.narg(google_business_url)
WHERE id = sqlc.arg(id)
RETURNING *;
