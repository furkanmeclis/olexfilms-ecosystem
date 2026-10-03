-- Stock transfer requests between siblings (TEC-197, K13). The giver
-- (from_org_id = organization_id) requests, the receiver or the common
-- parent (approver_org_id) decides, the giver ships, the receiver receives.

-- name: InsertTransferRequest :one
INSERT INTO stock_transfer_requests (
    organization_id, brand_id, from_org_id, to_org_id, approver_org_id,
    currency, reason, requested_by_user_id, kind
)
VALUES (
    sqlc.arg(from_org_id), sqlc.arg(brand_id), sqlc.arg(from_org_id), sqlc.arg(to_org_id),
    sqlc.arg(approver_org_id), sqlc.arg(currency), sqlc.narg(reason), sqlc.narg(requested_by_user_id),
    COALESCE(NULLIF(sqlc.arg(kind)::text, ''), 'sibling')
)
RETURNING *;

-- name: GetTransferRequestByUUID :one
SELECT * FROM stock_transfer_requests
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockTransferRequestByUUID :one
SELECT * FROM stock_transfer_requests
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- Requests where org is the giver, the receiver or the common parent.
-- direction: '' (all), 'outgoing' (giver), 'incoming' (receiver),
-- 'approval' (parent).
-- name: ListTransferRequestsForOrg :many
SELECT * FROM stock_transfer_requests
WHERE brand_id = sqlc.arg(brand_id)
  AND (
      (sqlc.arg(direction)::text IN ('', 'outgoing') AND from_org_id = sqlc.arg(org_id)::bigint)
      OR (sqlc.arg(direction)::text IN ('', 'incoming') AND to_org_id = sqlc.arg(org_id)::bigint)
      OR (sqlc.arg(direction)::text IN ('', 'approval') AND approver_org_id = sqlc.arg(org_id)::bigint)
  )
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountTransferRequestsForOrg :one
SELECT COUNT(*) FROM stock_transfer_requests
WHERE brand_id = sqlc.arg(brand_id)
  AND (
      (sqlc.arg(direction)::text IN ('', 'outgoing') AND from_org_id = sqlc.arg(org_id)::bigint)
      OR (sqlc.arg(direction)::text IN ('', 'incoming') AND to_org_id = sqlc.arg(org_id)::bigint)
      OR (sqlc.arg(direction)::text IN ('', 'approval') AND approver_org_id = sqlc.arg(org_id)::bigint)
  )
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text);

