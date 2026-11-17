-- TEC-1226: stock adjustment documents, their settings and photos, and the
-- return inspection queue.

-- name: GetStockAdjustmentSettings :one
SELECT * FROM stock_adjustment_settings WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertStockAdjustmentSettings :one
INSERT INTO stock_adjustment_settings (organization_id, threshold_quantity, threshold_meters, updated_by_user_id, updated_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(threshold_quantity), sqlc.arg(threshold_meters), sqlc.narg(user_id), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET threshold_quantity = EXCLUDED.threshold_quantity,
    threshold_meters = EXCLUDED.threshold_meters,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_at = NOW()
RETURNING *;

-- name: CreateStockAdjustment :one
INSERT INTO stock_adjustments (organization_id, brand_id, warehouse_id, type, reason_code, note, source, created_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.narg(warehouse_id), sqlc.arg(type), sqlc.narg(reason_code),
        sqlc.narg(note), sqlc.arg(source), sqlc.narg(user_id))
RETURNING *;

-- name: GetStockAdjustmentByUUID :one
SELECT * FROM stock_adjustments WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: LockStockAdjustmentByUUID :one
SELECT * FROM stock_adjustments WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id) FOR UPDATE;

-- name: UpdateStockAdjustmentDraft :one
UPDATE stock_adjustments
SET type = sqlc.arg(type), reason_code = sqlc.narg(reason_code), note = sqlc.narg(note),
    warehouse_id = sqlc.narg(warehouse_id), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: SubmitStockAdjustment :one
UPDATE stock_adjustments
SET status = 'pending_approval', submitted_by_user_id = sqlc.narg(user_id), submitted_at = NOW(),
    total_quantity = sqlc.arg(total_quantity), total_meters = sqlc.arg(total_meters),
    requires_second_approval = sqlc.arg(requires_second_approval), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: PostStockAdjustment :one
UPDATE stock_adjustments
SET status = 'posted', approved_by_user_id = sqlc.narg(user_id), posted_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'pending_approval'
RETURNING *;

-- name: CancelStockAdjustment :one
UPDATE stock_adjustments
SET status = 'cancelled', cancelled_by_user_id = sqlc.narg(user_id), cancelled_at = NOW(),
    cancel_reason = sqlc.narg(reason), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('draft', 'pending_approval')
RETURNING *;

