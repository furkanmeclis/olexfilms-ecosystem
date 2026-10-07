-- TEC-472 (F5-02a): fleet schema. A fleet is an organization of type
-- 'fleet' outside the center -> distributor -> dealer tree (parent_id NULL,
-- global like a customer within its brand). Endpoints land in TEC-473+
-- (F5-02b..e).
--
--   * organizations.type 'fleet': no parent, never a parent. The tree
--     queries (subtree, scoped lists, features, territories) leave it out.
--   * fleet_profiles: one row per fleet organization. tax_number is a VKN
--     (10 digits) or TCKN (11 digits); the checksum is validated by the
--     usecase (422), the database only checks the shape. primary_user_id is
--     the user that owns the fleet vehicles (vehicles.user_id), so service
--     records keep working with customer_user_id unchanged.
--   * fleet_dealer_links: the dealers (or a serving distributor) a fleet
--     works with. One open (pending or active) link per (fleet, dealer);
--     ended links stay as history. cari_account_id is the fleet's cari in
--     the dealer's ledger (counterparty_type 'organization').
--   * vehicles.fleet_org_id: the fleet a vehicle belongs to.
--   * fleet_reports: periodic (monthly / quarterly) fleet report PDFs.
--   * permissions fleets.read, fleets.manage, fleets.plan and
--     fleet.portal.read.

-- 1. Organization type --------------------------------------------------------
ALTER TABLE organizations
    DROP CONSTRAINT chk_organizations_type,
    DROP CONSTRAINT chk_organizations_parent;

ALTER TABLE organizations
    ADD CONSTRAINT chk_organizations_type
        CHECK (type IN ('center', 'distributor', 'dealer', 'fleet')),
    ADD CONSTRAINT chk_organizations_parent
        CHECK ((type IN ('center', 'fleet') AND parent_id IS NULL)
            OR (type IN ('distributor', 'dealer') AND parent_id IS NOT NULL));

-- A fleet never becomes the parent of another organization, and an
-- organization never changes into or out of the fleet type (the tree would
-- gain or lose a node silently).
CREATE FUNCTION organizations_check_fleet() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    p_type VARCHAR(16);
BEGIN
    IF TG_OP = 'UPDATE' AND (OLD.type = 'fleet') IS DISTINCT FROM (NEW.type = 'fleet') THEN
        RAISE EXCEPTION 'organizations: type of organization % cannot change between % and %',
            NEW.id, OLD.type, NEW.type
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.parent_id IS NOT NULL THEN
        SELECT type INTO p_type FROM organizations WHERE id = NEW.parent_id;
        IF p_type = 'fleet' THEN
            RAISE EXCEPTION 'organizations: fleet % cannot be a parent', NEW.parent_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_organizations_check_fleet
    BEFORE INSERT OR UPDATE OF type, parent_id ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION organizations_check_fleet();

-- Shared check: the organization in column TG_ARGV[0] is of type TG_ARGV[1]
-- (a list, comma separated) and belongs to NEW.brand_id. NULL passes.
CREATE FUNCTION fleet_check_org() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_id    BIGINT;
    o_type    VARCHAR(16);
    o_brand   BIGINT;
BEGIN
    org_id := (to_jsonb(NEW) ->> TG_ARGV[0])::bigint;
    IF org_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = org_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF NOT (o_type = ANY (string_to_array(TG_ARGV[1], ','))) THEN
        RAISE EXCEPTION '%: % % is a % organization, expected %',
            TG_TABLE_NAME, TG_ARGV[0], org_id, o_type, TG_ARGV[1]
            USING ERRCODE = 'check_violation';
    END IF;
    IF o_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION '%: brand % does not match organization % (brand %)',
            TG_TABLE_NAME, NEW.brand_id, org_id, o_brand
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

