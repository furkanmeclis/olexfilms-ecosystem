-- TEC-261: migrator step 9 (orders). Written only by cmd/migrator inside a
-- step transaction. Legacy orders are history: no status history row, no
-- outbox event and no accounting entry is written for them (K9).

-- name: MigratorOrderBuyer :one
-- A buyer organization with its parent, the order's seller (K6).
SELECT o.id, o.type, o.brand_id, p.id AS parent_id, p.type AS parent_type
FROM organizations o
JOIN organizations p ON p.id = o.parent_id
WHERE o.uuid = sqlc.arg(uuid);

-- name: MigratorUserIDByUUID :one
SELECT id FROM users WHERE uuid = sqlc.arg(uuid);

-- name: MigratorBrandCurrency :one
SELECT currency::text AS currency FROM brands WHERE id = sqlc.arg(id);

-- name: MigratorFindOrderByExternalReference :one
SELECT uuid FROM orders
WHERE brand_id = sqlc.arg(brand_id) AND external_reference = sqlc.arg(external_reference)::text;

-- name: MigratorOrderByUUID :one
-- touched: the application already moved the order (a status history row),
-- so the migrator no longer rewrites it.
SELECT o.id, o.organization_id, o.status,
       EXISTS (SELECT 1 FROM order_status_history h WHERE h.order_id = o.id) AS touched
FROM orders o
WHERE o.uuid = sqlc.arg(uuid) AND o.brand_id = sqlc.arg(brand_id);

-- name: MigratorInsertOrder :one
INSERT INTO orders (
    uuid, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
    rate_snapshot, try_rate, tracking_no, external_reference, note, created_by_user_id,
    submitted_at, approved_at, shipped_at, delivered_at, received_at, cancelled_at, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(seller_org_id), sqlc.arg(brand_id), sqlc.arg(seller_org_id), sqlc.arg(buyer_org_id),
    sqlc.arg(status), sqlc.arg(currency), sqlc.narg(rate_snapshot)::jsonb, sqlc.narg(try_rate)::numeric,
    sqlc.narg(tracking_no), sqlc.narg(external_reference), sqlc.narg(note), sqlc.narg(created_by_user_id),
    sqlc.narg(submitted_at)::timestamptz, sqlc.narg(approved_at)::timestamptz, sqlc.narg(shipped_at)::timestamptz,
    sqlc.narg(delivered_at)::timestamptz, sqlc.narg(received_at)::timestamptz, sqlc.narg(cancelled_at)::timestamptz,
    COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateOrder :execrows
-- A legacy change of an order the application has not moved yet. Parties,
-- currency and totals stay as inserted.
UPDATE orders
SET status = sqlc.arg(status), rate_snapshot = sqlc.narg(rate_snapshot)::jsonb,
    try_rate = sqlc.narg(try_rate)::numeric, tracking_no = sqlc.narg(tracking_no),
    note = sqlc.narg(note), submitted_at = sqlc.narg(submitted_at)::timestamptz,
    approved_at = sqlc.narg(approved_at)::timestamptz, shipped_at = sqlc.narg(shipped_at)::timestamptz,
    delivered_at = sqlc.narg(delivered_at)::timestamptz, received_at = sqlc.narg(received_at)::timestamptz,
    cancelled_at = sqlc.narg(cancelled_at)::timestamptz
WHERE orders.id = sqlc.arg(id) AND orders.brand_id = sqlc.arg(brand_id)
  AND NOT EXISTS (SELECT 1 FROM order_status_history h WHERE h.order_id = orders.id);

-- name: MigratorOrderItemByUUID :one
SELECT id, order_id, product_id, quantity FROM order_items
WHERE uuid = sqlc.arg(uuid) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorFindOrderItem :one
SELECT uuid FROM order_items WHERE order_id = sqlc.arg(order_id) AND product_id = sqlc.arg(product_id);

-- name: MigratorInsertOrderItem :one
-- Legacy orders carry no prices: the line is frozen at 0, so a later receipt
-- books no sale for it either (K9, no double count).
INSERT INTO order_items (
    uuid, order_id, organization_id, brand_id, product_id, quantity, unit_price, price_source, line_total, created_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(order_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id),
    sqlc.arg(quantity), 0, 'list', 0, COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
)
RETURNING id;

-- name: MigratorUpdateOrderItemQuantity :exec
UPDATE order_items SET quantity = sqlc.arg(quantity)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: MigratorInsertOrderItemUnit :execrows
INSERT INTO order_item_units (order_item_id, organization_id, brand_id, unit_id, quantity, assigned_at)
VALUES (
    sqlc.arg(order_item_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id),
    sqlc.arg(quantity), COALESCE(sqlc.narg(assigned_at)::timestamptz, NOW())
)
ON CONFLICT (order_item_id, unit_id) DO NOTHING;
