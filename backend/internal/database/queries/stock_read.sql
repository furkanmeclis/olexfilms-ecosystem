-- TEC-155 (F1-02c): stock read API. Read-only queries over the ledger and
-- its projections; scope narrowing happens in modules/stock/usecase on
-- holder_org_id (TEC-94 decision 4).

-- name: ListStockMovementsByUnitPage :many
-- Barcode history page in ledger order (oldest first).
SELECT * FROM stock_movements
WHERE unit_id = sqlc.arg(unit_id)
ORDER BY created_at, id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockMovementsByUnit :one
SELECT COUNT(*)::bigint FROM stock_movements WHERE unit_id = sqlc.arg(unit_id);

-- name: ListWarehouseLocationsByIDs :many
SELECT * FROM warehouse_locations WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListOrganizationsByIDs :many
SELECT * FROM organizations WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListOrganizationProductStockRows :many
-- organization_product_stocks with product and category. in_stock: true =
-- quantity or meters above zero, false = both zero. TEC-373: sort_key
-- product (name), sku, category (name), quantity, meters, updated_at; the
-- product id is the tiebreak (one row per product).
SELECT s.product_id, s.quantity, s.meters::text AS meters, s.updated_at,
       p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       p.uses_fixed_barcode, p.active AS product_active,
       c.uuid AS category_uuid, c.name AS category_name
FROM organization_product_stocks s
JOIN products p ON p.id = s.product_id
JOIN product_categories c ON c.id = p.category_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR s.brand_id = sqlc.narg(brand_id)::bigint)
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

-- name: CountOrganizationProductStockRows :one
SELECT COUNT(*)::bigint
FROM organization_product_stocks s
JOIN products p ON p.id = s.product_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR s.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR s.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(in_stock)::bool IS NULL
       OR (s.quantity > 0 OR s.meters > 0) = sqlc.narg(in_stock)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: ListBinProductStockRows :many
SELECT s.product_id, s.quantity, s.meters::text AS meters, s.updated_at,
       p.uuid AS product_uuid, p.sku, p.name AS product_name, p.unit_type,
       p.uses_fixed_barcode, p.active AS product_active,
       c.uuid AS category_uuid, c.name AS category_name
FROM bin_product_stocks s
JOIN products p ON p.id = s.product_id
JOIN product_categories c ON c.id = p.category_id
WHERE s.location_id = sqlc.arg(location_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR s.brand_id = sqlc.narg(brand_id)::bigint)
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

-- name: CountBinProductStockRows :one
SELECT COUNT(*)::bigint
FROM bin_product_stocks s
JOIN products p ON p.id = s.product_id
WHERE s.location_id = sqlc.arg(location_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR s.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR s.product_id = sqlc.narg(product_id)::bigint)
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(in_stock)::bool IS NULL
       OR (s.quantity > 0 OR s.meters > 0) = sqlc.narg(in_stock)::bool)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
  );

-- name: ListFixedBarcodeQuantitiesByHolder :many
-- Fixed barcode quantities on hand at an organization (its locations and
-- the organization owner itself), per barcode, for the listed products.
SELECT u.product_id, u.uuid AS unit_uuid, u.barcode, SUM(h.quantity_on_hand)::int AS quantity
FROM fixed_barcode_holdings h
JOIN units u ON u.id = h.unit_id
WHERE h.holder_org_id = sqlc.arg(holder_org_id)
  AND h.owner_type IN ('warehouse_location', 'organization')
  AND h.quantity_on_hand > 0
  AND u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
GROUP BY u.product_id, u.uuid, u.barcode
ORDER BY u.product_id, u.barcode;

-- name: GetProductIDByUUID :one
-- Brand-independent product lookup for the bin stock filter (K20); the
-- projection rows are already narrowed by scope.
SELECT id FROM products WHERE uuid = sqlc.arg(uuid);

-- name: GetProductCategoryIDByUUID :one
SELECT id FROM product_categories WHERE uuid = sqlc.arg(uuid);

-- name: ListFixedBarcodeQuantitiesByLocation :many
SELECT u.product_id, u.uuid AS unit_uuid, u.barcode, SUM(h.quantity_on_hand)::int AS quantity
FROM fixed_barcode_holdings h
JOIN units u ON u.id = h.unit_id
WHERE h.owner_type = 'warehouse_location' AND h.owner_id = sqlc.arg(location_id)
  AND h.quantity_on_hand > 0
  AND u.product_id = ANY(sqlc.arg(product_ids)::bigint[])
GROUP BY u.product_id, u.uuid, u.barcode
ORDER BY u.product_id, u.barcode;

-- TEC-216 (F1-12a): unit list of an organization. Serial units come from
-- unit_current_state, fixed barcodes from fixed_barcode_holdings (summed
-- per unit); both narrowed on holder_org_id. Without a status filter the
-- list holds the units counted as stock (available, placed); a status
-- filter lists exactly that status.
-- TEC-373 (DT-BE-5): statuses is any-of (empty: available + placed),
-- barcode exact or barcode_prefix (LIKE, the caller escapes % and _),
-- location_uuids any-of (fixed barcodes have no single location and drop
-- out), updated_from / updated_before (exclusive) on the holding's last
-- change. sort_key: product, barcode, status (unit flow rank), quantity,
-- meters (remaining, empty last), updated_at; id is the tiebreak.

