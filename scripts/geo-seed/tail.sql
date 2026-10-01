
-- Organizations: country FK (prepared by TEC-83) + province/district.
-- city/district text stay as the display copy (letterhead, legacy rows).
UPDATE organizations SET country_id = NULL
WHERE country_id IS NOT NULL AND country_id NOT IN (SELECT id FROM countries);

ALTER TABLE organizations
    ADD CONSTRAINT fk_organizations_country
        FOREIGN KEY (country_id) REFERENCES countries (id) ON DELETE RESTRICT,
    ADD COLUMN province_id BIGINT NULL REFERENCES provinces (id) ON DELETE RESTRICT,
    ADD COLUMN district_id BIGINT NULL REFERENCES districts (id) ON DELETE RESTRICT,
    ADD CONSTRAINT chk_organizations_address_chain CHECK (
        (district_id IS NULL OR province_id IS NOT NULL) AND (province_id IS NULL OR country_id IS NOT NULL)
    );

-- Backfill: existing (Turkish) organizations whose city text names a TR
-- province get the FK; the district is matched inside that province.
-- Unmatched rows stay NULL (dirty legacy text).
UPDATE organizations o
SET country_id = p.country_id, province_id = p.id
FROM provinces p
JOIN countries c ON c.id = p.country_id AND c.iso2 = 'TR'
WHERE o.province_id IS NULL
  AND (o.country_id IS NULL OR o.country_id = c.id)
  AND btrim(o.city) <> ''
  AND lower(translate(btrim(o.city), 'İIıÇŞĞÜÖ', 'iiiçşğüö')) = lower(translate(p.name, 'İIıÇŞĞÜÖ', 'iiiçşğüö'));

UPDATE organizations o
SET district_id = d.id
FROM districts d
WHERE o.district_id IS NULL
  AND o.province_id = d.province_id
  AND btrim(o.district) <> ''
  AND lower(translate(btrim(o.district), 'İIıÇŞĞÜÖ', 'iiiçşğüö')) = lower(translate(d.name, 'İIıÇŞĞÜÖ', 'iiiçşğüö'));

CREATE INDEX idx_organizations_address ON organizations (country_id, province_id, district_id);

-- Territories (K5): a distributor covers a country, a province or a district.
-- An area belongs to one distributor per brand; ancestor/descendant overlaps
-- (NL with A, Amsterdam with B) are rejected by the application inside a
-- transaction holding an advisory lock per (brand, country).
CREATE TABLE territories (
    id                 BIGSERIAL   PRIMARY KEY,
    uuid               UUID        NOT NULL DEFAULT gen_random_uuid(),
    brand_id           BIGINT      NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    -- The distributor that owns the area.
    organization_id    BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    country_id         BIGINT      NOT NULL REFERENCES countries (id) ON DELETE RESTRICT,
    province_id        BIGINT      NULL REFERENCES provinces (id) ON DELETE RESTRICT,
    district_id        BIGINT      NULL REFERENCES districts (id) ON DELETE RESTRICT,
    level              VARCHAR(16) GENERATED ALWAYS AS (
        CASE
            WHEN district_id IS NOT NULL THEN 'district'
            WHEN province_id IS NOT NULL THEN 'province'
            ELSE 'country'
        END
    ) STORED,
    created_by_user_id BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_territories_uuid UNIQUE (uuid),
    CONSTRAINT chk_territories_chain CHECK (district_id IS NULL OR province_id IS NOT NULL),
    CONSTRAINT uq_territories_area UNIQUE NULLS NOT DISTINCT (brand_id, country_id, province_id, district_id)
);

CREATE INDEX idx_territories_organization ON territories (organization_id);

