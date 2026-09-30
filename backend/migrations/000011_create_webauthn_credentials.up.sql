CREATE TABLE webauthn_credentials (
    id                   BIGSERIAL PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id              BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id        TEXT         NOT NULL,
    public_key           TEXT         NOT NULL,
    counter              BIGINT       NOT NULL DEFAULT 0,
    device_type          TEXT         NOT NULL DEFAULT 'unknown',
    backed_up            BOOLEAN      NOT NULL DEFAULT false,
    transports           TEXT         NULL,
    provider_account_id  TEXT         NOT NULL DEFAULT '',
    name                 TEXT         NULL,
    last_used_at         TIMESTAMPTZ  NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_webauthn_credentials_uuid UNIQUE (uuid),
    CONSTRAINT uq_webauthn_credentials_credential_id UNIQUE (credential_id),
    CONSTRAINT chk_webauthn_credentials_counter CHECK (counter >= 0)
);

CREATE INDEX idx_webauthn_credentials_user_id ON webauthn_credentials (user_id);

CREATE TRIGGER trg_webauthn_credentials_set_updated_at
    BEFORE UPDATE ON webauthn_credentials
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
