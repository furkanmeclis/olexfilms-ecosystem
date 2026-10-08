-- Reverts TEC-491. Data loss: subtree metric projection rows.
DELETE FROM performance_metrics_monthly WHERE scope = 'subtree';

DROP INDEX IF EXISTS idx_performance_metrics_scope_period;

ALTER TABLE performance_metrics_monthly
    DROP CONSTRAINT uq_performance_metrics_monthly,
    DROP CONSTRAINT chk_performance_metrics_scope,
    DROP COLUMN scope,
    ADD CONSTRAINT uq_performance_metrics_monthly UNIQUE (organization_id, period, metric);
