-- Reverts TEC-501. Data loss: e-invoice settings, counters and archived
-- invoice metadata. Stored XML/PDF objects are not deleted from S3.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('einvoice.read', 'einvoice.manage', 'einvoice.settings')
);
DELETE FROM permissions
WHERE slug IN ('einvoice.read', 'einvoice.manage', 'einvoice.settings');

DROP TABLE IF EXISTS einvoices;
DROP FUNCTION IF EXISTS einvoices_check_scope();
DROP FUNCTION IF EXISTS einvoices_guard_archived_delete();
DROP TABLE IF EXISTS einvoice_counters;
DROP TABLE IF EXISTS einvoice_settings;
DROP FUNCTION IF EXISTS einvoice_settings_check_center();

DROP INDEX IF EXISTS idx_organizations_einvoice_registered;
DROP INDEX IF EXISTS idx_organizations_invoice_tax;
ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_email,
    DROP CONSTRAINT IF EXISTS chk_organizations_einvoice_alias,
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_legal_name,
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_tax_office,
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_tax_id,
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_tckn,
    DROP CONSTRAINT IF EXISTS chk_organizations_invoice_vkn,
    DROP COLUMN IF EXISTS invoice_email,
    DROP COLUMN IF EXISTS einvoice_alias,
    DROP COLUMN IF EXISTS einvoice_registered,
    DROP COLUMN IF EXISTS invoice_legal_name,
    DROP COLUMN IF EXISTS invoice_tax_office,
    DROP COLUMN IF EXISTS invoice_tckn,
    DROP COLUMN IF EXISTS invoice_vkn;

ALTER TABLE service_subscription_periods
    DROP CONSTRAINT IF EXISTS uq_service_subscription_periods_uuid,
    DROP COLUMN IF EXISTS uuid;
