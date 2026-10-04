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

-- name: GetLiveWarrantyClaimByWarranty :one
-- The live (not rejected / closed) claim of a warranty, if any.
SELECT * FROM warranty_claims
WHERE warranty_id = sqlc.arg(warranty_id)
  AND status NOT IN ('rejected', 'closed');

-- name: ListWarrantyClaimsByWarranty :many
SELECT * FROM warranty_claims
WHERE warranty_id = sqlc.arg(warranty_id) AND brand_id = sqlc.arg(brand_id)
ORDER BY created_at DESC, id DESC;

-- name: ListWarrantyClaimsInScope :many
SELECT * FROM warranty_claims
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.arg(statuses)::varchar[] IS NULL OR status = ANY(sqlc.arg(statuses)::varchar[]))
  AND (sqlc.narg(warranty_id)::bigint IS NULL OR warranty_id = sqlc.narg(warranty_id)::bigint)
  AND (sqlc.narg(service_id)::bigint IS NULL OR service_id = sqlc.narg(service_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL OR description ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWarrantyClaimsInScope :one
SELECT COUNT(*) FROM warranty_claims
WHERE brand_id = sqlc.arg(brand_id)
  AND (sqlc.arg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.arg(organization_ids)::bigint[]))
  AND (sqlc.arg(statuses)::varchar[] IS NULL OR status = ANY(sqlc.arg(statuses)::varchar[]))
  AND (sqlc.narg(warranty_id)::bigint IS NULL OR warranty_id = sqlc.narg(warranty_id)::bigint)
  AND (sqlc.narg(service_id)::bigint IS NULL OR service_id = sqlc.narg(service_id)::bigint)
  AND (sqlc.narg(vehicle_id)::bigint IS NULL OR vehicle_id = sqlc.narg(vehicle_id)::bigint)
  AND (sqlc.narg(customer_user_id)::bigint IS NULL OR customer_user_id = sqlc.narg(customer_user_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(q)::text IS NULL OR description ILIKE '%' || sqlc.narg(q)::text || '%');

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
       s.service_no, s.plate,
       p.name AS product_name,
       o.name AS organization_name, o.parent_id AS organization_parent_id,
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
