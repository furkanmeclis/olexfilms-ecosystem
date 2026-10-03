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
  -- TEC-230: a corrected item (unit returned to stock) gets no warranty.
  AND NOT EXISTS (SELECT 1 FROM service_item_corrections c WHERE c.service_item_id = si.id)
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

-- Public warranty lookup (TEC-189): only the fields the public page shows.
-- No users join: the holder's personal data is never read, so an anonymized
-- customer's warranty answers the same way (K19). Vehicle fields come from
-- the service snapshot first, then the vehicle. TEC-260: an old hub number
-- merged into another warranty resolves through warranty_public_code_aliases
-- to the kept warranty; the row carries that warranty's own public_code.
-- name: GetPublicWarrantyByCode :one
SELECT w.public_code, w.status, w.start_at, w.end_at,
       p.name AS product_name,
       o.name AS organization_name, o.city AS organization_city,
       COALESCE(pr.name, '')::text AS organization_province,
       cb.uuid AS car_brand_uuid, cb.name AS car_brand_name,
       (cb.logo_object_key IS NOT NULL)::boolean AS car_brand_has_logo,
       cm.name AS car_model_name,
       s.model_year AS service_model_year, s.plate AS service_plate,
       s.plate_country AS service_plate_country, s.vin AS service_vin,
       v.model_year AS vehicle_model_year, v.plate AS vehicle_plate,
       v.plate_country AS vehicle_plate_country, v.vin AS vehicle_vin
FROM warranties w
JOIN products p ON p.id = w.product_id
JOIN organizations o ON o.id = w.organization_id
LEFT JOIN provinces pr ON pr.id = o.province_id
JOIN services s ON s.id = w.service_id
JOIN car_brands cb ON cb.id = s.car_brand_id
JOIN car_models cm ON cm.id = s.car_model_id
JOIN vehicles v ON v.id = w.vehicle_id
WHERE w.brand_id = sqlc.arg(brand_id)::bigint
  AND (w.public_code = sqlc.arg(public_code)::text
       OR w.id = (SELECT a.warranty_id FROM warranty_public_code_aliases a
                  WHERE a.code = sqlc.arg(public_code)::text AND a.brand_id = sqlc.arg(brand_id)::bigint))
ORDER BY (w.public_code = sqlc.arg(public_code)::text) DESC
LIMIT 1;

-- Warranty certificate (TEC-188, one PDF per service, decision 2): the
-- active warranties of a service with the covered product, unit and item.
-- holder_user_id narrows to the portal customer's own warranties (a
-- transferred vehicle's warranties belong to the new holder).
-- name: ListWarrantyCertificateItems :many
SELECT w.id, w.uuid, w.public_code, w.item_kind, w.start_at, w.end_at, w.holder_user_id,
       p.name AS product_name, p.sku AS product_sku,
       u.barcode AS unit_barcode, si.meters
FROM warranties w
JOIN products p ON p.id = w.product_id
JOIN units u ON u.id = w.unit_id
JOIN service_items si ON si.id = w.service_item_id
WHERE w.service_id = sqlc.arg(service_id) AND w.brand_id = sqlc.arg(brand_id) AND w.status = 'active'
  AND (sqlc.narg(holder_user_id)::bigint IS NULL OR w.holder_user_id = sqlc.narg(holder_user_id)::bigint)
ORDER BY w.id;

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

-- Notification context of the cron events (TEC-187): plate, product and the
-- organization's name and time zone (end date is shown in the org zone).
-- name: ListWarrantyNoticeContexts :many
SELECT w.id, v.plate, p.name AS product_name, o.name AS organization_name, o.timezone
FROM warranties w
JOIN vehicles v ON v.id = w.vehicle_id
JOIN products p ON p.id = w.product_id
JOIN organizations o ON o.id = w.organization_id
WHERE w.id = ANY (sqlc.arg(ids)::bigint[]);

-- service.completed consumer (TEC-186): the service, its organization's
-- time zone (end_at is the end of the last day there, decision 4) and its
-- brand slug (Glorian services get no warranty, K2).
-- name: GetWarrantyServiceContext :one
SELECT s.id, s.uuid, s.service_no, s.status, s.organization_id, s.brand_id, s.vehicle_id,
       s.customer_user_id, s.completed_at, o.timezone, b.slug AS brand_slug
