-- TEC-165 (F1-04a): orders, order lines, assigned units, status history,
-- stock reservations and sibling transfer requests (migration 000049).
-- Every read is brand-bound (K20); seller-side reads filter organization_id,
-- buyer-side reads filter buyer_org_id. Transitions lock the row first
-- (Lock* ... FOR UPDATE) in the use case transaction.

-- ---------------------------------------------------------------------------
-- Orders.

-- name: CreateOrder :one
INSERT INTO orders (
    organization_id, brand_id, seller_org_id, buyer_org_id,
    seller_warehouse_location_id, buyer_warehouse_location_id,
    currency, delivery_mode, external_reference, note, created_by_user_id
)
VALUES (
    sqlc.arg(seller_org_id), sqlc.arg(brand_id), sqlc.arg(seller_org_id), sqlc.arg(buyer_org_id),
    sqlc.narg(seller_warehouse_location_id), sqlc.narg(buyer_warehouse_location_id),
    sqlc.arg(currency), sqlc.narg(delivery_mode), sqlc.narg(external_reference), sqlc.narg(note),
    sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetOrderByUUID :one
SELECT * FROM orders
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: GetOrderByExternalReference :one
SELECT * FROM orders
WHERE brand_id = sqlc.arg(brand_id) AND external_reference = sqlc.arg(external_reference);

-- name: LockOrder :one
SELECT * FROM orders
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: LockOrderByUUID :one
SELECT * FROM orders
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: UpdateOrderDraft :one
UPDATE orders
SET seller_warehouse_location_id = sqlc.narg(seller_warehouse_location_id),
    buyer_warehouse_location_id  = sqlc.narg(buyer_warehouse_location_id),
    delivery_mode                = sqlc.narg(delivery_mode),
    note                         = sqlc.narg(note)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: UpdateOrderTotals :one
UPDATE orders
SET subtotal = sqlc.arg(subtotal), tax_total = sqlc.arg(tax_total), total = sqlc.arg(total)
WHERE id = sqlc.arg(id)
RETURNING *;

-- Recomputes the subtotal from the lines (total = subtotal + tax_total).
-- name: RecalculateOrderTotals :one
UPDATE orders o
SET subtotal = s.subtotal, total = s.subtotal + o.tax_total
FROM (
    SELECT COALESCE(SUM(line_total), 0)::numeric(18,2) AS subtotal
    FROM order_items WHERE order_id = sqlc.arg(id)
) s
WHERE o.id = sqlc.arg(id)
RETURNING o.*;

-- Moves the order to status and stamps the matching timestamp once. The
-- caller has locked the row and validated the transition.
-- name: UpdateOrderStatus :one
UPDATE orders
SET status = sqlc.arg(status)::text,
    submitted_at = CASE WHEN sqlc.arg(status)::text = 'submitted' THEN COALESCE(submitted_at, NOW()) ELSE submitted_at END,
    ready_at = CASE WHEN sqlc.arg(status)::text = 'ready' THEN COALESCE(ready_at, NOW()) ELSE ready_at END,
    shipped_at = CASE WHEN sqlc.arg(status)::text = 'shipped' THEN COALESCE(shipped_at, NOW()) ELSE shipped_at END,
    delivered_at = CASE WHEN sqlc.arg(status)::text = 'delivered' THEN COALESCE(delivered_at, NOW()) ELSE delivered_at END,
    received_at = CASE WHEN sqlc.arg(status)::text = 'received' THEN COALESCE(received_at, NOW()) ELSE received_at END,
    cancel_requested_at = CASE WHEN sqlc.arg(status)::text = 'cancelling' THEN COALESCE(cancel_requested_at, NOW()) ELSE cancel_requested_at END,
    cancelled_at = CASE WHEN sqlc.arg(status)::text = 'cancelled' THEN COALESCE(cancelled_at, NOW()) ELSE cancelled_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- Seller approval: freezes the rate (decision 2).
-- name: ApproveOrder :one
UPDATE orders
SET status = 'approved',
    approved_at = NOW(),
    approved_by_user_id = sqlc.narg(approved_by_user_id),
    rate_snapshot = sqlc.arg(rate_snapshot),
    try_rate = sqlc.arg(try_rate)
WHERE id = sqlc.arg(id) AND approved_at IS NULL
RETURNING *;

-- name: SetOrderShipping :one
UPDATE orders
SET delivery_mode = sqlc.narg(delivery_mode),
    tracking_no = sqlc.narg(tracking_no),
    shipping_document_key = sqlc.narg(shipping_document_key)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetOrderReceiptDocument :one
UPDATE orders
SET receipt_document_key = sqlc.narg(receipt_document_key)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetOrderCancelReason :one
UPDATE orders
SET cancel_reason = sqlc.narg(cancel_reason)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetOrderExternalReference :one
UPDATE orders
SET external_reference = sqlc.narg(external_reference)
WHERE id = sqlc.arg(id)
RETURNING *;

-- Seller side: orders the organization sells.
-- name: ListOrdersBySeller :many
SELECT * FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND organization_id = sqlc.arg(seller_org_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountOrdersBySeller :one
SELECT COUNT(*) FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND organization_id = sqlc.arg(seller_org_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- Buyer side: orders the organization buys.
-- name: ListOrdersByBuyer :many
SELECT * FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND buyer_org_id = sqlc.arg(buyer_org_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountOrdersByBuyer :one
SELECT COUNT(*) FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND buyer_org_id = sqlc.arg(buyer_org_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- Scope list: orders where any of org_ids is the seller or the buyer
-- (org_ids NULL = whole brand, for brand/all scopes).
-- name: ListOrdersInScope :many
SELECT * FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountOrdersInScope :one
SELECT COUNT(*) FROM orders
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- ---------------------------------------------------------------------------
-- Order lines.

-- name: CreateOrderItem :one
INSERT INTO order_items (
    order_id, organization_id, brand_id, product_id, quantity, meters,
    unit_price, price_source, recommended_price_snapshot, line_total, note
)
SELECT o.id, o.organization_id, o.brand_id, sqlc.arg(product_id), sqlc.narg(quantity), sqlc.narg(meters),
       sqlc.arg(unit_price), sqlc.arg(price_source), sqlc.narg(recommended_price_snapshot),
       sqlc.arg(line_total), sqlc.narg(note)
FROM orders o
WHERE o.id = sqlc.arg(order_id)
RETURNING *;

-- name: GetOrderItem :one
SELECT * FROM order_items
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetOrderItemByUUID :one
SELECT * FROM order_items
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockOrderItem :one
SELECT * FROM order_items
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: ListOrderItems :many
SELECT * FROM order_items
WHERE order_id = sqlc.arg(order_id)
ORDER BY id;

-- name: LockOrderItems :many
SELECT * FROM order_items
WHERE order_id = sqlc.arg(order_id)
ORDER BY id
FOR UPDATE;

-- name: UpdateOrderItem :one
UPDATE order_items
SET quantity = sqlc.narg(quantity),
    meters = sqlc.narg(meters),
    unit_price = sqlc.arg(unit_price),
    price_source = sqlc.arg(price_source),
    recommended_price_snapshot = sqlc.narg(recommended_price_snapshot),
    line_total = sqlc.arg(line_total),
    note = sqlc.narg(note)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteOrderItem :execrows
DELETE FROM order_items
WHERE id = sqlc.arg(id);

-- ---------------------------------------------------------------------------
-- Units assigned to order lines.

-- name: CreateOrderItemUnit :one
INSERT INTO order_item_units (
    order_item_id, organization_id, brand_id, unit_id, quantity, meters,
    reservation_id, movement_id, assigned_by_user_id
)
SELECT i.id, i.organization_id, i.brand_id, sqlc.arg(unit_id), sqlc.narg(quantity), sqlc.narg(meters),
       sqlc.narg(reservation_id), sqlc.narg(movement_id), sqlc.narg(assigned_by_user_id)
FROM order_items i
WHERE i.id = sqlc.arg(order_item_id)
RETURNING *;

-- name: GetOrderItemUnit :one
SELECT * FROM order_item_units
WHERE order_item_id = sqlc.arg(order_item_id) AND unit_id = sqlc.arg(unit_id);

-- name: ListOrderItemUnitsByItem :many
SELECT * FROM order_item_units
WHERE order_item_id = sqlc.arg(order_item_id)
ORDER BY id;

-- name: ListOrderItemUnitsByOrder :many
SELECT oiu.*, u.barcode, u.unit_kind, u.uuid AS unit_uuid
FROM order_item_units oiu
JOIN order_items i ON i.id = oiu.order_item_id
JOIN units u ON u.id = oiu.unit_id
WHERE i.order_id = sqlc.arg(order_id)
ORDER BY oiu.order_item_id, oiu.id;

-- name: SetOrderItemUnitMovement :one
UPDATE order_item_units
SET movement_id = sqlc.arg(movement_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteOrderItemUnit :execrows
DELETE FROM order_item_units
WHERE id = sqlc.arg(id);

-- ---------------------------------------------------------------------------
-- Status history (append-only).

-- name: InsertOrderStatusHistory :one
INSERT INTO order_status_history (
    order_id, organization_id, brand_id, from_status, to_status, actor_user_id, reason, metadata
)
SELECT o.id, o.organization_id, o.brand_id, sqlc.narg(from_status), sqlc.arg(to_status),
       sqlc.narg(actor_user_id), sqlc.narg(reason), sqlc.arg(metadata)
FROM orders o
WHERE o.id = sqlc.arg(order_id)
RETURNING *;

-- name: ListOrderStatusHistory :many
SELECT * FROM order_status_history
WHERE order_id = sqlc.arg(order_id)
ORDER BY created_at, id;

-- ---------------------------------------------------------------------------
-- Stock reservations (TEC-94 decision 2).

-- name: CreateStockReservation :one
INSERT INTO stock_reservations (
    organization_id, brand_id, unit_id, unit_kind, order_item_id, quantity, meters, created_by_user_id
)
SELECT i.organization_id, i.brand_id, u.id, u.unit_kind, i.id, sqlc.narg(quantity), sqlc.narg(meters),
       sqlc.narg(created_by_user_id)
FROM order_items i
JOIN units u ON u.id = sqlc.arg(unit_id) AND u.brand_id = i.brand_id
WHERE i.id = sqlc.arg(order_item_id)
RETURNING *;

-- name: GetStockReservation :one
SELECT * FROM stock_reservations
WHERE id = sqlc.arg(id);

-- name: LockStockReservation :one
SELECT * FROM stock_reservations
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- Active reservations of a unit (at most one for a serial unit).
-- name: LockActiveReservationsByUnit :many
SELECT * FROM stock_reservations
WHERE unit_id = sqlc.arg(unit_id) AND status = 'active'
ORDER BY id
FOR UPDATE;

-- name: ListActiveReservationsByItem :many
SELECT * FROM stock_reservations
WHERE order_item_id = sqlc.arg(order_item_id) AND status = 'active'
ORDER BY id;

-- name: ListReservationsByOrder :many
SELECT r.* FROM stock_reservations r
JOIN order_items i ON i.id = r.order_item_id
WHERE i.order_id = sqlc.arg(order_id)
ORDER BY r.id;

-- Fixed barcodes: total quantity actively reserved by the seller
-- organization, checked against what that organization holds by the use
-- case (sum <= on hand, under the unit row lock).
-- name: SumActiveReservedQuantityByUnit :one
SELECT COALESCE(SUM(quantity), 0)::bigint AS reserved_quantity
FROM stock_reservations
WHERE unit_id = sqlc.arg(unit_id) AND organization_id = sqlc.arg(organization_id) AND status = 'active';

-- name: ReleaseStockReservation :one
UPDATE stock_reservations
SET status = 'released', released_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'active'
RETURNING *;

-- name: ConsumeStockReservation :one
UPDATE stock_reservations
SET status = 'consumed', consumed_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'active'
RETURNING *;

-- Cancel: releases every active reservation of the order.
-- name: ReleaseReservationsByOrder :execrows
UPDATE stock_reservations r
SET status = 'released', released_at = NOW()
FROM order_items i
WHERE i.id = r.order_item_id AND i.order_id = sqlc.arg(order_id) AND r.status = 'active';

-- ---------------------------------------------------------------------------
-- Sibling dealer transfer requests (K13).

-- name: CreateStockTransferRequest :one
INSERT INTO stock_transfer_requests (
    organization_id, brand_id, from_org_id, to_org_id, approver_org_id,
    product_id, unit_id, quantity, meters, currency, reason, requested_by_user_id
)
VALUES (
    sqlc.arg(from_org_id), sqlc.arg(brand_id), sqlc.arg(from_org_id), sqlc.arg(to_org_id),
    sqlc.arg(approver_org_id), sqlc.arg(product_id), sqlc.narg(unit_id), sqlc.narg(quantity),
    sqlc.narg(meters), sqlc.arg(currency), sqlc.narg(reason), sqlc.narg(requested_by_user_id)
)
RETURNING *;

-- name: GetStockTransferRequest :one
SELECT * FROM stock_transfer_requests
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetStockTransferRequestByUUID :one
SELECT * FROM stock_transfer_requests
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: LockStockTransferRequest :one
SELECT * FROM stock_transfer_requests
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- Approval freezes A's purchase price (K13).
-- name: ApproveStockTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'approved',
    unit_price = sqlc.arg(unit_price),
    line_total = sqlc.arg(line_total),
    rate_snapshot = sqlc.narg(rate_snapshot),
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(decision_note)
WHERE id = sqlc.arg(id) AND status = 'requested'
RETURNING *;

-- name: RejectStockTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'rejected',
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decided_at = NOW(),
    decision_note = sqlc.narg(decision_note)
WHERE id = sqlc.arg(id) AND status = 'requested'
RETURNING *;

-- name: CompleteStockTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'completed', completed_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'approved'
RETURNING *;

-- name: CancelStockTransferRequest :one
UPDATE stock_transfer_requests
SET status = 'cancelled', cancelled_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('requested', 'approved')
RETURNING *;

-- Scope list: requests where any of org_ids is the giver, the receiver or
-- the approver (org_ids NULL = whole brand).
-- name: ListStockTransferRequestsInScope :many
SELECT * FROM stock_transfer_requests
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL
       OR organization_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR to_org_id = ANY (sqlc.narg(org_ids)::bigint[])
       OR approver_org_id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
