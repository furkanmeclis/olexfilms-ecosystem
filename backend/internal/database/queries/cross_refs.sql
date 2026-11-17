-- TEC-1216 (cross-record refs): detail summaries of records that had no
-- tenant detail endpoint. Callers authorize first (organizations.read /
-- members.read scope) and show a counter only when the caller holds that
-- module's read permission.

-- name: OrganizationSummaryCounts :one
-- Counters of one organization: its members, direct children, customers,
-- services, warranties, orders (as seller or buyer), open tasks about it,
-- leads and warehouses.
SELECT
  (SELECT COUNT(*) FROM organization_members m WHERE m.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS members,
  (SELECT COUNT(*) FROM organizations c WHERE c.parent_id = sqlc.arg(organization_id)::bigint AND c.deleted_at IS NULL)::bigint AS children,
  (SELECT COUNT(*) FROM customer_organizations co WHERE co.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS customers,
  (SELECT COUNT(*) FROM services s WHERE s.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS services,
  (SELECT COUNT(*) FROM warranties w WHERE w.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS warranties,
  (SELECT COUNT(*) FROM orders o WHERE o.organization_id = sqlc.arg(organization_id)::bigint
      OR o.buyer_org_id = sqlc.arg(organization_id)::bigint)::bigint AS orders,
  (SELECT COUNT(*) FROM tasks t WHERE t.subject_org_id = sqlc.arg(organization_id)::bigint
      AND t.status IN ('open', 'in_progress'))::bigint AS open_tasks,
  (SELECT COUNT(*) FROM leads l WHERE l.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS leads,
  (SELECT COUNT(*) FROM warehouses wh WHERE wh.organization_id = sqlc.arg(organization_id)::bigint)::bigint AS warehouses;

-- name: TenantMemberUser :one
-- A staff user who is a member of at least one organization of the brand
-- inside org_ids (NULL = the whole brand).
SELECT u.id, u.uuid, u.name, u.surname, u.email, u.phone_e164, u.status, u.last_login_at, u.created_at
FROM users u
WHERE u.uuid = sqlc.arg(uuid)
  AND EXISTS (
    SELECT 1 FROM organization_members om
    JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
    WHERE om.user_id = u.id AND o.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]))
  );

-- name: TenantMemberMemberships :many
-- The memberships of a user inside the brand and org_ids (NULL = brand),
-- with organization roles and warehouse memberships.
SELECT om.role, om.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       ARRAY(
           SELECT r.slug FROM organization_member_roles mr
           JOIN roles r ON r.id = mr.role_id
           WHERE mr.member_id = om.id
           ORDER BY r.slug
       )::text[] AS role_slugs,
       COALESCE((
           SELECT jsonb_agg(jsonb_build_object(
                      'uuid', w.uuid, 'name', w.name, 'code', w.code, 'is_default', wm.is_default
                  ) ORDER BY w.name)
           FROM warehouse_members wm
           JOIN warehouses w ON w.id = wm.warehouse_id
           WHERE wm.user_id = om.user_id AND wm.organization_id = om.organization_id
       ), '[]'::jsonb)::jsonb AS warehouses
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE om.user_id = sqlc.arg(user_id)
  AND o.brand_id = sqlc.arg(brand_id)
  AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.id = ANY (sqlc.narg(org_ids)::bigint[]))
ORDER BY o.type ASC, o.name ASC;

-- name: TenantMemberCounts :one
-- What a staff user did inside the brand and org_ids (NULL = brand).
SELECT
  (SELECT COUNT(*) FROM services s WHERE s.created_by_user_id = sqlc.arg(user_id)::bigint AND s.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[])))::bigint AS services_created,
  (SELECT COUNT(*) FROM services s WHERE COALESCE(s.performed_by_user_id, s.created_by_user_id) = sqlc.arg(user_id)::bigint
      AND s.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR s.organization_id = ANY (sqlc.narg(org_ids)::bigint[])))::bigint AS services_performed,
  (SELECT COUNT(*) FROM orders o WHERE o.created_by_user_id = sqlc.arg(user_id)::bigint AND o.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR o.organization_id = ANY (sqlc.narg(org_ids)::bigint[])
           OR o.buyer_org_id = ANY (sqlc.narg(org_ids)::bigint[])))::bigint AS orders_created,
  (SELECT COUNT(*) FROM tasks t WHERE t.assignee_user_id = sqlc.arg(user_id)::bigint AND t.brand_id = sqlc.arg(brand_id)
      AND t.status IN ('open', 'in_progress'))::bigint AS open_tasks,
  (SELECT COUNT(*) FROM leads l WHERE l.assignee_user_id = sqlc.arg(user_id)::bigint AND l.brand_id = sqlc.arg(brand_id)
      AND (sqlc.narg(org_ids)::bigint[] IS NULL OR l.organization_id = ANY (sqlc.narg(org_ids)::bigint[])))::bigint AS leads_assigned;
