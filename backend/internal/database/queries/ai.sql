-- TEC-383 (F4-01a): AI assistant settings, conversations, messages, pending
-- actions and the usage ledger with its monthly projection.

-- Settings ---------------------------------------------------------------------

-- name: GetAISettings :one
SELECT * FROM ai_settings WHERE id = 1;

-- name: UpdateAISettings :one
UPDATE ai_settings
SET default_model = sqlc.arg(default_model),
    fast_model = sqlc.arg(fast_model),
    default_monthly_token_quota = sqlc.arg(default_monthly_token_quota),
    system_pool_monthly_quota = sqlc.arg(system_pool_monthly_quota),
    tool_toggles = sqlc.arg(tool_toggles),
    extra_instructions = sqlc.arg(extra_instructions),
    knowledge_text = sqlc.arg(knowledge_text),
    updated_by_user_id = sqlc.narg(updated_by_user_id)
WHERE id = 1
RETURNING *;

-- name: GetAIOrgSettings :one
SELECT * FROM ai_org_settings WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertAIOrgSettings :one
INSERT INTO ai_org_settings (organization_id, brand_id, enabled, monthly_token_quota, updated_by_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(enabled),
        sqlc.narg(monthly_token_quota), sqlc.narg(updated_by_user_id))
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled,
    monthly_token_quota = EXCLUDED.monthly_token_quota,
    updated_by_user_id = EXCLUDED.updated_by_user_id
RETURNING *;

-- Conversations ----------------------------------------------------------------

-- name: CreateAIConversation :one
INSERT INTO ai_conversations (organization_id, brand_id, user_id, channel, title)
VALUES (sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(channel), sqlc.arg(title))
RETURNING *;

-- name: GetAIConversationForUser :one
SELECT * FROM ai_conversations
WHERE uuid = sqlc.arg(uuid)
  AND organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL;

-- name: ListAIConversations :many
-- Sort: docs/list-contract.md, keys from ai/repository.ConversationSort.
SELECT * FROM ai_conversations c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.user_id = sqlc.arg(user_id)
  AND c.channel = sqlc.arg(channel)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'title' THEN c.title END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'title' THEN c.title END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN c.created_at WHEN 'updated_at' THEN c.updated_at END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'created_at' THEN c.created_at WHEN 'updated_at' THEN c.updated_at END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAIConversations :one
SELECT COUNT(*) FROM ai_conversations c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.user_id = sqlc.arg(user_id)
  AND c.channel = sqlc.arg(channel)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL OR c.title ILIKE '%' || sqlc.narg(q)::text || '%');

-- name: UpdateAIConversationTitle :one
UPDATE ai_conversations SET title = sqlc.arg(title)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: TouchAIConversation :one
-- Counts a newly stored message and moves the conversation to the top.
UPDATE ai_conversations
SET message_count = message_count + 1, last_message_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SoftDeleteAIConversation :execrows
UPDATE ai_conversations SET deleted_at = NOW()
WHERE uuid = sqlc.arg(uuid)
  AND organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL;

-- Messages ---------------------------------------------------------------------

-- name: CreateAIMessage :one
INSERT INTO ai_messages (
    conversation_id, organization_id, brand_id, role, status, content, ui, model,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
VALUES (
    sqlc.arg(conversation_id), sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(role),
    sqlc.arg(status), sqlc.arg(content), sqlc.arg(ui), sqlc.arg(model),
    sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens))
RETURNING *;

-- name: FinishAIMessage :one
-- Completes a pending assistant turn (complete, error or cancelled).
UPDATE ai_messages
SET status = sqlc.arg(status),
    content = sqlc.arg(content),
    ui = sqlc.arg(ui),
    model = sqlc.arg(model),
    input_tokens = sqlc.arg(input_tokens),
    output_tokens = sqlc.arg(output_tokens),
    cache_read_tokens = sqlc.arg(cache_read_tokens),
    cache_write_tokens = sqlc.arg(cache_write_tokens),
    error = sqlc.narg(error)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: ListAIMessages :many