-- Licence plate formats. The plate country is chosen on the vehicle and is
-- independent of the customer's country/locale. regex runs on the compact
-- form (upper case, spaces/dashes/dots removed) and must be RE2 compatible
-- (Go regexp; no lookaround).
CREATE TABLE plate_formats (
    id               BIGSERIAL   PRIMARY KEY,
    country_id       BIGINT      NOT NULL REFERENCES countries (id) ON DELETE CASCADE,
    regex            TEXT        NOT NULL,
    input_mask       VARCHAR(64) NOT NULL DEFAULT '',
    example          VARCHAR(32) NOT NULL DEFAULT '',
    country_label    VARCHAR(4)  NOT NULL,
    strip_color      CHAR(7)     NOT NULL DEFAULT '#003399',
    background_color CHAR(7)     NOT NULL DEFAULT '#FFFFFF',
    text_color       CHAR(7)     NOT NULL DEFAULT '#000000',
    is_active        BOOLEAN     NOT NULL DEFAULT true,
    sort_order       INT         NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_plate_formats_country UNIQUE (country_id),
    CONSTRAINT chk_plate_formats_colors CHECK (
        strip_color ~ '^#[0-9A-Fa-f]{6}$' AND background_color ~ '^#[0-9A-Fa-f]{6}$' AND text_color ~ '^#[0-9A-Fa-f]{6}$'
    )
);

CREATE TRIGGER trg_plate_formats_set_updated_at
    BEFORE UPDATE ON plate_formats
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO plate_formats (country_id, regex, input_mask, example, country_label, strip_color, background_color, text_color, sort_order)
SELECT c.id, v.regex, v.mask, v.example, v.label, v.strip, v.bg, '#000000', v.sort
FROM (VALUES
    ('TR', '^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$', '99 AAA 9999', '34 ABC 123', 'TR', '#003399', '#FFFFFF', 10),
    ('DE', '^[A-ZÄÖÜ]{1,3}[A-Z]{1,2}[1-9][0-9]{0,3}[EH]?$', 'AAA AA 9999', 'B AB 1234', 'D', '#003399', '#FFFFFF', 20),
    ('NL', '^([A-Z]{2}[0-9]{4}|[0-9]{4}[A-Z]{2}|[0-9]{2}[A-Z]{2}[0-9]{2}|[A-Z]{2}[0-9]{2}[A-Z]{2}|[A-Z]{4}[0-9]{2}|[0-9]{2}[A-Z]{4}|[0-9]{2}[A-Z]{3}[0-9]|[0-9][A-Z]{3}[0-9]{2}|[A-Z]{2}[0-9]{3}[A-Z]|[A-Z][0-9]{3}[A-Z]{2}|[A-Z]{3}[0-9]{2}[A-Z]|[A-Z][0-9]{2}[A-Z]{3}|[0-9][A-Z]{2}[0-9]{3}|[0-9]{3}[A-Z]{2}[0-9])$', 'XX-999-X', 'AB-123-C', 'NL', '#003399', '#FFCC00', 30)
) AS v (iso2, regex, mask, example, label, strip, bg, sort)
JOIN countries c ON c.iso2 = v.iso2;

-- Currencies (ISO 4217) used by organizations and exchange rates.
CREATE TABLE currencies (
    code       CHAR(3)     PRIMARY KEY,
    name       VARCHAR(64) NOT NULL,
    symbol     VARCHAR(8)  NOT NULL DEFAULT '',
    decimals   SMALLINT    NOT NULL DEFAULT 2,
    is_active  BOOLEAN     NOT NULL DEFAULT true,
    sort_order INT         NOT NULL DEFAULT 0,
    CONSTRAINT chk_currencies_code CHECK (code ~ '^[A-Z]{3}$')
);

