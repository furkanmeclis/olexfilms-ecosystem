-- TEC-1090 (TEC-1069): product totals of one warehouse, summed over its
-- locations (bin_product_stocks). quantity counts pieces, rolls and fixed
-- barcode quantities like the organization list; roll_count is the serial
-- roll units in stock there (remaining meters above zero); meters the
-- meters left on them. No prices. in_stock / q / product / category and the
-- sort keys follow ListOrganizationProductStockRows; the product id is the
-- tiebreak (one row per product).

-- name: ListWarehouseProductStockRows :many
WITH totals AS (
    SELECT b.product_id, SUM(b.quantity)::int AS quantity, SUM(b.meters)::numeric(14,2) AS meters,
           MAX(b.updated_at)::timestamptz AS updated_at
    FROM bin_product_stocks b
    JOIN warehouse_locations wl ON wl.id = b.location_id
    WHERE wl.warehouse_id = sqlc.arg(warehouse_id)::bigint
      AND b.organization_id = sqlc.arg(organization_id)::bigint
    GROUP BY b.product_id
), rolls AS (
    SELECT u.product_id, COUNT(*)::int AS roll_count
    FROM unit_current_state st
    JOIN units u ON u.id = st.unit_id
    JOIN warehouse_locations wl ON wl.id = st.owner_location_id
    WHERE st.owner_type = 'warehouse_location'
      AND wl.warehouse_id = sqlc.arg(warehouse_id)::bigint
      AND st.holder_org_id = sqlc.arg(organization_id)::bigint
      AND st.status IN ('available', 'placed')
      AND u.unit_kind = 'serial'
      AND u.remaining_meters > 0
    GROUP BY u.product_id
)
SELECT s.product_id, s.quantity, s.meters::text AS meters, s.updated_at,
       COALESCE(r.roll_count, 0)::int AS roll_count,
       p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       p.uses_fixed_barcode, p.active AS product_active,
       c.uuid AS category_uuid, c.name AS category_name
FROM totals s
JOIN products p ON p.id = s.product_id
JOIN product_categories c ON c.id = p.category_id
LEFT JOIN rolls r ON r.product_id = s.product_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR p.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR s.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(in_stock)::bool IS NULL
       OR (s.quantity > 0 OR s.meters > 0) = sqlc.narg(in_stock)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'product' THEN p.name WHEN 'sku' THEN p.sku WHEN 'category' THEN c.name END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'product' THEN p.name WHEN 'sku' THEN p.sku WHEN 'category' THEN c.name END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'quantity' THEN s.quantity END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'quantity' THEN s.quantity END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'meters' THEN s.meters END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'meters' THEN s.meters END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN s.updated_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN s.updated_at END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN p.id END DESC,
  p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountWarehouseProductStockRows :one
WITH totals AS (
    SELECT b.product_id, SUM(b.quantity)::int AS quantity, SUM(b.meters) AS meters
    FROM bin_product_stocks b
    JOIN warehouse_locations wl ON wl.id = b.location_id
    WHERE wl.warehouse_id = sqlc.arg(warehouse_id)::bigint
      AND b.organization_id = sqlc.arg(organization_id)::bigint
    GROUP BY b.product_id
)
SELECT COUNT(*)::bigint
FROM totals s
JOIN products p ON p.id = s.product_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR p.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR s.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(in_stock)::bool IS NULL
       OR (s.quantity > 0 OR s.meters > 0) = sqlc.narg(in_stock)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: ListFixedBarcodeQuantitiesByWarehouse :many
SELECT u.product_id, u.uuid AS unit_uuid, u.barcode, SUM(h.quantity_on_hand)::int AS quantity
FROM fixed_barcode_holdings h
JOIN units u ON u.id = h.unit_id
JOIN warehouse_locations wl ON wl.id = h.owner_id
WHERE h.owner_type = 'warehouse_location'
  AND wl.warehouse_id = sqlc.arg(warehouse_id)::bigint
  AND h.holder_org_id = sqlc.arg(organization_id)::bigint
  AND h.quantity_on_hand > 0
  AND u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
GROUP BY u.product_id, u.uuid, u.barcode
ORDER BY u.product_id, u.barcode;
