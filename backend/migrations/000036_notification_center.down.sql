-- Reverts 000036. Rows on channels the old vocabulary lacks (expo_push,
-- whatsapp) are dropped; webpush goes back to push.

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.slug IN ('notifications.templates.manage', 'notification_deliveries.read');
DELETE FROM permissions WHERE slug IN ('notifications.templates.manage', 'notification_deliveries.read');

DROP TABLE IF EXISTS device_push_tokens;
DROP TABLE IF EXISTS notification_channel_settings;

-- Preferences: global rows -> bool columns (missing row = old default).
CREATE TABLE notification_preferences_old (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    inapp_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    realtime_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    push_enabled     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO notification_preferences_old (user_id, email_enabled, inapp_enabled, realtime_enabled, push_enabled, created_at, updated_at)
SELECT user_id,
       COALESCE(bool_and(enabled) FILTER (WHERE channel = 'email'), TRUE),
       COALESCE(bool_and(enabled) FILTER (WHERE channel = 'inapp'), TRUE),
       COALESCE(bool_and(enabled) FILTER (WHERE channel = 'inapp'), TRUE),
       COALESCE(bool_and(enabled) FILTER (WHERE channel = 'webpush'), FALSE),
       MIN(created_at), MAX(updated_at)
FROM notification_preferences
WHERE event_code IS NULL
GROUP BY user_id;

DROP TABLE notification_preferences;
ALTER TABLE notification_preferences_old RENAME TO notification_preferences;
ALTER SEQUENCE notification_preferences_old_id_seq RENAME TO notification_preferences_id_seq;
ALTER INDEX notification_preferences_old_pkey RENAME TO notification_preferences_pkey;
ALTER TABLE notification_preferences
    ADD CONSTRAINT notification_preferences_uuid_uq UNIQUE (uuid),
    ADD CONSTRAINT notification_preferences_user_uq UNIQUE (user_id);

CREATE TRIGGER trg_notification_preferences_set_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Notifications.
DROP INDEX IF EXISTS notifications_delivery_idx;
DROP INDEX IF EXISTS notifications_org_user_idx;
ALTER TABLE notifications
    DROP COLUMN delivery_id,
    DROP COLUMN language,
    DROP COLUMN event_id,
    DROP COLUMN brand_id,
    DROP COLUMN organization_id;

DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_events;

ALTER TABLE notifications DROP CONSTRAINT notifications_channel_chk;
DELETE FROM notifications WHERE channel IN ('expo_push', 'whatsapp');
UPDATE notifications SET channel = 'push' WHERE channel = 'webpush';
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_chk CHECK (channel IN (
    'inapp', 'email', 'realtime', 'sms', 'push'
));

-- Templates: keep only the generic, brand-less rows of the old channels.
DROP INDEX IF EXISTS notification_templates_active_idx;
ALTER TABLE notification_templates
    DROP CONSTRAINT notification_templates_key_uq,
    DROP CONSTRAINT notification_templates_language_chk,
    DROP CONSTRAINT notification_templates_format_chk,
    DROP CONSTRAINT notification_templates_role_chk,
    DROP CONSTRAINT notification_templates_channel_chk;
DELETE FROM notification_templates
WHERE role <> 'generic'
   OR brand_id IS NOT NULL
   OR channel IN ('expo_push', 'whatsapp')
   OR language NOT IN ('tr', 'en')
   OR (code, channel, language) IN (
       ('notifications.test', 'sms', 'tr'), ('notifications.test', 'sms', 'en'),
       ('notifications.test', 'webpush', 'tr'), ('notifications.test', 'webpush', 'en')
   )
   OR code = 'features.module_requested'
   OR code IN ('customers.assigned', 'customers.status_blocked', 'conversations.assigned',
               'ai.pipeline.escalated', 'ai.draft.pending');
UPDATE notification_templates SET channel = 'push' WHERE channel = 'webpush';
ALTER TABLE notification_templates
    DROP COLUMN updated_by_user_id,
    DROP COLUMN format,
    DROP COLUMN brand_id,
    DROP COLUMN role,
    ADD CONSTRAINT notification_templates_code_channel_lang_uq UNIQUE (code, channel, language),
    ADD CONSTRAINT notification_templates_channel_chk CHECK (channel IN (
        'inapp', 'email', 'realtime', 'sms', 'push'
    ));
CREATE INDEX notification_templates_active_idx
    ON notification_templates (code, channel, language)
    WHERE active = TRUE;
