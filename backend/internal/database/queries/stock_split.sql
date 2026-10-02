-- TEC-184 roll split. Written by ledger.Split only.

-- name: CreateStockSplit :one
INSERT INTO stock_splits (
    organization_id, brand_id, product_id, source_unit_id, new_unit_id, meters,
    idempotency_key, reference_type, reference_id, created_by_user_id
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(product_id), sqlc.arg(source_unit_id),
    sqlc.arg(new_unit_id), sqlc.arg(meters), sqlc.arg(idempotency_key),
    sqlc.narg(reference_type), sqlc.narg(reference_id), sqlc.narg(created_by_user_id)
)
ON CONFLICT (organization_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetStockSplitByKey :one
SELECT * FROM stock_splits
WHERE organization_id = sqlc.arg(organization_id) AND idempotency_key = sqlc.arg(idempotency_key);

-- name: CountStockSplitsBySource :one
SELECT COUNT(*)::bigint FROM stock_splits WHERE source_unit_id = sqlc.arg(source_unit_id);