-- name: DecideTransferRequest :one
UPDATE stock_transfer_requests
SET status = sqlc.arg(status)::text,
    total = sqlc.narg(total),
    decided_by_user_id = sqlc.narg(actor_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(note)
WHERE id = sqlc.arg(id) AND status = 'requested'
RETURNING *;

-- name: ShipTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'shipped', shipped_by_user_id = sqlc.narg(actor_user_id), shipped_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'approved'
RETURNING *;

-- name: ReceiveTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'received', received_by_user_id = sqlc.narg(actor_user_id), received_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'shipped'
RETURNING *;

-- name: CancelTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'cancelled', cancelled_by_user_id = sqlc.narg(actor_user_id), cancelled_at = NOW(),
    cancel_reason = sqlc.narg(reason)
WHERE id = sqlc.arg(id) AND status IN ('requested', 'approved', 'shipped')
RETURNING *;

-- name: InsertTransferRequestItem :one
INSERT INTO stock_transfer_request_items (
    request_id, organization_id, brand_id, unit_id, product_id, quantity, meters
)
VALUES (
    sqlc.arg(request_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id),
    sqlc.arg(product_id), sqlc.narg(quantity), sqlc.narg(meters)
)
RETURNING *;

-- name: ListTransferRequestItems :many
SELECT i.*, u.uuid AS unit_uuid, u.barcode, u.unit_kind,
       p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name, p.unit_type AS product_unit_type
FROM stock_transfer_request_items i
JOIN units u ON u.id = i.unit_id
JOIN products p ON p.id = i.product_id
WHERE i.request_id = sqlc.arg(request_id)
ORDER BY i.id;

-- name: CountTransferRequestItems :one
SELECT COUNT(*) FROM stock_transfer_request_items WHERE request_id = sqlc.arg(request_id);

-- name: SetTransferItemPrice :exec
UPDATE stock_transfer_request_items
SET unit_price = sqlc.narg(unit_price), line_total = sqlc.narg(line_total)
WHERE id = sqlc.arg(id);

-- name: SetTransferItemOutMovement :exec
UPDATE stock_transfer_request_items SET out_movement_id = sqlc.arg(movement_id)
WHERE id = sqlc.arg(id) AND out_movement_id IS NULL;

-- name: SetTransferItemInMovement :exec
UPDATE stock_transfer_request_items SET in_movement_id = sqlc.arg(movement_id)
WHERE id = sqlc.arg(id) AND in_movement_id IS NULL;

-- name: SetTransferItemRestoreMovement :exec
UPDATE stock_transfer_request_items SET restore_movement_id = sqlc.arg(movement_id)
WHERE id = sqlc.arg(id) AND restore_movement_id IS NULL;

-- A unit is on at most one open (requested or approved) request; the
-- caller holds the unit row lock.
-- name: CountOpenTransferItemsByUnit :one
SELECT COUNT(*) FROM stock_transfer_request_items i
JOIN stock_transfer_requests r ON r.id = i.request_id
WHERE i.unit_id = sqlc.arg(unit_id)
  AND r.status IN ('requested', 'approved')
  AND r.id <> sqlc.arg(exclude_request_id);

-- Fixed barcode quantity on open requests of the giver (excluding one).
-- name: SumOpenTransferQuantityByUnit :one
SELECT COALESCE(SUM(i.quantity), 0)::bigint AS open_quantity
FROM stock_transfer_request_items i
JOIN stock_transfer_requests r ON r.id = i.request_id
WHERE i.unit_id = sqlc.arg(unit_id)
  AND r.from_org_id = sqlc.arg(from_org_id)
  AND r.status IN ('requested', 'approved')
  AND r.id <> sqlc.arg(exclude_request_id);

-- Active organizations of the same type, brand and parent (K13 siblings).
-- name: ListTransferSiblings :many
SELECT * FROM organizations
WHERE brand_id = sqlc.arg(brand_id)
  AND parent_id = sqlc.arg(parent_id)
  AND type = sqlc.arg(type)
  AND id <> sqlc.arg(org_id)
  AND deleted_at IS NULL
ORDER BY name, id;

-- Members of an organization whose organization roles grant a permission
-- (TEC-200: recipients of the transfers.* notifications).
-- name: ListTransferNotifyUserIDs :many
SELECT DISTINCT om.user_id
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
JOIN organization_member_roles mr ON mr.member_id = om.id
JOIN role_permissions rp ON rp.role_id = mr.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND p.slug = sqlc.arg(permission_slug)::text
ORDER BY om.user_id;

-- TEC-223: the price a unit was sold at to buyer by seller (its latest
-- order line with the unit assigned, the order not cancelled), in currency.
-- name: GetUnitLastOrderPrice :one
SELECT oi.unit_price
FROM order_item_units oiu
JOIN order_items oi ON oi.id = oiu.order_item_id
JOIN orders o ON o.id = oi.order_id
WHERE oiu.unit_id = sqlc.arg(unit_id)
  AND o.seller_org_id = sqlc.arg(seller_org_id)
  AND o.buyer_org_id = sqlc.arg(buyer_org_id)
  AND o.currency = sqlc.arg(currency)::text
  AND o.status NOT IN ('draft', 'cancelling', 'cancelled')
ORDER BY oiu.assigned_at DESC, oiu.id DESC
LIMIT 1;
