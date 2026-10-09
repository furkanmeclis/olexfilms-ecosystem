-- TEC-508 (F5-10a): persistent module requests (migration 000131).

-- name: UpsertPendingModuleRequest :one
-- One pending request per organization x module: asking again refreshes the
-- note and the requesting user of the open request.
INSERT INTO module_requests (organization_id, brand_id, module_key, note, requested_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(module_key), sqlc.arg(note),
        sqlc.narg(requested_by_user_id))
ON CONFLICT (organization_id, module_key) WHERE status = 'pending' DO UPDATE SET
    note = EXCLUDED.note,
    requested_by_user_id = EXCLUDED.requested_by_user_id
RETURNING *;

-- name: GetModuleRequestByUUID :one
SELECT * FROM module_requests WHERE uuid = sqlc.arg(uuid);

-- name: DecideModuleRequest :one
-- Only a pending request is decided; a decided one returns no row.
UPDATE module_requests
SET status = sqlc.arg(status)::text,
    decided_by_user_id = sqlc.narg(decided_by_user_id),
    decision_note = sqlc.arg(decision_note),
    decided_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: CancelPendingModuleRequest :one
UPDATE module_requests
SET status = 'cancelled', decided_by_user_id = sqlc.narg(decided_by_user_id), decided_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND module_key = sqlc.arg(module_key) AND status = 'pending'
RETURNING *;

-- name: ListPendingModuleRequestsByKey :many
-- Auto approval: open requests of a module that may have been switched on.
SELECT * FROM module_requests
WHERE module_key = sqlc.arg(module_key) AND status = 'pending'
ORDER BY id;

-- name: ListPendingModuleRequestsForOrgs :many
SELECT * FROM module_requests
WHERE organization_id = ANY (sqlc.arg(org_ids)::bigint[]) AND status = 'pending'
ORDER BY id;

-- name: ListLatestModuleRequestsForOrg :many
-- The newest request of every module of an organization (Özellikler page).
SELECT DISTINCT ON (module_key) *
FROM module_requests
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY module_key, created_at DESC, id DESC;

-- name: ListModuleRequestsPage :many
-- Decision queue. queue 'distributor': requests of the distributor's direct
-- dealers; queue 'platform': requests of distributors and of dealers without
-- a distributor parent. Sort: docs/list-contract.md, keys from
-- features handler ModuleRequestsSortSpec.
SELECT r.id, r.uuid, r.module_key, r.note, r.status, r.decision_note, r.decided_at, r.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       ru.uuid AS requested_by_uuid,
       COALESCE(NULLIF(TRIM(CONCAT(ru.name, ' ', ru.surname)), ''), '')::text AS requested_by_name,
       du.uuid AS decided_by_uuid,
       COALESCE(NULLIF(TRIM(CONCAT(du.name, ' ', du.surname)), ''), '')::text AS decided_by_name
FROM module_requests r
JOIN organizations o ON o.id = r.organization_id
LEFT JOIN organizations p ON p.id = o.parent_id
LEFT JOIN users ru ON ru.id = r.requested_by_user_id
LEFT JOIN users du ON du.id = r.decided_by_user_id
WHERE (
    (sqlc.arg(queue)::text = 'distributor' AND o.type = 'dealer' AND o.parent_id = sqlc.narg(distributor_id)::bigint)
    OR (sqlc.arg(queue)::text = 'platform' AND (
        o.type = 'distributor' OR (o.type = 'dealer' AND (p.id IS NULL OR p.type <> 'distributor'))))
  )
  AND (sqlc.narg(brand_id)::bigint IS NULL OR r.brand_id = sqlc.narg(brand_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(module_keys)::text[]), 0) = 0 OR r.module_key = ANY (sqlc.narg(module_keys)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR r.module_key ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'module_key' THEN r.module_key::text WHEN 'status' THEN r.status::text
      WHEN 'organization_name' THEN o.name::text END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'module_key' THEN r.module_key::text WHEN 'status' THEN r.status::text
      WHEN 'organization_name' THEN o.name::text END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN r.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN r.decided_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN r.decided_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN r.id END DESC,
  r.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountModuleRequestsPage :one
SELECT COUNT(*)::bigint
FROM module_requests r
JOIN organizations o ON o.id = r.organization_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE (
    (sqlc.arg(queue)::text = 'distributor' AND o.type = 'dealer' AND o.parent_id = sqlc.narg(distributor_id)::bigint)
    OR (sqlc.arg(queue)::text = 'platform' AND (
        o.type = 'distributor' OR (o.type = 'dealer' AND (p.id IS NULL OR p.type <> 'distributor'))))
  )
  AND (sqlc.narg(brand_id)::bigint IS NULL OR r.brand_id = sqlc.narg(brand_id)::bigint)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR r.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(module_keys)::text[]), 0) = 0 OR r.module_key = ANY (sqlc.narg(module_keys)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR r.module_key ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR r.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR r.created_at < sqlc.narg(created_before)::timestamptz);

-- name: ListModuleBundlePrices :many
-- TEC-508: active module_bundle catalog items of a brand that contain a
-- module, with the distributor override of the buyer (or of its parent
-- distributor) when one exists. Cheapest bundle with the fewest modules first.
SELECT m.module_key, i.uuid AS item_uuid, i.name AS item_name, i.recurrence,
       COALESCE(ov.price, i.default_price)::numeric(18,2) AS price,
       COALESCE(ov.currency, i.currency)::text AS currency,
       (SELECT COUNT(*) FROM service_catalog_modules m2 WHERE m2.item_id = i.id)::int AS module_count
FROM service_catalog_modules m
JOIN service_catalog_items i ON i.id = m.item_id
LEFT JOIN service_price_overrides ov ON ov.item_id = i.id AND ov.organization_id = sqlc.narg(override_org_id)::bigint
WHERE i.brand_id = sqlc.arg(brand_id) AND i.is_active AND i.category = 'module_bundle'
ORDER BY m.module_key, module_count ASC, price ASC, i.id ASC;
