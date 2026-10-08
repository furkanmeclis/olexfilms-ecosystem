-- TEC-491 (F5-05b): keep own-organization and subtree metric projections
-- side by side. Existing rows are the own-organization scope.
ALTER TABLE performance_metrics_monthly
    DROP CONSTRAINT uq_performance_metrics_monthly;

ALTER TABLE performance_metrics_monthly
    ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'org',
    ADD CONSTRAINT chk_performance_metrics_scope CHECK (scope IN ('org', 'subtree')),
    ADD CONSTRAINT uq_performance_metrics_monthly UNIQUE (organization_id, period, scope, metric);

CREATE INDEX idx_performance_metrics_scope_period
    ON performance_metrics_monthly (brand_id, period, scope, metric);
