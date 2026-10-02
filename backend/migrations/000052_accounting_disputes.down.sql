-- Reverts TEC-174. Data loss: every accounting dispute (the ledger rows a
-- resolution posted stay; finance_entries is append-only).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'accounting.resolve');
DELETE FROM permissions WHERE slug = 'accounting.resolve';

DROP TABLE IF EXISTS accounting_disputes;
DROP FUNCTION IF EXISTS accounting_disputes_check_row();
