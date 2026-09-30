CREATE TABLE otp_codes (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id       BIGINT       NULL REFERENCES users (id) ON DELETE CASCADE,
    email         VARCHAR(255) NOT NULL,
    code_hash     TEXT         NOT NULL,
    type          VARCHAR(32)  NOT NULL,
    expires_at    TIMESTAMPTZ  NOT NULL,
    consumed_at   TIMESTAMPTZ  NULL,
    attempt_count INTEGER      NOT NULL DEFAULT 0,
    max_attempts  INTEGER      NOT NULL DEFAULT 5,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_otp_codes_uuid UNIQUE (uuid),
    CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification')
    ),
    CONSTRAINT chk_otp_codes_attempts CHECK (attempt_count >= 0 AND max_attempts >= 1)
);

CREATE INDEX idx_otp_codes_email_type_active
    ON otp_codes (email, type)
    WHERE consumed_at IS NULL;

CREATE INDEX idx_otp_codes_expires_at ON otp_codes (expires_at);
