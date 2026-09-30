ALTER TABLE organizations
    ADD COLUMN email         VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN website       VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN tagline       VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN footer_text   TEXT         NOT NULL DEFAULT '',
    ADD COLUMN paper_size    VARCHAR(8)   NOT NULL DEFAULT '',
    ADD COLUMN primary_color VARCHAR(16)  NOT NULL DEFAULT '';

ALTER TABLE import_jobs
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE;

CREATE INDEX idx_import_jobs_organization
    ON import_jobs (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant export settings', 'tenant.settings.read'),
    ('Write tenant export settings', 'tenant.settings.write'),
    ('Read tenant import jobs', 'tenant.imports.read')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read',
    'tenant.settings.write',
    'tenant.imports.read'
)
WHERE r.slug IN ('organization_owner', 'super_admin')
ON CONFLICT DO NOTHING;
