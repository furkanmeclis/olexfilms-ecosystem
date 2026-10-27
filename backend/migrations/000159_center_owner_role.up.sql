-- TEC-1026: center_owner. The center's owner membership used to map to
-- center_staff only (83 grants), so the owner saw no warehouse, accounting,
-- social or service point screens. center_owner is the union of the five
-- center roles (broadest scope on overlap); the Go catalog
-- (internal/platform/rbac) derives it the same way and roles-sync keeps it
-- in step when a center role gains a permission.
INSERT INTO roles (name, slug, description, is_system, org_type) VALUES
    ('Center owner', 'center_owner',
     'Center owner: every center department (staff, warehouse, accounting, social and service point) in one role',
     TRUE, 'center')
ON CONFLICT (slug) DO NOTHING;

-- Follow-ups found in the live demo, applied before the union so the owner
-- inherits them:
--  * einvoice.settings was super_admin-only; the center accounting team
--    could not fill the seller profile / series form.
--  * center_staff had no warranty_claims.write, so the center could not open
--    a claim for its own service point (TEC-1003, K4).
UPDATE permissions
SET super_admin_only = FALSE, is_sensitive = TRUE, scopes = ARRAY['brand', 'all']
WHERE slug = 'einvoice.settings';

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'brand'
FROM roles r JOIN permissions p ON p.slug = 'einvoice.settings'
WHERE r.slug = 'center_accounting'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r JOIN permissions p ON p.slug = 'warranty_claims.write'
WHERE r.slug = 'center_staff'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- Union of the member roles: for every permission the broadest scope held by
-- any of them (all > brand > subtree > managed > assigned > own).
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT owner.id, u.permission_id, u.scope
FROM roles owner
CROSS JOIN LATERAL (
    SELECT DISTINCT ON (rp.permission_id) rp.permission_id, rp.scope
    FROM role_permissions rp
    JOIN roles src ON src.id = rp.role_id
    WHERE src.slug IN ('center_staff', 'center_warehouse', 'center_accounting', 'center_social', 'center_service')
    ORDER BY rp.permission_id,
             CASE rp.scope
                 WHEN 'all' THEN 6 WHEN 'brand' THEN 5 WHEN 'subtree' THEN 4
                 WHEN 'managed' THEN 3 WHEN 'assigned' THEN 2 WHEN 'own' THEN 1 ELSE 0
             END DESC
) u
WHERE owner.slug = 'center_owner'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;

-- Existing center owner memberships: grant center_owner and drop the
-- department roles that were added by hand to work around the gap (the
-- owner role covers all of them).
INSERT INTO organization_member_roles (member_id, role_id)
SELECT om.id, r.id
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.type = 'center'
JOIN roles r ON r.slug = 'center_owner'
WHERE om.role = 'owner'
ON CONFLICT DO NOTHING;

DELETE FROM organization_member_roles mr
USING organization_members om, organizations o, roles r
WHERE mr.member_id = om.id
  AND om.organization_id = o.id AND o.type = 'center' AND om.role = 'owner'
  AND r.id = mr.role_id
  AND r.slug IN ('center_staff', 'center_warehouse', 'center_accounting', 'center_social', 'center_service');
