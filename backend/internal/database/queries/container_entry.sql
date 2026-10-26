-- TEC-1007: stock entry from a container list. The delivery on the entry
-- header, the arrival scan (expected vs received), remembered supplier
-- product codes and saved import column mappings.

-- name: SetStockEntryHeader :one
UPDATE stock_entries
SET supplier_id = sqlc.narg(supplier_id),
    container_no = sqlc.narg(container_no),
    waybill_no = sqlc.narg(waybill_no),
    arrival_date = sqlc.narg(arrival_date),
    default_location_id = sqlc.narg(default_location_id),
    note = sqlc.narg(note)
WHERE id = sqlc.arg(id) AND status = 'draft'
RETURNING *;

-- name: FindStockEntryLineByBarcode :one
SELECT l.* FROM stock_entry_lines l
JOIN units u ON u.id = l.unit_id
WHERE l.entry_id = sqlc.arg(entry_id) AND upper(u.barcode) = upper(sqlc.arg(barcode)::text);

-- name: SetStockEntryLineScanned :one
UPDATE stock_entry_lines
SET scanned_at = NOW(), scanned_by = sqlc.arg(user_id)
WHERE id = sqlc.arg(id) AND scanned_at IS NULL
RETURNING *;

-- name: ClearStockEntryLineScan :execrows
UPDATE stock_entry_lines
SET scanned_at = NULL, scanned_by = NULL
WHERE id = sqlc.arg(id) AND scanned_at IS NOT NULL;

-- name: InsertStockEntryScan :one
INSERT INTO stock_entry_scans (entry_id, barcode, unit_id, scanned_by)
VALUES (sqlc.arg(entry_id), sqlc.arg(barcode), sqlc.narg(unit_id), sqlc.arg(scanned_by))
ON CONFLICT (entry_id, upper(barcode)) DO NOTHING
RETURNING *;

-- name: DeleteStockEntryScan :execrows
DELETE FROM stock_entry_scans
WHERE entry_id = sqlc.arg(entry_id) AND upper(barcode) = upper(sqlc.arg(barcode)::text);

-- name: ListStockEntryScans :many
SELECT * FROM stock_entry_scans WHERE entry_id = sqlc.arg(entry_id) ORDER BY id;

-- name: CountStockEntryScanned :one
SELECT count(*) FILTER (WHERE scanned_at IS NOT NULL)::bigint AS scanned,
       count(*)::bigint AS expected
FROM stock_entry_lines WHERE entry_id = sqlc.arg(entry_id);

-- name: ListSupplierProductCodes :many
SELECT code, product_id FROM supplier_product_codes
WHERE supplier_id = sqlc.arg(supplier_id);

-- name: UpsertSupplierProductCode :exec
INSERT INTO supplier_product_codes (organization_id, brand_id, supplier_id, code, product_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(supplier_id), sqlc.arg(code), sqlc.arg(product_id))
ON CONFLICT (supplier_id, upper(code)) DO UPDATE SET product_id = EXCLUDED.product_id;

-- name: ListImportMappingTemplates :many
SELECT t.*, s.uuid AS supplier_uuid, s.name AS supplier_name
FROM import_mapping_templates t
LEFT JOIN suppliers s ON s.id = t.supplier_id
WHERE t.organization_id = sqlc.arg(organization_id) AND t.resource = sqlc.arg(resource)
ORDER BY lower(t.name), t.id;

-- name: CreateImportMappingTemplate :one
INSERT INTO import_mapping_templates (organization_id, resource, name, supplier_id, mapping, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(resource), sqlc.arg(name), sqlc.narg(supplier_id),
        sqlc.arg(mapping), sqlc.narg(created_by))
ON CONFLICT (organization_id, resource, lower(name)) DO UPDATE
SET supplier_id = EXCLUDED.supplier_id, mapping = EXCLUDED.mapping
RETURNING *;

-- name: DeleteImportMappingTemplate :execrows
DELETE FROM import_mapping_templates
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id) AND resource = sqlc.arg(resource);

-- name: GetSupplierRef :one
SELECT uuid, name FROM suppliers WHERE id = sqlc.arg(id);
