-- TEC-768: delivery method catalog of a selling organization.

-- name: EnsureSystemDeliveryMethod :exec
-- The system method "warehouse_pickup" (Ambar teslim), one per organization.
INSERT INTO delivery_methods (organization_id, brand_id, code, name, is_system, sort_order)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), 'warehouse_pickup', sqlc.arg(name), true, 0)
ON CONFLICT DO NOTHING;

-- name: ListDeliveryMethodsFiltered :many
-- Sort: docs/list-contract.md, keys from orders/usecase.DeliveryMethodSortSpec.
SELECT *
FROM delivery_methods m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR m.active = sqlc.narg(active))
  AND (sqlc.narg(is_system)::bool IS NULL OR m.is_system = sqlc.narg(is_system))
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg(q) || '%'
    OR m.code ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN m.name WHEN 'code' THEN m.code END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'name' THEN m.name WHEN 'code' THEN m.code END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort_order' THEN m.sort_order END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'sort_order' THEN m.sort_order END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN m.created_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN m.id END DESC,
  m.id ASC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountDeliveryMethodsFiltered :one
SELECT count(*)
FROM delivery_methods m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(active)::bool IS NULL OR m.active = sqlc.narg(active))
  AND (sqlc.narg(is_system)::bool IS NULL OR m.is_system = sqlc.narg(is_system))
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.name ILIKE '%' || sqlc.narg(q) || '%'
    OR m.code ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: GetDeliveryMethodByUUID :one
SELECT * FROM delivery_methods
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id);

-- name: LockDeliveryMethodByUUID :one
SELECT * FROM delivery_methods
WHERE uuid = sqlc.arg(uuid) AND organization_id = sqlc.arg(organization_id)
FOR UPDATE;

-- name: GetDeliveryMethodByID :one
SELECT * FROM delivery_methods WHERE id = sqlc.arg(id);

-- name: CreateDeliveryMethod :one
INSERT INTO delivery_methods (organization_id, brand_id, code, name, tracking_url_template, active, sort_order)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(code), sqlc.arg(name),
    sqlc.narg(tracking_url_template), sqlc.arg(active), sqlc.arg(sort_order)
)
RETURNING *;

-- name: UpdateDeliveryMethod :one
UPDATE delivery_methods
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    tracking_url_template = sqlc.narg(tracking_url_template),
    active = sqlc.arg(active),
    sort_order = sqlc.arg(sort_order)
WHERE id = sqlc.arg(id) AND NOT is_system
RETURNING *;

-- name: DeleteDeliveryMethod :execrows
DELETE FROM delivery_methods WHERE id = sqlc.arg(id) AND NOT is_system;

-- name: CountOrdersByDeliveryMethod :one
SELECT count(*) FROM orders WHERE delivery_method_id = sqlc.arg(delivery_method_id);
