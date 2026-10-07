-- TEC-334 (F3-06a): warranty claim schema (migration 000092). Reads are
-- bounded by the brand and, when given, the resolved organization ids
-- (NULL = the whole brand); the API layer owns scope resolution.

-- name: CreateWarrantyClaim :one
-- claim_no is the next number of the organization. Two concurrent creates
-- of one organization may collide on uq_warranty_claims_org_no; the caller
-- retries. A second live claim of the warranty hits
-- uq_warranty_claims_live_warranty.
INSERT INTO warranty_claims (
    organization_id, brand_id, claim_no, warranty_id, service_id, vehicle_id,
    customer_user_id, description, status, coverage_check,
    created_by_user_id, updated_by_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id),
    (SELECT COALESCE(MAX(c.claim_no), 0)::bigint + 1 FROM warranty_claims c
     WHERE c.organization_id = sqlc.arg(organization_id)),
    sqlc.arg(warranty_id), sqlc.arg(service_id), sqlc.arg(vehicle_id),
    sqlc.arg(customer_user_id), sqlc.arg(description), sqlc.arg(status),
    sqlc.arg(coverage_check), sqlc.narg(created_by_user_id), sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetWarrantyClaimByUUID :one
SELECT * FROM warranty_claims
WHERE uuid = sqlc.arg(uuid)
  AND brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]));

-- name: GetWarrantyClaimByUUIDForUpdate :one
SELECT * FROM warranty_claims
WHERE uuid = sqlc.arg(uuid)
  AND brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
FOR UPDATE;

-- name: GetWarrantyClaimByID :one
SELECT * FROM warranty_claims
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id);

-- name: GetWarrantyClaimByIDForUpdate :one
SELECT * FROM warranty_claims
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
FOR UPDATE;

-- name: GetLiveWarrantyClaimByWarranty :one
-- The live (not rejected / closed) claim of a warranty, if any.
SELECT * FROM warranty_claims
WHERE warranty_id = sqlc.arg(warranty_id)
  AND status NOT IN ('rejected', 'closed');

-- name: ListWarrantyClaimsByWarranty :many
SELECT * FROM warranty_claims
WHERE warranty_id = sqlc.arg(warranty_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY created_at DESC, id DESC;

-- TEC-377 (DT-BE-7): organization_uuids (multi-value), q also matches the
-- claim number exactly (q_exact) and the service number; sort keys from
-- warranty_claims usecase.ListSort (status by flow rank, decided_at NULLS
-- LAST both ways).
-- name: ListWarrantyClaimsInScope :many
SELECT * FROM warranty_claims
WHERE warranty_claims.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR warranty_claims.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.arg(statuses)::varchar[] IS NULL OR warranty_claims.status = ANY(sqlc.arg(statuses)::varchar[]))
  AND (sqlc.narg(warranty_id)::bigint IS NULL OR warranty_claims.warranty_id = sqlc.narg(warranty_id)::bigint)
  AND (sqlc.narg(service_id)::bigint IS NULL OR warranty_claims.service_id = sqlc.narg(service_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR warranty_claims.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR warranty_claims.customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR warranty_claims.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR warranty_claims.created_at < sqlc.narg(created_to)::timestamptz)
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR warranty_claims.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(q)::text IS NULL
       OR warranty_claims.description ILIKE '%' || sqlc.narg(q)::text || '%'
       OR warranty_claims.claim_no::text = sqlc.narg(q_exact)::text
       OR EXISTS (SELECT 1 FROM services qs WHERE qs.id = warranty_claims.service_id
                  AND qs.service_no ILIKE '%' || sqlc.narg(q)::text || '%'))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'claim_no' THEN claim_no END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'claim_no' THEN claim_no END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE status WHEN 'open' THEN 0 WHEN 'dealer_review' THEN 1 WHEN 'center_review' THEN 2 WHEN 'approved' THEN 3
      WHEN 'rejected' THEN 4 WHEN 'reapplied' THEN 5 WHEN 'closed' THEN 6 ELSE 7 END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'status' THEN
    CASE status WHEN 'open' THEN 0 WHEN 'dealer_review' THEN 1 WHEN 'center_review' THEN 2 WHEN 'approved' THEN 3
      WHEN 'rejected' THEN 4 WHEN 'reapplied' THEN 5 WHEN 'closed' THEN 6 ELSE 7 END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN decided_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'decided_at' THEN decided_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN id END DESC,
  id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWarrantyClaimsInScope :one
