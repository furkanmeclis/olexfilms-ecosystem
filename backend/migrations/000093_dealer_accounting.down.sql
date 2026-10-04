-- Reverts TEC-341. Data loss: dealer prices, product sales, suppliers,
-- purchases, staff cards and staff payments are removed; services lose
-- their income entry link. Ledger rows (finance_entries) are kept.
DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug IN ('dealer_owner', 'dealer_accounting'))
  AND permission_id IN (SELECT id FROM permissions WHERE slug = 'accounting.write');

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'dealer_pricing.write', 'product_sales.write', 'suppliers.manage',
        'purchases.write', 'staff.manage', 'staff_payments.write')
);
DELETE FROM permissions WHERE slug IN (
    'dealer_pricing.write', 'product_sales.write', 'suppliers.manage',
    'purchases.write', 'staff.manage', 'staff_payments.write');

DROP INDEX IF EXISTS idx_services_income_entry;
ALTER TABLE services
    DROP CONSTRAINT IF EXISTS chk_services_income_amount,
    DROP CONSTRAINT IF EXISTS fk_services_income_entry,
    DROP COLUMN IF EXISTS income_amount,
    DROP COLUMN IF EXISTS income_entry_id;

DROP TABLE IF EXISTS staff_payments;
DROP TABLE IF EXISTS staff_profiles;
DROP TABLE IF EXISTS purchase_lines;
DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS product_sale_lines;
DROP TABLE IF EXISTS product_sales;
DROP TABLE IF EXISTS dealer_product_prices;

DROP TRIGGER IF EXISTS trg_cari_accounts_check_customer ON cari_accounts;
DROP FUNCTION IF EXISTS cari_accounts_check_customer();
DROP INDEX IF EXISTS idx_cari_accounts_counterparty_user;

ALTER TABLE finance_entries DROP CONSTRAINT IF EXISTS uq_finance_entries_id_org;
