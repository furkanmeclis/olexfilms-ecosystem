CREATE TABLE auth_settings (
    id                         SMALLINT     PRIMARY KEY DEFAULT 1,
    registration_enabled       BOOLEAN      NOT NULL DEFAULT FALSE,
    default_role_id            BIGINT       NULL REFERENCES roles (id) ON DELETE SET NULL,
    password_login_enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    password_register_enabled  BOOLEAN      NOT NULL DEFAULT FALSE,
    passkey_login_enabled      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_settings_singleton CHECK (id = 1)
);

INSERT INTO auth_settings (id) VALUES (1);

CREATE TRIGGER trg_auth_settings_set_updated_at
    BEFORE UPDATE ON auth_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE oauth_provider_settings (
    provider           VARCHAR(32)  PRIMARY KEY,
    login_enabled      BOOLEAN      NOT NULL DEFAULT FALSE,
    register_enabled   BOOLEAN      NOT NULL DEFAULT FALSE,
    client_id          TEXT         NOT NULL DEFAULT '',
    client_secret_enc  TEXT         NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT oauth_provider_settings_provider_chk CHECK (
        provider IN ('google', 'facebook', 'apple')
    )
);

INSERT INTO oauth_provider_settings (provider) VALUES
    ('google'),
    ('facebook'),
    ('apple');

CREATE TRIGGER trg_oauth_provider_settings_set_updated_at
    BEFORE UPDATE ON oauth_provider_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
