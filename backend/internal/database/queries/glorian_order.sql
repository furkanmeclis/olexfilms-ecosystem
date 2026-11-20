-- TEC-271 (F2-02f): Glorian order outbound. An order of the glorian brand
-- whose lines hold products synced from a connection is sent to that
-- connection's hub as one order per connection (order_outbounds).

-- name: GetGlorianOutboundOrder :one
SELECT * FROM orders
WHERE id = sqlc.arg(id);

-- name: ListGlorianOrderUnits :many
-- Serial units assigned to the order's lines whose product is synced from
-- a connection, in line order. Olex and local products have no connection
-- and never appear. TEC-1252: a meter line goes to the hub with its
-- meters, credits and plotter mark; each unit is a piece: piece_meters is
-- what the line took (the assignment's meters, a whole roll's remaining),
-- piece_credits its share of the line's credits (NULL: not spread) and
-- source_barcode the roll it was cut from (empty for a whole box or roll).
SELECT i.id AS order_item_id,
       p.connection_id::bigint AS connection_id,
       p.external_id::text AS product_external_id,
       u.barcode,
       i.sale_measure::text AS sale_measure,
       COALESCE(i.meters::text, '')::text AS line_meters,
       i.line_credits,
       i.plotter_cut,
       COALESCE(oiu.meters::text, u.remaining_meters::text, '')::text AS piece_meters,
       oiu.credits AS piece_credits,
       COALESCE(su.barcode, '')::text AS source_barcode
FROM order_items i
JOIN products p ON p.id = i.product_id
JOIN order_item_units oiu ON oiu.order_item_id = i.id
JOIN units u ON u.id = oiu.unit_id
LEFT JOIN stock_splits ss ON ss.new_unit_id = u.id
LEFT JOIN units su ON su.id = ss.source_unit_id
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

-- name: ListGlorianPartiesForOrganization :many
-- TEC-1265 (karar 6): the active dealers the buyer organization matches on
-- the connection (glorian_dealer_matches: manual, then dealer code, then
-- phone). Exactly one row links; none or several hold the order.
SELECT p.* FROM glorian_dealer_matches m
JOIN integration_external_parties p ON p.id = m.party_id
WHERE m.connection_id = sqlc.arg(connection_id)
  AND m.organization_id = sqlc.arg(organization_id)::bigint
  AND p.active
ORDER BY p.id
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
