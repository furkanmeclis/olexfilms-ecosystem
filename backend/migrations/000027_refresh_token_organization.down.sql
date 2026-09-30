DROP INDEX IF EXISTS idx_refresh_tokens_organization_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS organization_id;
