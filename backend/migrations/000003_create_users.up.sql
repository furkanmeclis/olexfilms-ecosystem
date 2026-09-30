CREATE TABLE users (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    email             VARCHAR(255) NOT NULL,
    password_hash     TEXT         NOT NULL,
    name              VARCHAR(100) NOT NULL,
    surname           VARCHAR(100) NOT NULL,
    status            VARCHAR(32)  NOT NULL DEFAULT 'pending',
    email_verified_at TIMESTAMPTZ  NULL,
    last_login_at     TIMESTAMPTZ  NULL,
    locale            VARCHAR(8)   NOT NULL DEFAULT 'tr',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  NULL,
    CONSTRAINT uq_users_uuid UNIQUE (uuid),
    CONSTRAINT chk_users_status CHECK (status IN ('active', 'disabled', 'pending')),
    CONSTRAINT chk_users_locale CHECK (locale IN ('tr', 'en'))
);

CREATE UNIQUE INDEX uq_users_email_active
    ON users (email)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_users_status ON users (status)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
