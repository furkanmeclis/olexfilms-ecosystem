-- TEC-1036: the platform "user 360" view (GET /v1/platform/users/{uuid}/overview).

-- name: UserOverviewMemberships :many
-- Every organization membership of a user across brands, with its
-- organization roles and warehouse memberships.
SELECT om.role, om.created_at,
       o.uuid AS organization_uuid, o.name AS organization_name, o.slug AS organization_slug,
       o.type AS organization_type, o.status AS organization_status,
       b.slug AS brand_slug, p.name AS parent_name,
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
JOIN brands b ON b.id = o.brand_id
LEFT JOIN organizations p ON p.id = o.parent_id
WHERE om.user_id = $1
ORDER BY o.type ASC, o.name ASC;

-- name: UserOverviewCustomerOrganizations :many
-- Organizations the user is a customer of (K11: a customer is global).
SELECT o.uuid AS organization_uuid, o.name AS organization_name, o.type AS organization_type,
       b.slug AS brand_slug, co.first_service_at, co.created_at
FROM customer_organizations co
JOIN organizations o ON o.id = co.organization_id AND o.deleted_at IS NULL
JOIN brands b ON b.id = co.brand_id
WHERE co.user_id = $1
ORDER BY co.created_at DESC;

-- name: UserOverviewCustomer :one
SELECT
    EXISTS (SELECT 1 FROM customer_profiles cp WHERE cp.user_id = sqlc.arg(user_id))::boolean AS has_profile,
    COALESCE((SELECT cp.type FROM customer_profiles cp WHERE cp.user_id = sqlc.arg(user_id)), '')::text AS profile_type,
    COALESCE((SELECT cp.company_name FROM customer_profiles cp WHERE cp.user_id = sqlc.arg(user_id)), '')::text AS company_name,
    (SELECT COUNT(*) FROM vehicles v WHERE v.user_id = sqlc.arg(user_id) AND v.deleted_at IS NULL)::bigint AS vehicle_count;

-- name: UserOverviewActiveSessions :many
-- Unrevoked, unexpired refresh sessions (newest first).
SELECT rt.uuid, rt.realm, rt.client, rt.device_name, rt.user_agent, rt.ip_address, rt.created_at,
       rt.expires_at, o.name AS organization_name, (rt.impersonator_user_id IS NOT NULL)::boolean AS impersonated
FROM refresh_tokens rt
LEFT JOIN organizations o ON o.id = rt.organization_id
WHERE rt.user_id = $1 AND rt.revoked_at IS NULL AND rt.expires_at > NOW()
ORDER BY rt.created_at DESC
LIMIT 20;
