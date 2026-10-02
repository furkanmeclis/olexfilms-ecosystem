-- TEC-202: label templates and barcode batches.

-- ---------------------------------------------------------------------------
-- Label templates.

-- name: ListLabelTemplates :many
SELECT * FROM label_templates
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text)
ORDER BY kind, name, id;

-- name: GetLabelTemplateByUUID :one
SELECT * FROM label_templates
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: GetLabelTemplateByID :one
SELECT * FROM label_templates WHERE id = sqlc.arg(id);

-- name: GetDefaultLabelTemplate :one
SELECT * FROM label_templates
WHERE organization_id = sqlc.arg(organization_id) AND kind = sqlc.arg(kind) AND is_default AND active;

-- name: CreateLabelTemplate :one
INSERT INTO label_templates (
    organization_id, name, kind, symbology, logo_mode, logo_text, logo_image,
    width_mm, height_mm, grid_columns, show_name, show_code_text, is_default, active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(kind), sqlc.arg(symbology),
    sqlc.arg(logo_mode), sqlc.narg(logo_text), sqlc.narg(logo_image),
    sqlc.arg(width_mm), sqlc.arg(height_mm), sqlc.arg(grid_columns),
    sqlc.arg(show_name), sqlc.arg(show_code_text), sqlc.arg(is_default), sqlc.arg(active)
)
RETURNING *;

-- name: UpdateLabelTemplate :one
UPDATE label_templates
SET name = sqlc.arg(name),
    symbology = sqlc.arg(symbology),
    logo_mode = sqlc.arg(logo_mode),
    logo_text = sqlc.narg(logo_text),
    logo_image = sqlc.narg(logo_image),
    width_mm = sqlc.arg(width_mm),
    height_mm = sqlc.arg(height_mm),
    grid_columns = sqlc.arg(grid_columns),
    show_name = sqlc.arg(show_name),
    show_code_text = sqlc.arg(show_code_text),
    is_default = sqlc.arg(is_default),
    active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: ClearDefaultLabelTemplate :exec
UPDATE label_templates SET is_default = false
WHERE organization_id = sqlc.arg(organization_id) AND kind = sqlc.arg(kind) AND is_default
  AND id <> sqlc.arg(keep_id);

-- name: DeleteLabelTemplate :execrows
DELETE FROM label_templates
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- ---------------------------------------------------------------------------
-- Barcode counters and batches.

-- name: LockBarcodeCounter :one
-- Creates the counter on first use and locks it for the batch allocation.
INSERT INTO barcode_counters (brand_id, prefix, next_seq)
VALUES (sqlc.arg(brand_id), sqlc.arg(prefix), 1)
ON CONFLICT (brand_id, prefix) DO UPDATE SET next_seq = barcode_counters.next_seq
RETURNING *;

-- name: SetBarcodeCounter :exec
UPDATE barcode_counters SET next_seq = sqlc.arg(next_seq)
WHERE brand_id = sqlc.arg(brand_id) AND prefix = sqlc.arg(prefix);

-- name: CreateBarcodeBatch :one
INSERT INTO barcode_batches (
    organization_id, brand_id, product_id, quantity, prefix, first_seq, last_seq,
    meters, template_id, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(quantity),
    sqlc.arg(prefix), sqlc.arg(first_seq), sqlc.arg(last_seq),
    sqlc.narg(meters), sqlc.narg(template_id), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetBarcodeBatchByUUID :one
SELECT * FROM barcode_batches
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: ListBarcodeBatches :many
SELECT * FROM barcode_batches
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountBarcodeBatches :one
SELECT COUNT(*)::bigint FROM barcode_batches WHERE organization_id = sqlc.arg(organization_id);

-- name: MarkBarcodeBatchPrinted :one
UPDATE barcode_batches
SET print_count = print_count + 1, last_printed_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateBatchUnit :one
INSERT INTO units (
    organization_id, brand_id, product_id, barcode, unit_kind, source, status,
    initial_meters, remaining_meters, batch_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(barcode),
    sqlc.arg(unit_kind), 'generated', sqlc.arg(status),
    sqlc.narg(initial_meters), sqlc.narg(remaining_meters), sqlc.arg(batch_id)
)
RETURNING *;

-- name: ListUnitsByBatch :many
SELECT * FROM units WHERE batch_id = sqlc.arg(batch_id) ORDER BY barcode, id;

-- name: ListUnitsByBrandBarcodes :many
SELECT * FROM units
WHERE brand_id = sqlc.arg(brand_id) AND barcode = ANY(sqlc.arg(barcodes)::text[])
ORDER BY barcode, id;

-- name: ListUnitsByBarcodes :many
-- Brand-independent (K20): the caller narrows the rows by scope.
SELECT * FROM units
WHERE barcode = ANY(sqlc.arg(barcodes)::text[])
ORDER BY barcode, brand_id, id;
