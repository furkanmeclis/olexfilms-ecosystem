DROP INDEX IF EXISTS idx_export_jobs_organization;

ALTER TABLE export_jobs DROP COLUMN IF EXISTS organization_id;
