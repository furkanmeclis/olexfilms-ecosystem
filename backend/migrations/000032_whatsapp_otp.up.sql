-- TEC-92 (F0-11): WhatsApp provider (wuzapi), phone OTP and KVKK notices.

-- 1. Phone identity on users (K11/K29): customers sign in with a WhatsApp
-- phone OTP and may have no e-mail. At least one of e-mail / phone is set.
ALTER TABLE users
    ALTER COLUMN email DROP NOT NULL,
    ADD COLUMN phone_e164        VARCHAR(20) NULL,
    ADD COLUMN phone_verified_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_users_phone_e164 CHECK (phone_e164 IS NULL OR phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    ADD CONSTRAINT chk_users_email_or_phone CHECK (email IS NOT NULL OR phone_e164 IS NOT NULL);

CREATE UNIQUE INDEX uq_users_phone_e164_active
    ON users (phone_e164)
    WHERE deleted_at IS NULL AND phone_e164 IS NOT NULL;

-- 2. otp_codes: phone-keyed codes with delivery evidence (channel,
-- provider_ref, message hash, ip, user agent, KVKK notice version).
ALTER TABLE otp_codes
    ALTER COLUMN email DROP NOT NULL,
    ADD COLUMN phone_e164     VARCHAR(20) NULL,
    ADD COLUMN channel        VARCHAR(16) NULL,
    ADD COLUMN provider_ref   TEXT        NULL,
    ADD COLUMN message_sha256 CHAR(64)    NULL,
    ADD COLUMN ip             VARCHAR(64) NULL,
    ADD COLUMN user_agent     TEXT        NULL,
    ADD COLUMN kvkk_locale    VARCHAR(8)  NULL,
    ADD COLUMN kvkk_version   INTEGER     NULL,
    ADD COLUMN delivered_at   TIMESTAMPTZ NULL,
    ADD COLUMN delivery_error TEXT        NULL,
    DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification', 'customer_login', 'contract_sign')
    ),
    ADD CONSTRAINT chk_otp_codes_channel CHECK (channel IS NULL OR channel IN ('email', 'whatsapp', 'sms')),
    ADD CONSTRAINT chk_otp_codes_email_or_phone CHECK (email IS NOT NULL OR phone_e164 IS NOT NULL);

CREATE INDEX idx_otp_codes_phone_type_active
    ON otp_codes (phone_e164, type)
    WHERE consumed_at IS NULL AND phone_e164 IS NOT NULL;

CREATE INDEX idx_otp_codes_phone_created
    ON otp_codes (phone_e164, created_at DESC)
    WHERE phone_e164 IS NOT NULL;

-- 3. KVKK notice texts (K11): per locale, versioned, admin-editable. Every
-- edit appends a new version; the OTP row records the version it carried.
CREATE TABLE kvkk_notices (
    id         BIGSERIAL   PRIMARY KEY,
    locale     VARCHAR(8)  NOT NULL,
    version    INTEGER     NOT NULL,
    body       TEXT        NOT NULL,
    created_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kvkk_notices_locale_version UNIQUE (locale, version),
    CONSTRAINT chk_kvkk_notices_version CHECK (version >= 1),
    CONSTRAINT chk_kvkk_notices_body CHECK (length(btrim(body)) > 0)
);

INSERT INTO kvkk_notices (locale, version, body) VALUES
    ('tr', 1, 'KVKK: Telefon numaranız yalnızca kimlik doğrulama ve hizmet bildirimleri için 6698 sayılı KVKK kapsamında işlenir. Ayrıntılı aydınlatma metni: olexfilms.app/kvkk'),
    ('en', 1, 'Privacy: your phone number is processed only for verification and service notifications under the Turkish Personal Data Protection Law (KVKK). Full notice: olexfilms.app/kvkk');

