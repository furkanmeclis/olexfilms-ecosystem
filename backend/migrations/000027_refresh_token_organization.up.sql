ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_organization_id
    ON refresh_tokens (organization_id)
    WHERE organization_id IS NOT NULL;
