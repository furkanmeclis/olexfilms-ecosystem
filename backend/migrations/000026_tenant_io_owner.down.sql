DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.settings.read',
        'tenant.settings.write',
        'tenant.imports.read'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.settings.read',
    'tenant.settings.write',
    'tenant.imports.read'
);

DROP INDEX IF EXISTS idx_import_jobs_organization;

ALTER TABLE import_jobs DROP COLUMN IF EXISTS organization_id;

ALTER TABLE organizations
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS website,
    DROP COLUMN IF EXISTS tagline,
    DROP COLUMN IF EXISTS footer_text,
    DROP COLUMN IF EXISTS paper_size,
    DROP COLUMN IF EXISTS primary_color;
