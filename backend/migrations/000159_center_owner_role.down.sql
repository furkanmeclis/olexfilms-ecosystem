-- Center owner memberships fall back to center_staff (the pre-TEC-1026
-- default); the role and its grants are removed (member rows cascade).
INSERT INTO organization_member_roles (member_id, role_id)
SELECT om.id, r.id
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.type = 'center'
JOIN organization_member_roles mr ON mr.member_id = om.id
JOIN roles owner ON owner.id = mr.role_id AND owner.slug = 'center_owner'
JOIN roles r ON r.slug = 'center_staff'
ON CONFLICT DO NOTHING;

DELETE FROM roles WHERE slug = 'center_owner';

DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND ((r.slug = 'center_accounting' AND p.slug = 'einvoice.settings')
    OR (r.slug = 'center_staff' AND p.slug = 'warranty_claims.write'));

UPDATE permissions
SET super_admin_only = TRUE, is_sensitive = FALSE, scopes = ARRAY['all']
WHERE slug = 'einvoice.settings';
