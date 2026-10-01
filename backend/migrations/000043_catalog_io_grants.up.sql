-- TEC-145 (F1-01b): the center writes the catalog through the I/O engine
-- (product import/export). Import and export jobs of an organization are read
-- through tenant.imports.read / tenant.exports.read, which until now only the
-- distributor and dealer owners held; center_staff gets them too so the
-- center can preview, confirm and download its own catalog jobs.
-- Source of truth: internal/platform/rbac/catalog.go.
INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.imports.read', 'tenant.exports.read')
WHERE r.slug = 'center_staff'
ON CONFLICT DO NOTHING;
