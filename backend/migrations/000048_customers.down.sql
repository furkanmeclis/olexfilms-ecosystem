-- Reverts TEC-159. Data loss: every customer profile, customer link and
-- vehicle, and the merge pointers. Anonymized users become disabled.
-- Organization phones that were moved aside are restored where the
-- normalizer did not fill phone.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('vehicles.read', 'vehicles.write', 'customers.anonymize', 'customers.merge')
);
DELETE FROM permissions
WHERE slug IN ('vehicles.read', 'vehicles.write', 'customers.anonymize', 'customers.merge');

ALTER TABLE organizations DROP CONSTRAINT IF EXISTS chk_organizations_phone_e164;
UPDATE organizations SET phone = phone_raw WHERE phone = '' AND phone_raw IS NOT NULL;
ALTER TABLE organizations DROP COLUMN IF EXISTS phone_raw;

DROP TABLE IF EXISTS vehicles;
DROP FUNCTION IF EXISTS vehicles_check_model();
DROP TABLE IF EXISTS customer_organizations;
DROP FUNCTION IF EXISTS customer_scope_check_org();
DROP TABLE IF EXISTS customer_profiles;

DROP INDEX IF EXISTS idx_users_merged_into;
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_merged_not_self,
    DROP COLUMN IF EXISTS merged_into_user_id;

UPDATE users SET status = 'disabled' WHERE status = 'anonymized';
ALTER TABLE users DROP CONSTRAINT chk_users_status;
ALTER TABLE users ADD CONSTRAINT chk_users_status
    CHECK (status IN ('active', 'disabled', 'pending'));
