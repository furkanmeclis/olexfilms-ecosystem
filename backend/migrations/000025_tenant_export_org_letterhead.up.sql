ALTER TABLE export_jobs
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE;

CREATE INDEX idx_export_jobs_organization
    ON export_jobs (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;
