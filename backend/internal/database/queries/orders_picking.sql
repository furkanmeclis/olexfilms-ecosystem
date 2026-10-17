-- TEC-768: barcode suggestions, pick locations and the pick list of an order.

-- name: ListOrderSuggestSerialCandidates :many
-- Serial units (pieces, rolls) of a product the seller can assign: held in
-- stock (available/placed), with meters left (rolls), not reserved and not on
-- an open warehouse transfer. Placed units (a warehouse location owns them)
-- come first, then the oldest entry (FIFO: units.created_at), id tiebreak.
-- Units the organization holds without a location only with include_unlocated.
SELECT u.id, u.uuid, u.barcode, u.unit_kind, u.remaining_meters, u.created_at,
       s.owner_type, s.status,
       wl.uuid AS location_uuid,
       COALESCE(wl.full_code, wl.code, '')::text AS location_code
FROM units u
JOIN unit_current_state s ON s.unit_id = u.id
LEFT JOIN warehouse_locations wl ON wl.id = s.owner_location_id
WHERE u.product_id = sqlc.arg(product_id)
  AND s.holder_org_id = sqlc.arg(seller_org_id)
  AND s.status IN ('available', 'placed')
  AND (
    s.owner_type = 'warehouse_location'
    OR (sqlc.arg(include_unlocated)::bool AND s.owner_type = 'organization')
  )
  AND (u.remaining_meters IS NULL OR u.remaining_meters > 0)
  AND NOT EXISTS (
    SELECT 1 FROM stock_reservations r WHERE r.unit_id = u.id AND r.status = 'active'
  )
  AND NOT EXISTS (
    SELECT 1 FROM warehouse_transfer_lines t WHERE t.unit_id = u.id AND t.is_open
  )
ORDER BY (s.owner_type = 'warehouse_location') DESC, u.created_at ASC, u.id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListOrderSuggestFixedCandidates :many
-- Fixed barcodes of a product the seller holds: the quantity at warehouse
-- locations and without one, and the largest single holding of each (one
-- assignment ships from one owner), oldest entry first.
SELECT u.id, u.uuid, u.barcode, u.created_at,
       COALESCE(SUM(h.quantity_on_hand) FILTER (WHERE h.owner_type = 'warehouse_location'), 0)::bigint AS located_quantity,
       COALESCE(SUM(h.quantity_on_hand) FILTER (WHERE h.owner_type = 'organization'), 0)::bigint AS unlocated_quantity,
       COALESCE(MAX(h.quantity_on_hand) FILTER (WHERE h.owner_type = 'warehouse_location'), 0)::bigint AS located_max,
       COALESCE(MAX(h.quantity_on_hand) FILTER (WHERE h.owner_type = 'organization'), 0)::bigint AS unlocated_max
FROM units u
JOIN fixed_barcode_holdings h ON h.unit_id = u.id
WHERE u.product_id = sqlc.arg(product_id)
  AND u.unit_kind = 'fixed'
  AND h.holder_org_id = sqlc.arg(seller_org_id)
  AND h.quantity_on_hand > 0
GROUP BY u.id, u.uuid, u.barcode, u.created_at
ORDER BY u.created_at ASC, u.id ASC
LIMIT 50;

-- name: ListFixedUnitLocations :many
-- Warehouse locations of the seller holding a fixed barcode, largest first.
SELECT wl.uuid AS location_uuid, COALESCE(wl.full_code, wl.code)::text AS location_code,
       h.quantity_on_hand
FROM fixed_barcode_holdings h
JOIN warehouse_locations wl ON wl.id = h.owner_location_id
WHERE h.unit_id = sqlc.arg(unit_id)
  AND h.holder_org_id = sqlc.arg(seller_org_id)
  AND h.quantity_on_hand > 0
ORDER BY h.quantity_on_hand DESC, location_code ASC;

-- name: ListProductPickLocations :many
-- The seller's warehouse locations holding a product (bin_product_stocks:
-- pieces/rolls and meters), by location code.
SELECT wl.uuid AS location_uuid, COALESCE(wl.full_code, wl.code)::text AS location_code,
       wl.name AS location_name, b.quantity, b.meters
FROM bin_product_stocks b
JOIN warehouse_locations wl ON wl.id = b.location_id
WHERE b.organization_id = sqlc.arg(seller_org_id)
  AND b.product_id = sqlc.arg(product_id)
  AND (b.quantity > 0 OR b.meters > 0)
ORDER BY location_code ASC, wl.id ASC;

-- name: SumProductUnlocatedStock :one
-- What the seller holds of a product without a location: serial units in
-- stock owned by the organization and fixed quantities on the organization.
SELECT
  ((
    SELECT count(*)
    FROM unit_current_state s JOIN units u ON u.id = s.unit_id
    WHERE u.product_id = sqlc.arg(product_id) AND s.holder_org_id = sqlc.arg(seller_org_id)
      AND s.owner_type = 'organization' AND s.status IN ('available', 'placed')
      AND (u.remaining_meters IS NULL OR u.remaining_meters > 0)
  )::bigint
  + (
    SELECT COALESCE(SUM(h.quantity_on_hand), 0)
    FROM fixed_barcode_holdings h JOIN units u ON u.id = h.unit_id
    WHERE u.product_id = sqlc.arg(product_id) AND h.holder_org_id = sqlc.arg(seller_org_id)
      AND h.owner_type = 'organization'
  ))::bigint AS quantity,
  (
    SELECT COALESCE(SUM(u.remaining_meters), 0)
    FROM unit_current_state s JOIN units u ON u.id = s.unit_id
    WHERE u.product_id = sqlc.arg(product_id) AND s.holder_org_id = sqlc.arg(seller_org_id)
      AND s.owner_type = 'organization' AND s.status IN ('available', 'placed')
  )::numeric AS meters;

-- name: ListOrderAssignedUnitLocations :many
-- Assigned (not yet shipped) serial units of an order with their location.
SELECT oiu.order_item_id, u.id AS unit_id, u.barcode, u.unit_kind, oiu.quantity, oiu.meters,
       wl.uuid AS location_uuid, COALESCE(wl.full_code, wl.code, '')::text AS location_code
FROM order_item_units oiu
JOIN order_items oi ON oi.id = oiu.order_item_id
JOIN units u ON u.id = oiu.unit_id
LEFT JOIN unit_current_state s ON s.unit_id = u.id
LEFT JOIN warehouse_locations wl ON wl.id = s.owner_location_id
WHERE oi.order_id = sqlc.arg(order_id)
ORDER BY oiu.id;

-- name: SetOrderShippingDocument :one
UPDATE orders
SET shipping_document_key = sqlc.narg(shipping_document_key)
WHERE id = sqlc.arg(id)
RETURNING *;