SELECT * FROM ai_messages
WHERE conversation_id = sqlc.arg(conversation_id)
ORDER BY id ASC;

-- Pending actions --------------------------------------------------------------

-- name: CreateAIPendingAction :one
INSERT INTO ai_pending_actions (
    organization_id, brand_id, user_id, source, source_ref, tool_use_id, tool_name,
    input, preview, idempotency_key, expires_at)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(user_id), sqlc.arg(source),
    sqlc.narg(source_ref), sqlc.arg(tool_use_id), sqlc.arg(tool_name), sqlc.arg(input),
    sqlc.arg(preview), sqlc.arg(idempotency_key), sqlc.arg(expires_at))
RETURNING *;

-- name: GetAIPendingActionForUser :one
SELECT * FROM ai_pending_actions
WHERE uuid = sqlc.arg(uuid)
  AND organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListAIPendingActionsForUser :many
SELECT * FROM ai_pending_actions
WHERE organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'pending'
  AND expires_at > NOW()
ORDER BY created_at DESC, id DESC;

-- name: ClaimAIPendingAction :one
-- Compare-and-set pending → executing: of two concurrent confirmations only
-- one gets the row; the other gets pgx.ErrNoRows.
UPDATE ai_pending_actions
SET status = 'executing'
WHERE uuid = sqlc.arg(uuid)
  AND organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'pending'
  AND expires_at > NOW()
RETURNING *;

-- name: ResolveAIPendingAction :one
-- Finishes a claimed action: executing → confirmed | failed.
UPDATE ai_pending_actions
SET status = sqlc.arg(status),
    result = sqlc.narg(result),
    error = sqlc.narg(error),
    resolved_at = NOW()
WHERE id = sqlc.arg(id)
  AND status = 'executing'
  AND sqlc.arg(status)::text IN ('confirmed', 'failed')
RETURNING *;

-- name: CancelAIPendingAction :one
UPDATE ai_pending_actions
SET status = 'cancelled', resolved_at = NOW()
WHERE uuid = sqlc.arg(uuid)
  AND organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'pending'
RETURNING *;

-- name: ExpireAIPendingActions :execrows
-- Stale cleanup: pending actions past their expiry become expired.
UPDATE ai_pending_actions
SET status = 'expired', resolved_at = NOW()
WHERE status = 'pending' AND expires_at <= sqlc.arg(now)::timestamptz;

-- Usage ledger -----------------------------------------------------------------

-- name: InsertAIUsage :one
-- Append-only. Always call through ai/repository.RecordUsage, which upserts
-- ai_usage_monthly in the same transaction.
INSERT INTO ai_usage (
    organization_id, brand_id, pool, user_id, channel, purpose, model,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(pool), sqlc.narg(user_id),
    sqlc.arg(channel), sqlc.arg(purpose), sqlc.arg(model),
    sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens))
RETURNING *;

-- name: AddAIUsageMonthly :one
-- Projection of one ledger row (period = YYYY-MM of created_at, UTC).
INSERT INTO ai_usage_monthly (
    organization_id, brand_id, pool, period, quota_tokens, input_tokens, output_tokens,
    cache_read_tokens, cache_write_tokens, request_count)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(brand_id), sqlc.arg(pool), sqlc.arg(period),
    sqlc.arg(quota_tokens), sqlc.arg(input_tokens), sqlc.arg(output_tokens),
    sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens), 1)
ON CONFLICT (organization_id, pool, period) DO UPDATE
SET quota_tokens = ai_usage_monthly.quota_tokens + EXCLUDED.quota_tokens,
    input_tokens = ai_usage_monthly.input_tokens + EXCLUDED.input_tokens,
    output_tokens = ai_usage_monthly.output_tokens + EXCLUDED.output_tokens,
    cache_read_tokens = ai_usage_monthly.cache_read_tokens + EXCLUDED.cache_read_tokens,
    cache_write_tokens = ai_usage_monthly.cache_write_tokens + EXCLUDED.cache_write_tokens,
    request_count = ai_usage_monthly.request_count + 1,
    updated_at = NOW()
