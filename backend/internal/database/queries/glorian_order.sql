-- TEC-271 (F2-02f): Glorian order outbound. An order of the glorian brand
-- whose lines hold products synced from a connection is sent to that
-- connection's hub as one order per connection (order_outbounds).

-- name: GetGlorianOutboundOrder :one
SELECT * FROM orders
WHERE id = sqlc.arg(id);

-- name: ListGlorianOrderUnits :many
-- Serial units assigned to the order's lines whose product is synced from
-- a connection, in line order. Olex and local products have no connection
-- and never appear.
SELECT i.id AS order_item_id,
       p.connection_id::bigint AS connection_id,
       p.external_id::text AS product_external_id,
       u.barcode
FROM order_items i
JOIN products p ON p.id = i.product_id
JOIN order_item_units oiu ON oiu.order_item_id = i.id
JOIN units u ON u.id = oiu.unit_id
WHERE i.order_id = sqlc.arg(order_id)
  AND p.connection_id IS NOT NULL
  AND p.external_id IS NOT NULL
  AND u.unit_kind = 'serial'
ORDER BY i.id, oiu.id;

-- name: ListOrderOutboundsByOrder :many
SELECT * FROM order_outbounds
WHERE order_id = sqlc.arg(order_id)
ORDER BY id;

-- name: GetOrderOutboundByID :one
SELECT * FROM order_outbounds
WHERE id = sqlc.arg(id);

-- name: ListGlorianPartiesByPhone :many
-- Active dealers of the connection with the buyer's phone: the order's
-- customer link (exactly one match links, none or several hold).
SELECT * FROM integration_external_parties
WHERE connection_id = sqlc.arg(connection_id)
  AND phone_e164 = sqlc.arg(phone_e164)
  AND active
ORDER BY id
LIMIT 2;

-- name: ListHeldOrderOutbounds :many
-- Held outbounds of a connection (0: every connection) after the keyset
-- id, oldest first.
SELECT * FROM order_outbounds
WHERE state = 'held'
  AND (sqlc.arg(connection_id)::bigint = 0 OR connection_id = sqlc.arg(connection_id)::bigint)
  AND id > sqlc.arg(after_id)::bigint
ORDER BY id
LIMIT sqlc.arg(row_limit);
