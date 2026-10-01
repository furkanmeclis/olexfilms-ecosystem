-- TEC-85 (F0-08): scoped RBAC and the role matrix.
--
-- Permissions become module + action + scope. The slug stays
-- `<module>.<action>`; the catalog declares which scopes a permission may be
-- granted with, and each role grant stores the scope it was given.
--
-- Scopes (broadest first):
--   all      every record (platform operators only)
--   brand    every organization of the active brand (center roles)
--   subtree  the active organization and every organization below it
--   managed  records of the active organization
--   assigned records assigned to the user
--   own      records the user created
--   customer records of the customer user themself (portal)
--
-- Roles carry an org_type. platform/customer/fleet roles are global
-- (user_roles); center/distributor/dealer roles are granted per membership
-- (organization_member_roles). The Go catalog in internal/platform/rbac is the
-- single source of truth; `cmd/roles-sync` reconciles a database with it and
-- must report no changes right after this migration.

ALTER TABLE permissions
    ADD COLUMN module           VARCHAR(60) NOT NULL DEFAULT 'platform',
    ADD COLUMN scopes           TEXT[]      NOT NULL DEFAULT ARRAY['all'],
    ADD COLUMN is_sensitive     BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN super_admin_only BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN description      TEXT        NULL,
    ADD COLUMN sort_order       INT         NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_permissions_scopes CHECK (
        cardinality(scopes) > 0
        AND scopes <@ ARRAY['all', 'brand', 'subtree', 'managed', 'assigned', 'own', 'customer']::text[]
    );

CREATE INDEX idx_permissions_module ON permissions (module, sort_order);

ALTER TABLE roles
    ADD COLUMN org_type VARCHAR(16) NULL,
    ADD CONSTRAINT chk_roles_org_type CHECK (
        org_type IS NULL
        OR org_type IN ('platform', 'center', 'distributor', 'dealer', 'customer', 'fleet')
    );

ALTER TABLE role_permissions
    ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'all',
    ADD CONSTRAINT chk_role_permissions_scope CHECK (
        scope IN ('all', 'brand', 'subtree', 'managed', 'assigned', 'own', 'customer')
    );

