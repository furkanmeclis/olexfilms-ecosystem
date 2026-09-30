CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    user_agent TEXT        NULL,
    ip_address INET        NULL,
    impersonator_user_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_refresh_tokens_uuid UNIQUE (uuid),
    CONSTRAINT uq_refresh_tokens_token_hash UNIQUE (token_hash)
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_refresh_tokens_impersonator ON refresh_tokens (impersonator_user_id)
    WHERE impersonator_user_id IS NOT NULL;
