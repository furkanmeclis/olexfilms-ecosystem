-- WhatsApp gateway, KVKK notices, conversations and messages (TEC-92).

-- name: GetWhatsAppSettings :one
SELECT * FROM whatsapp_settings WHERE id = 1;

-- name: SetWhatsAppInstance :one
UPDATE whatsapp_settings
SET instance_name = sqlc.arg(instance_name),
    instance_id = sqlc.narg(instance_id),
    user_token_enc = sqlc.narg(user_token_enc)
WHERE id = 1
RETURNING *;

-- name: UpdateWhatsAppStatus :one
UPDATE whatsapp_settings
SET status = sqlc.arg(status)::text,
    jid = COALESCE(sqlc.narg(jid), jid),
    phone_e164 = COALESCE(sqlc.narg(phone_e164), phone_e164),
    last_seen_at = CASE WHEN sqlc.arg(status)::text = 'connected' THEN sqlc.arg(at)::timestamptz ELSE last_seen_at END,
    last_event_at = sqlc.arg(at)::timestamptz,
    last_error_reason = sqlc.narg(last_error_reason)
WHERE id = 1
RETURNING *;

-- name: SetWhatsAppSMSFallback :one
UPDATE whatsapp_settings
SET sms_fallback_enabled = $1
WHERE id = 1
RETURNING *;

-- name: InsertWhatsAppConnectionEvent :one
INSERT INTO whatsapp_connection_events (type, reason, alarm, raw)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListWhatsAppConnectionEvents :many
SELECT * FROM whatsapp_connection_events
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: GetLatestKVKKNotice :one
SELECT * FROM kvkk_notices
WHERE locale = $1
ORDER BY version DESC
LIMIT 1;

-- name: ListLatestKVKKNotices :many
SELECT DISTINCT ON (locale) *
FROM kvkk_notices
ORDER BY locale, version DESC;

-- name: InsertKVKKNotice :one
INSERT INTO kvkk_notices (locale, version, body, created_by)
SELECT sqlc.arg(locale)::text, COALESCE(MAX(version), 0) + 1, sqlc.arg(body)::text, sqlc.narg(created_by)::bigint
FROM kvkk_notices
WHERE locale = sqlc.arg(locale)::text
RETURNING *;

-- name: UpsertConversation :one
INSERT INTO conversations (channel, contact_e164, contact_name, user_id, last_message_at)
VALUES (sqlc.arg(channel), sqlc.arg(contact_e164), sqlc.narg(contact_name), sqlc.narg(user_id), sqlc.narg(last_message_at))
ON CONFLICT (channel, contact_e164) DO UPDATE SET
    contact_name = COALESCE(EXCLUDED.contact_name, conversations.contact_name),
    user_id = COALESCE(conversations.user_id, EXCLUDED.user_id),
    last_message_at = GREATEST(conversations.last_message_at, EXCLUDED.last_message_at)
RETURNING *;

-- name: InsertMessage :one
INSERT INTO messages (
    conversation_id, organization_id, brand_id, channel, direction, sender_type,
    external_id, body, media, status, raw, sent_at
) VALUES (
    sqlc.arg(conversation_id), sqlc.narg(organization_id), sqlc.narg(brand_id), sqlc.arg(channel),
    sqlc.arg(direction), sqlc.arg(sender_type), sqlc.arg(external_id), sqlc.narg(body), sqlc.narg(media),
    sqlc.arg(status), sqlc.narg(raw), sqlc.narg(sent_at)
)
ON CONFLICT (channel, external_id) DO NOTHING
RETURNING *;

-- name: UpdateMessageStatusByExternalIDs :execrows
UPDATE messages
SET status = sqlc.arg(status)
WHERE channel = sqlc.arg(channel)
  AND external_id = ANY(sqlc.arg(external_ids)::text[])
  AND direction = 'out'
  AND status <> 'read';

-- name: CountMessagesByExternalID :one
SELECT COUNT(*)::bigint FROM messages WHERE channel = $1 AND external_id = $2;

-- name: ListWhatsAppAlarmRecipients :many
SELECT DISTINCT u.id, u.uuid, u.locale
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.deleted_at IS NULL
  AND u.status = 'active'
  AND (r.slug = 'super_admin' OR p.slug = 'whatsapp.manage');