-- 4. WhatsApp gateway state (K16: one wuzapi instance, one number).
CREATE TABLE whatsapp_settings (
    id                   SMALLINT     PRIMARY KEY DEFAULT 1,
    provider             VARCHAR(32)  NOT NULL DEFAULT 'wuzapi',
    instance_name        VARCHAR(64)  NOT NULL DEFAULT 'olexfilms',
    instance_id          TEXT         NULL,
    user_token_enc       TEXT         NULL,
    status               VARCHAR(32)  NOT NULL DEFAULT 'unknown',
    jid                  TEXT         NULL,
    phone_e164           VARCHAR(20)  NULL,
    last_seen_at         TIMESTAMPTZ  NULL,
    last_event_at        TIMESTAMPTZ  NULL,
    last_error_reason    TEXT         NULL,
    sms_fallback_enabled BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT whatsapp_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_whatsapp_settings_status CHECK (
        status IN ('unknown', 'disconnected', 'connecting', 'qr', 'connected', 'logged_out', 'banned')
    )
);

INSERT INTO whatsapp_settings (id) VALUES (1);

CREATE TRIGGER trg_whatsapp_settings_set_updated_at
    BEFORE UPDATE ON whatsapp_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Connection history (alarms: logged out, temporary ban, disconnects).
CREATE TABLE whatsapp_connection_events (
    id         BIGSERIAL   PRIMARY KEY,
    type       VARCHAR(32) NOT NULL,
    reason     TEXT        NULL,
    alarm      BOOLEAN     NOT NULL DEFAULT FALSE,
    raw        JSONB       NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_whatsapp_connection_events_created ON whatsapp_connection_events (created_at DESC);

-- 5. Conversations and messages (inbound webhook; F4 AI pipeline reads them).
-- A customer is global (K11), so organization/brand are optional here and
-- set when a conversation is attributed to a dealer later.
CREATE TABLE conversations (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NULL REFERENCES organizations (id) ON DELETE SET NULL,
    brand_id        BIGINT      NULL REFERENCES brands (id) ON DELETE SET NULL,
    channel         VARCHAR(16) NOT NULL,
    contact_e164    VARCHAR(20) NOT NULL,
    contact_name    TEXT        NULL,
    user_id         BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    last_message_at TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_conversations_uuid UNIQUE (uuid),
    CONSTRAINT uq_conversations_channel_contact UNIQUE (channel, contact_e164),
    CONSTRAINT chk_conversations_channel CHECK (channel IN ('whatsapp', 'sms'))
);

CREATE TRIGGER trg_conversations_set_updated_at
    BEFORE UPDATE ON conversations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE messages (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    conversation_id BIGINT      NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    organization_id BIGINT      NULL REFERENCES organizations (id) ON DELETE SET NULL,
    brand_id        BIGINT      NULL REFERENCES brands (id) ON DELETE SET NULL,
    channel         VARCHAR(16) NOT NULL,
    direction       VARCHAR(8)  NOT NULL,
    sender_type     VARCHAR(16) NOT NULL DEFAULT 'contact',
    external_id     TEXT        NOT NULL,
    body            TEXT        NULL,
    media           JSONB       NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'received',
    raw             JSONB       NULL,
    sent_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_messages_uuid UNIQUE (uuid),
    CONSTRAINT uq_messages_channel_external UNIQUE (channel, external_id),
    CONSTRAINT chk_messages_channel CHECK (channel IN ('whatsapp', 'sms')),
    CONSTRAINT chk_messages_direction CHECK (direction IN ('in', 'out')),
    CONSTRAINT chk_messages_sender_type CHECK (sender_type IN ('contact', 'staff', 'system', 'ai')),
    CONSTRAINT chk_messages_status CHECK (status IN ('received', 'sent', 'delivered', 'read', 'failed'))
);

CREATE INDEX idx_messages_conversation_created ON messages (conversation_id, created_at DESC);

-- 6. Permission: whatsapp.manage (catalog entry in internal/platform/rbac).
-- Appended after the existing catalog so its sort order follows whatever the
-- earlier migrations seeded.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage WhatsApp integration', 'whatsapp.manage', 'whatsapp', ARRAY['all']::text[], false, false,
       'Connect the WhatsApp number, send test messages, edit OTP/KVKK texts.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r, permissions p
WHERE r.slug = 'super_admin' AND p.slug = 'whatsapp.manage'
ON CONFLICT DO NOTHING;
