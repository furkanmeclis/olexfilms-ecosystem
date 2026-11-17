-- TEC-1230: location suggestions for placing a stock entry. Same idea as
-- the picking suggestion (orders_picking.sql: placed units first): put the
-- product where the warehouse already keeps it, otherwise an empty
-- location of the warehouse.

-- name: ListEntryLineProducts :many
-- The products of an entry's lines that are not placed yet.
SELECT DISTINCT u.product_id
FROM stock_entry_lines l
JOIN units u ON u.id = l.unit_id
WHERE l.entry_id = sqlc.arg(entry_id)
  AND l.location_id IS NULL
  AND l.undo_movement_id IS NULL;

-- name: ListWarehouseProductLocations :many
-- Active locations of the warehouse holding the products on hand (serial
-- units and fixed barcode pieces), most stocked first.
WITH held AS (
    SELECT u.product_id, s.owner_id AS location_id, 1::int AS quantity
    FROM unit_current_state s
    JOIN units u ON u.id = s.unit_id
    WHERE s.holder_org_id = sqlc.arg(organization_id)
      AND s.owner_type = 'warehouse_location'
      AND u.product_id = ANY (sqlc.arg(product_ids)::bigint[])
    UNION ALL
    SELECT u.product_id, h.owner_id AS location_id, h.quantity_on_hand AS quantity
    FROM fixed_barcode_holdings h
    JOIN units u ON u.id = h.unit_id
    WHERE h.holder_org_id = sqlc.arg(organization_id)
      AND h.owner_type = 'warehouse_location'
      AND h.quantity_on_hand > 0
      AND u.product_id = ANY (sqlc.arg(product_ids)::bigint[])
)
SELECT held.product_id, wl.uuid AS location_uuid, wl.code AS location_code,
       wl.full_code AS location_full_code, wl.name AS location_name,
       SUM(held.quantity)::int AS quantity
FROM held
JOIN warehouse_locations wl ON wl.id = held.location_id
WHERE wl.organization_id = sqlc.arg(organization_id)
  AND wl.warehouse_id = sqlc.arg(warehouse_id)
  AND wl.active
GROUP BY held.product_id, wl.id
ORDER BY held.product_id, SUM(held.quantity) DESC, wl.sort_order, wl.id;

-- name: ListEmptyWarehouseLocations :many
-- Active typed leaf locations of the warehouse that hold nothing.
SELECT wl.uuid AS location_uuid, wl.code AS location_code,
       wl.full_code AS location_full_code, wl.name AS location_name
FROM warehouse_locations wl
WHERE wl.organization_id = sqlc.arg(organization_id)
  AND wl.warehouse_id = sqlc.arg(warehouse_id)
  AND wl.active
  AND wl.type IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM warehouse_locations c WHERE c.parent_id = wl.id AND c.active
  )
  AND NOT EXISTS (
      SELECT 1 FROM unit_current_state s
      WHERE s.owner_type = 'warehouse_location' AND s.owner_id = wl.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM fixed_barcode_holdings h
      WHERE h.owner_type = 'warehouse_location' AND h.owner_id = wl.id AND h.quantity_on_hand > 0
  )
ORDER BY wl.full_code NULLS LAST, wl.sort_order, wl.id
LIMIT sqlc.arg(limit_count);
