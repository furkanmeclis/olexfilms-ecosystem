-- Notification center (TEC-87): recipients, templates, preferences,
-- deliveries, channel switches, event catalog mirror, Expo push tokens.

-- name: GetNotificationRecipient :one
-- Everything Dispatch needs about one recipient: addresses, stored locale
-- sources (user -> organization -> brand center) and the template role.
-- The organization is the user's membership (the event organization first),
-- else the event organization itself (customers of a dealer).
SELECT
    u.id,
    u.uuid,
    COALESCE(u.email, '')::text      AS email,
    COALESCE(u.phone_e164, '')::text AS phone_e164,
    COALESCE(u.locale, '')::text     AS user_locale,
    COALESCE(u.timezone, '')::text   AS user_timezone,
    COALESCE(m.type, '')::text       AS member_org_type,
    COALESCE(m.locale, e.locale, '')::text AS org_locale,
    COALESCE(c.locale, '')::text     AS center_locale,
    COALESCE(m.brand_id, e.brand_id, sqlc.narg(brand_id)::bigint, 0)::bigint AS brand_id,
    EXISTS (
        SELECT 1 FROM user_roles ur
        JOIN roles r ON r.id = ur.role_id
        WHERE ur.user_id = u.id AND (r.slug = 'super_admin' OR r.slug LIKE 'center\_%')
    ) AS platform_staff
FROM users u
LEFT JOIN LATERAL (
    SELECT o.type, o.locale, o.brand_id
    FROM organization_members om
    JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
    WHERE om.user_id = u.id
    ORDER BY (o.id = sqlc.narg(organization_id)::bigint) DESC NULLS LAST,
             CASE o.type WHEN 'center' THEN 0 WHEN 'distributor' THEN 1 ELSE 2 END,
             o.id
    LIMIT 1
) m ON TRUE
LEFT JOIN organizations e
    ON e.id = sqlc.narg(organization_id)::bigint AND e.deleted_at IS NULL
LEFT JOIN organizations c
    ON c.type = 'center'
   AND c.deleted_at IS NULL
   AND c.brand_id = COALESCE(m.brand_id, e.brand_id, sqlc.narg(brand_id)::bigint)
WHERE u.id = sqlc.arg(user_id) AND u.deleted_at IS NULL;

-- name: ListActiveTemplatesForEvent :many
-- Candidates for one event x channel; the usecase picks role/language/brand.
SELECT * FROM notification_templates
WHERE code = sqlc.arg(code)
  AND channel = sqlc.arg(channel)
  AND active = TRUE
  AND (brand_id IS NULL OR brand_id = sqlc.narg(brand_id)::bigint);

-- name: ListNotificationTemplates :many
SELECT * FROM notification_templates
WHERE (sqlc.narg(code)::text IS NULL OR code = sqlc.narg(code))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(language)::text IS NULL OR language = sqlc.narg(language))
  AND (sqlc.narg(role)::text IS NULL OR role = sqlc.narg(role))
ORDER BY code, role, channel, language, brand_id NULLS FIRST;

-- name: UpsertNotificationTemplate :one
INSERT INTO notification_templates (
    code, role, channel, language, brand_id, subject, body, format, active, updated_by_user_id
) VALUES (
    sqlc.arg(code), sqlc.arg(role), sqlc.arg(channel), sqlc.arg(language), sqlc.narg(brand_id),
    sqlc.arg(subject), sqlc.arg(body), sqlc.arg(format), sqlc.arg(active), sqlc.narg(updated_by_user_id)
)
ON CONFLICT ON CONSTRAINT notification_templates_key_uq DO UPDATE SET
    subject = EXCLUDED.subject,
    body = EXCLUDED.body,
    format = EXCLUDED.format,
    active = EXCLUDED.active,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- Default templates of code-registered events (TEC-187): inserted once,
-- an existing row (admin edit) is never overwritten.
-- name: InsertNotificationTemplateIfMissing :execrows
INSERT INTO notification_templates (code, role, channel, language, subject, body, format, active)
VALUES (
    sqlc.arg(code), sqlc.arg(role), sqlc.arg(channel), sqlc.arg(language),
    sqlc.arg(subject), sqlc.arg(body), sqlc.arg(format), TRUE
)
ON CONFLICT ON CONSTRAINT notification_templates_key_uq DO NOTHING;

-- name: ListNotificationPreferenceRows :many
SELECT * FROM notification_preferences
WHERE user_id = $1
ORDER BY event_code NULLS FIRST, channel;

-- name: UpsertNotificationPreferenceRow :one
INSERT INTO notification_preferences (user_id, event_code, channel, enabled)
VALUES (sqlc.arg(user_id), sqlc.narg(event_code), sqlc.arg(channel), sqlc.arg(enabled))
ON CONFLICT ON CONSTRAINT notification_prefs_rule_uq DO UPDATE SET enabled = EXCLUDED.enabled
RETURNING *;

-- name: DeleteNotificationPreferenceRow :exec
DELETE FROM notification_preferences
WHERE user_id = sqlc.arg(user_id)
  AND event_code IS NOT DISTINCT FROM sqlc.narg(event_code)::text
  AND channel = sqlc.arg(channel);

-- name: ListNotificationChannelSettings :many
SELECT * FROM notification_channel_settings ORDER BY channel;

-- name: SetNotificationChannelEnabled :one
UPDATE notification_channel_settings
SET enabled = sqlc.arg(enabled), updated_by_user_id = sqlc.narg(updated_by_user_id), updated_at = NOW()
WHERE channel = sqlc.arg(channel)
RETURNING *;

