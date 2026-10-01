-- Data loss: drops brand, tree, locale/currency and contract columns, and
-- deletes the seeded center organizations (their memberships cascade).
DROP INDEX IF EXISTS uq_organizations_brand_center;
DROP INDEX IF EXISTS idx_organizations_brand_type;
DROP INDEX IF EXISTS idx_organizations_parent;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_currency,
    DROP CONSTRAINT IF EXISTS chk_organizations_parent_not_self,
    DROP CONSTRAINT IF EXISTS chk_organizations_parent,
    DROP CONSTRAINT IF EXISTS chk_organizations_type;

ALTER TABLE organizations DROP COLUMN IF EXISTS parent_id;

DELETE FROM organizations WHERE type = 'center';

UPDATE organizations SET status = 'suspended' WHERE status = 'read_only';
ALTER TABLE organizations DROP CONSTRAINT chk_organizations_status;
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_status
    CHECK (status IN ('pending', 'active', 'suspended', 'expired'));

ALTER TABLE organizations
    DROP COLUMN IF EXISTS settings,
    DROP COLUMN IF EXISTS contract_valid_until,
    DROP COLUMN IF EXISTS contract_pdf_key,
    DROP COLUMN IF EXISTS country_id,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS brand_id,
    DROP COLUMN IF EXISTS type;

DROP TABLE IF EXISTS brand_domains;
DROP TABLE IF EXISTS brands;
