-- TEC-157 (F1-02e): reclassification requests (barcode kept, product
-- changed). Scope narrowing happens in modules/stock/usecase.

-- name: GetProductByIDAnyBrand :one
-- Reclassification target check: a product of another brand must be told
-- apart from a missing one (wrong routing, K1).
SELECT * FROM products WHERE id = sqlc.arg(id);

-- name: GetProductByUUIDAnyBrand :one
SELECT * FROM products WHERE uuid = sqlc.arg(uuid);

-- name: GetStockReclassificationByUUID :one
SELECT * FROM stock_reclassifications WHERE uuid = sqlc.arg(uuid);

-- name: LockStockReclassificationByUUID :one
SELECT * FROM stock_reclassifications WHERE uuid = sqlc.arg(uuid) FOR UPDATE;

-- name: ListStockReclassificationsScoped :many
SELECT * FROM stock_reclassifications
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockReclassificationsScoped :one
SELECT COUNT(*)::bigint FROM stock_reclassifications
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ListUsersByIDs :many
SELECT id, uuid, name, surname FROM users WHERE id = ANY(sqlc.arg(ids)::bigint[]);