-- 2. Fleet profiles -----------------------------------------------------------
CREATE TABLE fleet_profiles (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    tax_number          VARCHAR(11)   NOT NULL,
    tax_office          VARCHAR(120)  NULL,
    legal_name          VARCHAR(255)  NOT NULL,
    contact_name        VARCHAR(160)  NULL,
    contact_phone       VARCHAR(20)   NULL,
    billing_email       VARCHAR(255)  NULL,
    report_frequency    VARCHAR(16)   NOT NULL DEFAULT 'monthly',
    report_locale       VARCHAR(8)    NOT NULL DEFAULT 'tr',
    primary_user_id     BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fleet_profiles_uuid UNIQUE (uuid),
    CONSTRAINT uq_fleet_profiles_org UNIQUE (organization_id),
    -- A tax number opens one fleet per brand; a second dealer adding the
    -- same VKN finds it and requests a link (F5 Q9).
    CONSTRAINT uq_fleet_profiles_brand_tax_number UNIQUE (brand_id, tax_number),
    CONSTRAINT chk_fleet_profiles_tax_number CHECK (tax_number ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_fleet_profiles_tax_office CHECK (tax_office IS NULL OR btrim(tax_office) <> ''),
    CONSTRAINT chk_fleet_profiles_legal_name CHECK (btrim(legal_name) <> ''),
    CONSTRAINT chk_fleet_profiles_contact_name CHECK (contact_name IS NULL OR btrim(contact_name) <> ''),
    CONSTRAINT chk_fleet_profiles_contact_phone CHECK (
        contact_phone IS NULL OR contact_phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_fleet_profiles_billing_email CHECK (
        billing_email IS NULL OR billing_email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'),
    CONSTRAINT chk_fleet_profiles_report_frequency CHECK (
        report_frequency IN ('monthly', 'quarterly', 'off')),
    CONSTRAINT chk_fleet_profiles_report_locale CHECK (
        report_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar'))
);

CREATE INDEX idx_fleet_profiles_primary_user ON fleet_profiles (primary_user_id)
    WHERE primary_user_id IS NOT NULL;
CREATE INDEX idx_fleet_profiles_report_due ON fleet_profiles (report_frequency)
    WHERE report_frequency <> 'off';

CREATE TRIGGER trg_fleet_profiles_set_updated_at
    BEFORE UPDATE ON fleet_profiles
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_fleet_profiles_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON fleet_profiles
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('organization_id', 'fleet');

-- 3. Fleet <-> dealer links ---------------------------------------------------
CREATE TABLE fleet_dealer_links (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    fleet_org_id        BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    dealer_org_id       BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    status              VARCHAR(16)   NOT NULL DEFAULT 'pending',
    created_by_org_id   BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    cari_account_id     BIGINT        NULL,
    started_at          TIMESTAMPTZ   NULL,
    ended_at            TIMESTAMPTZ   NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fleet_dealer_links_uuid UNIQUE (uuid),
    -- The cari lives in the dealer's ledger.
    CONSTRAINT fk_fleet_dealer_links_cari FOREIGN KEY (cari_account_id, dealer_org_id)
        REFERENCES cari_accounts (id, organization_id) ON DELETE RESTRICT,
    CONSTRAINT chk_fleet_dealer_links_status CHECK (status IN ('pending', 'active', 'ended')),
    CONSTRAINT chk_fleet_dealer_links_started CHECK (status <> 'active' OR started_at IS NOT NULL),
    CONSTRAINT chk_fleet_dealer_links_ended CHECK ((status = 'ended') = (ended_at IS NOT NULL)),
    CONSTRAINT chk_fleet_dealer_links_period CHECK (
        started_at IS NULL OR ended_at IS NULL OR ended_at >= started_at),
    CONSTRAINT chk_fleet_dealer_links_creator CHECK (
        created_by_org_id = fleet_org_id OR created_by_org_id = dealer_org_id)
);

-- One open link per (fleet, dealer): a second pending or active link fails.
CREATE UNIQUE INDEX uq_fleet_dealer_links_open ON fleet_dealer_links (fleet_org_id, dealer_org_id)
    WHERE status <> 'ended';
CREATE INDEX idx_fleet_dealer_links_dealer ON fleet_dealer_links (dealer_org_id, status);
CREATE INDEX idx_fleet_dealer_links_fleet ON fleet_dealer_links (fleet_org_id, status);
CREATE INDEX idx_fleet_dealer_links_cari ON fleet_dealer_links (cari_account_id)
    WHERE cari_account_id IS NOT NULL;

CREATE TRIGGER trg_fleet_dealer_links_set_updated_at
    BEFORE UPDATE ON fleet_dealer_links
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_fleet_dealer_links_check_fleet
    BEFORE INSERT OR UPDATE OF fleet_org_id, brand_id ON fleet_dealer_links
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('fleet_org_id', 'fleet');

CREATE TRIGGER trg_fleet_dealer_links_check_dealer
    BEFORE INSERT OR UPDATE OF dealer_org_id, brand_id ON fleet_dealer_links
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('dealer_org_id', 'dealer,distributor');

-- The linked cari has the fleet as its counterparty organization.
CREATE FUNCTION fleet_dealer_links_check_cari() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    c_type VARCHAR(16);
    c_org  BIGINT;
BEGIN
    IF NEW.cari_account_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT counterparty_type, counterparty_org_id INTO c_type, c_org
    FROM cari_accounts WHERE id = NEW.cari_account_id;
    IF NOT FOUND THEN
        RETURN NEW; -- the foreign key reports it
    END IF;
    IF c_type <> 'organization' OR c_org IS DISTINCT FROM NEW.fleet_org_id THEN
        RAISE EXCEPTION 'fleet_dealer_links: cari % is not the cari of fleet %',
            NEW.cari_account_id, NEW.fleet_org_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_fleet_dealer_links_check_cari
    BEFORE INSERT OR UPDATE OF cari_account_id, fleet_org_id ON fleet_dealer_links
    FOR EACH ROW
    EXECUTE FUNCTION fleet_dealer_links_check_cari();

-- 4. Fleet vehicles -----------------------------------------------------------
ALTER TABLE vehicles
    ADD COLUMN fleet_org_id BIGINT NULL REFERENCES organizations (id) ON DELETE RESTRICT;

CREATE INDEX idx_vehicles_fleet ON vehicles (fleet_org_id)
    WHERE fleet_org_id IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER trg_vehicles_check_fleet
    BEFORE INSERT OR UPDATE OF fleet_org_id, brand_id ON vehicles
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('fleet_org_id', 'fleet');

-- 5. Fleet reports ------------------------------------------------------------
CREATE TABLE fleet_reports (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    fleet_org_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    period_kind      VARCHAR(16)   NOT NULL,
    period_start     DATE          NOT NULL,
    period_end       DATE          NOT NULL,
    locale           VARCHAR(8)    NOT NULL DEFAULT 'tr',
    storage_key      TEXT          NULL,
    status           VARCHAR(16)   NOT NULL DEFAULT 'pending',
    error            TEXT          NULL,
    emailed_at       TIMESTAMPTZ   NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_fleet_reports_uuid UNIQUE (uuid),
    -- One report per fleet and period; the worker retries the same row.
    CONSTRAINT uq_fleet_reports_period UNIQUE (fleet_org_id, period_kind, period_start),
    CONSTRAINT chk_fleet_reports_period_kind CHECK (period_kind IN ('monthly', 'quarterly')),
    CONSTRAINT chk_fleet_reports_period CHECK (period_end >= period_start),
    CONSTRAINT chk_fleet_reports_status CHECK (status IN ('pending', 'ready', 'failed')),
    CONSTRAINT chk_fleet_reports_storage_key CHECK (
        (status <> 'ready' OR storage_key IS NOT NULL)
        AND (storage_key IS NULL OR btrim(storage_key) <> '')),
    CONSTRAINT chk_fleet_reports_emailed CHECK (emailed_at IS NULL OR status = 'ready'),
    CONSTRAINT chk_fleet_reports_locale CHECK (
        locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar'))
);

CREATE INDEX idx_fleet_reports_fleet ON fleet_reports (fleet_org_id, period_start DESC, id DESC);
CREATE INDEX idx_fleet_reports_pending ON fleet_reports (created_at) WHERE status = 'pending';

CREATE TRIGGER trg_fleet_reports_set_updated_at
    BEFORE UPDATE ON fleet_reports
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_fleet_reports_check_org
    BEFORE INSERT OR UPDATE OF fleet_org_id, brand_id ON fleet_reports
    FOR EACH ROW
    EXECUTE FUNCTION fleet_check_org('fleet_org_id', 'fleet');

-- 6. Permissions --------------------------------------------------------------
-- Appended in catalog order (sort_order MAX + 10 each).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read fleets', 'fleets.read', 'fleet',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Fleets linked to the organization, their vehicles, services and reports.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage fleets', 'fleets.manage', 'fleet',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Open fleets, invite fleet users, add vehicles and manage dealer links.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Plan fleet services', 'fleets.plan', 'fleet',
       ARRAY['managed', 'all']::text[], false, false,
       'Bulk service plans and bulk vehicle intake for a linked fleet.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read own fleet (portal)', 'fleet.portal.read', 'fleet',
       ARRAY['own']::text[], false, false,
       'Fleet portal: vehicles, services, warranties, accounts and reports of one''s own fleet.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'fleets.read', 'all'),
    ('super_admin', 'fleets.manage', 'all'),
    ('super_admin', 'fleets.plan', 'all'),
    ('super_admin', 'fleet.portal.read', 'own'),
    ('center_staff', 'fleets.read', 'brand'),
    ('center_staff', 'fleets.manage', 'brand'),
    ('distributor_owner', 'fleets.read', 'subtree'),
    ('distributor_owner', 'fleets.manage', 'subtree'),
    ('dealer_owner', 'fleets.read', 'managed'),
    ('dealer_owner', 'fleets.manage', 'managed'),
    ('dealer_owner', 'fleets.plan', 'managed'),
    ('dealer_staff', 'fleets.read', 'managed'),
    ('dealer_staff', 'fleets.plan', 'managed'),
    ('fleet', 'fleet.portal.read', 'own')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
