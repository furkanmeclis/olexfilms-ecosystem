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
--
-- TEC-1252: a piece product sold by the meter also pushes the units a box
-- opening (measure_conversion) or a cut (split, both the roll and the new
-- piece) touched in a warehouse bin. The meter columns are the unit's
-- current state (absolute, so a repeated push is harmless); a cut piece
-- carries its source roll and the source's current meters, which go in
-- the same bulk call (the hub records the piece first, then the source's
-- absolute remaining). credits is what the piece would carry
-- (unit_credit_default: the meter line's share), 0 before it is
-- assigned.
SELECT m.id AS movement_id,
       m.created_at,
       u.barcode,
       p.external_id::text AS product_external_id,
       (p.unit_type = 'piece' AND u.initial_meters IS NOT NULL)::bool AS metered_piece,
       COALESCE(u.initial_meters::text, '')::text AS initial_meters,
       COALESCE(u.remaining_meters::text, '')::text AS remaining_meters,
       COALESCE(su.barcode, '')::text AS source_barcode,
       COALESCE(su.initial_meters::text, '')::text AS source_initial_meters,
       COALESCE(su.remaining_meters::text, '')::text AS source_remaining_meters,
       COALESCE(unit_credit_default(u.id), 0)::int AS credits
FROM stock_movements m
JOIN units u ON u.id = m.unit_id
JOIN products p ON p.id = m.product_id
LEFT JOIN stock_splits ss ON ss.new_unit_id = u.id
LEFT JOIN units su ON su.id = ss.source_unit_id
WHERE m.brand_id = sqlc.arg(brand_id)
  AND p.connection_id = sqlc.arg(connection_id)::bigint
  AND p.external_id IS NOT NULL
  AND m.to_owner_type = 'warehouse_location'
  AND u.unit_kind = 'serial'
  AND (
      m.type IN ('entry', 'placement')
      OR (m.type IN ('split', 'measure_conversion') AND p.unit_type = 'piece')
  )
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
       p.external_id,
       -- TEC-1252: an opened box or a cut piece of a piece product; the
       -- hub knows it now (bulk push), its exit is patched like a box.
       (p.unit_type = 'piece' AND u.initial_meters IS NOT NULL)::bool AS metered_piece
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

-- name: ListGlorianPriceRows :many
-- TEC-1265, kullanıcı kararı 5: the prices pushed to the hub. Products
-- synced from the connection with a USD list row (the center's
-- sale_to_distributor_price: what a buyer pays at list) after the keyset
-- id. Glorian keeps 2 decimals; meter_price is '' when the list row has
-- none (the hub then clears it and falls back to box price ÷ box meters).
SELECT p.id,
       p.external_id::text AS product_external_id,
       round(pp.sale_to_distributor_price, 2)::text AS price,
       COALESCE(round(pp.meter_price, 2)::text, '')::text AS meter_price
FROM products p
JOIN product_prices pp ON pp.product_id = p.id AND pp.brand_id = p.brand_id AND pp.currency = 'USD'
WHERE p.brand_id = sqlc.arg(brand_id)
  AND p.connection_id = sqlc.arg(connection_id)::bigint
  AND p.external_id IS NOT NULL
  AND pp.sale_to_distributor_price IS NOT NULL
  AND p.id > sqlc.arg(after_id)::bigint
ORDER BY p.id
LIMIT sqlc.arg(row_limit);
