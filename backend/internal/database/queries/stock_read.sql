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
-- quantity or meters above zero, false = both zero.
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
ORDER BY p.name, p.id
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
ORDER BY p.name, p.id
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
