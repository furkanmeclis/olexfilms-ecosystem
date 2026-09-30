-- name: CreateNotification :one
INSERT INTO notifications (
    user_id, channel, status, priority,
    title, body, payload, action_url, recipient, template_code, source_event,
    scheduled_at, max_attempts
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9, $10, $11,
    $12, $13
)
RETURNING *;

-- name: GetNotificationByUUID :one
SELECT * FROM notifications WHERE uuid = $1;

-- name: GetNotificationByID :one
SELECT * FROM notifications WHERE id = $1;

-- name: ListNotificationsForUser :many
SELECT * FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (
    sqlc.narg(unread)::bool IS NULL
    OR (sqlc.narg(unread)::bool = TRUE AND read_at IS NULL AND channel = 'inapp')
    OR (sqlc.narg(unread)::bool = FALSE AND (read_at IS NOT NULL OR channel <> 'inapp'))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(template_code, '') ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(recipient, '') ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountNotificationsForUser :one
SELECT COUNT(*)::bigint FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (
    sqlc.narg(unread)::bool IS NULL
    OR (sqlc.narg(unread)::bool = TRUE AND read_at IS NULL AND channel = 'inapp')
    OR (sqlc.narg(unread)::bool = FALSE AND (read_at IS NOT NULL OR channel <> 'inapp'))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(template_code, '') ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(recipient, '') ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: CountUnreadInappForUser :one
SELECT COUNT(*)::bigint FROM notifications
WHERE user_id = $1
  AND channel = 'inapp'
  AND read_at IS NULL
  AND status NOT IN ('cancelled', 'failed');

-- name: ListPlatformNotifications :many
SELECT * FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPlatformNotifications :one
SELECT COUNT(*)::bigint FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListPlatformNotificationsForExport :many
SELECT * FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC;

-- name: MarkNotificationProcessing :one
UPDATE notifications
SET status = 'processing',
    attempt_count = attempt_count + 1
WHERE id = $1
  AND status IN ('queued', 'failed')
RETURNING *;

-- name: ListStuckProcessingNotificationIDs :many
SELECT id FROM notifications
WHERE status = 'processing'
  AND updated_at < NOW() - make_interval(mins => sqlc.arg(stale_minutes)::int)
ORDER BY id
LIMIT 500;

-- name: MarkNotificationSent :one
-- Cast status on every use. Reusing an untyped param as both VARCHAR assignment
-- and IN ('delivered', 'read') makes Postgres raise 42P08
-- (inconsistent types deduced for parameter $2).
UPDATE notifications
SET status = sqlc.arg(status)::varchar,
    sent_at = COALESCE(sent_at, NOW()),
    delivered_at = CASE
        WHEN sqlc.arg(status)::varchar IN ('delivered', 'read') THEN COALESCE(delivered_at, NOW())
        ELSE delivered_at
    END,
    provider = sqlc.arg(provider),
    provider_reference = sqlc.arg(provider_reference),
    last_error = NULL
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkNotificationFailed :one
UPDATE notifications
SET status = 'failed',
    failed_at = NOW(),
    last_error = $2
WHERE id = $1
RETURNING *;

-- name: MarkNotificationRead :one
UPDATE notifications
SET status = 'read',
    read_at = COALESCE(read_at, NOW())
WHERE uuid = $1
  AND user_id = $2
  AND channel = 'inapp'
RETURNING *;

-- name: MarkAllNotificationsReadForUser :execrows
UPDATE notifications
SET status = 'read',
    read_at = COALESCE(read_at, NOW())
WHERE user_id = $1
  AND channel = 'inapp'
  AND read_at IS NULL;

-- name: InsertNotificationHistory :one
INSERT INTO notification_history (notification_id, event, metadata)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTemplateByCodeChannelLang :one
SELECT * FROM notification_templates
WHERE code = $1
  AND channel = $2
  AND language = $3
  AND active = TRUE;

-- name: GetNotificationPreferences :one
SELECT * FROM notification_preferences
WHERE user_id = $1;

-- name: UpsertNotificationPreferences :one
INSERT INTO notification_preferences (user_id, email_enabled, inapp_enabled, realtime_enabled, push_enabled)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE
SET email_enabled = EXCLUDED.email_enabled,
    inapp_enabled = EXCLUDED.inapp_enabled,
    realtime_enabled = EXCLUDED.realtime_enabled,
    push_enabled = EXCLUDED.push_enabled
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET name = COALESCE(sqlc.narg(name), name),
    surname = COALESCE(sqlc.narg(surname), surname)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetUserEmailVerified :one
UPDATE users
SET email_verified_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (user_id, endpoint, key_p256dh, key_auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, endpoint) DO UPDATE SET
  key_p256dh = EXCLUDED.key_p256dh,
  key_auth = EXCLUDED.key_auth,
  updated_at = now()
RETURNING *;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions
WHERE user_id = $1 AND endpoint = $2;

-- name: ListPushSubscriptionsByUser :many
SELECT * FROM push_subscriptions WHERE user_id = $1;
