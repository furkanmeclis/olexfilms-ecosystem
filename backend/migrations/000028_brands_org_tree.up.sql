-- TEC-83 (F0-06): brands + organization tree (center -> distributor -> dealer).
-- K1/K3/K20: every organization belongs to exactly one brand; the brand of a
-- request is resolved from its host (brand_domains).

CREATE TABLE brands (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID         NOT NULL DEFAULT gen_random_uuid(),
    slug        VARCHAR(32)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    status      VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_brands_uuid UNIQUE (uuid),
    CONSTRAINT uq_brands_slug UNIQUE (slug),
    CONSTRAINT chk_brands_status CHECK (status IN ('active', 'inactive'))
);

CREATE TRIGGER trg_brands_set_updated_at
    BEFORE UPDATE ON brands
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE brand_domains (
    id          BIGSERIAL PRIMARY KEY,
    brand_id    BIGINT       NOT NULL REFERENCES brands (id) ON DELETE CASCADE,
    host        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_brand_domains_host UNIQUE (host),
    CONSTRAINT chk_brand_domains_host_lower CHECK (host = lower(host))
);

CREATE INDEX idx_brand_domains_brand ON brand_domains (brand_id);

-- K2/K3: Glorian stays on its own install for now, so the brand is inactive.
INSERT INTO brands (slug, name, status) VALUES
    ('olex', 'Olex Films', 'active'),
    ('glorian', 'Glorian PPF', 'inactive');

INSERT INTO brand_domains (brand_id, host)
SELECT b.id, d.host
FROM brands b
JOIN (VALUES
    ('olex', 'olexfilms.app'),
    ('olex', 'localhost'),
    ('glorian', 'warranty.glorianppf.com')
) AS d (slug, host) ON d.slug = b.slug;

ALTER TABLE organizations
    ADD COLUMN type                 VARCHAR(16)  NOT NULL DEFAULT 'dealer',
    ADD COLUMN parent_id            BIGINT       NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    ADD COLUMN brand_id             BIGINT       NULL REFERENCES brands (id) ON DELETE RESTRICT,
    ADD COLUMN currency             CHAR(3)      NOT NULL DEFAULT 'TRY',
    ADD COLUMN locale               VARCHAR(16)  NOT NULL DEFAULT 'tr-TR',
    ADD COLUMN timezone             VARCHAR(64)  NOT NULL DEFAULT 'Europe/Istanbul',
    -- countries arrives with TEC-84 (country > city > district); the FK is
    -- added there. Until then this is a plain nullable reference.
    ADD COLUMN country_id           BIGINT       NULL,
    ADD COLUMN contract_pdf_key     TEXT         NULL,
    ADD COLUMN contract_valid_until DATE         NULL,
    ADD COLUMN settings             JSONB        NOT NULL DEFAULT '{}'::jsonb;

-- Linear asks for active|read_only|suspended. pending/expired are kept for
-- backward compatibility (access checks and public registration use them).
ALTER TABLE organizations DROP CONSTRAINT chk_organizations_status;
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_status
    CHECK (status IN ('pending', 'active', 'read_only', 'suspended', 'expired'));

-- One center per brand (seed).
INSERT INTO organizations (slug, name, status, plan_code, type, brand_id, currency, locale, timezone)
SELECT c.slug, c.name, 'active', NULL, 'center', b.id, 'TRY', 'tr-TR', 'Europe/Istanbul'
FROM brands b
JOIN (VALUES
    ('olex', 'olex-merkez', 'Olex Merkez'),
    ('glorian', 'glorian-merkez', 'Glorian Merkez')
) AS c (brand_slug, slug, name) ON c.brand_slug = b.slug;

-- Backfill: every existing organization becomes an Olex dealer under the
-- Olex center.
UPDATE organizations o
SET brand_id = b.id,
    parent_id = center.id
FROM brands b
JOIN organizations center ON center.brand_id = b.id AND center.type = 'center'
WHERE b.slug = 'olex'
  AND o.brand_id IS NULL;

ALTER TABLE organizations ALTER COLUMN brand_id SET NOT NULL;
ALTER TABLE organizations ALTER COLUMN type DROP DEFAULT;

ALTER TABLE organizations
    ADD CONSTRAINT chk_organizations_type
        CHECK (type IN ('center', 'distributor', 'dealer')),
    ADD CONSTRAINT chk_organizations_parent
        CHECK ((type = 'center' AND parent_id IS NULL) OR (type <> 'center' AND parent_id IS NOT NULL)),
    ADD CONSTRAINT chk_organizations_parent_not_self
        CHECK (parent_id IS NULL OR parent_id <> id),
    ADD CONSTRAINT chk_organizations_currency
        CHECK (currency ~ '^[A-Z]{3}$');

CREATE INDEX idx_organizations_parent ON organizations (parent_id);
CREATE INDEX idx_organizations_brand_type ON organizations (brand_id, type);
CREATE UNIQUE INDEX uq_organizations_brand_center ON organizations (brand_id) WHERE type = 'center';
