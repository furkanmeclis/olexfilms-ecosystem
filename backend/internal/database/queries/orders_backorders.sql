-- TEC-1227: partial shipment, backorders and receipt discrepancies.

-- name: MarkOrderBackorder :one
-- A new backorder takes the approval of the order it was split from: the
-- frozen rate, the approver, the seller warehouse and the link. It starts
-- approved without a new price approval.
UPDATE orders b
SET status = 'approved',
    approved_at = NOW(),
    approved_by_user_id = src.approved_by_user_id,
    rate_snapshot = src.rate_snapshot,
    try_rate = src.try_rate,
    seller_warehouse_id = src.seller_warehouse_id,
    seller_warehouse_location_id = src.seller_warehouse_location_id,
    buyer_warehouse_location_id = src.buyer_warehouse_location_id,
    backorder_of_order_id = src.id
FROM orders src
WHERE b.id = sqlc.arg(id) AND src.id = sqlc.arg(source_id) AND b.approved_at IS NULL
RETURNING b.*;

-- name: ListOrderBackorders :many
-- The backorders split from an order, oldest first.
SELECT * FROM orders
WHERE backorder_of_order_id = sqlc.arg(backorder_of_order_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY created_at, id;

-- name: MoveOrderItemToOrder :one
-- A line nothing was picked for moves to the backorder whole (its
-- released reservations stay with it as history).
UPDATE order_items
SET order_id = sqlc.arg(order_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetOrderItemAmount :one
-- The picked part of a line stays on the shipped order.
UPDATE order_items
SET quantity = sqlc.narg(quantity),
    meters = sqlc.narg(meters),
    line_total = sqlc.arg(line_total)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateOrderReceiptDiscrepancy :one
INSERT INTO order_receipt_discrepancies (
    organization_id, brand_id, order_id, order_item_unit_id, unit_id, kind, reason, created_by_user_id
)
SELECT o.organization_id, o.brand_id, o.id, sqlc.arg(order_item_unit_id), sqlc.arg(unit_id),
       sqlc.arg(kind), sqlc.arg(reason), sqlc.narg(created_by_user_id)
FROM orders o
WHERE o.id = sqlc.arg(order_id)
RETURNING *;

-- name: ListOrderReceiptDiscrepancies :many
SELECT d.*, u.uuid AS unit_uuid, u.barcode, u.unit_kind,
       oiu.quantity, oiu.meters, p.name AS product_name, p.sku AS product_sku,
       cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname,
       ru.uuid AS resolved_by_uuid, ru.name AS resolved_by_name, ru.surname AS resolved_by_surname
FROM order_receipt_discrepancies d
JOIN units u ON u.id = d.unit_id
JOIN order_item_units oiu ON oiu.id = d.order_item_unit_id
JOIN order_items i ON i.id = oiu.order_item_id
JOIN products p ON p.id = i.product_id
LEFT JOIN users cu ON cu.id = d.created_by_user_id
LEFT JOIN users ru ON ru.id = d.resolved_by_user_id
WHERE d.order_id = sqlc.arg(order_id)
ORDER BY d.id;

-- name: LockOrderReceiptDiscrepancy :one
SELECT * FROM order_receipt_discrepancies
WHERE uuid = sqlc.arg(uuid) AND order_id = sqlc.arg(order_id)
FOR UPDATE;

-- name: ResolveOrderReceiptDiscrepancy :one
UPDATE order_receipt_discrepancies
SET status = 'resolved',
    resolution = sqlc.arg(resolution),
    resolution_note = sqlc.narg(resolution_note),
    resolved_by_user_id = sqlc.narg(resolved_by_user_id),
    resolved_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'open'
RETURNING *;

-- name: AddOrderReceiptDiscrepancyPhoto :one
UPDATE order_receipt_discrepancies
SET photo_keys = array_append(photo_keys, sqlc.arg(photo_key)::text)
WHERE id = sqlc.arg(id) AND cardinality(photo_keys) < 5
RETURNING *;

-- name: CountOpenOrderReceiptDiscrepancies :one
SELECT count(*)::bigint FROM order_receipt_discrepancies
WHERE order_id = sqlc.arg(order_id) AND status = 'open';

-- name: GetOrderItemUnitByID :one
SELECT * FROM order_item_units WHERE id = sqlc.arg(id);