SELECT COUNT(*) FROM warranty_claims
WHERE warranty_claims.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR warranty_claims.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.arg(statuses)::varchar[] IS NULL OR warranty_claims.status = ANY(sqlc.arg(statuses)::varchar[]))
  AND (sqlc.narg(warranty_id)::bigint IS NULL OR warranty_claims.warranty_id = sqlc.narg(warranty_id)::bigint)
  AND (sqlc.narg(service_id)::bigint IS NULL OR warranty_claims.service_id = sqlc.narg(service_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR warranty_claims.vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR warranty_claims.customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR warranty_claims.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR warranty_claims.created_at < sqlc.narg(created_to)::timestamptz)
  AND (
    COALESCE(cardinality(sqlc.narg(organization_uuids)::uuid[]), 0) = 0
    OR warranty_claims.organization_id IN (SELECT fo.id FROM organizations fo WHERE fo.uuid = ANY (sqlc.narg(organization_uuids)::uuid[]))
  )
  AND (sqlc.narg(q)::text IS NULL
       OR warranty_claims.description ILIKE '%' || sqlc.narg(q)::text || '%'
       OR warranty_claims.claim_no::text = sqlc.narg(q_exact)::text
       OR EXISTS (SELECT 1 FROM services qs WHERE qs.id = warranty_claims.service_id
                  AND qs.service_no ILIKE '%' || sqlc.narg(q)::text || '%'));

-- name: SetWarrantyClaimStatus :one
-- Moves the claim from from_status to status (no row when the claim moved
-- meanwhile). approved / rejected stamp the decision; rejected stores the
-- reason; closed stamps closed_at. The status_changed event is written by
-- trigger with actor = updated_by_user_id.
UPDATE warranty_claims
SET status             = sqlc.arg(status)::varchar,
    rejection_reason   = CASE WHEN sqlc.arg(status)::varchar = 'rejected'
                              THEN sqlc.narg(rejection_reason)::text ELSE rejection_reason END,
    decided_by_user_id = CASE WHEN sqlc.arg(status)::varchar IN ('approved', 'rejected')
                              THEN sqlc.narg(actor_user_id)::bigint ELSE decided_by_user_id END,
    decided_at         = CASE WHEN sqlc.arg(status)::varchar IN ('approved', 'rejected')
                              THEN NOW() ELSE decided_at END,
    closed_at          = CASE WHEN sqlc.arg(status)::varchar = 'closed' THEN NOW() ELSE closed_at END,
    updated_by_user_id = sqlc.narg(actor_user_id)::bigint
WHERE id = sqlc.arg(id)
  AND brand_id = sqlc.arg(brand_id)
  AND status = sqlc.arg(from_status)::varchar
RETURNING *;

-- name: SetWarrantyClaimCoverageCheck :one
UPDATE warranty_claims
SET coverage_check     = sqlc.arg(coverage_check),
    updated_by_user_id = sqlc.narg(actor_user_id)::bigint
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: SetWarrantyClaimAITriage :one
UPDATE warranty_claims
SET ai_damage_type = sqlc.narg(ai_damage_type),
    ai_summary     = sqlc.narg(ai_summary),
    ai_confidence  = sqlc.narg(ai_confidence),
    ai_triaged_at  = NOW()
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING *;

-- name: LinkWarrantyClaimReapplyService :one
-- Links the re-application service to an approved claim and moves it to
-- reapplied. The reverse link is SetServiceWarrantyClaim.
UPDATE warranty_claims
SET reapply_service_id = sqlc.arg(reapply_service_id),
    status             = 'reapplied',
    updated_by_user_id = sqlc.narg(actor_user_id)::bigint
WHERE id = sqlc.arg(id)
  AND brand_id = sqlc.arg(brand_id)
  AND status = 'approved'
RETURNING *;

-- name: ClearWarrantyClaimReapplyService :one
-- A cancelled re-application service releases the claim for a new attempt.
UPDATE warranty_claims
SET reapply_service_id = NULL,
    status             = 'approved',
    updated_by_user_id = sqlc.narg(actor_user_id)::bigint
WHERE id = sqlc.arg(id)
  AND brand_id = sqlc.arg(brand_id)
  AND reapply_service_id = sqlc.arg(reapply_service_id)
  AND status = 'reapplied'
RETURNING *;

-- name: GetWarrantyClaimReapplyService :one
SELECT * FROM services
WHERE id = sqlc.arg(reapply_service_id)
  AND brand_id = sqlc.arg(brand_id)
  AND warranty_claim_id = sqlc.arg(claim_id);

-- name: SetServiceWarrantyClaim :one
UPDATE services
SET warranty_claim_id = sqlc.arg(warranty_claim_id)
WHERE id = sqlc.arg(id) AND brand_id = sqlc.arg(brand_id)
RETURNING id, warranty_claim_id;

-- name: AddWarrantyClaimPart :one
INSERT INTO warranty_claim_parts (
    claim_id, organization_id, brand_id, part_key, service_item_id, product_id, unit_id, note
) VALUES (
    sqlc.arg(claim_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(part_key),
    sqlc.narg(service_item_id), sqlc.narg(product_id), sqlc.narg(unit_id), sqlc.arg(note)
)
RETURNING *;

-- name: ListWarrantyClaimParts :many
SELECT * FROM warranty_claim_parts
WHERE claim_id = sqlc.arg(claim_id)
ORDER BY id;

-- name: ListWarrantyClaimReapplyItems :many
SELECT DISTINCT ON (p.part_key)
       p.part_key, si.product_id, si.unit_id, si.kind, si.quantity, si.meters
FROM warranty_claim_parts p
JOIN service_items si ON si.id = p.service_item_id
WHERE p.claim_id = sqlc.arg(claim_id)
  AND p.product_id IS NOT NULL
  AND p.unit_id IS NOT NULL
ORDER BY p.part_key, p.id;

-- name: DeleteWarrantyClaimPart :execrows
DELETE FROM warranty_claim_parts
WHERE uuid = sqlc.arg(uuid) AND claim_id = sqlc.arg(claim_id);

-- name: AddWarrantyClaimPhoto :one
INSERT INTO warranty_claim_photos (
    claim_id, organization_id, brand_id, storage_key, mime_type, size_bytes, sha256,
    uploaded_by_user_id
) VALUES (
    sqlc.arg(claim_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(storage_key),
    sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.arg(sha256), sqlc.narg(uploaded_by_user_id)
)
RETURNING *;

-- name: ListWarrantyClaimPhotos :many
SELECT * FROM warranty_claim_photos
WHERE claim_id = sqlc.arg(claim_id)
ORDER BY created_at, id;

-- name: CountWarrantyClaimPhotos :one
SELECT COUNT(*) FROM warranty_claim_photos
WHERE claim_id = sqlc.arg(claim_id);

-- name: DeleteWarrantyClaimPhoto :one
-- Returns the storage key so the caller can remove the object.
DELETE FROM warranty_claim_photos
WHERE uuid = sqlc.arg(uuid) AND claim_id = sqlc.arg(claim_id)
RETURNING storage_key;

-- name: AddWarrantyClaimEvent :one
INSERT INTO warranty_claim_events (
    claim_id, organization_id, brand_id, event_type, note, payload, actor_user_id
) VALUES (
    sqlc.arg(claim_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(event_type),
    sqlc.narg(note), sqlc.arg(payload), sqlc.narg(actor_user_id)
)
RETURNING *;

-- name: ListWarrantyClaimEvents :many
SELECT * FROM warranty_claim_events
WHERE claim_id = sqlc.arg(claim_id)
ORDER BY created_at, id;

-- name: GetWarrantyClaimOpenContext :one
SELECT c.id AS claim_id, c.uuid AS claim_uuid, c.organization_id, c.brand_id,
       c.claim_no, c.warranty_id, c.service_id, c.vehicle_id,
       c.customer_user_id, c.status, c.created_at,
       w.uuid AS warranty_uuid, w.public_code, w.start_at, w.end_at,
       w.status AS warranty_status, w.service_item_id,
       s.service_no, s.plate, s.uuid AS service_uuid,
       p.name AS product_name,
       o.name AS organization_name, o.parent_id AS organization_parent_id,
       o.uuid AS organization_uuid,
       parent.type AS organization_parent_type
FROM warranty_claims c
JOIN warranties w ON w.id = c.warranty_id
JOIN services s ON s.id = c.service_id
JOIN products p ON p.id = w.product_id
JOIN organizations o ON o.id = c.organization_id
LEFT JOIN organizations parent ON parent.id = o.parent_id
WHERE c.id = sqlc.arg(id) AND c.brand_id = sqlc.arg(brand_id);

-- name: GetWarrantyClaimCoverageContext :one
SELECT w.id AS warranty_id, w.status AS warranty_status, w.start_at, w.end_at,
       w.service_item_id AS warranty_service_item_id,
       si.product_id AS service_item_product_id, si.applied_parts,
       p.warranty_duration_months
FROM warranties w
JOIN service_items si ON si.id = w.service_item_id
JOIN products p ON p.id = si.product_id
WHERE w.id = sqlc.arg(warranty_id) AND w.brand_id = sqlc.arg(brand_id);

-- name: ListWarrantyClaimNotifyUsersByOrg :many
SELECT DISTINCT om.user_id
FROM organization_members om
JOIN organization_member_roles mr ON mr.member_id = om.id
JOIN roles r ON r.id = mr.role_id
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id
WHERE om.organization_id = ANY(sqlc.arg(organization_ids)::bigint[])
  AND p.slug = sqlc.arg(permission_slug)::text
ORDER BY om.user_id;

-- name: ListWarrantyClaimCenterNotifyUsers :many
SELECT DISTINCT om.user_id
FROM organizations o
JOIN organization_members om ON om.organization_id = o.id
JOIN organization_member_roles mr ON mr.member_id = om.id
JOIN roles r ON r.id = mr.role_id
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id
WHERE o.brand_id = sqlc.arg(brand_id)
  AND o.type = 'center'
  AND p.slug = sqlc.arg(permission_slug)::text
ORDER BY om.user_id;

-- name: WarrantyClaimFailureRateByProduct :many
WITH warranty_counts AS (
    SELECT w.product_id, COUNT(*)::bigint AS warranty_count
    FROM warranties w
    WHERE w.brand_id = sqlc.arg(brand_id)
      AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR w.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
      AND (sqlc.narg(from_at)::timestamptz IS NULL OR w.created_at >= sqlc.narg(from_at)::timestamptz)
      AND (sqlc.narg(to_at)::timestamptz IS NULL OR w.created_at < sqlc.narg(to_at)::timestamptz)
    GROUP BY w.product_id
),
claim_counts AS (
    SELECT w.product_id,
           COUNT(DISTINCT c.id)::bigint AS claim_count,
           COUNT(DISTINCT c.id) FILTER (WHERE c.status IN ('approved', 'reapplied', 'closed'))::bigint AS approved_claim_count
    FROM warranty_claims c
    JOIN warranties w ON w.id = c.warranty_id
    WHERE c.brand_id = sqlc.arg(brand_id)
      AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR c.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
      AND (sqlc.narg(from_at)::timestamptz IS NULL OR c.created_at >= sqlc.narg(from_at)::timestamptz)
      AND (sqlc.narg(to_at)::timestamptz IS NULL OR c.created_at < sqlc.narg(to_at)::timestamptz)
    GROUP BY w.product_id
)
SELECT p.id AS product_id, p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       COALESCE(wc.warranty_count, 0)::bigint AS warranty_count,
       COALESCE(cc.claim_count, 0)::bigint AS claim_count,
       COALESCE(cc.approved_claim_count, 0)::bigint AS approved_claim_count
FROM warranty_counts wc
JOIN products p ON p.id = wc.product_id
LEFT JOIN claim_counts cc ON cc.product_id = wc.product_id
ORDER BY approved_claim_count DESC, claim_count DESC, p.name, p.id;

-- name: WarrantyClaimFailureRateByLot :many
WITH warranty_counts AS (
    SELECT w.unit_id, COUNT(*)::bigint AS warranty_count
    FROM warranties w
    WHERE w.brand_id = sqlc.arg(brand_id)
      AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR w.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
      AND (sqlc.narg(from_at)::timestamptz IS NULL OR w.created_at >= sqlc.narg(from_at)::timestamptz)
      AND (sqlc.narg(to_at)::timestamptz IS NULL OR w.created_at < sqlc.narg(to_at)::timestamptz)
    GROUP BY w.unit_id
),
claim_counts AS (
    SELECT w.unit_id,
           COUNT(DISTINCT c.id)::bigint AS claim_count,
           COUNT(DISTINCT c.id) FILTER (WHERE c.status IN ('approved', 'reapplied', 'closed'))::bigint AS approved_claim_count
    FROM warranty_claims c
    JOIN warranties w ON w.id = c.warranty_id
    WHERE c.brand_id = sqlc.arg(brand_id)
      AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR c.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
      AND (sqlc.narg(from_at)::timestamptz IS NULL OR c.created_at >= sqlc.narg(from_at)::timestamptz)
      AND (sqlc.narg(to_at)::timestamptz IS NULL OR c.created_at < sqlc.narg(to_at)::timestamptz)
    GROUP BY w.unit_id
)
SELECT u.id AS unit_id, u.uuid AS unit_uuid, u.barcode AS lot_code,
       p.id AS product_id, p.uuid AS product_uuid, p.sku AS product_sku, p.name AS product_name,
       COALESCE(wc.warranty_count, 0)::bigint AS warranty_count,
       COALESCE(cc.claim_count, 0)::bigint AS claim_count,
       COALESCE(cc.approved_claim_count, 0)::bigint AS approved_claim_count
FROM warranty_counts wc
JOIN units u ON u.id = wc.unit_id
JOIN products p ON p.id = u.product_id
LEFT JOIN claim_counts cc ON cc.unit_id = wc.unit_id
ORDER BY approved_claim_count DESC, claim_count DESC, p.name, u.barcode, u.id;

-- name: WarrantyClaimsByDealerReport :many
SELECT o.id AS organization_id, o.uuid AS organization_uuid, o.name AS organization_name,
       o.type AS organization_type,
       COUNT(c.id)::bigint AS claim_count,
       COUNT(c.id) FILTER (WHERE c.status IN ('approved', 'reapplied', 'closed'))::bigint AS approved_claim_count,
       COUNT(c.id) FILTER (WHERE c.status = 'rejected')::bigint AS rejected_claim_count
FROM warranty_claims c
JOIN organizations o ON o.id = c.organization_id
WHERE c.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR c.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR c.created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR c.created_at < sqlc.narg(to_at)::timestamptz)
GROUP BY o.id, o.uuid, o.name, o.type
ORDER BY claim_count DESC, approved_claim_count DESC, o.name, o.id;

-- name: WarrantyClaimPartsReport :many
SELECT cp.part_key,
       COALESCE(p.id, 0)::bigint AS product_id,
       p.uuid AS product_uuid,
       COALESCE(p.sku, '')::text AS product_sku,
       COALESCE(p.name, '')::text AS product_name,
       COUNT(cp.id)::bigint AS part_count,
       COUNT(DISTINCT cp.claim_id)::bigint AS claim_count,
       COUNT(DISTINCT cp.claim_id) FILTER (WHERE c.status IN ('approved', 'reapplied', 'closed'))::bigint AS approved_claim_count
FROM warranty_claim_parts cp
JOIN warranty_claims c ON c.id = cp.claim_id
LEFT JOIN products p ON p.id = cp.product_id
WHERE c.brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR c.organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR c.created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR c.created_at < sqlc.narg(to_at)::timestamptz)
GROUP BY cp.part_key, p.id, p.uuid, p.sku, p.name
ORDER BY part_count DESC, claim_count DESC, cp.part_key, p.name;

-- TEC-337 (F3-06d): consumed quantity of each item of a completed
-- re-application service (meters of a cut, pieces, a whole roll, else one
-- piece) with the price buyer_org_id paid for the unit on its latest received
-- order (NULL when it has none; the caller falls back to the F1 price chain).
-- name: ListWarrantyReapplyItemCosts :many
SELECT si.id, si.product_id, si.unit_id,
       COALESCE(si.meters, si.quantity::numeric, u.initial_meters, 1)::numeric AS consumed,
       bought.unit_price::numeric AS order_unit_price,
       COALESCE(bought.currency, '')::text AS order_currency
FROM service_items si
JOIN services s ON s.id = si.service_id
JOIN units u ON u.id = si.unit_id
LEFT JOIN LATERAL (
    SELECT oi.unit_price, o.currency
    FROM order_item_units oiu
    JOIN order_items oi ON oi.id = oiu.order_item_id
    JOIN orders o ON o.id = oi.order_id
    WHERE oiu.unit_id = si.unit_id
      AND o.buyer_org_id = sqlc.arg(buyer_org_id)::bigint
      AND o.status = 'received'
    ORDER BY o.id DESC, oi.id DESC, oiu.id DESC
    LIMIT 1
) bought ON TRUE
WHERE s.id = sqlc.arg(service_id) AND s.brand_id = sqlc.arg(brand_id)
ORDER BY si.id;

-- name: CountWarrantyClaimFinanceEntries :one
SELECT COUNT(*)::bigint FROM finance_entries
WHERE source_type = 'warranty_claim' AND source_uuid = sqlc.arg(source_uuid)::uuid;

-- Open (unreversed) warranty_claim rows of one organization's book (cost
-- summary of the claim detail).
-- name: ListWarrantyClaimCostEntries :many
SELECT e.role, e.direction, e.category, e.currency, e.amount
FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.source_type = 'warranty_claim' AND e.source_uuid = sqlc.arg(source_uuid)::uuid
  AND e.reversal_of_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM finance_entries r WHERE r.reversal_of_id = e.id)
ORDER BY e.id;
