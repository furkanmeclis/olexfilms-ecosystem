-- TEC-87 (F0-10): notification center skeleton.
--   * six channels: inapp, email, webpush, expo_push, sms, whatsapp
--     (realtime folds into inapp as its Centrifugo side effect, push -> webpush)
--   * event catalog mirror, templates per event x role x channel x language
--   * row based preferences, deliveries (idempotency + log), channel switches
--   * Expo push tokens (device_push_tokens, used by TEC-91 too)

-- 1. Channel vocabulary on notifications.
ALTER TABLE notifications DROP CONSTRAINT notifications_channel_chk;
UPDATE notifications SET channel = 'inapp' WHERE channel = 'realtime';
UPDATE notifications SET channel = 'webpush' WHERE channel = 'push';
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_chk CHECK (channel IN (
    'inapp', 'email', 'webpush', 'expo_push', 'sms', 'whatsapp'
));

-- 2. Templates: event x role x channel x language (+ optional brand override).
ALTER TABLE notification_templates DROP CONSTRAINT notification_templates_channel_chk;
DELETE FROM notification_templates t
WHERE t.channel = 'realtime'
  AND EXISTS (
    SELECT 1 FROM notification_templates i
    WHERE i.code = t.code AND i.language = t.language AND i.channel = 'inapp'
  );
UPDATE notification_templates SET channel = 'inapp' WHERE channel = 'realtime';
DELETE FROM notification_templates t
WHERE t.channel = 'push'
  AND EXISTS (
    SELECT 1 FROM notification_templates i
    WHERE i.code = t.code AND i.language = t.language AND i.channel = 'webpush'
  );
UPDATE notification_templates SET channel = 'webpush' WHERE channel = 'push';

ALTER TABLE notification_templates DROP CONSTRAINT notification_templates_code_channel_lang_uq;
DROP INDEX IF EXISTS notification_templates_active_idx;

ALTER TABLE notification_templates
    ADD COLUMN role               VARCHAR(16) NOT NULL DEFAULT 'generic',
    ADD COLUMN brand_id           BIGINT      NULL REFERENCES brands (id) ON DELETE CASCADE,
    ADD COLUMN format             VARCHAR(16) NOT NULL DEFAULT 'markdown',
    ADD COLUMN updated_by_user_id BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT notification_templates_channel_chk CHECK (channel IN (
        'inapp', 'email', 'webpush', 'expo_push', 'sms', 'whatsapp'
    )),
    ADD CONSTRAINT notification_templates_role_chk CHECK (role IN (
        'generic', 'customer', 'dealer', 'distributor', 'center'
    )),
    ADD CONSTRAINT notification_templates_format_chk CHECK (format IN ('markdown', 'text')),
    ADD CONSTRAINT notification_templates_language_chk CHECK (language IN (
        'tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar'
    )),
    ADD CONSTRAINT notification_templates_key_uq
        UNIQUE NULLS NOT DISTINCT (code, role, channel, language, brand_id);

CREATE INDEX notification_templates_active_idx
    ON notification_templates (code, channel)
    WHERE active = TRUE;

-- 3. Event catalog mirror (Go catalog is the source; synced at start-up).
CREATE TABLE notification_events (
    code              VARCHAR(64) PRIMARY KEY,
    module            VARCHAR(64) NOT NULL,
    default_channels  TEXT[]      NOT NULL DEFAULT '{}',
    critical          BOOLEAN     NOT NULL DEFAULT FALSE,
    audience_roles    TEXT[]      NOT NULL DEFAULT '{}',
    placeholders      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    user_configurable BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_notification_events_set_updated_at
    BEFORE UPDATE ON notification_events
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 4. Deliveries: one row per event x user x channel (idempotency key) and the
-- delivery log. Swept after 90 days.
CREATE TABLE notification_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    event_id        UUID         NOT NULL,
    event_code      VARCHAR(64)  NOT NULL,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id BIGINT       NULL REFERENCES organizations (id) ON DELETE SET NULL,
    brand_id        BIGINT       NULL REFERENCES brands (id) ON DELETE SET NULL,
    channel         VARCHAR(32)  NOT NULL,
    role            VARCHAR(16)  NULL,
    language        VARCHAR(8)   NULL,
    template_id     BIGINT       NULL REFERENCES notification_templates (id) ON DELETE SET NULL,
    status          VARCHAR(32)  NOT NULL DEFAULT 'queued',
    provider        VARCHAR(64)  NULL,
    provider_ref    VARCHAR(255) NULL,
    error           TEXT         NULL,
    attempts        INTEGER      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_deliveries_uuid_uq UNIQUE (uuid),
    CONSTRAINT notification_deliveries_idem_uq UNIQUE (event_id, user_id, channel),
    CONSTRAINT notification_deliveries_channel_chk CHECK (channel IN (
        'inapp', 'email', 'webpush', 'expo_push', 'sms', 'whatsapp'
    )),
    CONSTRAINT notification_deliveries_status_chk CHECK (status IN (
        'queued', 'processing', 'sent', 'delivered', 'failed',
        'skipped_disabled', 'skipped_preference', 'skipped_no_template', 'skipped_no_recipient'
    ))
);

