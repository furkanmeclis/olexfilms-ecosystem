-- TEC-272 (F2-02g): Glorian reconcile (read only). The local side of the
-- drift report: serial units of the connection's brand that belong to the
-- sync (their product is synced from the connection, or the unit already
-- mirrors a remote stock item of it), with the product's remote id and the
-- unit's current owner from the ledger projection.

-- name: ListGlorianReconcileUnits :many
SELECT u.id,
       u.uuid,
       u.barcode,
       u.external_id,
       u.external_status,
       p.external_id AS product_external_id,
       s.owner_type,
       s.owner_id,
       s.holder_org_id
FROM units u
JOIN products p ON p.id = u.product_id
LEFT JOIN unit_current_state s ON s.unit_id = u.id
WHERE u.brand_id = sqlc.arg(brand_id)
  AND u.unit_kind = 'serial'
  AND (p.connection_id = sqlc.arg(connection_id)::bigint OR u.connection_id = sqlc.arg(connection_id)::bigint)
  AND u.id > sqlc.arg(after_id)::bigint
ORDER BY u.id
LIMIT sqlc.arg(row_limit);

-- name: GetGlorianReconcileUnitByBarcode :one
-- A serial unit of the brand outside the synced set (e.g. its product is
-- not linked yet), so a remote item with its barcode is paired instead of
-- being reported as remote only.
SELECT u.id,
       u.uuid,
       u.barcode,
       u.external_id,
       u.external_status,
       p.external_id AS product_external_id,
       s.owner_type,
       s.owner_id,
       s.holder_org_id
FROM units u
JOIN products p ON p.id = u.product_id
LEFT JOIN unit_current_state s ON s.unit_id = u.id
WHERE u.brand_id = sqlc.arg(brand_id)
  AND u.barcode = sqlc.arg(barcode)
  AND u.unit_kind = 'serial';
