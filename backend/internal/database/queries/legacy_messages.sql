-- TEC-263: read-only access to the old hub's message archive
-- (legacy_messages). No UI; the table is append-only and only the K19
-- anonymization may mask a row.

-- name: ListLegacyMessagesByUser :many
-- A person's archived messages of a brand, newest first (keyset paging on
-- sent_at, id; pass NULL cursors for the first page).
SELECT id, uuid, organization_id, brand_id, channel, recipient, user_id, body, payload, sent_at,
       source_table, source_id, anonymized_at, created_at
FROM legacy_messages
WHERE brand_id = sqlc.arg(brand_id)::bigint
  AND (user_id = sqlc.narg(user_id)::bigint OR recipient = sqlc.narg(recipient)::varchar)
  AND (sqlc.narg(before_id)::bigint IS NULL
       OR (COALESCE(sent_at, created_at), id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::bigint))
ORDER BY COALESCE(sent_at, created_at) DESC, id DESC
LIMIT sqlc.arg(page_size)::int;

-- name: CountLegacyMessagesByChannel :many
SELECT channel, COUNT(*)::bigint AS total
FROM legacy_messages
WHERE brand_id = sqlc.arg(brand_id)::bigint
GROUP BY channel
ORDER BY channel;

-- name: AnonymizeLegacyMessages :one
-- K19: masks a person's archived messages; returns the rows masked.
SELECT anonymize_legacy_messages(sqlc.narg(user_id)::bigint, sqlc.narg(phone)::text)::int AS masked;
