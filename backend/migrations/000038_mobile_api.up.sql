-- TEC-91 (F0-15): mobile API contract and auth skeleton.

-- 1. Mobile sessions on refresh_tokens. The mobile app signs in with a
-- device description; its refresh rows carry the device, and every rotation
-- inherits family_id (one chain per device sign-in). A rotated token that
-- comes back (rotated_at set) revokes the whole chain (reuse detection).
ALTER TABLE refresh_tokens
    DROP CONSTRAINT chk_refresh_tokens_realm,
    ADD CONSTRAINT chk_refresh_tokens_realm CHECK (realm IN ('panel', 'portal', 'mobile')),
    ADD COLUMN client      VARCHAR(16)  NOT NULL DEFAULT 'web',
    ADD COLUMN device_id   VARCHAR(128) NULL,
    ADD COLUMN device_name VARCHAR(128) NULL,
    ADD COLUMN platform    VARCHAR(16)  NULL,
    ADD COLUMN app_version VARCHAR(32)  NULL,
    ADD COLUMN family_id   UUID         NULL,
    ADD COLUMN rotated_at  TIMESTAMPTZ  NULL,
    ADD CONSTRAINT chk_refresh_tokens_client CHECK (client IN ('web', 'mobile')),
    ADD CONSTRAINT chk_refresh_tokens_platform CHECK (platform IS NULL OR platform IN ('ios', 'android')),
    ADD CONSTRAINT chk_refresh_tokens_mobile_device CHECK (client <> 'mobile' OR device_id IS NOT NULL);

CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id)
    WHERE family_id IS NOT NULL;
CREATE INDEX idx_refresh_tokens_user_device ON refresh_tokens (user_id, device_id)
    WHERE client = 'mobile' AND revoked_at IS NULL;

-- 2. QR web sign-in: the web shows a code, the signed-in mobile app approves
-- or rejects it, the web exchanges code + its own secret for a panel session.
-- The code is public (it is in the QR and the Centrifugo channel qr:{code});
-- only secret_hash proves the browser that started the challenge.
CREATE TABLE qr_login_challenges (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    code                VARCHAR(64)  NOT NULL,
    secret_hash         CHAR(64)     NOT NULL,
    status              VARCHAR(16)  NOT NULL DEFAULT 'pending',
    -- Domain brand of the web request (K3) and the organization of the
    -- approving mobile session (oid of the issued panel session).
    brand_id            BIGINT       NULL REFERENCES brands (id) ON DELETE SET NULL,
    organization_id     BIGINT       NULL REFERENCES organizations (id) ON DELETE SET NULL,
    web_ip              VARCHAR(64)  NULL,
    web_user_agent      TEXT         NULL,
    user_id             BIGINT       NULL REFERENCES users (id) ON DELETE CASCADE,
    approver_session_id BIGINT       NULL REFERENCES refresh_tokens (id) ON DELETE SET NULL,
    approver_device     VARCHAR(128) NULL,
    expires_at          TIMESTAMPTZ  NOT NULL,
    scanned_at          TIMESTAMPTZ  NULL,
    decided_at          TIMESTAMPTZ  NULL,
    consumed_at         TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_qr_login_challenges_uuid UNIQUE (uuid),
    CONSTRAINT uq_qr_login_challenges_code UNIQUE (code),
    CONSTRAINT chk_qr_login_challenges_status CHECK (
        status IN ('pending', 'scanned', 'approved', 'rejected', 'consumed')
    ),
    CONSTRAINT chk_qr_login_challenges_user CHECK (
        status NOT IN ('approved', 'consumed') OR user_id IS NOT NULL
    )
);

CREATE INDEX idx_qr_login_challenges_expires ON qr_login_challenges (expires_at);