-- name: ListOrganizationStockUnitRows :many
WITH held AS (
    SELECT s.unit_id, 1::int AS quantity, s.owner_type, s.owner_id, s.updated_at
    FROM unit_current_state s
    WHERE s.holder_org_id = sqlc.arg(organization_id)
      AND s.owner_type IN ('organization', 'warehouse_location')
    UNION ALL
    SELECT h.unit_id, SUM(h.quantity_on_hand)::int AS quantity,
           NULL::varchar AS owner_type, NULL::bigint AS owner_id, MAX(h.updated_at) AS updated_at
    FROM fixed_barcode_holdings h
    WHERE h.holder_org_id = sqlc.arg(organization_id)
      AND h.owner_type IN ('organization', 'warehouse_location')
    GROUP BY h.unit_id
    HAVING SUM(h.quantity_on_hand) > 0
)
SELECT u.id, u.uuid, u.barcode, u.unit_kind, u.status,
       u.initial_meters, u.remaining_meters,
       held.quantity, held.updated_at,
       p.id AS product_id, p.uuid AS product_uuid, p.sku, p.name AS product_name,
       p.unit_type, p.uses_fixed_barcode,
       l.uuid AS location_uuid, l.code AS location_code, l.name AS location_name
FROM held
JOIN units u ON u.id = held.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations l ON held.owner_type = 'warehouse_location' AND l.id = held.owner_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR u.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
  AND ((COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 AND u.status IN ('available', 'placed'))
       OR u.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(barcode)::text IS NULL OR u.barcode = sqlc.narg(barcode)::text)
  AND (sqlc.narg(barcode_prefix)::text IS NULL OR u.barcode LIKE sqlc.narg(barcode_prefix)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(location_uuids)::uuid[]), 0) = 0
       OR l.uuid = ANY (sqlc.narg(location_uuids)::uuid[]))
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR held.updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR held.updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR u.uuid = ANY (sqlc.narg(uuids)::uuid[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'product' THEN p.name WHEN 'barcode' THEN u.barcode END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'product' THEN p.name WHEN 'barcode' THEN u.barcode END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'quantity' THEN held.quantity WHEN 'status' THEN
    CASE u.status WHEN 'reserved' THEN 1 WHEN 'printed' THEN 2 WHEN 'available' THEN 3 WHEN 'placed' THEN 4
                 WHEN 'in_transit' THEN 5 WHEN 'used' THEN 6 ELSE 7 END END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'quantity' THEN held.quantity WHEN 'status' THEN
    CASE u.status WHEN 'reserved' THEN 1 WHEN 'printed' THEN 2 WHEN 'available' THEN 3 WHEN 'placed' THEN 4
                 WHEN 'in_transit' THEN 5 WHEN 'used' THEN 6 ELSE 7 END END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'meters' THEN u.remaining_meters END END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'meters' THEN u.remaining_meters END END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN held.updated_at END END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'updated_at' THEN held.updated_at END END DESC NULLS LAST,
  -- product sort: barcode inside one product (the former fixed order)
  CASE WHEN sqlc.arg(sort_key)::text = 'product' AND NOT sqlc.arg(sort_desc)::bool THEN u.barcode END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'product' AND sqlc.arg(sort_desc)::bool THEN u.barcode END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN u.id END DESC,
  u.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationStockUnitRows :one
WITH held AS (
    SELECT s.unit_id, s.owner_type, s.owner_id, s.updated_at
    FROM unit_current_state s
    WHERE s.holder_org_id = sqlc.arg(organization_id)
      AND s.owner_type IN ('organization', 'warehouse_location')
    UNION ALL
    SELECT h.unit_id, NULL::varchar AS owner_type, NULL::bigint AS owner_id, MAX(h.updated_at) AS updated_at
    FROM fixed_barcode_holdings h
    WHERE h.holder_org_id = sqlc.arg(organization_id)
      AND h.owner_type IN ('organization', 'warehouse_location')
    GROUP BY h.unit_id
    HAVING SUM(h.quantity_on_hand) > 0
)
SELECT COUNT(*)::bigint
FROM held
JOIN units u ON u.id = held.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations l ON held.owner_type = 'warehouse_location' AND l.id = held.owner_id
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR u.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(product_id)::bigint IS NULL OR u.product_id = sqlc.narg(product_id)::bigint)
  AND ((COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 AND u.status IN ('available', 'placed'))
       OR u.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(barcode)::text IS NULL OR u.barcode = sqlc.narg(barcode)::text)
  AND (sqlc.narg(barcode_prefix)::text IS NULL OR u.barcode LIKE sqlc.narg(barcode_prefix)::text || '%')
  AND (COALESCE(cardinality(sqlc.narg(location_uuids)::uuid[]), 0) = 0
       OR l.uuid = ANY (sqlc.narg(location_uuids)::uuid[]))
  AND (sqlc.narg(updated_from)::timestamptz IS NULL OR held.updated_at >= sqlc.narg(updated_from)::timestamptz)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR held.updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
    OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
  );
