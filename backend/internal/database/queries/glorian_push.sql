-- TEC-270 (F2-02e): Glorian barcode push. Units of products synced from a
-- connection (products.connection_id + external_id) are pushed to the hub:
-- entries and placements into a center bin in bulk, exits by barcode.

-- name: GetIntegrationConnectionByID :one
SELECT * FROM integration_connections
WHERE id = sqlc.arg(id);

-- name: GetProductPushLink :one
-- The sync link of a product: its brand and, for a synced product, the
-- connection and remote product id.
SELECT id, brand_id, connection_id, external_id
FROM products
WHERE id = sqlc.arg(id);

-- name: ListGlorianPushUnits :many
-- Serial units entered or placed into a warehouse bin after the keyset
-- (created_at, id), for the products synced from the connection. A unit
-- placed twice comes twice; the caller deduplicates by barcode.
SELECT m.id AS movement_id,
       m.created_at,
       u.barcode,
       p.external_id::text AS product_external_id
FROM stock_movements m
JOIN units u ON u.id = m.unit_id
JOIN products p ON p.id = m.product_id
WHERE m.brand_id = sqlc.arg(brand_id)
  AND p.connection_id = sqlc.arg(connection_id)::bigint
  AND p.external_id IS NOT NULL
  AND m.type IN ('entry', 'placement')
  AND m.to_owner_type = 'warehouse_location'
  AND u.unit_kind = 'serial'
  AND (m.created_at, m.id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::bigint)
ORDER BY m.created_at, m.id
LIMIT sqlc.arg(row_limit);

-- name: GetGlorianPushMovement :one
-- A movement with its unit's barcode and the product's sync link, for the
-- outbound PATCH of one barcode.
SELECT m.id AS movement_id,
       m.type,
       m.organization_id,
       m.brand_id,
       m.to_owner_type,
       u.barcode,
       u.unit_kind,
       p.connection_id,
       p.external_id
FROM stock_movements m
JOIN units u ON u.id = m.unit_id
JOIN products p ON p.id = m.product_id
WHERE m.id = sqlc.arg(id);

-- name: LastIntegrationSyncRunWatermark :one
-- The watermark of the latest successful run of a kind that set one. The
-- barcode PATCH runs share the push_barcodes kind without a watermark, so
-- LastSucceededIntegrationSyncRun would lose the bulk push cursor.
SELECT watermark::timestamptz AS watermark
FROM integration_sync_runs
WHERE connection_id = sqlc.arg(connection_id)
  AND kind = sqlc.arg(kind)
  AND status = 'succeeded'
  AND watermark IS NOT NULL
ORDER BY started_at DESC, id DESC
LIMIT 1;
