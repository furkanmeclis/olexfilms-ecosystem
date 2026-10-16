-- name: CreateEntryBarcodeImport :exec
INSERT INTO entry_barcode_imports(job_id,entry_id,organization_id,brand_id,existing_policy)
VALUES(sqlc.arg(job_id),sqlc.arg(entry_id),sqlc.arg(organization_id),sqlc.arg(brand_id),sqlc.arg(existing_policy));

-- name: LockEntryBarcodeImport :one
SELECT * FROM entry_barcode_imports WHERE job_id=sqlc.arg(job_id) FOR UPDATE;

-- name: GetEntryBarcodeImport :one
SELECT * FROM entry_barcode_imports WHERE job_id=sqlc.arg(job_id);

-- name: SetEntryBarcodeImport :exec
UPDATE entry_barcode_imports SET status=sqlc.arg(status),rows=sqlc.arg(rows) WHERE job_id=sqlc.arg(job_id);