-- Seed / refresh the permission catalog before the grant triggers exist so
-- existing rows can be rewritten.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order) VALUES
    ('Auth session', 'auth.session', 'auth', ARRAY['own']::text[], false, false, NULL, 10),
    ('Read notifications', 'notifications.read', 'notifications', ARRAY['own']::text[], false, false, NULL, 20),
    ('Manage notifications', 'notifications.manage', 'notifications', ARRAY['own']::text[], false, false, NULL, 30),
    ('Read platform users', 'platform.users.read', 'platform', ARRAY['all']::text[], false, false, NULL, 40),
    ('Write platform users', 'platform.users.write', 'platform', ARRAY['all']::text[], false, false, NULL, 50),
    ('Export platform users', 'platform.users.export', 'platform', ARRAY['all']::text[], false, false, NULL, 60),
    ('Import platform users', 'platform.users.import', 'platform', ARRAY['all']::text[], false, false, NULL, 70),
    ('Bulk disable platform users', 'platform.users.bulk.disable', 'platform', ARRAY['all']::text[], false, false, NULL, 80),
    ('Bulk enable platform users', 'platform.users.bulk.enable', 'platform', ARRAY['all']::text[], false, false, NULL, 90),
    ('Impersonate platform users', 'platform.users.impersonate', 'platform', ARRAY['all']::text[], true, true, 'Only super_admin may hold this permission.', 100),
    ('Read platform roles', 'platform.roles.read', 'platform', ARRAY['all']::text[], false, false, NULL, 110),
    ('Write platform roles', 'platform.roles.write', 'platform', ARRAY['all']::text[], false, false, NULL, 120),
    ('Export platform roles', 'platform.roles.export', 'platform', ARRAY['all']::text[], false, false, NULL, 130),
    ('Import platform roles', 'platform.roles.import', 'platform', ARRAY['all']::text[], false, false, NULL, 140),
    ('Bulk delete platform roles', 'platform.roles.bulk.delete', 'platform', ARRAY['all']::text[], false, false, NULL, 150),
    ('Read bulk jobs', 'platform.bulk.read', 'platform', ARRAY['all']::text[], false, false, NULL, 160),
    ('Read platform notifications', 'platform.notifications.read', 'platform', ARRAY['all']::text[], false, false, NULL, 170),
    ('Read all platform notifications', 'platform.notifications.read_all', 'platform', ARRAY['all']::text[], false, false, NULL, 180),
    ('Export platform notifications', 'platform.notifications.export', 'platform', ARRAY['all']::text[], false, false, NULL, 190),
    ('Read platform settings', 'platform.settings.read', 'platform', ARRAY['all']::text[], false, false, NULL, 200),
    ('Write platform settings', 'platform.settings.write', 'platform', ARRAY['all']::text[], false, false, NULL, 210),
    ('Read activity log', 'platform.activity.read', 'platform', ARRAY['all']::text[], false, false, NULL, 220),
    ('Read import jobs', 'platform.imports.read', 'platform', ARRAY['all']::text[], false, false, NULL, 230),
    ('Read export jobs', 'platform.exports.read', 'platform', ARRAY['all']::text[], false, false, NULL, 240),
    ('Read platform storage', 'platform.storage.read', 'platform', ARRAY['all']::text[], false, false, NULL, 250),
    ('Write platform storage', 'platform.storage.write', 'platform', ARRAY['all']::text[], false, false, NULL, 260),
    ('Read platform logs', 'platform.logs.read', 'platform', ARRAY['all']::text[], false, false, NULL, 270),
    ('Write platform logs', 'platform.logs.write', 'platform', ARRAY['all']::text[], false, false, NULL, 280),
    ('Read access settings', 'platform.access.read', 'platform', ARRAY['all']::text[], false, false, NULL, 290),
    ('Write access settings', 'platform.access.write', 'platform', ARRAY['all']::text[], false, false, NULL, 300),
    ('Read GitHub integration settings', 'platform.integrations.github.read', 'platform', ARRAY['all']::text[], false, false, NULL, 310),
    ('Write GitHub integration settings', 'platform.integrations.github.write', 'platform', ARRAY['all']::text[], false, false, NULL, 320),
    ('Read Google integration settings', 'platform.integrations.google.read', 'platform', ARRAY['all']::text[], false, false, NULL, 330),
    ('Write Google integration settings', 'platform.integrations.google.write', 'platform', ARRAY['all']::text[], false, false, NULL, 340),
    ('Read Facebook integration settings', 'platform.integrations.facebook.read', 'platform', ARRAY['all']::text[], false, false, NULL, 350),
    ('Write Facebook integration settings', 'platform.integrations.facebook.write', 'platform', ARRAY['all']::text[], false, false, NULL, 360),
    ('Read Apple integration settings', 'platform.integrations.apple.read', 'platform', ARRAY['all']::text[], false, false, NULL, 370),
    ('Write Apple integration settings', 'platform.integrations.apple.write', 'platform', ARRAY['all']::text[], false, false, NULL, 380),
    ('Read auth registration settings', 'platform.auth.settings.read', 'platform', ARRAY['all']::text[], false, false, NULL, 390),
    ('Write auth registration settings', 'platform.auth.settings.write', 'platform', ARRAY['all']::text[], false, false, NULL, 400),
    ('Read platform organizations', 'platform.organizations.read', 'platform', ARRAY['all']::text[], false, false, NULL, 410),
    ('Write platform organizations', 'platform.organizations.write', 'platform', ARRAY['all']::text[], false, false, NULL, 420),
    ('Read organization settings', 'tenant.settings.read', 'tenant', ARRAY['managed']::text[], false, false, NULL, 430),
    ('Write organization settings', 'tenant.settings.write', 'tenant', ARRAY['managed']::text[], false, false, NULL, 440),
    ('Read organization import jobs', 'tenant.imports.read', 'tenant', ARRAY['managed']::text[], false, false, NULL, 450),
    ('Read organization export jobs', 'tenant.exports.read', 'tenant', ARRAY['managed']::text[], false, false, NULL, 460),
    ('Read organizations', 'organizations.read', 'organizations', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 470),
    ('Write organizations', 'organizations.write', 'organizations', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 480),
    ('Change organization supplier', 'organizations.supplier.write', 'organizations', ARRAY['brand', 'all']::text[], true, false, 'Move a dealer under another distributor (K25). Requires step-up.', 490),
    ('Read organization members', 'members.read', 'members', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 500),
    ('Write organization members', 'members.write', 'members', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 510),
    ('Read services', 'services.read', 'services', ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false, NULL, 520),
    ('Write services', 'services.write', 'services', ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 530),
    ('Read customers', 'customers.read', 'customers', ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all', 'customer']::text[], false, false, NULL, 540),
    ('Write customers', 'customers.write', 'customers', ARRAY['own', 'assigned', 'managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 550),
    ('Read purchase prices', 'pricing.purchase.read', 'pricing', ARRAY['managed', 'subtree', 'brand', 'all']::text[], true, false, NULL, 560),
    ('Read sale prices', 'pricing.sale.read', 'pricing', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 570),
    ('Write sale prices', 'pricing.sale.write', 'pricing', ARRAY['managed', 'subtree', 'brand', 'all']::text[], true, false, 'Requires step-up.', 580),
    ('Read recommended prices', 'pricing.recommended.read', 'pricing', ARRAY['managed', 'subtree', 'brand', 'all']::text[], true, false, NULL, 590),
    ('Publish recommended prices', 'pricing.recommended.write', 'pricing', ARRAY['brand', 'all']::text[], true, false, 'Requires step-up.', 600),
    ('Read accounting', 'accounting.read', 'accounting', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 610),
    ('Write accounting', 'accounting.write', 'accounting', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 620),
    ('Read warehouse', 'warehouse.read', 'warehouse', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 630),
    ('Write warehouse', 'warehouse.write', 'warehouse', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 640),
    ('Read campaigns', 'campaigns.read', 'campaigns', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 650),
    ('Write campaigns', 'campaigns.write', 'campaigns', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 660),
    ('Read leads', 'leads.read', 'leads', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 670),
    ('Write leads', 'leads.write', 'leads', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 680),
    ('Read social', 'social.read', 'social', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 690),
    ('Write social', 'social.write', 'social', ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false, NULL, 700),
    ('Anonymize personal data', 'privacy.anonymize', 'privacy', ARRAY['managed', 'brand', 'all']::text[], true, false, 'KVKK/GDPR anonymization (K19). Requires step-up.', 710)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    module = EXCLUDED.module,
    scopes = EXCLUDED.scopes,
    is_sensitive = EXCLUDED.is_sensitive,
    super_admin_only = EXCLUDED.super_admin_only,
    description = EXCLUDED.description,
    sort_order = EXCLUDED.sort_order;

INSERT INTO roles (name, slug, description, is_system, org_type) VALUES
    ('Platform Admin', 'super_admin', 'Platform-level operator with all permissions', true, 'platform'),
    ('Center staff', 'center_staff', 'Center staff: services, customers and the organization tree of the brand', true, 'center'),
    ('Center warehouse', 'center_warehouse', 'Center warehouse operator', true, 'center'),
    ('Center accounting', 'center_accounting', 'Center accounting and price management', true, 'center'),
    ('Center social', 'center_social', 'Leads, campaigns and social media for the brand', true, 'center'),
    ('Distributor owner', 'distributor_owner', 'Distributor owner: own prices, accounting and warehouse; services and dealers of the subtree', true, 'distributor'),
    ('Distributor staff', 'distributor_staff', 'Distributor staff: services of the subtree', true, 'distributor'),
    ('Distributor warehouse staff', 'distributor_warehouse_staff', 'Distributor warehouse operator', true, 'distributor'),
    ('Distributor accounting', 'distributor_accounting', 'Distributor accounting and prices', true, 'distributor'),
    ('Dealer owner', 'dealer_owner', 'Dealer owner: sets final prices (K8), sees accounting and dealer stock', true, 'dealer'),
    ('Dealer staff', 'dealer_staff', 'Dealer staff: reads the dealer''s services, writes own records; no prices or accounting', true, 'dealer'),
    ('Dealer accounting', 'dealer_accounting', 'Dealer accounting', true, 'dealer'),
    ('Customer', 'customer', 'Portal customer: own services and warranties', true, 'customer'),
    ('Fleet', 'fleet', 'Fleet account (read-only)', true, 'fleet')
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    is_system = true,
    org_type = EXCLUDED.org_type;


-- System role packages are rewritten from the catalog.
DELETE FROM role_permissions rp
USING roles r
WHERE rp.role_id = r.id
  AND r.slug IN (
    'super_admin',
    'center_staff', 'center_warehouse', 'center_accounting', 'center_social',
    'distributor_owner', 'distributor_staff', 'distributor_warehouse_staff', 'distributor_accounting',
    'dealer_owner', 'dealer_staff', 'dealer_accounting',
    'customer', 'fleet'
  );

-- Custom (non-system) role grants created before scopes keep their access at
-- the broadest scope their permission allows.
UPDATE role_permissions rp
SET scope = (
    SELECT s FROM unnest(ARRAY['all', 'brand', 'subtree', 'managed', 'assigned', 'own', 'customer']) AS s
    WHERE s = ANY (p.scopes)
    LIMIT 1
)
FROM permissions p
WHERE p.id = rp.permission_id
  AND NOT (rp.scope = ANY (p.scopes));

-- A grant's scope must be one the permission allows.
CREATE FUNCTION check_role_permission_scope() RETURNS trigger AS $$
DECLARE
    perm permissions%ROWTYPE;
    role_slug TEXT;
BEGIN
    SELECT * INTO perm FROM permissions WHERE id = NEW.permission_id;
    IF NOT (NEW.scope = ANY (perm.scopes)) THEN
        RAISE EXCEPTION 'scope % is not allowed for permission %', NEW.scope, perm.slug
            USING ERRCODE = 'check_violation';
    END IF;
    IF perm.super_admin_only THEN
        SELECT slug INTO role_slug FROM roles WHERE id = NEW.role_id;
        IF role_slug IS DISTINCT FROM 'super_admin' THEN
            RAISE EXCEPTION 'permission % may only be granted to super_admin', perm.slug
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_role_permissions_check_scope
    BEFORE INSERT OR UPDATE ON role_permissions
    FOR EACH ROW
    EXECUTE FUNCTION check_role_permission_scope();

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'accounting.read', 'all'),
    ('super_admin', 'accounting.write', 'all'),
    ('super_admin', 'auth.session', 'own'),
    ('super_admin', 'campaigns.read', 'all'),
    ('super_admin', 'campaigns.write', 'all'),
    ('super_admin', 'customers.read', 'all'),
    ('super_admin', 'customers.write', 'all'),
    ('super_admin', 'leads.read', 'all'),
    ('super_admin', 'leads.write', 'all'),
    ('super_admin', 'members.read', 'all'),
    ('super_admin', 'members.write', 'all'),
    ('super_admin', 'notifications.manage', 'own'),
    ('super_admin', 'notifications.read', 'own'),
    ('super_admin', 'organizations.read', 'all'),
    ('super_admin', 'organizations.supplier.write', 'all'),
    ('super_admin', 'organizations.write', 'all'),
    ('super_admin', 'platform.access.read', 'all'),
    ('super_admin', 'platform.access.write', 'all'),
    ('super_admin', 'platform.activity.read', 'all'),
    ('super_admin', 'platform.auth.settings.read', 'all'),
    ('super_admin', 'platform.auth.settings.write', 'all'),
    ('super_admin', 'platform.bulk.read', 'all'),
    ('super_admin', 'platform.exports.read', 'all'),
    ('super_admin', 'platform.imports.read', 'all'),
    ('super_admin', 'platform.integrations.apple.read', 'all'),
    ('super_admin', 'platform.integrations.apple.write', 'all'),
    ('super_admin', 'platform.integrations.facebook.read', 'all'),
    ('super_admin', 'platform.integrations.facebook.write', 'all'),
    ('super_admin', 'platform.integrations.github.read', 'all'),
    ('super_admin', 'platform.integrations.github.write', 'all'),
    ('super_admin', 'platform.integrations.google.read', 'all'),
    ('super_admin', 'platform.integrations.google.write', 'all'),
    ('super_admin', 'platform.logs.read', 'all'),
    ('super_admin', 'platform.logs.write', 'all'),
    ('super_admin', 'platform.notifications.export', 'all'),
    ('super_admin', 'platform.notifications.read', 'all'),
    ('super_admin', 'platform.notifications.read_all', 'all'),
    ('super_admin', 'platform.organizations.read', 'all'),
    ('super_admin', 'platform.organizations.write', 'all'),
    ('super_admin', 'platform.roles.bulk.delete', 'all'),
    ('super_admin', 'platform.roles.export', 'all'),
    ('super_admin', 'platform.roles.import', 'all'),
    ('super_admin', 'platform.roles.read', 'all'),
    ('super_admin', 'platform.roles.write', 'all'),
    ('super_admin', 'platform.settings.read', 'all'),
    ('super_admin', 'platform.settings.write', 'all'),
    ('super_admin', 'platform.storage.read', 'all'),
    ('super_admin', 'platform.storage.write', 'all'),
    ('super_admin', 'platform.users.bulk.disable', 'all'),
    ('super_admin', 'platform.users.bulk.enable', 'all'),
    ('super_admin', 'platform.users.export', 'all'),
    ('super_admin', 'platform.users.impersonate', 'all'),
    ('super_admin', 'platform.users.import', 'all'),
    ('super_admin', 'platform.users.read', 'all'),
    ('super_admin', 'platform.users.write', 'all'),
    ('super_admin', 'pricing.purchase.read', 'all'),
    ('super_admin', 'pricing.recommended.read', 'all'),
    ('super_admin', 'pricing.recommended.write', 'all'),
    ('super_admin', 'pricing.sale.read', 'all'),
    ('super_admin', 'pricing.sale.write', 'all'),
    ('super_admin', 'privacy.anonymize', 'all'),
    ('super_admin', 'services.read', 'all'),
    ('super_admin', 'services.write', 'all'),
    ('super_admin', 'social.read', 'all'),
    ('super_admin', 'social.write', 'all'),
    ('super_admin', 'tenant.exports.read', 'managed'),
    ('super_admin', 'tenant.imports.read', 'managed'),
    ('super_admin', 'tenant.settings.read', 'managed'),
    ('super_admin', 'tenant.settings.write', 'managed'),
    ('super_admin', 'warehouse.read', 'all'),
    ('super_admin', 'warehouse.write', 'all'),
    ('center_staff', 'auth.session', 'own'),
    ('center_staff', 'customers.read', 'brand'),
    ('center_staff', 'customers.write', 'brand'),
    ('center_staff', 'members.read', 'brand'),
    ('center_staff', 'notifications.manage', 'own'),
    ('center_staff', 'notifications.read', 'own'),
    ('center_staff', 'organizations.read', 'brand'),
    ('center_staff', 'pricing.recommended.read', 'brand'),
    ('center_staff', 'services.read', 'brand'),
    ('center_staff', 'services.write', 'brand'),
    ('center_warehouse', 'auth.session', 'own'),
    ('center_warehouse', 'notifications.manage', 'own'),
    ('center_warehouse', 'notifications.read', 'own'),
    ('center_warehouse', 'organizations.read', 'brand'),
    ('center_warehouse', 'warehouse.read', 'brand'),
    ('center_warehouse', 'warehouse.write', 'brand'),
    ('center_accounting', 'accounting.read', 'brand'),
    ('center_accounting', 'accounting.write', 'brand'),
    ('center_accounting', 'auth.session', 'own'),
    ('center_accounting', 'notifications.manage', 'own'),
    ('center_accounting', 'notifications.read', 'own'),
    ('center_accounting', 'organizations.read', 'brand'),
    ('center_accounting', 'pricing.purchase.read', 'brand'),
    ('center_accounting', 'pricing.recommended.read', 'brand'),
    ('center_accounting', 'pricing.recommended.write', 'brand'),
    ('center_accounting', 'pricing.sale.read', 'brand'),
    ('center_accounting', 'pricing.sale.write', 'brand'),
    ('center_social', 'auth.session', 'own'),
    ('center_social', 'campaigns.read', 'brand'),
    ('center_social', 'campaigns.write', 'brand'),
    ('center_social', 'customers.read', 'brand'),
    ('center_social', 'leads.read', 'brand'),
    ('center_social', 'leads.write', 'brand'),
    ('center_social', 'notifications.manage', 'own'),
    ('center_social', 'notifications.read', 'own'),
    ('center_social', 'organizations.read', 'brand'),
    ('center_social', 'social.read', 'brand'),
    ('center_social', 'social.write', 'brand'),
    ('distributor_owner', 'accounting.read', 'managed'),
    ('distributor_owner', 'accounting.write', 'managed'),
    ('distributor_owner', 'auth.session', 'own'),
    ('distributor_owner', 'customers.read', 'subtree'),
    ('distributor_owner', 'customers.write', 'subtree'),
    ('distributor_owner', 'members.read', 'subtree'),
    ('distributor_owner', 'members.write', 'subtree'),
    ('distributor_owner', 'notifications.manage', 'own'),
    ('distributor_owner', 'notifications.read', 'own'),
    ('distributor_owner', 'organizations.read', 'subtree'),
    ('distributor_owner', 'organizations.write', 'subtree'),
    ('distributor_owner', 'pricing.purchase.read', 'managed'),
    ('distributor_owner', 'pricing.recommended.read', 'managed'),
    ('distributor_owner', 'pricing.sale.read', 'managed'),
    ('distributor_owner', 'pricing.sale.write', 'managed'),
    ('distributor_owner', 'services.read', 'subtree'),
    ('distributor_owner', 'services.write', 'subtree'),
    ('distributor_owner', 'tenant.exports.read', 'managed'),
    ('distributor_owner', 'tenant.imports.read', 'managed'),
    ('distributor_owner', 'tenant.settings.read', 'managed'),
    ('distributor_owner', 'tenant.settings.write', 'managed'),
    ('distributor_owner', 'warehouse.read', 'managed'),
    ('distributor_owner', 'warehouse.write', 'managed'),
    ('distributor_staff', 'auth.session', 'own'),
    ('distributor_staff', 'customers.read', 'subtree'),
    ('distributor_staff', 'customers.write', 'subtree'),
    ('distributor_staff', 'notifications.manage', 'own'),
    ('distributor_staff', 'notifications.read', 'own'),
    ('distributor_staff', 'organizations.read', 'subtree'),
    ('distributor_staff', 'services.read', 'subtree'),
    ('distributor_staff', 'services.write', 'subtree'),
    ('distributor_warehouse_staff', 'auth.session', 'own'),
    ('distributor_warehouse_staff', 'notifications.manage', 'own'),
    ('distributor_warehouse_staff', 'notifications.read', 'own'),
    ('distributor_warehouse_staff', 'warehouse.read', 'managed'),
    ('distributor_warehouse_staff', 'warehouse.write', 'managed'),
    ('distributor_accounting', 'accounting.read', 'managed'),
    ('distributor_accounting', 'accounting.write', 'managed'),
    ('distributor_accounting', 'auth.session', 'own'),
    ('distributor_accounting', 'notifications.manage', 'own'),
    ('distributor_accounting', 'notifications.read', 'own'),
    ('distributor_accounting', 'pricing.purchase.read', 'managed'),
    ('distributor_accounting', 'pricing.recommended.read', 'managed'),
    ('distributor_accounting', 'pricing.sale.read', 'managed'),
    ('distributor_accounting', 'pricing.sale.write', 'managed'),
    ('dealer_owner', 'accounting.read', 'managed'),
    ('dealer_owner', 'accounting.write', 'managed'),
    ('dealer_owner', 'auth.session', 'own'),
    ('dealer_owner', 'customers.read', 'managed'),
    ('dealer_owner', 'customers.write', 'managed'),
    ('dealer_owner', 'members.read', 'managed'),
    ('dealer_owner', 'members.write', 'managed'),
    ('dealer_owner', 'notifications.manage', 'own'),
    ('dealer_owner', 'notifications.read', 'own'),
    ('dealer_owner', 'organizations.read', 'managed'),
    ('dealer_owner', 'pricing.purchase.read', 'managed'),
    ('dealer_owner', 'pricing.recommended.read', 'managed'),
    ('dealer_owner', 'pricing.sale.read', 'managed'),
    ('dealer_owner', 'pricing.sale.write', 'managed'),
    ('dealer_owner', 'services.read', 'managed'),
    ('dealer_owner', 'services.write', 'managed'),
    ('dealer_owner', 'tenant.exports.read', 'managed'),
    ('dealer_owner', 'tenant.imports.read', 'managed'),
    ('dealer_owner', 'tenant.settings.read', 'managed'),
    ('dealer_owner', 'tenant.settings.write', 'managed'),
    ('dealer_owner', 'warehouse.read', 'managed'),
    ('dealer_staff', 'auth.session', 'own'),
    ('dealer_staff', 'customers.read', 'managed'),
    ('dealer_staff', 'customers.write', 'own'),
    ('dealer_staff', 'notifications.manage', 'own'),
    ('dealer_staff', 'notifications.read', 'own'),
    ('dealer_staff', 'services.read', 'managed'),
    ('dealer_staff', 'services.write', 'own'),
    ('dealer_accounting', 'accounting.read', 'managed'),
    ('dealer_accounting', 'accounting.write', 'managed'),
    ('dealer_accounting', 'auth.session', 'own'),
    ('dealer_accounting', 'notifications.manage', 'own'),
    ('dealer_accounting', 'notifications.read', 'own'),
    ('dealer_accounting', 'pricing.purchase.read', 'managed'),
    ('dealer_accounting', 'pricing.recommended.read', 'managed'),
    ('dealer_accounting', 'pricing.sale.read', 'managed'),
    ('customer', 'auth.session', 'own'),
    ('customer', 'customers.read', 'customer'),
    ('customer', 'notifications.manage', 'own'),
    ('customer', 'notifications.read', 'own'),
    ('customer', 'services.read', 'customer'),
    ('fleet', 'auth.session', 'own'),
    ('fleet', 'customers.read', 'customer'),
    ('fleet', 'notifications.manage', 'own'),
    ('fleet', 'notifications.read', 'own'),
    ('fleet', 'services.read', 'customer')
) AS g (role_slug, permission_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.permission_slug;

-- Organization roles are granted per membership. organization_members.role
-- (owner|staff) stays as the membership kind.
CREATE TABLE organization_member_roles (
    member_id  BIGINT      NOT NULL REFERENCES organization_members (id) ON DELETE CASCADE,
    role_id    BIGINT      NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (member_id, role_id)
);

CREATE INDEX idx_organization_member_roles_role ON organization_member_roles (role_id);

-- Backfill: owner/staff memberships move to the role of their organization
-- type (center -> center_staff; distributor/dealer -> <type>_owner|_staff).
INSERT INTO organization_member_roles (member_id, role_id)
SELECT om.id, r.id
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id
JOIN roles r ON r.slug = CASE
    WHEN o.type = 'center' THEN 'center_staff'
    WHEN o.type = 'distributor' AND om.role = 'owner' THEN 'distributor_owner'
    WHEN o.type = 'distributor' THEN 'distributor_staff'
    WHEN om.role = 'owner' THEN 'dealer_owner'
    ELSE 'dealer_staff'
END
ON CONFLICT DO NOTHING;

-- The global organization_user / organization_owner roles granted tenant
-- permissions in every organization of the user; membership roles replace
-- them (their user_roles rows cascade).
DELETE FROM roles WHERE slug IN ('organization_user', 'organization_owner');