INSERT INTO currencies (code, name, symbol, decimals, sort_order) VALUES
    ('TRY', 'Turkish Lira', '₺', 2, 10),
    ('EUR', 'Euro', '€', 2, 20),
    ('USD', 'US Dollar', '$', 2, 30),
    ('GBP', 'Pound Sterling', '£', 2, 40),
    ('UAH', 'Ukrainian Hryvnia', '₴', 2, 50),
    ('BGN', 'Bulgarian Lev', 'лв', 2, 60),
    ('AZN', 'Azerbaijani Manat', '₼', 2, 70),
    ('CNY', 'Chinese Yuan', '¥', 2, 80),
    ('AED', 'UAE Dirham', 'د.إ', 2, 90),
    ('RUB', 'Russian Ruble', '₽', 2, 100),
    ('CHF', 'Swiss Franc', 'CHF', 2, 110),
    ('JPY', 'Japanese Yen', '¥', 0, 120),
    ('SAR', 'Saudi Riyal', '﷼', 2, 130),
    ('PLN', 'Polish Zloty', 'zł', 2, 140),
    ('RON', 'Romanian Leu', 'lei', 2, 150),
    ('SEK', 'Swedish Krona', 'kr', 2, 160),
    ('NOK', 'Norwegian Krone', 'kr', 2, 170),
    ('DKK', 'Danish Krone', 'kr', 2, 180),
    ('CAD', 'Canadian Dollar', '$', 2, 190),
    ('AUD', 'Australian Dollar', '$', 2, 200),
    ('KWD', 'Kuwaiti Dinar', 'KD', 3, 210),
    ('QAR', 'Qatari Riyal', 'QR', 2, 220);

-- Exchange rates: 1 base = rate quote on rate_date. Sources: tcmb (TRY
-- pairs, ForexSelling/Unit), ecb (EUR pairs), manual (admin override).
-- Resolution: latest rate_date <= the asked date, manual > tcmb > ecb.
CREATE TABLE exchange_rates (
    id                 BIGSERIAL      PRIMARY KEY,
    rate_date          DATE           NOT NULL,
    base               CHAR(3)        NOT NULL,
    quote              CHAR(3)        NOT NULL,
    rate               NUMERIC(20, 10) NOT NULL,
    source             VARCHAR(16)    NOT NULL,
    note               TEXT           NULL,
    created_by_user_id BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    fetched_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_exchange_rates_day_pair_source UNIQUE (rate_date, base, quote, source),
    CONSTRAINT chk_exchange_rates_codes CHECK (base ~ '^[A-Z]{3}$' AND quote ~ '^[A-Z]{3}$' AND base <> quote),
    CONSTRAINT chk_exchange_rates_rate CHECK (rate > 0),
    CONSTRAINT chk_exchange_rates_source CHECK (source IN ('tcmb', 'ecb', 'manual'))
);

CREATE INDEX idx_exchange_rates_pair_date ON exchange_rates (base, quote, rate_date DESC);

CREATE TRIGGER trg_exchange_rates_set_updated_at
    BEFORE UPDATE ON exchange_rates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Permissions (rbac catalog). Appended after the last catalog row: the sort
-- order continues from the highest existing value, matching the Go catalog
-- position (Permissions slice index * 10).
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT v.name, v.slug, 'platform', ARRAY['all']::text[], false, false, v.description,
       (SELECT COALESCE(MAX(sort_order), 0) FROM permissions WHERE slug NOT IN (
           'platform.geo.write', 'platform.territories.read', 'platform.territories.write',
           'platform.rates.read', 'platform.rates.write')) + v.n * 10
FROM (VALUES
    (1, 'Write geography', 'platform.geo.write', 'Add or remove provinces and districts; activate countries.'),
    (2, 'Read territories', 'platform.territories.read', 'Distributor territories (country, province, district).'),
    (3, 'Write territories', 'platform.territories.write', 'Assign or remove distributor territories (K5).'),
    (4, 'Read exchange rates', 'platform.rates.read', 'Daily TCMB/ECB rates and manual overrides.'),
    (5, 'Write exchange rates', 'platform.rates.write', 'Manual rate overrides and on-demand fetch.')
) AS v (n, name, slug, description)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    module = EXCLUDED.module,
    scopes = EXCLUDED.scopes,
    is_sensitive = EXCLUDED.is_sensitive,
    super_admin_only = EXCLUDED.super_admin_only,
    description = EXCLUDED.description,
    sort_order = EXCLUDED.sort_order;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r
JOIN permissions p ON p.slug IN (
    'platform.geo.write', 'platform.territories.read', 'platform.territories.write',
    'platform.rates.read', 'platform.rates.write')
WHERE r.slug = 'super_admin'
ON CONFLICT DO NOTHING;
