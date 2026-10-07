-- TEC-395 (F4-02d): WhatsApp outgoing queue, delivery receipts and inbound
-- media storage.

-- name: InsertQueuedMessage :one
-- An outgoing AI/staff/system message waiting for the whatsapp:send task.
-- uuid is chosen by the caller (idempotency key); external_id is the client
-- message id handed to the provider.
INSERT INTO messages (
    uuid, conversation_id, organization_id, brand_id, channel, direction, sender_type,
    external_id, body, media, status, sender_user_id, ai_run_id,
    media_storage_key, media_mime, media_size
) VALUES (
    sqlc.arg(uuid), sqlc.arg(conversation_id), sqlc.narg(organization_id), sqlc.narg(brand_id),
    sqlc.arg(channel), 'out', sqlc.arg(sender_type), sqlc.arg(external_id), sqlc.narg(body),
    sqlc.narg(media), 'queued', sqlc.narg(sender_user_id), sqlc.narg(ai_run_id),
    sqlc.narg(media_storage_key), sqlc.narg(media_mime), sqlc.narg(media_size)
)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = sqlc.arg(id);

-- name: LockQueuedMessage :one
-- The send task holds the row while it calls the provider: a second run of
-- the same task (or a concurrent one) finds nothing and sends nothing.
SELECT * FROM messages
WHERE id = sqlc.arg(id) AND status = 'queued' AND direction = 'out'
FOR UPDATE SKIP LOCKED;

-- name: MarkMessageSent :one
UPDATE messages
SET status = 'sent',
    external_id = sqlc.arg(external_id),
    sent_at = sqlc.arg(sent_at),
    send_attempts = send_attempts + 1,
    failure_reason = NULL,
    delivery_status_at = sqlc.arg(sent_at)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkMessageSendError :one
-- A failed provider call: failed = final (no retry left or a permanent
-- error), else the message stays queued for the next attempt.
UPDATE messages
SET status = CASE WHEN sqlc.arg(failed)::boolean THEN 'failed' ELSE 'queued' END,
    send_attempts = send_attempts + 1,
    failure_reason = sqlc.arg(failure_reason),
    delivery_status_at = CASE WHEN sqlc.arg(failed)::boolean THEN sqlc.arg(at)::timestamptz ELSE delivery_status_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListStaleQueuedMessages :many
-- Queued messages older than the cutoff whose send task may have been lost
-- (enqueue failure, Redis flush); the sweep enqueues them again.
SELECT id, uuid FROM messages
WHERE status = 'queued' AND created_at < sqlc.arg(before)
ORDER BY created_at, id
LIMIT sqlc.arg(limit_count);

-- name: ApplyMessageReceipt :many
-- Delivery receipt of outgoing messages. A receipt only moves forward
-- (sent → delivered → read); read is final.
UPDATE messages
SET status = sqlc.arg(status),
    delivery_status_at = sqlc.arg(at)
WHERE channel = sqlc.arg(channel)
  AND external_id = ANY(sqlc.arg(external_ids)::text[])
  AND direction = 'out'
  AND status <> 'read'
  AND (sqlc.arg(status)::text = 'read' OR status <> 'delivered')
RETURNING *;

-- name: SetMessageMediaNote :one
-- Inbound media that was not stored (too large, unsupported type, download
-- error): the reason is kept in the media JSON for the inbox.
UPDATE messages
SET media = COALESCE(media, '{}'::jsonb) || jsonb_build_object('storage_skipped', sqlc.arg(reason)::text)
WHERE id = sqlc.arg(id)
RETURNING *;
