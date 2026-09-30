-- name: InsertOutboxEvent :one
INSERT INTO outbox_events (event_name, payload, status, available_at)
VALUES ($1, $2, 'pending', NOW())
RETURNING *;

-- name: ClaimOutboxEvents :many
WITH picked AS (
    SELECT id
    FROM outbox_events
    WHERE status = 'pending'
      AND available_at <= NOW()
    ORDER BY available_at ASC, id ASC
    LIMIT sqlc.arg(batch_limit)
    FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events o
SET available_at = NOW() + (sqlc.arg(lease_ms)::bigint * INTERVAL '1 millisecond')
FROM picked
WHERE o.id = picked.id
RETURNING o.*;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events
SET status = 'published',
    published_at = NOW(),
    last_error = NULL
WHERE id = $1
  AND status = 'pending';

-- name: MarkOutboxRetry :exec
UPDATE outbox_events
SET status = 'pending',
    attempts = $2,
    last_error = $3,
    available_at = $4
WHERE id = $1
  AND status = 'pending';

-- name: MarkOutboxFailed :exec
UPDATE outbox_events
SET status = 'failed',
    attempts = $2,
    last_error = $3,
    available_at = NOW()
WHERE id = $1
  AND status = 'pending';

-- name: CountOutboxByStatus :one
SELECT COUNT(*)::bigint
FROM outbox_events
WHERE status = $1;
