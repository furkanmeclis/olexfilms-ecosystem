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
-- TEC-763 list contract (docs/list-contract.md): status and requested_by
-- (user uuids) are CSV, created_from/_to, from/to product, q (barcode
-- prefix, product name or SKU, reason). Sort keys from
-- usecase.ReclassificationSort, id tiebreak.
SELECT r.* FROM stock_reclassifications r
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR r.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR r.organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(requested_by)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM users ru WHERE ru.id = r.requested_by_user_id AND ru.uuid = ANY(sqlc.narg(requested_by)::uuid[])))
  AND (sqlc.narg(from_product_uuid)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM products fp WHERE fp.id = r.from_product_id AND fp.uuid = sqlc.narg(from_product_uuid)::uuid))
  AND (sqlc.narg(to_product_uuid)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM products tp WHERE tp.id = r.to_product_id AND tp.uuid = sqlc.narg(to_product_uuid)::uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR EXISTS (SELECT 1 FROM units qu WHERE qu.id = r.unit_id AND qu.barcode ILIKE sqlc.narg(q)::text || '%')
       OR EXISTS (SELECT 1 FROM products qp WHERE qp.id IN (r.from_product_id, r.to_product_id)
                    AND (qp.name ILIKE '%' || sqlc.narg(q)::text || '%' OR qp.sku ILIKE '%' || sqlc.narg(q)::text || '%'))
       OR r.reason ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'status' THEN r.status::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN r.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN r.created_at END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN r.decided_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN r.decided_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN r.id END DESC,
  r.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockReclassificationsScoped :one
SELECT COUNT(*)::bigint FROM stock_reclassifications r
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR r.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR r.organization_id = ANY(sqlc.narg(org_ids)::bigint[]))
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(requested_by)::uuid[]), 0) = 0
       OR EXISTS (SELECT 1 FROM users ru WHERE ru.id = r.requested_by_user_id AND ru.uuid = ANY(sqlc.narg(requested_by)::uuid[])))
  AND (sqlc.narg(from_product_uuid)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM products fp WHERE fp.id = r.from_product_id AND fp.uuid = sqlc.narg(from_product_uuid)::uuid))
  AND (sqlc.narg(to_product_uuid)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM products tp WHERE tp.id = r.to_product_id AND tp.uuid = sqlc.narg(to_product_uuid)::uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL
       OR EXISTS (SELECT 1 FROM units qu WHERE qu.id = r.unit_id AND qu.barcode ILIKE sqlc.narg(q)::text || '%')
       OR EXISTS (SELECT 1 FROM products qp WHERE qp.id IN (r.from_product_id, r.to_product_id)
                    AND (qp.name ILIKE '%' || sqlc.narg(q)::text || '%' OR qp.sku ILIKE '%' || sqlc.narg(q)::text || '%'))
       OR r.reason ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: ListUsersByIDs :many
SELECT id, uuid, name, surname FROM users WHERE id = ANY(sqlc.arg(ids)::bigint[]);