-- name: ListStockAdjustments :many
-- docs/list-contract.md; keys from warehouse usecase AdjustmentSort. status
-- sorts by flow rank. q: number, note or reason code.
SELECT a.* FROM stock_adjustments a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR a.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(types)::text[]), 0) = 0 OR a.type = ANY (sqlc.narg(types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR a.source = ANY (sqlc.narg(sources)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = a.warehouse_id AND fw.uuid = ANY (sqlc.narg(warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(uuids)::uuid[]), 0) = 0 OR a.uuid = ANY (sqlc.narg(uuids)::uuid[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR a.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR a.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR a.adjustment_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.reason_code ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN a.created_at END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN a.created_at END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'adjustment_no' THEN a.adjustment_no END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'adjustment_no' THEN a.adjustment_no END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'type' THEN a.type END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'type' THEN a.type END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE a.status WHEN 'draft' THEN 1 WHEN 'pending_approval' THEN 2 WHEN 'posted' THEN 3 ELSE 4 END END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'status' THEN
    CASE a.status WHEN 'draft' THEN 1 WHEN 'pending_approval' THEN 2 WHEN 'posted' THEN 3 ELSE 4 END END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'total_quantity' THEN a.total_quantity END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'total_quantity' THEN a.total_quantity END END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'total_meters' THEN a.total_meters END END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN CASE sqlc.arg(sort_key)::text WHEN 'total_meters' THEN a.total_meters END END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN a.id END DESC,
  a.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountStockAdjustments :one
SELECT COUNT(*) FROM stock_adjustments a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR a.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(types)::text[]), 0) = 0 OR a.type = ANY (sqlc.narg(types)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(sources)::text[]), 0) = 0 OR a.source = ANY (sqlc.narg(sources)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(warehouse_uuids)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM warehouses fw
                  WHERE fw.id = a.warehouse_id AND fw.uuid = ANY (sqlc.narg(warehouse_uuids)::uuid[])))
  AND (COALESCE(cardinality(sqlc.narg(uuids)::uuid[]), 0) = 0 OR a.uuid = ANY (sqlc.narg(uuids)::uuid[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR a.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR a.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR a.adjustment_no ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.note ILIKE '%' || sqlc.narg(q)::text || '%'
       OR a.reason_code ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: InsertStockAdjustmentLine :one
INSERT INTO stock_adjustment_lines (adjustment_id, organization_id, unit_id, product_id, kind, quantity, meters, roll_meters, location_id)
VALUES (sqlc.arg(adjustment_id), sqlc.arg(organization_id), sqlc.arg(unit_id), sqlc.arg(product_id), sqlc.arg(kind),
        sqlc.narg(quantity), sqlc.narg(meters), sqlc.narg(roll_meters), sqlc.narg(location_id))
RETURNING *;

-- name: ListStockAdjustmentLines :many
SELECT l.*, u.uuid AS unit_uuid, u.barcode, u.unit_kind, (u.initial_meters IS NOT NULL)::bool AS is_roll,
       p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku,
       wl.uuid AS location_uuid, wl.full_code AS location_full_code
FROM stock_adjustment_lines l
JOIN units u ON u.id = l.unit_id
JOIN products p ON p.id = l.product_id
LEFT JOIN warehouse_locations wl ON wl.id = l.location_id
WHERE l.adjustment_id = sqlc.arg(adjustment_id)
ORDER BY l.id;

-- name: DeleteStockAdjustmentLine :execrows
DELETE FROM stock_adjustment_lines WHERE uuid = sqlc.arg(uuid) AND adjustment_id = sqlc.arg(adjustment_id);

-- name: SetStockAdjustmentLineMovement :exec
UPDATE stock_adjustment_lines SET movement_id = sqlc.arg(movement_id) WHERE id = sqlc.arg(id);

-- name: CountOpenAdjustmentLinesByUnit :one
-- Lines of draft or pending documents holding the unit (another document
-- may not adjust it meanwhile).
SELECT COUNT(*) FROM stock_adjustment_lines l
JOIN stock_adjustments a ON a.id = l.adjustment_id
WHERE l.unit_id = sqlc.arg(unit_id) AND a.status IN ('draft', 'pending_approval')
  AND a.id <> sqlc.arg(exclude_adjustment_id);

-- name: CountStockAdjustmentLines :one
SELECT COUNT(*) FROM stock_adjustment_lines WHERE adjustment_id = sqlc.arg(adjustment_id);

-- name: InsertStockAdjustmentPhoto :one
INSERT INTO stock_adjustment_photos (adjustment_id, organization_id, object_key, content_type, size_bytes, uploaded_by_user_id)
VALUES (sqlc.arg(adjustment_id), sqlc.arg(organization_id), sqlc.arg(object_key), sqlc.arg(content_type),
        sqlc.arg(size_bytes), sqlc.narg(user_id))
RETURNING *;

-- name: ListStockAdjustmentPhotos :many
SELECT * FROM stock_adjustment_photos WHERE adjustment_id = sqlc.arg(adjustment_id) ORDER BY id;

-- name: GetStockAdjustmentPhoto :one
SELECT * FROM stock_adjustment_photos WHERE uuid = sqlc.arg(uuid) AND adjustment_id = sqlc.arg(adjustment_id);

-- name: DeleteStockAdjustmentPhoto :execrows
DELETE FROM stock_adjustment_photos WHERE id = sqlc.arg(id);

-- name: CountStockAdjustmentPhotos :one
SELECT COUNT(*) FROM stock_adjustment_photos WHERE adjustment_id = sqlc.arg(adjustment_id);

-- name: ListQuarantinedUnits :many
-- Units of the organization waiting for inspection, newest first.
-- q: barcode, product name or SKU.
SELECT u.id AS unit_id, u.uuid AS unit_uuid, u.barcode, u.unit_kind, u.remaining_meters,
       (u.initial_meters IS NOT NULL)::bool AS is_roll,
       p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku,
       s.owner_type, s.owner_id, s.updated_at AS quarantined_at,
       wl.uuid AS location_uuid, wl.full_code AS location_full_code,
       EXISTS (SELECT 1 FROM stock_adjustment_lines al JOIN stock_adjustments aa ON aa.id = al.adjustment_id
               WHERE al.unit_id = u.id AND aa.status IN ('draft', 'pending_approval')) AS pending_scrap
FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN warehouse_locations wl ON s.owner_type = 'warehouse_location' AND wl.id = s.owner_id
WHERE s.holder_org_id = sqlc.arg(organization_id) AND s.status = 'quarantine'
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY s.updated_at DESC, u.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountQuarantinedUnits :one
SELECT COUNT(*) FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
JOIN products p ON p.id = u.product_id
WHERE s.holder_org_id = sqlc.arg(organization_id) AND s.status = 'quarantine'
  AND (sqlc.narg(q)::text IS NULL
       OR u.barcode ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: SumQuarantineByProduct :many
-- Quarantined pieces and roll meters per product of the organization (the
-- stock views show them apart from the counted stock).
SELECT p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku,
       COUNT(*)::bigint AS units,
       COALESCE(SUM(u.remaining_meters), 0)::numeric(14,2) AS meters
FROM unit_current_state s
JOIN units u ON u.id = s.unit_id
JOIN products p ON p.id = u.product_id
WHERE s.holder_org_id = sqlc.arg(organization_id) AND s.status = 'quarantine'
GROUP BY p.uuid, p.name, p.sku
ORDER BY p.name, p.uuid;

-- name: ListReturnInfoForUnits :many
-- The last received return line of each unit to the organization.
SELECT DISTINCT ON (i.unit_id) i.unit_id, i.id AS transfer_item_id, r.uuid AS transfer_uuid, r.transfer_no,
       r.reason AS return_note, r.received_at, fo.uuid AS from_org_uuid, fo.name AS from_org_name
FROM stock_transfer_request_items i
JOIN stock_transfer_requests r ON r.id = i.request_id
JOIN organizations fo ON fo.id = r.from_org_id
WHERE i.unit_id = ANY (sqlc.arg(unit_ids)::bigint[]) AND r.kind = 'return' AND r.to_org_id = sqlc.arg(organization_id)
  AND i.in_movement_id IS NOT NULL
ORDER BY i.unit_id, i.id DESC;

-- name: GetLastReturnItemForUnit :one
SELECT i.* FROM stock_transfer_request_items i
JOIN stock_transfer_requests r ON r.id = i.request_id
WHERE i.unit_id = sqlc.arg(unit_id) AND r.kind = 'return' AND r.to_org_id = sqlc.arg(organization_id)
  AND i.in_movement_id IS NOT NULL
ORDER BY i.id DESC
LIMIT 1;

-- name: InsertReturnInspection :one
INSERT INTO return_inspections (organization_id, brand_id, unit_id, transfer_item_id, decision, return_reason, note,
                                location_id, adjustment_id, movement_id, decided_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(unit_id), sqlc.narg(transfer_item_id), sqlc.arg(decision),
        sqlc.arg(return_reason), sqlc.narg(note), sqlc.narg(location_id), sqlc.narg(adjustment_id), sqlc.narg(movement_id),
        sqlc.narg(user_id))
RETURNING *;

-- name: UpdateStockAdjustmentTotals :one
-- Live totals of a draft (the threshold preview); frozen by the submit.
UPDATE stock_adjustments
SET total_quantity = sqlc.arg(total_quantity), total_meters = sqlc.arg(total_meters), updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetReturnInspectionMovement :exec
UPDATE return_inspections SET movement_id = sqlc.arg(movement_id) WHERE id = sqlc.arg(id);
