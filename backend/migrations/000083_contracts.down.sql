-- TEC-285: drops the contract schema and permissions. Data loss: every
-- contract template, instance, signer, signature and media row is removed,
-- and services.contract_id is cleared (it was a placeholder before 000083).

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('contracts.templates.manage', 'contracts.read', 'contracts.write', 'contracts.void')
);
DELETE FROM permissions
WHERE slug IN ('contracts.templates.manage', 'contracts.read', 'contracts.write', 'contracts.void');

DROP INDEX IF EXISTS idx_services_contract;
ALTER TABLE services DROP CONSTRAINT IF EXISTS fk_services_contract;
UPDATE services SET contract_id = NULL WHERE contract_id IS NOT NULL;

DROP TABLE IF EXISTS contract_media;
DROP TABLE IF EXISTS contract_signatures;
DROP TABLE IF EXISTS contract_signers;
DROP TABLE IF EXISTS contract_instances;
DROP TABLE IF EXISTS contract_counters;
DROP TABLE IF EXISTS contract_template_locales;
DROP TABLE IF EXISTS contract_templates;

DROP FUNCTION IF EXISTS contract_signatures_append_only();
DROP FUNCTION IF EXISTS contract_signers_check_otp();
DROP FUNCTION IF EXISTS contract_children_check_open();
DROP FUNCTION IF EXISTS contract_instances_no_delete();
DROP FUNCTION IF EXISTS contract_instances_check_row();
