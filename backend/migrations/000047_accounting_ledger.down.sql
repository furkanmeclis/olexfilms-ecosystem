-- Reverts TEC-171. Data loss: every cash/bank account, cari account and
-- ledger entry. Dealer roles get accounting.write back (000029 state).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'accounting.dispute');
DELETE FROM permissions WHERE slug = 'accounting.dispute';

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'managed'
FROM roles r
JOIN permissions p ON p.slug = 'accounting.write'
WHERE r.slug IN ('dealer_owner', 'dealer_accounting')
ON CONFLICT DO NOTHING;

DROP VIEW IF EXISTS finance_account_balances;
DROP VIEW IF EXISTS cari_account_balances;
DROP TABLE IF EXISTS finance_entries;
DROP FUNCTION IF EXISTS finance_entries_append_only();
DROP FUNCTION IF EXISTS finance_entries_check_insert();
DROP TABLE IF EXISTS cari_accounts;
DROP TABLE IF EXISTS finance_accounts;
DROP FUNCTION IF EXISTS accounting_accounts_check_org();
