-- TEC-501 (F5-08a): e-Invoice (UBL-TR) schema, center permissions and
-- sqlc repository contract. This is infrastructure only: no integrator
-- submission is created in F5 section 9.

-- 1. Buyer invoice profile fields on organizations --------------------------
ALTER TABLE organizations
    ADD COLUMN invoice_vkn          VARCHAR(10)  NULL,
    ADD COLUMN invoice_tckn         VARCHAR(11)  NULL,
    ADD COLUMN invoice_tax_office   VARCHAR(120) NULL,
    ADD COLUMN invoice_legal_name   VARCHAR(255) NULL,
    ADD COLUMN einvoice_registered  BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN einvoice_alias       VARCHAR(255) NULL,
    ADD COLUMN invoice_email        VARCHAR(255) NULL,
    ADD CONSTRAINT chk_organizations_invoice_vkn CHECK (
        invoice_vkn IS NULL OR invoice_vkn ~ '^[0-9]{10}$'),
    ADD CONSTRAINT chk_organizations_invoice_tckn CHECK (
        invoice_tckn IS NULL OR invoice_tckn ~ '^[0-9]{11}$'),
    ADD CONSTRAINT chk_organizations_invoice_tax_id CHECK (
        invoice_vkn IS NULL OR invoice_tckn IS NULL),
    ADD CONSTRAINT chk_organizations_invoice_tax_office CHECK (
        invoice_tax_office IS NULL OR btrim(invoice_tax_office) <> ''),
    ADD CONSTRAINT chk_organizations_invoice_legal_name CHECK (
        invoice_legal_name IS NULL OR btrim(invoice_legal_name) <> ''),
    ADD CONSTRAINT chk_organizations_einvoice_alias CHECK (
        einvoice_alias IS NULL OR btrim(einvoice_alias) <> ''),
    ADD CONSTRAINT chk_organizations_invoice_email CHECK (
        invoice_email IS NULL OR invoice_email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$');

CREATE INDEX idx_organizations_invoice_tax
    ON organizations (brand_id, COALESCE(invoice_vkn, invoice_tckn))
    WHERE invoice_vkn IS NOT NULL OR invoice_tckn IS NOT NULL;
CREATE INDEX idx_organizations_einvoice_registered
    ON organizations (brand_id, einvoice_registered)
    WHERE einvoice_registered;

ALTER TABLE service_subscription_periods
    ADD COLUMN uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD CONSTRAINT uq_service_subscription_periods_uuid UNIQUE (uuid);

-- 2. Center seller settings --------------------------------------------------
CREATE TABLE einvoice_settings (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    vkn                 VARCHAR(10)  NOT NULL,
    tax_office          VARCHAR(120) NOT NULL,
    legal_name          VARCHAR(255) NOT NULL,
    address             TEXT         NOT NULL,
    city                VARCHAR(100) NOT NULL,
    district            VARCHAR(100) NOT NULL,
    country             CHAR(2)      NOT NULL DEFAULT 'TR',
    iban                VARCHAR(34)  NULL,
    email               VARCHAR(255) NULL,
    phone               VARCHAR(20)  NULL,
    website             VARCHAR(255) NULL,
    trade_registry_no   VARCHAR(64)  NULL,
    mersis_no           VARCHAR(32)  NULL,
    default_note        TEXT         NULL,
    earchive_series     VARCHAR(3)   NOT NULL DEFAULT 'EAR',
    efatura_series      VARCHAR(3)   NOT NULL DEFAULT 'EFN',
    xslt_storage_key    TEXT         NULL,
    xslt_sha1           CHAR(40)     NULL,
    pdf_enabled         BOOLEAN      NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_einvoice_settings_uuid UNIQUE (uuid),
    CONSTRAINT uq_einvoice_settings_org UNIQUE (organization_id),
    CONSTRAINT chk_einvoice_settings_vkn CHECK (vkn ~ '^[0-9]{10}$'),
    CONSTRAINT chk_einvoice_settings_tax_office CHECK (btrim(tax_office) <> ''),
    CONSTRAINT chk_einvoice_settings_legal_name CHECK (btrim(legal_name) <> ''),
    CONSTRAINT chk_einvoice_settings_address CHECK (btrim(address) <> ''),
    CONSTRAINT chk_einvoice_settings_city CHECK (btrim(city) <> ''),
    CONSTRAINT chk_einvoice_settings_district CHECK (btrim(district) <> ''),
    CONSTRAINT chk_einvoice_settings_country CHECK (country ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_einvoice_settings_iban CHECK (
        iban IS NULL OR iban ~ '^[A-Z]{2}[0-9A-Z]{13,32}$'),
    CONSTRAINT chk_einvoice_settings_email CHECK (
        email IS NULL OR email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'),
    CONSTRAINT chk_einvoice_settings_phone CHECK (
        phone IS NULL OR phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_einvoice_settings_website CHECK (
        website IS NULL OR website ~ '^https?://'),
    CONSTRAINT chk_einvoice_settings_trade_registry CHECK (
        trade_registry_no IS NULL OR btrim(trade_registry_no) <> ''),
    CONSTRAINT chk_einvoice_settings_mersis CHECK (
        mersis_no IS NULL OR mersis_no ~ '^[0-9]{16}$'),
    CONSTRAINT chk_einvoice_settings_note CHECK (
        default_note IS NULL OR char_length(default_note) <= 4000),
    CONSTRAINT chk_einvoice_settings_earchive_series CHECK (earchive_series ~ '^[A-Z0-9]{3}$'),
    CONSTRAINT chk_einvoice_settings_efatura_series CHECK (efatura_series ~ '^[A-Z0-9]{3}$'),
    CONSTRAINT chk_einvoice_settings_xslt_sha1 CHECK (
        xslt_sha1 IS NULL OR xslt_sha1 ~ '^[0-9a-f]{40}$'),
    CONSTRAINT chk_einvoice_settings_xslt_pair CHECK (
        (xslt_storage_key IS NULL) = (xslt_sha1 IS NULL))
);

CREATE TRIGGER trg_einvoice_settings_set_updated_at
    BEFORE UPDATE ON einvoice_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION einvoice_settings_check_center() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    o_type  TEXT;
    o_brand BIGINT;
BEGIN
    SELECT type, brand_id INTO o_type, o_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND (o_type IS DISTINCT FROM 'center' OR o_brand IS DISTINCT FROM NEW.brand_id) THEN
        RAISE EXCEPTION 'einvoice_settings: organization % must be the center of brand %',
            NEW.organization_id, NEW.brand_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_einvoice_settings_check_center
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON einvoice_settings
    FOR EACH ROW
    EXECUTE FUNCTION einvoice_settings_check_center();

-- 3. Atomic series counters --------------------------------------------------
CREATE TABLE einvoice_counters (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id        BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    series          VARCHAR(3)  NOT NULL,
    year            INT         NOT NULL,
    last_no         BIGINT      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, series, year),
    CONSTRAINT chk_einvoice_counters_series CHECK (series ~ '^[A-Z0-9]{3}$'),
    CONSTRAINT chk_einvoice_counters_year CHECK (year BETWEEN 2000 AND 9999),
    CONSTRAINT chk_einvoice_counters_last_no CHECK (last_no >= 0 AND last_no <= 999999999)
);

CREATE TRIGGER trg_einvoice_counters_set_updated_at
    BEFORE UPDATE ON einvoice_counters
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_einvoice_counters_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON einvoice_counters
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. Invoice archive ---------------------------------------------------------
CREATE TABLE einvoices (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id             BIGINT         NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    number               VARCHAR(16)    NOT NULL,
    profile              VARCHAR(16)    NOT NULL,
    invoice_type         VARCHAR(16)    NOT NULL DEFAULT 'SATIS',
    source_type          VARCHAR(32)    NOT NULL,
    source_uuid          UUID           NOT NULL,
    buyer_org_id         BIGINT         NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    buyer                JSONB          NOT NULL DEFAULT '{}'::jsonb,
    seller               JSONB          NOT NULL DEFAULT '{}'::jsonb,
    lines                JSONB          NOT NULL DEFAULT '[]'::jsonb,
    currency             CHAR(3)        NOT NULL,
    rate_snapshot        JSONB          NOT NULL DEFAULT '{}'::jsonb,
    line_extension       NUMERIC(18,2)  NOT NULL,
    tax_exclusive        NUMERIC(18,2)  NOT NULL,
    tax_total            NUMERIC(18,2)  NOT NULL,
    payable              NUMERIC(18,2)  NOT NULL,
    tax_breakdown        JSONB          NOT NULL DEFAULT '[]'::jsonb,
    xml_storage_key      TEXT           NULL,
    xml_sha256           CHAR(64)       NULL,
    pdf_storage_key      TEXT           NULL,
    validation_status    VARCHAR(16)    NOT NULL DEFAULT 'valid',
    validation_messages  JSONB          NOT NULL DEFAULT '[]'::jsonb,
    status               VARCHAR(16)    NOT NULL DEFAULT 'draft',
    error                TEXT           NULL,
    voided_at            TIMESTAMPTZ    NULL,
    voided_by            BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    void_reason          TEXT           NULL,
    issue_date           DATE           NOT NULL,
    created_by           BIGINT         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_einvoices_uuid UNIQUE (uuid),
    CONSTRAINT uq_einvoices_org_number UNIQUE (organization_id, number),
    CONSTRAINT chk_einvoices_number CHECK (number ~ '^[A-Z0-9]{3}[0-9]{13}$'),
    CONSTRAINT chk_einvoices_profile CHECK (profile IN ('EARSIVFATURA', 'TEMELFATURA', 'TICARIFATURA')),
    CONSTRAINT chk_einvoices_type CHECK (invoice_type = 'SATIS'),
    CONSTRAINT chk_einvoices_source CHECK (source_type IN ('order', 'service_subscription', 'manual')),
    CONSTRAINT chk_einvoices_buyer CHECK (jsonb_typeof(buyer) = 'object'),
    CONSTRAINT chk_einvoices_seller CHECK (jsonb_typeof(seller) = 'object'),
    CONSTRAINT chk_einvoices_lines CHECK (jsonb_typeof(lines) = 'array'),
    CONSTRAINT chk_einvoices_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_einvoices_rate CHECK (jsonb_typeof(rate_snapshot) = 'object'),
    CONSTRAINT chk_einvoices_amounts CHECK (
        line_extension >= 0 AND tax_exclusive >= 0 AND tax_total >= 0 AND payable >= 0),
    CONSTRAINT chk_einvoices_tax_breakdown CHECK (jsonb_typeof(tax_breakdown) = 'array'),
    CONSTRAINT chk_einvoices_xml_sha256 CHECK (
        xml_sha256 IS NULL OR xml_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_einvoices_xml_pair CHECK (
        (xml_storage_key IS NULL) = (xml_sha256 IS NULL)),
    CONSTRAINT chk_einvoices_validation CHECK (
        validation_status IN ('valid', 'rule_warnings', 'invalid')),
    CONSTRAINT chk_einvoices_validation_messages CHECK (jsonb_typeof(validation_messages) = 'array'),
    CONSTRAINT chk_einvoices_status CHECK (status IN ('draft', 'archived', 'failed', 'voided')),
    CONSTRAINT chk_einvoices_archived_xml CHECK (
        status <> 'archived' OR xml_storage_key IS NOT NULL),
    CONSTRAINT chk_einvoices_void CHECK (
        (status = 'voided') = (voided_at IS NOT NULL)
        AND (voided_at IS NULL) = (void_reason IS NULL)
        AND (voided_by IS NULL OR voided_at IS NOT NULL)),
    CONSTRAINT chk_einvoices_error CHECK (
        (status = 'failed' AND error IS NOT NULL AND btrim(error) <> '')
        OR (status <> 'failed'))
);

CREATE UNIQUE INDEX uq_einvoices_source_active
    ON einvoices (organization_id, source_type, source_uuid)
    WHERE status <> 'voided';
CREATE INDEX idx_einvoices_list
    ON einvoices (brand_id, issue_date DESC, id DESC);
CREATE INDEX idx_einvoices_status
    ON einvoices (brand_id, status, issue_date DESC, id DESC);
CREATE INDEX idx_einvoices_buyer
    ON einvoices (brand_id, buyer_org_id, issue_date DESC, id DESC)
    WHERE buyer_org_id IS NOT NULL;

CREATE TRIGGER trg_einvoices_set_updated_at
    BEFORE UPDATE ON einvoices
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION einvoices_check_scope() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    org_brand   BIGINT;
    buyer_brand BIGINT;
BEGIN
    SELECT brand_id INTO org_brand FROM organizations WHERE id = NEW.organization_id;
    IF FOUND AND org_brand IS DISTINCT FROM NEW.brand_id THEN
        RAISE EXCEPTION 'einvoices: brand % does not match organization % (brand %)',
            NEW.brand_id, NEW.organization_id, org_brand
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.buyer_org_id IS NOT NULL THEN
        SELECT brand_id INTO buyer_brand FROM organizations WHERE id = NEW.buyer_org_id;
        IF FOUND AND buyer_brand IS DISTINCT FROM NEW.brand_id THEN
            RAISE EXCEPTION 'einvoices: brand % does not match buyer organization % (brand %)',
                NEW.brand_id, NEW.buyer_org_id, buyer_brand
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_einvoices_check_scope
    BEFORE INSERT OR UPDATE OF organization_id, brand_id, buyer_org_id ON einvoices
    FOR EACH ROW
    EXECUTE FUNCTION einvoices_check_scope();

CREATE FUNCTION einvoices_guard_archived_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'archived' THEN
        RAISE EXCEPTION 'archived e-invoices cannot be deleted; void them'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trg_einvoices_guard_archived_delete
    BEFORE DELETE ON einvoices
    FOR EACH ROW
    EXECUTE FUNCTION einvoices_guard_archived_delete();

-- 5. RBAC --------------------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read e-invoices', 'einvoice.read', 'einvoice',
       ARRAY['brand', 'all']::text[], false, false,
       'Read UBL-TR e-invoice settings, counters and archived invoices.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage e-invoices', 'einvoice.manage', 'einvoice',
       ARRAY['brand', 'all']::text[], true, false,
       'Archive, void and repair UBL-TR e-invoices. Requires step-up.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage e-invoice settings', 'einvoice.settings', 'einvoice',
       ARRAY['all']::text[], false, true,
       'Edit the center seller profile, UBL series and XSLT settings.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'einvoice.read', 'all'),
    ('super_admin', 'einvoice.manage', 'all'),
    ('super_admin', 'einvoice.settings', 'all'),
    ('center_accounting', 'einvoice.read', 'brand'),
    ('center_accounting', 'einvoice.manage', 'brand')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT DO NOTHING;
