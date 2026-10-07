-- Reverts TEC-487. Data loss: expected consumption definitions and
-- efficiency projections.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('efficiency.read', 'efficiency.expectations.manage')
);
DELETE FROM permissions WHERE slug IN ('efficiency.read', 'efficiency.expectations.manage');

DROP TABLE IF EXISTS roll_efficiency;
DROP TABLE IF EXISTS efficiency_facts;
DROP TRIGGER IF EXISTS trg_part_consumption_expectations_check ON part_consumption_expectations;
DROP FUNCTION IF EXISTS part_consumption_expectations_check();
DROP TABLE IF EXISTS part_consumption_expectations;
