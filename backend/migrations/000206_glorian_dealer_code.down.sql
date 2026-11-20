DROP VIEW IF EXISTS glorian_dealer_matches;
DROP INDEX IF EXISTS uq_integration_external_parties_linked_org;
DROP INDEX IF EXISTS idx_integration_external_parties_dealer_code;
ALTER TABLE integration_external_parties
    DROP COLUMN IF EXISTS linked_at,
    DROP COLUMN IF EXISTS linked_organization_id,
    DROP COLUMN IF EXISTS dealer_code;

DROP INDEX IF EXISTS uq_organizations_brand_glorian_dealer_code;
ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_glorian_dealer_code,
    DROP COLUMN IF EXISTS glorian_dealer_code;