-- name: InsertNotificationDelivery :one
-- Idempotent on (event_id, user_id, channel): a replayed event returns no row.
INSERT INTO notification_deliveries (
    event_id, event_code, user_id, organization_id, brand_id, channel,
    role, language, template_id, status, error
) VALUES (
    sqlc.arg(event_id), sqlc.arg(event_code), sqlc.arg(user_id), sqlc.narg(organization_id),
    sqlc.narg(brand_id), sqlc.arg(channel), sqlc.narg(role), sqlc.narg(language),
    sqlc.narg(template_id), sqlc.arg(status), sqlc.narg(error)
)
ON CONFLICT ON CONSTRAINT notification_deliveries_idem_uq DO NOTHING
RETURNING *;

-- name: AttachNotificationDelivery :exec
UPDATE notifications SET delivery_id = sqlc.arg(delivery_id) WHERE id = sqlc.arg(id);

-- name: MarkDeliveryProcessing :exec
UPDATE notification_deliveries
SET status = 'processing', attempts = attempts + 1
WHERE id = $1;

-- name: MarkDeliveryResult :exec
UPDATE notification_deliveries
SET status = sqlc.arg(status),
    provider = sqlc.narg(provider),
    provider_ref = sqlc.narg(provider_ref),
    error = sqlc.narg(error)
WHERE id = sqlc.arg(id);

-- name: ListNotificationDeliveries :many
SELECT d.*, u.uuid AS user_uuid, COALESCE(u.email, '')::text AS user_email
FROM notification_deliveries d
JOIN users u ON u.id = d.user_id
WHERE (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR d.channel = sqlc.narg(channel))
  AND (sqlc.narg(event_code)::text IS NULL OR d.event_code = sqlc.narg(event_code))
  AND (sqlc.narg(event_id)::uuid IS NULL OR d.event_id = sqlc.narg(event_id))
  AND (sqlc.narg(user_id)::bigint IS NULL OR d.user_id = sqlc.narg(user_id))
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountNotificationDeliveries :one
SELECT COUNT(*)::bigint
FROM notification_deliveries d
WHERE (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR d.channel = sqlc.narg(channel))
  AND (sqlc.narg(event_code)::text IS NULL OR d.event_code = sqlc.narg(event_code))
  AND (sqlc.narg(event_id)::uuid IS NULL OR d.event_id = sqlc.narg(event_id))
  AND (sqlc.narg(user_id)::bigint IS NULL OR d.user_id = sqlc.narg(user_id));

-- name: PurgeNotificationDeliveriesBefore :execrows
DELETE FROM notification_deliveries
WHERE id IN (
    SELECT old.id FROM notification_deliveries old
    WHERE old.created_at < sqlc.arg(cutoff)::timestamptz
    ORDER BY old.id
    LIMIT sqlc.arg(batch_size)::int
);

-- name: PurgeNotificationsBefore :execrows
DELETE FROM notifications
WHERE id IN (
    SELECT old.id FROM notifications old
    WHERE old.created_at < sqlc.arg(cutoff)::timestamptz
    ORDER BY old.id
    LIMIT sqlc.arg(batch_size)::int
);

-- name: UpsertNotificationEvent :exec
INSERT INTO notification_events (
    code, module, default_channels, critical, audience_roles, placeholders, user_configurable
) VALUES (
    sqlc.arg(code), sqlc.arg(module), sqlc.arg(default_channels), sqlc.arg(critical),
    sqlc.arg(audience_roles), sqlc.arg(placeholders), sqlc.arg(user_configurable)
)
ON CONFLICT (code) DO UPDATE SET
    module = EXCLUDED.module,
    default_channels = EXCLUDED.default_channels,
    critical = EXCLUDED.critical,
    audience_roles = EXCLUDED.audience_roles,
    placeholders = EXCLUDED.placeholders,
    user_configurable = EXCLUDED.user_configurable;

-- name: UpsertDevicePushToken :one
INSERT INTO device_push_tokens (user_id, device_id, platform, expo_token, app_version, last_seen_at, revoked_at)
VALUES (sqlc.arg(user_id), sqlc.arg(device_id), sqlc.arg(platform), sqlc.arg(expo_token), sqlc.narg(app_version), NOW(), NULL)
ON CONFLICT (expo_token) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    device_id = EXCLUDED.device_id,
    platform = EXCLUDED.platform,
    app_version = EXCLUDED.app_version,
    last_seen_at = NOW(),
    revoked_at = NULL
RETURNING *;

-- name: RevokeDevicePushToken :execrows
UPDATE device_push_tokens
SET revoked_at = NOW()
WHERE expo_token = sqlc.arg(expo_token)
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND revoked_at IS NULL;

-- name: ListActiveDevicePushTokens :many
SELECT * FROM device_push_tokens
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY last_seen_at DESC;

-- name: DeletePushSubscriptionByEndpoint :exec
DELETE FROM push_subscriptions WHERE endpoint = $1;

-- TEC-91: mobile sign-out drops the device's Expo tokens.

-- name: RevokeDevicePushTokensForDevice :execrows
UPDATE device_push_tokens
SET revoked_at = NOW()
WHERE user_id = $1
  AND device_id = $2
  AND revoked_at IS NULL;

-- name: RevokeAllDevicePushTokensForUser :execrows
UPDATE device_push_tokens
SET revoked_at = NOW()
WHERE user_id = $1
  AND revoked_at IS NULL;