RETURNING *;

-- name: GetAIUsageMonthly :one
-- Quota check: tokens of one pool in one month (no row = nothing used).
SELECT COALESCE((
    SELECT quota_tokens FROM ai_usage_monthly
    WHERE organization_id = sqlc.arg(organization_id)
      AND pool = sqlc.arg(pool)
      AND period = sqlc.arg(period)
), 0)::bigint AS quota_tokens;

-- name: ListAIUsageMonthly :many
-- Monthly totals of a brand for the quota report.
SELECT * FROM ai_usage_monthly
WHERE brand_id = sqlc.arg(brand_id)
  AND period = sqlc.arg(period)
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL OR organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
ORDER BY quota_tokens DESC, organization_id ASC, pool ASC;

-- name: ListAIUsage :many
-- Sort: docs/list-contract.md, keys from ai/repository.UsageSort.
SELECT * FROM ai_usage u
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR u.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL OR u.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND (sqlc.narg(user_ids)::bigint[] IS NULL OR u.user_id = ANY(sqlc.narg(user_ids)::bigint[]))
  AND (sqlc.narg(pools)::text[] IS NULL OR u.pool = ANY(sqlc.narg(pools)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR u.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(purposes)::text[] IS NULL OR u.purpose = ANY(sqlc.narg(purposes)::text[]))
  AND (sqlc.narg(models)::text[] IS NULL OR u.model = ANY(sqlc.narg(models)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(tokens_min)::bigint IS NULL OR u.quota_tokens >= sqlc.narg(tokens_min)::bigint)
  AND (sqlc.narg(tokens_max)::bigint IS NULL OR u.quota_tokens <= sqlc.narg(tokens_max)::bigint)
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'model' THEN u.model WHEN 'channel' THEN u.channel
      WHEN 'purpose' THEN u.purpose END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'model' THEN u.model WHEN 'channel' THEN u.channel
      WHEN 'purpose' THEN u.purpose END
  END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN u.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN u.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'tokens' THEN u.quota_tokens WHEN 'input_tokens' THEN u.input_tokens
      WHEN 'output_tokens' THEN u.output_tokens END
  END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN
    CASE sqlc.arg(sort_key)::text WHEN 'tokens' THEN u.quota_tokens WHEN 'input_tokens' THEN u.input_tokens
      WHEN 'output_tokens' THEN u.output_tokens END
  END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN u.id END DESC,
  u.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAIUsage :one
SELECT COUNT(*) FROM ai_usage u
WHERE (sqlc.narg(brand_id)::bigint IS NULL OR u.brand_id = sqlc.narg(brand_id)::bigint)
  AND (sqlc.narg(organization_ids)::bigint[] IS NULL OR u.organization_id = ANY(sqlc.narg(organization_ids)::bigint[]))
  AND (sqlc.narg(user_ids)::bigint[] IS NULL OR u.user_id = ANY(sqlc.narg(user_ids)::bigint[]))
  AND (sqlc.narg(pools)::text[] IS NULL OR u.pool = ANY(sqlc.narg(pools)::text[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR u.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(purposes)::text[] IS NULL OR u.purpose = ANY(sqlc.narg(purposes)::text[]))
  AND (sqlc.narg(models)::text[] IS NULL OR u.model = ANY(sqlc.narg(models)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR u.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_before)::timestamptz IS NULL OR u.created_at < sqlc.narg(created_before)::timestamptz)
  AND (sqlc.narg(tokens_min)::bigint IS NULL OR u.quota_tokens >= sqlc.narg(tokens_min)::bigint)
  AND (sqlc.narg(tokens_max)::bigint IS NULL OR u.quota_tokens <= sqlc.narg(tokens_max)::bigint);
