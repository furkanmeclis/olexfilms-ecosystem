CREATE TABLE github_app_settings (
    id                  SMALLINT     PRIMARY KEY DEFAULT 1,
    enabled             BOOLEAN      NOT NULL DEFAULT FALSE,
    register_enabled    BOOLEAN      NOT NULL DEFAULT FALSE,
    app_id              TEXT         NOT NULL DEFAULT '',
    client_id           TEXT         NOT NULL DEFAULT '',
    client_secret_enc   TEXT         NULL,
    private_key_enc     TEXT         NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT github_app_settings_singleton CHECK (id = 1)
);

INSERT INTO github_app_settings (id) VALUES (1);

CREATE TRIGGER trg_github_app_settings_set_updated_at
    BEFORE UPDATE ON github_app_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE oauth_accounts (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id              BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider             VARCHAR(32)  NOT NULL,
    provider_account_id  TEXT         NOT NULL,
    type                 VARCHAR(32)  NOT NULL DEFAULT 'oauth',
    access_token_enc     TEXT         NULL,
    refresh_token_enc    TEXT         NULL,
    expires_at           TIMESTAMPTZ  NULL,
    token_type           TEXT         NULL,
    scope                TEXT         NULL,
    github_login         TEXT         NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT oauth_accounts_provider_account_uq UNIQUE (provider, provider_account_id),
    CONSTRAINT oauth_accounts_user_provider_uq UNIQUE (user_id, provider)
);

CREATE INDEX idx_oauth_accounts_user_id ON oauth_accounts (user_id);

CREATE TRIGGER trg_oauth_accounts_set_updated_at
    BEFORE UPDATE ON oauth_accounts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
