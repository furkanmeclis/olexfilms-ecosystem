CREATE TABLE user_totp (
    user_id          BIGINT       PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret_enc       TEXT         NOT NULL,
    enabled          BOOLEAN      NOT NULL DEFAULT FALSE,
    confirmed_at     TIMESTAMPTZ  NULL,
    recovery_hashes  TEXT[]       NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_user_totp_set_updated_at
    BEFORE UPDATE ON user_totp
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