FROM services s
JOIN organizations o ON o.id = s.organization_id
JOIN brands b ON b.id = s.brand_id
WHERE s.id = sqlc.arg(service_id);

-- One row per service item with what the warranty rules need: the
-- product's warranty period (NULL/0 = none), the unit's source, external
-- connection and brand, and whether the unit ever left the system through
-- an external_outbound movement (no warranty for those, K2).
-- name: ListWarrantyCandidatesByService :many
SELECT si.id, si.kind, si.product_id, si.unit_id,
       p.warranty_duration_months,
       u.source AS unit_source, u.connection_id AS unit_connection_id,
       ub.slug AS unit_brand_slug,
       EXISTS (
           SELECT 1 FROM stock_movements m
           WHERE m.unit_id = si.unit_id AND m.type = 'external_outbound'
       )::boolean AS external_outbound
FROM service_items si
JOIN products p ON p.id = si.product_id
JOIN units u ON u.id = si.unit_id
JOIN brands ub ON ub.id = u.brand_id
WHERE si.service_id = sqlc.arg(service_id)
  -- TEC-230: corrected items (unit returned to stock) get no warranty.
  AND NOT EXISTS (SELECT 1 FROM service_item_corrections c WHERE c.service_item_id = si.id)
ORDER BY si.id;

-- TEC-194: repair scan. One page of completed services (completed in
-- [since, until], id > after_service_id, organization_id = 0 for all) that
-- still have an item without a warranty, with the same eligibility columns
-- as ListWarrantyCandidatesByService for those items. The page is cut by
-- service, so every missing item of a listed service is returned.
-- name: ListWarrantyRepairCandidates :many
WITH page AS (
    SELECT s.id
    FROM services s
    WHERE s.status = 'completed'
      AND s.completed_at >= sqlc.arg(since)
      AND s.completed_at <= sqlc.arg(until)
      AND s.id > sqlc.arg(after_service_id)::bigint
      AND (sqlc.arg(organization_id)::bigint = 0 OR s.organization_id = sqlc.arg(organization_id)::bigint)
      -- TEC-260: a service the migrator brought over already completed is
      -- legacy data; its items were left without a warranty on purpose
      -- (or skipped with warranty_skipped_*), so the scan must not open
      -- one. A migrated service completed in the new app after its
      -- migration stays in scope. idx_migration_map_target covers it.
      AND NOT EXISTS (
          SELECT 1 FROM migration_map mm
          WHERE mm.target_table = 'services'
            AND mm.target_uuid = s.uuid
            AND s.completed_at <= mm.migrated_at
      )
      AND EXISTS (
          SELECT 1 FROM service_items si
          WHERE si.service_id = s.id
            AND NOT EXISTS (SELECT 1 FROM warranties w WHERE w.service_item_id = si.id)
            AND NOT EXISTS (SELECT 1 FROM service_item_corrections c WHERE c.service_item_id = si.id)
      )
    ORDER BY s.id
    LIMIT sqlc.arg(service_limit)
)
SELECT s.id AS service_id, b.slug AS service_brand_slug,
       si.id, si.kind, si.product_id, si.unit_id,
       p.warranty_duration_months,
       u.source AS unit_source, u.connection_id AS unit_connection_id,
       ub.slug AS unit_brand_slug,
       EXISTS (
           SELECT 1 FROM stock_movements m
           WHERE m.unit_id = si.unit_id AND m.type = 'external_outbound'
       )::boolean AS external_outbound
FROM page
JOIN services s ON s.id = page.id
JOIN brands b ON b.id = s.brand_id
JOIN service_items si ON si.service_id = s.id
JOIN products p ON p.id = si.product_id
JOIN units u ON u.id = si.unit_id
JOIN brands ub ON ub.id = u.brand_id
WHERE NOT EXISTS (SELECT 1 FROM warranties w WHERE w.service_item_id = si.id)
  AND NOT EXISTS (SELECT 1 FROM service_item_corrections c WHERE c.service_item_id = si.id)
ORDER BY s.id, si.id;

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

-- Vehicle of a transfer (TEC-190): scope check and response of the verify /
-- cancel endpoints, which address the transfer, not the vehicle.
-- name: GetVehicleByID :one
SELECT * FROM vehicles
WHERE id = sqlc.arg(id);

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
