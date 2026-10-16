-- TEC-761: warehouse exports (I/O engine). Read only; every query is bound
-- to one organization (the export job organization).

-- name: ListStockEntryExportLines :many
-- The lines of the given entries with their unit, product and location.
SELECT l.entry_id, l.uuid, l.quantity, l.location_id,
       (l.undo_movement_id IS NOT NULL)::bool AS undone,
       u.barcode, u.unit_kind, u.status AS unit_status, u.initial_meters, u.remaining_meters,
       p.sku AS product_sku, p.name AS product_name,
       loc.code AS location_code, loc.full_code AS location_full_code
FROM stock_entry_lines l
JOIN stock_entries e ON e.id = l.entry_id
JOIN units u ON u.id = l.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations loc ON loc.id = l.location_id
WHERE l.entry_id = ANY (sqlc.arg(entry_ids)::bigint[])
  AND e.organization_id = sqlc.arg(organization_id)
ORDER BY l.entry_id, l.id;

-- name: ListWarehouseTransferExportLines :many
-- The lines of the given transfers with their unit, product and locations.
SELECT l.transfer_id, l.uuid,
       (l.out_movement_id IS NOT NULL)::bool AS shipped,
       (l.in_movement_id IS NOT NULL)::bool AS received,
       (l.placement_movement_id IS NOT NULL)::bool AS placed,
       (l.restore_movement_id IS NOT NULL)::bool AS restored,
       u.barcode, u.status AS unit_status, u.remaining_meters,
       p.sku AS product_sku, p.name AS product_name,
       src.code AS source_code, src.full_code AS source_full_code,
       tgt.code AS target_code, tgt.full_code AS target_full_code
FROM warehouse_transfer_lines l
JOIN warehouse_transfers t ON t.id = l.transfer_id
JOIN units u ON u.id = l.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations src ON src.id = l.source_location_id
LEFT JOIN warehouse_locations tgt ON tgt.id = l.target_location_id
WHERE l.transfer_id = ANY (sqlc.arg(transfer_ids)::bigint[])
  AND t.organization_id = sqlc.arg(organization_id)
ORDER BY l.transfer_id, l.id;

-- name: WarehouseTransferProductSummary :many
-- Transfer aggregate report: per product, the transfers and units of the
-- organization's warehouse transfers matching the list filters (status,
-- from / to warehouse, created range, q) and the product filter. shipped:
-- left the source and not restored; delivered: received in the target;
-- in_transit: shipped, neither received nor restored.
SELECT p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       count(DISTINCT t.id)::bigint AS transfer_count,
       count(*)::bigint AS unit_count,
       (count(*) FILTER (WHERE l.out_movement_id IS NOT NULL AND l.restore_movement_id IS NULL))::bigint AS shipped_count,
       (count(*) FILTER (WHERE l.in_movement_id IS NOT NULL))::bigint AS delivered_count,
       (count(*) FILTER (WHERE l.out_movement_id IS NOT NULL AND l.in_movement_id IS NULL
                         AND l.restore_movement_id IS NULL))::bigint AS in_transit_count,
       (count(*) FILTER (WHERE t.status = 'cancelled'))::bigint AS cancelled_count
FROM warehouse_transfer_lines l
JOIN warehouse_transfers t ON t.id = l.transfer_id
JOIN units u ON u.id = l.unit_id
JOIN products p ON p.id = u.product_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR t.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(from_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = t.from_warehouse_id AND fw.uuid = ANY (sqlc.narg(from_warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(to_warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses tw
                  WHERE tw.id = t.to_warehouse_id AND tw.uuid = ANY (sqlc.narg(to_warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(product_uuids)::uuid[]), 0) = 0 OR p.uuid = ANY (sqlc.narg(product_uuids)::uuid[]))
  AND (COALESCE(cardinality(sqlc.narg(transfer_uuids)::uuid[]), 0) = 0 OR t.uuid = ANY (sqlc.narg(transfer_uuids)::uuid[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR t.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR t.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR t.transfer_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR t.note ILIKE '%' || sqlc.narg(q)::text || '%')
GROUP BY p.id, p.uuid, p.sku, p.name
ORDER BY p.name, p.id;

-- name: ListOrganizationLocationsForExport :many
-- Every typed location (aisle, shelf, bin) of the organization with its
-- warehouse and room; callers resolve the parents from the same set.
SELECT l.id, l.uuid, l.parent_id, l.type, l.code, l.name, l.full_code, l.active,
       w.uuid AS warehouse_uuid, w.code AS warehouse_code, w.name AS warehouse_name,
       r.uuid AS room_uuid, r.code AS room_code, r.name AS room_name
FROM warehouse_locations l
JOIN warehouses w ON w.id = l.warehouse_id
JOIN rooms r ON r.id = l.room_id
WHERE l.organization_id = sqlc.arg(organization_id) AND l.room_id IS NOT NULL
ORDER BY w.sort_order, w.id, r.sort_order, r.id, l.full_code, l.id;

-- name: ListProductsByUUIDsAnyBrand :many
-- Product names of an export filter (the warehouse side is brand
-- independent, K20).
SELECT uuid, sku, name FROM products
WHERE uuid = ANY (sqlc.arg(uuids)::uuid[])
ORDER BY name, id;
