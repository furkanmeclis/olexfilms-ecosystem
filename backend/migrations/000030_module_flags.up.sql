-- TEC-86 (F0-09): module packages (feature flags).
--
-- modules is the catalog (single source: internal/platform/features). The
-- platform admin may change default_enabled / is_paid; level and sort_order
-- follow the Go catalog (features.SyncCatalog).
--
-- module_flags holds explicit switches, never copies:
--   system           admin closes a module everywhere (no exception)
--   org              a value for one organization (source admin|distributor|service)
--   dealer_standard  the distributor's standard for its dealers
-- The effective value is resolved on read (live inheritance), so a change of
-- the dealer standard reaches every dealer without rewriting rows, and a new
-- dealer gets the standard the moment it is created.

CREATE TABLE modules (
    key             VARCHAR(64) PRIMARY KEY,
    level           VARCHAR(16) NOT NULL,
    default_enabled BOOLEAN     NOT NULL,
    is_paid         BOOLEAN     NOT NULL DEFAULT false,
    sort_order      INT         NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_modules_level CHECK (level IN ('core', 'standard', 'addon')),
    CONSTRAINT chk_modules_core_on CHECK (level <> 'core' OR default_enabled)
);

CREATE TRIGGER trg_modules_set_updated_at
    BEFORE UPDATE ON modules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE module_flags (
    id              BIGSERIAL PRIMARY KEY,
    scope           VARCHAR(16) NOT NULL,
    organization_id BIGINT      NULL REFERENCES organizations (id) ON DELETE CASCADE,
    module_key      VARCHAR(64) NOT NULL REFERENCES modules (key) ON DELETE CASCADE,
    enabled         BOOLEAN     NOT NULL,
    source          VARCHAR(16) NOT NULL,
    set_by_user_id  BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    -- F3: the "module package" service that switched the module on.
    service_id      BIGINT      NULL,
    note            TEXT        NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_module_flags_scope CHECK (scope IN ('system', 'org', 'dealer_standard')),
    CONSTRAINT chk_module_flags_source CHECK (source IN ('default', 'admin', 'distributor', 'service')),
    CONSTRAINT chk_module_flags_org CHECK ((scope = 'system') = (organization_id IS NULL))
);

CREATE UNIQUE INDEX uq_module_flags_system ON module_flags (module_key) WHERE scope = 'system';
CREATE UNIQUE INDEX uq_module_flags_org ON module_flags (scope, organization_id, module_key)
    WHERE organization_id IS NOT NULL;
CREATE INDEX idx_module_flags_org ON module_flags (organization_id) WHERE organization_id IS NOT NULL;

CREATE TRIGGER trg_module_flags_set_updated_at
    BEFORE UPDATE ON module_flags
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO modules (key, level, default_enabled, is_paid, sort_order) VALUES
    ('organizations', 'core', true, false, 10),
    ('regions', 'core', true, false, 20),
    ('catalog', 'core', true, false, 30),
    ('stock', 'core', true, false, 40),
    ('warehouse', 'core', true, false, 50),
    ('orders', 'core', true, false, 60),
    ('services', 'core', true, false, 70),
    ('customers', 'core', true, false, 80),
    ('notifications', 'core', true, false, 90),
    ('accounting', 'core', true, false, 100),
    ('search', 'core', true, false, 110),
    ('import_export', 'core', true, false, 120),
    ('tasks', 'core', true, false, 130),
    ('system_settings', 'core', true, false, 140),
    ('intake_contracts', 'standard', true, false, 150),
    ('measurements', 'standard', true, false, 160),
    ('leads', 'standard', true, false, 170),
    ('appointments', 'standard', true, false, 180),
    ('announcements', 'standard', true, false, 190),
    ('warranty_claims', 'standard', true, false, 200),
    ('dealer_accounting', 'standard', true, false, 210),
    ('service_catalog', 'standard', true, false, 220),
    ('dealer_transfers', 'standard', true, false, 230),
    ('ai_assistant', 'addon', false, true, 240),
    ('whatsapp_gateway', 'addon', false, true, 250),
    ('mcp', 'addon', false, true, 260),
    ('dealer_showcase', 'addon', false, true, 270),
    ('fleet', 'addon', false, true, 280),
    ('certificates', 'addon', false, true, 290),
    ('stock_forecast', 'addon', false, true, 300),
    ('performance', 'addon', false, true, 310),
    ('efficiency', 'addon', false, true, 320),
    ('campaigns', 'addon', false, true, 330),
    ('photo_standard', 'addon', false, false, 340),
    ('reviews', 'addon', false, true, 350),
    ('e_invoice', 'addon', false, true, 360),
    ('short_url', 'addon', false, true, 370);

-- Permissions (rbac catalog, appended after privacy.anonymize = 710).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order) VALUES
    ('Read module settings', 'modules.read', 'modules', ARRAY['managed', 'subtree']::text[], false, false,
     'Module flags of the organization (managed) or of its dealers (subtree); request a module.', 720),
    ('Manage dealer modules', 'modules.manage', 'modules', ARRAY['subtree']::text[], false, false,
     'Switch modules of the distributor''s dealers and edit the dealer standard.', 730),
    ('Read platform modules', 'platform.modules.read', 'platform', ARRAY['all']::text[], false, false, NULL, 740),
    ('Write platform modules', 'platform.modules.write', 'platform', ARRAY['all']::text[], false, false, NULL, 750)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    module = EXCLUDED.module,
    scopes = EXCLUDED.scopes,
    is_sensitive = EXCLUDED.is_sensitive,
    super_admin_only = EXCLUDED.super_admin_only,
    description = EXCLUDED.description,
    sort_order = EXCLUDED.sort_order;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'modules.read', 'subtree'),
    ('super_admin', 'modules.manage', 'subtree'),
    ('super_admin', 'platform.modules.read', 'all'),
    ('super_admin', 'platform.modules.write', 'all'),
    ('distributor_owner', 'modules.read', 'subtree'),
    ('distributor_owner', 'modules.manage', 'subtree'),
    ('dealer_owner', 'modules.read', 'managed')
) AS g (role_slug, permission_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.permission_slug
ON CONFLICT DO NOTHING;
