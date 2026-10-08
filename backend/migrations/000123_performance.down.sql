-- Reverts TEC-490. Data loss: performance metrics, targets, staff targets,
-- bonus rules/accruals/settings, weak dealer rules and the rule/period
-- markers of automatic tasks (the tasks stay, with source 'auto').
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'performance.read', 'performance.targets.manage', 'performance.staff_targets.manage',
        'performance.bonus.manage', 'performance.rules.manage'
    )
);
DELETE FROM permissions WHERE slug IN (
    'performance.read', 'performance.targets.manage', 'performance.staff_targets.manage',
    'performance.bonus.manage', 'performance.rules.manage'
);

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS uq_tasks_auto,
    DROP CONSTRAINT IF EXISTS chk_tasks_auto_rule,
    DROP COLUMN IF EXISTS auto_period,
    DROP COLUMN IF EXISTS auto_rule_id;

DROP TABLE IF EXISTS weak_dealer_rules;
DROP FUNCTION IF EXISTS weak_dealer_rules_check_row();
DROP TABLE IF EXISTS bonus_accruals;
ALTER TABLE staff_payments DROP CONSTRAINT IF EXISTS uq_staff_payments_id_org;
DROP TABLE IF EXISTS bonus_settings;
DROP TABLE IF EXISTS bonus_rules;
DROP TABLE IF EXISTS staff_targets;
DROP TABLE IF EXISTS performance_targets;
DROP FUNCTION IF EXISTS performance_targets_check_row();
DROP TABLE IF EXISTS performance_metrics_monthly;
