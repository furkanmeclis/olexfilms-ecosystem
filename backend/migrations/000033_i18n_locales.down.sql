-- Data loss: users.timezone is dropped; user locales outside tr/en (and NULL)
-- become tr; organization locales go back to the 'tr-TR' style for tr/en and
-- keep the short code otherwise.
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS chk_organizations_locale;
ALTER TABLE organizations ALTER COLUMN locale TYPE VARCHAR(16);
ALTER TABLE organizations ALTER COLUMN locale SET DEFAULT 'tr-TR';
UPDATE organizations
SET locale = CASE locale
    WHEN 'tr' THEN 'tr-TR'
    WHEN 'en' THEN 'en-US'
    ELSE locale
END;

ALTER TABLE users DROP COLUMN IF EXISTS timezone;

ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_locale;
UPDATE users SET locale = 'tr' WHERE locale IS NULL OR locale NOT IN ('tr', 'en');
ALTER TABLE users
    ALTER COLUMN locale SET DEFAULT 'tr',
    ALTER COLUMN locale SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT chk_users_locale CHECK (locale IN ('tr', 'en'));
