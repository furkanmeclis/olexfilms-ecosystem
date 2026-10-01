-- TEC-136 (F0-13a): 13 locales (K10), user timezone, short org locale codes.
-- Canonical codes: tr, en, bg, de, el, uk, ru, fr, es, it, zh-CN, az, ar
-- (platform/i18n.Supported holds the same list).

-- users.locale: NULL now means "inherit from the organization". Existing
-- tr/en values are kept.
ALTER TABLE users DROP CONSTRAINT chk_users_locale;
ALTER TABLE users
    ALTER COLUMN locale DROP NOT NULL,
    ALTER COLUMN locale DROP DEFAULT;
ALTER TABLE users ADD CONSTRAINT chk_users_locale CHECK (
    locale IS NULL OR locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
);

-- users.timezone: IANA name, validated in the application (time.LoadLocation).
-- NULL means "inherit from the organization".
ALTER TABLE users ADD COLUMN timezone VARCHAR(64) NULL;

-- organizations.locale: 'tr-TR' style -> short code. zh variants become
-- zh-CN; anything unsupported falls back to tr.
UPDATE organizations
SET locale = CASE
    WHEN lower(split_part(replace(locale, '_', '-'), '-', 1)) = 'zh' THEN 'zh-CN'
    WHEN lower(split_part(replace(locale, '_', '-'), '-', 1))
        IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'az', 'ar')
        THEN lower(split_part(replace(locale, '_', '-'), '-', 1))
    ELSE 'tr'
END;

ALTER TABLE organizations ALTER COLUMN locale TYPE VARCHAR(8);
ALTER TABLE organizations ALTER COLUMN locale SET DEFAULT 'tr';
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar')
);
