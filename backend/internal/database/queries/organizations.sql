-- name: CreateOrganization :one
INSERT INTO organizations (
    slug, name, city, district, phone, address, status, plan_code,
    access_starts_at, access_ends_at,
    type, parent_id, brand_id, currency, locale, timezone, settings,
    country_id, province_id, district_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17,
    $18, $19, $20
)
RETURNING *;

-- name: GetOrganizationByUUID :one
SELECT * FROM organizations
WHERE uuid = $1 AND deleted_at IS NULL;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations
WHERE slug = $1 AND deleted_at IS NULL;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL;

-- name: SlugExists :one
SELECT EXISTS(
    SELECT 1 FROM organizations WHERE slug = $1 AND deleted_at IS NULL
) AS exists;

-- name: ListOrganizationsFiltered :many
SELECT sqlc.embed(o), b.slug AS brand_slug, p.uuid AS parent_uuid, p.name AS parent_name
FROM organizations o
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE o.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR o.status = sqlc.narg(status))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR o.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(type)::text IS NULL OR o.type = sqlc.narg(type))
  AND (sqlc.narg(parent_id)::bigint IS NULL OR o.parent_id = sqlc.narg(parent_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR o.name ILIKE '%' || sqlc.narg(q) || '%'
    OR o.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR o.city ILIKE '%' || sqlc.narg(q) || '%'
    OR o.phone ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY o.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizations :one
SELECT COUNT(*)::bigint
FROM organizations
WHERE deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type))
  AND (sqlc.narg(parent_id)::bigint IS NULL OR parent_id = sqlc.narg(parent_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
    OR city ILIKE '%' || sqlc.narg(q) || '%'
    OR phone ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: UpdateOrganizationPlatform :one
UPDATE organizations
SET name = COALESCE(sqlc.narg(name), name),
    city = COALESCE(sqlc.narg(city), city),
    district = COALESCE(sqlc.narg(district), district),
    phone = COALESCE(sqlc.narg(phone), phone),
    address = COALESCE(sqlc.narg(address), address),
    status = COALESCE(sqlc.narg(status), status),
    plan_code = COALESCE(sqlc.narg(plan_code), plan_code),
    access_starts_at = COALESCE(sqlc.narg(access_starts_at), access_starts_at),
    access_ends_at = sqlc.narg(access_ends_at),
    currency = COALESCE(sqlc.narg(currency), currency),
    locale = COALESCE(sqlc.narg(locale), locale),
    timezone = COALESCE(sqlc.narg(timezone), timezone),
    country_id = CASE WHEN sqlc.arg(set_address)::bool THEN sqlc.narg(country_id)::bigint ELSE country_id END,
    province_id = CASE WHEN sqlc.arg(set_address)::bool THEN sqlc.narg(province_id)::bigint ELSE province_id END,
    district_id = CASE WHEN sqlc.arg(set_address)::bool THEN sqlc.narg(district_id)::bigint ELSE district_id END
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateOrganizationLetterhead :one
UPDATE organizations
SET name = COALESCE(sqlc.narg(name), name),
    city = COALESCE(sqlc.narg(city), city),
    district = COALESCE(sqlc.narg(district), district),
    phone = COALESCE(sqlc.narg(phone), phone),
    address = COALESCE(sqlc.narg(address), address),
    email = COALESCE(sqlc.narg(email), email),
    website = COALESCE(sqlc.narg(website), website),
    tagline = COALESCE(sqlc.narg(tagline), tagline),
    footer_text = COALESCE(sqlc.narg(footer_text), footer_text),
    paper_size = COALESCE(sqlc.narg(paper_size), paper_size),
    primary_color = COALESCE(sqlc.narg(primary_color), primary_color)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetOrganizationLogo :one
UPDATE organizations
SET logo_object_key = $2
WHERE uuid = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ClearOrganizationLogo :one
UPDATE organizations
SET logo_object_key = NULL
WHERE uuid = $1 AND deleted_at IS NULL
RETURNING *;

-- name: CreateOrganizationMember :one
INSERT INTO organization_members (organization_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationMember :one
SELECT om.*, o.uuid AS organization_uuid, o.slug AS organization_slug
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE om.organization_id = $1 AND om.user_id = $2;

-- name: GetOrganizationMemberByUserAndSlug :one
SELECT om.*, o.uuid AS organization_uuid, o.slug AS organization_slug, o.status AS organization_status,
       o.access_starts_at, o.access_ends_at, o.name AS organization_name, o.logo_object_key,
       o.brand_id AS organization_brand_id, o.type AS organization_type
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE om.user_id = $1 AND o.slug = $2;

-- name: GetOrganizationMemberByUserAndOrgUUID :one
SELECT om.id, om.organization_id, om.user_id, om.role, om.created_at,
       o.uuid AS organization_uuid, o.slug AS organization_slug, o.status AS organization_status,
       o.access_starts_at, o.access_ends_at, o.name AS organization_name,
       o.brand_id AS organization_brand_id, o.type AS organization_type, b.slug AS brand_slug
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
JOIN brands b ON b.id = o.brand_id
WHERE om.user_id = $1 AND o.uuid = $2;

-- name: ListOrganizationMembersByUserID :many
SELECT om.role, o.uuid, o.slug, o.name, o.logo_object_key, o.status, o.access_ends_at,
       o.type, o.brand_id, b.slug AS brand_slug, b.name AS brand_name,
       p.uuid AS parent_uuid, p.slug AS parent_slug, p.name AS parent_name
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE om.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(brand_id)::bigint IS NULL OR o.brand_id = sqlc.narg(brand_id))
ORDER BY o.name ASC;

-- name: ListOrganizationMembers :many
SELECT u.uuid, u.email, u.name, u.surname, u.status, om.role, om.created_at
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1
ORDER BY om.created_at ASC;

-- name: GetOrganizationMemberByUserUUID :one
SELECT om.id, om.organization_id, om.user_id, om.role, om.created_at,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.status
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1 AND u.uuid = $2;

-- name: ListOrganizationMemberOptions :many
SELECT u.uuid, u.email, u.name, u.surname, om.role
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1 AND u.status = 'active'
ORDER BY u.name ASC, u.surname ASC;

-- name: SupplierOf :one
-- The supplier of an organization is its parent in the tree (K9).
-- Returns no rows for a center.
SELECT p.*
FROM organizations o
JOIN organizations p ON p.id = o.parent_id AND p.deleted_at IS NULL
WHERE o.id = $1 AND o.deleted_at IS NULL;

-- name: Descendants :many
-- Every organization below the given one (not including itself).
WITH RECURSIVE tree AS (
    SELECT c.id, 1 AS depth
    FROM organizations c
    WHERE c.parent_id = sqlc.arg(id)::bigint AND c.deleted_at IS NULL
    UNION ALL
    SELECT c.id, t.depth + 1
    FROM organizations c
    JOIN tree t ON c.parent_id = t.id
    WHERE c.deleted_at IS NULL AND t.depth < 16
)
SELECT o.*
FROM tree
JOIN organizations o ON o.id = tree.id
ORDER BY tree.depth ASC, o.name ASC;

-- name: ListOrganizationChildren :many
SELECT sqlc.embed(o), b.slug AS brand_slug, p.uuid AS parent_uuid, p.name AS parent_name
FROM organizations o
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE o.parent_id = $1 AND o.deleted_at IS NULL
ORDER BY o.name ASC;

-- name: GetOrganizationTreeByUUID :one
SELECT sqlc.embed(o), b.slug AS brand_slug, p.uuid AS parent_uuid, p.name AS parent_name
FROM organizations o
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE o.uuid = $1 AND o.deleted_at IS NULL;

-- name: UpdateOrganizationParent :one
UPDATE organizations
SET parent_id = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- InsertOrganizationParentChange records one re-parenting (K25, TEC-198);
-- its uuid is the change id the cari transfer rows are sourced by.
-- name: InsertOrganizationParentChange :one
INSERT INTO organization_parent_changes (
    organization_id, brand_id, old_parent_id, new_parent_id, actor_user_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(old_parent_id),
    sqlc.arg(new_parent_id), sqlc.narg(actor_user_id)
)
RETURNING *;

-- name: AssignMemberRoleBySlug :exec
INSERT INTO organization_member_roles (member_id, role_id)
SELECT sqlc.arg(member_id), r.id
FROM roles r
WHERE r.slug = sqlc.arg(slug)
ON CONFLICT DO NOTHING;

-- name: DeleteMemberRoles :exec
DELETE FROM organization_member_roles WHERE member_id = $1;

-- name: ListMemberRolesByOrganization :many
SELECT om.id AS member_id, r.slug
FROM organization_members om
INNER JOIN organization_member_roles mr ON mr.member_id = om.id
INNER JOIN roles r ON r.id = mr.role_id
WHERE om.organization_id = $1
ORDER BY om.id, r.slug;

-- name: ListOrganizationsInScope :many
-- Organizations reachable by a scope filter: an explicit id set
-- (managed/subtree) or a whole brand (brand), or every brand (all, both NULL).
SELECT sqlc.embed(o), b.slug AS brand_slug, p.uuid AS parent_uuid, p.name AS parent_name
FROM organizations o
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE o.deleted_at IS NULL
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]))
  AND (sqlc.narg(brand_id)::bigint IS NULL OR o.brand_id = sqlc.narg(brand_id))
  AND (sqlc.narg(type)::text IS NULL OR o.type = sqlc.narg(type))
  AND (sqlc.narg(q)::text IS NULL OR o.name ILIKE '%' || sqlc.narg(q)::text || '%' OR o.slug ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(uuids)::uuid[] IS NULL OR o.uuid = ANY (sqlc.narg(uuids)::uuid[]))
ORDER BY o.type ASC, o.name ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
