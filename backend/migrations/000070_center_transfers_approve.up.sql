-- TEC-228: a distributor returns stock to its parent, the center (TEC-223),
-- but no center role held transfers.approve, so only super_admin could
-- decide such a return. The center staff (owner/admin) and the center
-- warehouse now hold it at the brand scope; center_accounting and
-- center_social do not. Grants only; the permission itself exists since
-- 000049. Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('center_staff', 'transfers.approve', 'brand'),
    ('center_warehouse', 'transfers.approve', 'brand')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
