CREATE TABLE notification_templates (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID         NOT NULL DEFAULT gen_random_uuid(),
    code       VARCHAR(64)  NOT NULL,
    channel    VARCHAR(32)  NOT NULL,
    language   VARCHAR(8)   NOT NULL DEFAULT 'tr',
    subject    VARCHAR(512) NOT NULL DEFAULT '',
    body       TEXT         NOT NULL,
    active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_templates_uuid_uq UNIQUE (uuid),
    CONSTRAINT notification_templates_code_channel_lang_uq UNIQUE (code, channel, language),
    CONSTRAINT notification_templates_channel_chk CHECK (channel IN (
        'inapp', 'email', 'realtime', 'sms', 'push'
    ))
);

CREATE INDEX notification_templates_code_idx ON notification_templates (code);
CREATE INDEX notification_templates_active_idx
    ON notification_templates (code, channel, language)
    WHERE active = TRUE;

CREATE TRIGGER trg_notification_templates_set_updated_at
    BEFORE UPDATE ON notification_templates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE notifications (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id            BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    channel            VARCHAR(32)  NOT NULL,
    status             VARCHAR(32)  NOT NULL DEFAULT 'queued',
    priority           VARCHAR(16)  NOT NULL DEFAULT 'normal',
    title              VARCHAR(512) NOT NULL DEFAULT '',
    body               TEXT         NOT NULL DEFAULT '',
    payload            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    action_url         TEXT         NULL,
    recipient          VARCHAR(512) NULL,
    template_code      VARCHAR(64)  NULL,
    source_event       VARCHAR(128) NULL,
    scheduled_at       TIMESTAMPTZ  NULL,
    sent_at            TIMESTAMPTZ  NULL,
    delivered_at       TIMESTAMPTZ  NULL,
    read_at            TIMESTAMPTZ  NULL,
    failed_at          TIMESTAMPTZ  NULL,
    cancelled_at       TIMESTAMPTZ  NULL,
    attempt_count      INTEGER      NOT NULL DEFAULT 0,
    max_attempts       INTEGER      NOT NULL DEFAULT 5,
    last_error         TEXT         NULL,
    provider           VARCHAR(64)  NULL,
    provider_reference VARCHAR(255) NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT notifications_uuid_uq UNIQUE (uuid),
    CONSTRAINT notifications_channel_chk CHECK (channel IN (
        'inapp', 'email', 'realtime', 'sms', 'push'
    )),
    CONSTRAINT notifications_status_chk CHECK (status IN (
        'queued', 'processing', 'sent', 'delivered', 'read', 'failed', 'cancelled'
    )),
    CONSTRAINT notifications_priority_chk CHECK (priority IN (
        'low', 'normal', 'high', 'critical'
    )),
    CONSTRAINT notifications_attempt_chk CHECK (attempt_count >= 0 AND max_attempts >= 1)
);

CREATE INDEX notifications_user_status_idx ON notifications (user_id, status)
    WHERE user_id IS NOT NULL;
CREATE INDEX notifications_channel_status_idx ON notifications (channel, status);
CREATE INDEX notifications_created_at_idx ON notifications (created_at DESC);

CREATE TRIGGER trg_notifications_set_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_history (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    notification_id BIGINT      NOT NULL REFERENCES notifications (id) ON DELETE CASCADE,
    event           VARCHAR(64) NOT NULL,
    metadata        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_history_uuid_uq UNIQUE (uuid)
);

CREATE INDEX notification_history_notification_id_idx
    ON notification_history (notification_id, created_at);

CREATE TABLE notification_preferences (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    inapp_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    realtime_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    push_enabled     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_preferences_uuid_uq UNIQUE (uuid),
    CONSTRAINT notification_preferences_user_uq UNIQUE (user_id)
);

CREATE TRIGGER trg_notification_preferences_set_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('auth.welcome', 'inapp', 'tr', 'App''e hoş geldiniz',
     'Merhaba {{name}}, hesabınız oluşturuldu.', TRUE),
    ('auth.welcome', 'email', 'tr', 'App''e hoş geldiniz',
     'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.', TRUE),
    ('auth.password_reset', 'email', 'tr', 'Şifre sıfırlama kodu',
     'Merhaba {{name}}, şifre sıfırlama kodunuz: {{code}}. Geçerlilik: {{expires_minutes}} dakika.', TRUE),
    ('auth.email_verification', 'email', 'tr', 'E-posta doğrulama kodu',
     'Merhaba {{name}}, doğrulama kodunuz: {{code}}. Geçerlilik: {{expires_minutes}} dakika.', TRUE),
    ('auth.profile_updated', 'inapp', 'tr', 'Profil güncellendi',
     'Profil bilgileriniz güncellendi.', TRUE),
    ('notifications.test', 'inapp', 'tr', 'Test bildirimi',
     'Bu bir test bildirimidir.', TRUE),
    ('notifications.test', 'email', 'tr', 'Test bildirimi',
     'Bu bir test e-postasıdır.', TRUE),
    ('auth.welcome', 'inapp', 'en', 'Welcome to App',
     'Hello {{name}}, your account has been created.', TRUE),
    ('auth.welcome', 'email', 'en', 'Welcome to App',
     'Hello {{name}}, your App account was created successfully.', TRUE),
    ('auth.password_reset', 'email', 'en', 'Password reset code',
     'Hello {{name}}, your password reset code is: {{code}}. Valid for {{expires_minutes}} minutes.', TRUE),
    ('auth.email_verification', 'email', 'en', 'Email verification code',
     'Hello {{name}}, your verification code is: {{code}}. Valid for {{expires_minutes}} minutes.', TRUE),
    ('auth.profile_updated', 'inapp', 'en', 'Profile updated',
     'Your profile information was updated.', TRUE),
    ('notifications.test', 'inapp', 'en', 'Test notification',
     'This is a test notification.', TRUE),
    ('notifications.test', 'email', 'en', 'Test notification',
     'This is a test email.', TRUE);
