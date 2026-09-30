CREATE TABLE stepup_settings (
    id                            SMALLINT     PRIMARY KEY DEFAULT 1,
    ttl_hours                     INT          NOT NULL DEFAULT 4,
    password_enabled              BOOLEAN      NOT NULL DEFAULT TRUE,
    passkey_enabled               BOOLEAN      NOT NULL DEFAULT TRUE,
    totp_enabled                  BOOLEAN      NOT NULL DEFAULT FALSE,
    password_login_totp_required  BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT stepup_settings_singleton CHECK (id = 1),
    CONSTRAINT stepup_settings_ttl_hours_chk CHECK (ttl_hours BETWEEN 1 AND 168),
    CONSTRAINT stepup_settings_methods_chk CHECK (
        password_enabled OR passkey_enabled OR totp_enabled
    )
);

INSERT INTO stepup_settings (id) VALUES (1);

CREATE TRIGGER trg_stepup_settings_set_updated_at
    BEFORE UPDATE ON stepup_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