CREATE INDEX notification_deliveries_created_at_idx ON notification_deliveries (created_at);
CREATE INDEX notification_deliveries_user_idx ON notification_deliveries (user_id, created_at DESC);
CREATE INDEX notification_deliveries_org_idx ON notification_deliveries (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;
CREATE INDEX notification_deliveries_event_idx ON notification_deliveries (event_code, created_at DESC);

CREATE TRIGGER trg_notification_deliveries_set_updated_at
    BEFORE UPDATE ON notification_deliveries
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Notifications carry tenant + event context.
ALTER TABLE notifications
    ADD COLUMN organization_id BIGINT     NULL REFERENCES organizations (id) ON DELETE SET NULL,
    ADD COLUMN brand_id        BIGINT     NULL REFERENCES brands (id) ON DELETE SET NULL,
    ADD COLUMN event_id        UUID       NULL,
    ADD COLUMN language        VARCHAR(8) NULL,
    ADD COLUMN delivery_id     BIGINT     NULL REFERENCES notification_deliveries (id) ON DELETE SET NULL;

CREATE INDEX notifications_org_user_idx ON notifications (organization_id, user_id)
    WHERE organization_id IS NOT NULL;
CREATE INDEX notifications_delivery_idx ON notifications (delivery_id) WHERE delivery_id IS NOT NULL;

-- 6. Row based preferences: (user, event NULL = every event, channel) -> enabled.
CREATE TABLE notification_preferences_rows (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_code VARCHAR(64) NULL,
    channel    VARCHAR(32) NOT NULL,
    enabled    BOOLEAN     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_prefs_rule_uq UNIQUE NULLS NOT DISTINCT (user_id, event_code, channel),
    CONSTRAINT notification_prefs_channel_chk CHECK (channel IN (
        'inapp', 'email', 'webpush', 'expo_push', 'sms', 'whatsapp'
    ))
);

-- Bool columns -> global rows. realtime_enabled has no channel of its own any
-- more (Centrifugo is part of inapp) and is dropped; push -> webpush.
INSERT INTO notification_preferences_rows (user_id, event_code, channel, enabled, created_at, updated_at)
SELECT user_id, NULL, 'email', email_enabled, created_at, updated_at FROM notification_preferences
UNION ALL
SELECT user_id, NULL, 'inapp', inapp_enabled, created_at, updated_at FROM notification_preferences
UNION ALL
SELECT user_id, NULL, 'webpush', push_enabled, created_at, updated_at FROM notification_preferences;

DROP TABLE notification_preferences;
ALTER TABLE notification_preferences_rows RENAME TO notification_preferences;
ALTER SEQUENCE notification_preferences_rows_id_seq RENAME TO notification_preferences_id_seq;
ALTER INDEX notification_preferences_rows_pkey RENAME TO notification_preferences_pkey;

CREATE TRIGGER trg_notification_preferences_set_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 7. Admin channel switches. SMS is off until an admin opens it (K21).
CREATE TABLE notification_channel_settings (
    channel            VARCHAR(32) PRIMARY KEY,
    enabled            BOOLEAN     NOT NULL,
    updated_by_user_id BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_channel_settings_channel_chk CHECK (channel IN (
        'inapp', 'email', 'webpush', 'expo_push', 'sms', 'whatsapp'
    ))
);

INSERT INTO notification_channel_settings (channel, enabled) VALUES
    ('inapp', TRUE),
    ('email', TRUE),
    ('webpush', TRUE),
    ('expo_push', TRUE),
    ('sms', FALSE),
    ('whatsapp', TRUE);

-- 8. Expo push tokens (mobile). TEC-91 registers devices into this table.
CREATE TABLE device_push_tokens (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id      BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id    VARCHAR(128) NOT NULL,
    platform     VARCHAR(16)  NOT NULL,
    expo_token   VARCHAR(255) NOT NULL,
    app_version  VARCHAR(32)  NULL,
    last_seen_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    revoked_at   TIMESTAMPTZ  NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT device_push_tokens_uuid_uq UNIQUE (uuid),
    CONSTRAINT device_push_tokens_expo_token_uq UNIQUE (expo_token),
    CONSTRAINT device_push_tokens_platform_chk CHECK (platform IN ('ios', 'android'))
);

CREATE INDEX device_push_tokens_user_idx ON device_push_tokens (user_id) WHERE revoked_at IS NULL;

CREATE TRIGGER trg_device_push_tokens_set_updated_at
    BEFORE UPDATE ON device_push_tokens
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 9. Default templates (generic role; tr + en + ar). Role or brand specific
-- rows are added from the admin editor.
INSERT INTO notification_templates (code, role, channel, language, subject, body, format) VALUES
    ('notifications.test', 'generic', 'inapp', 'ar', 'إشعار تجريبي', 'هذا إشعار تجريبي.', 'text'),
    ('notifications.test', 'generic', 'email', 'ar', 'إشعار تجريبي', 'هذه رسالة بريد إلكتروني تجريبية.', 'markdown'),
    ('notifications.test', 'generic', 'sms', 'tr', 'Test bildirimi', 'Bu bir test SMS mesajıdır.', 'text'),
    ('notifications.test', 'generic', 'sms', 'en', 'Test notification', 'This is a test SMS.', 'text'),
    ('notifications.test', 'generic', 'whatsapp', 'tr', 'Test bildirimi', 'Bu bir test WhatsApp mesajıdır.', 'text'),
    ('notifications.test', 'generic', 'whatsapp', 'en', 'Test notification', 'This is a test WhatsApp message.', 'text'),
    ('notifications.test', 'generic', 'webpush', 'tr', 'Test bildirimi', 'Bu bir test bildirimidir.', 'text'),
    ('notifications.test', 'generic', 'webpush', 'en', 'Test notification', 'This is a test notification.', 'text'),
    ('notifications.test', 'generic', 'expo_push', 'tr', 'Test bildirimi', 'Bu bir test bildirimidir.', 'text'),
    ('notifications.test', 'generic', 'expo_push', 'en', 'Test notification', 'This is a test notification.', 'text'),

    ('features.module_requested', 'generic', 'inapp', 'tr', 'Modül talebi',
     '{{organization_name}}, {{module_key}} modülünü talep etti. {{note}}', 'text'),
    ('features.module_requested', 'generic', 'inapp', 'en', 'Module request',
     '{{organization_name}} requested the module {{module_key}}. {{note}}', 'text'),
    ('features.module_requested', 'generic', 'inapp', 'ar', 'طلب وحدة',
     'طلبت {{organization_name}} الوحدة {{module_key}}. {{note}}', 'text'),
    ('features.module_requested', 'generic', 'email', 'tr', 'Modül talebi: {{module_key}}',
     '**{{organization_name}}**, **{{module_key}}** modülünü talep etti.

{{note}}', 'markdown'),
    ('features.module_requested', 'generic', 'email', 'en', 'Module request: {{module_key}}',
     '**{{organization_name}}** requested the module **{{module_key}}**.

{{note}}', 'markdown'),
    ('features.module_requested', 'generic', 'email', 'ar', 'طلب وحدة: {{module_key}}',
     'طلبت **{{organization_name}}** الوحدة **{{module_key}}**.

{{note}}', 'markdown'),
    ('features.module_requested', 'center', 'inapp', 'tr', 'Modül talebi (bayi/distribütör)',
     '{{organization_name}}, {{module_key}} modülünün açılmasını istiyor. {{note}}', 'text'),
    ('features.module_requested', 'center', 'inapp', 'en', 'Module request (partner)',
     '{{organization_name}} asks to switch on {{module_key}}. {{note}}', 'text'),

    ('customers.assigned', 'generic', 'inapp', 'tr', 'Müşteri atandı', '{{customer_name}} size atandı.', 'text'),
    ('customers.assigned', 'generic', 'inapp', 'en', 'Customer assigned', '{{customer_name}} was assigned to you.', 'text'),
    ('customers.status_blocked', 'generic', 'inapp', 'tr', 'Müşteri engellendi',
     '{{customer_name}} durumu engellendi olarak güncellendi.', 'text'),
    ('customers.status_blocked', 'generic', 'inapp', 'en', 'Customer blocked',
     'The status of {{customer_name}} was changed to blocked.', 'text'),
    ('conversations.assigned', 'generic', 'inapp', 'tr', 'Konuşma atandı', 'Bir konuşma size atandı.', 'text'),
    ('conversations.assigned', 'generic', 'inapp', 'en', 'Conversation assigned', 'A conversation was assigned to you.', 'text'),
    ('ai.pipeline.escalated', 'generic', 'inapp', 'tr', 'AI ekibe iletti', 'Bir konuşma AI tarafından ekibe iletildi.', 'text'),
    ('ai.pipeline.escalated', 'generic', 'inapp', 'en', 'AI escalation', 'A conversation was escalated to the team by AI.', 'text'),
    ('ai.draft.pending', 'generic', 'inapp', 'tr', 'AI taslağı bekliyor',
     'İncelemeniz için yeni bir AI yanıt taslağı oluşturuldu.', 'text'),
    ('ai.draft.pending', 'generic', 'inapp', 'en', 'AI draft pending',
     'A new AI reply draft is waiting for your review.', 'text');

-- 10. Permissions (catalog entries in internal/platform/rbac), appended after
-- the existing catalog.
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT v.name, v.slug, 'notifications', ARRAY['all']::text[], false, false, v.description,
       (SELECT COALESCE(MAX(sort_order), 0) FROM permissions) + v.step
FROM (VALUES
    ('Manage notification templates', 'notifications.templates.manage',
     'Edit notification templates and switch notification channels (SMS).', 10),
    ('Read notification deliveries', 'notification_deliveries.read',
     'Read the notification delivery log.', 20)
) AS v (name, slug, description, step)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r, permissions p
WHERE r.slug = 'super_admin'
  AND p.slug IN ('notifications.templates.manage', 'notification_deliveries.read')
ON CONFLICT DO NOTHING;
