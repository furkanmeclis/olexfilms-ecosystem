-- TEC-393 (F4-02a): WhatsApp conversation inbox (list contract, cursor
-- timeline, counters, state changes), AI run log and contact opt-outs.

-- Conversations ----------------------------------------------------------------

-- name: GetConversationByID :one
SELECT * FROM conversations WHERE id = sqlc.arg(id);

-- name: GetConversationByUUID :one
SELECT * FROM conversations WHERE uuid = sqlc.arg(uuid);

-- name: ListConversations :many
-- Sort: docs/list-contract.md, keys from whatsapp/repository.ConversationSort.
-- q matches the contact name, the identity user's name and (digits only)
-- the phone number.
SELECT c.* FROM conversations c
WHERE (sqlc.narg(channel)::text IS NULL OR c.channel = sqlc.narg(channel)::text)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(identity_kinds)::text[]), 0) = 0 OR c.identity_kind = ANY (sqlc.narg(identity_kinds)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(ai_modes)::text[]), 0) = 0 OR c.ai_mode = ANY (sqlc.narg(ai_modes)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(assigned_user_ids)::bigint[]), 0) = 0 OR c.assigned_user_id = ANY (sqlc.narg(assigned_user_ids)::bigint[]))
  AND (sqlc.narg(last_message_from)::timestamptz IS NULL OR c.last_message_at >= sqlc.narg(last_message_from)::timestamptz)
  AND (sqlc.narg(last_message_before)::timestamptz IS NULL OR c.last_message_at < sqlc.narg(last_message_before)::timestamptz)
  AND (sqlc.narg(unread)::bool IS NULL OR (c.unread_count > 0) = sqlc.narg(unread)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR c.contact_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (regexp_replace(sqlc.narg(q)::text, '[^0-9]', '', 'g') <> ''
           AND c.contact_e164 LIKE '%' || regexp_replace(sqlc.narg(q)::text, '[^0-9]', '', 'g') || '%')
       OR EXISTS (
           SELECT 1 FROM users u
           WHERE u.id = c.identity_user_id
             AND (u.name || ' ' || COALESCE(u.surname, '')) ILIKE '%' || sqlc.narg(q)::text || '%'))
ORDER BY
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'created_at' THEN c.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_message_at' THEN c.last_message_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'last_message_at' THEN c.last_message_at END DESC NULLS LAST,
  CASE WHEN NOT sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'unread_count' THEN c.unread_count END ASC,
  CASE WHEN sqlc.arg(sort_desc)::bool AND sqlc.arg(sort_key)::text = 'unread_count' THEN c.unread_count END DESC,
  CASE WHEN sqlc.arg(sort_desc)::bool THEN c.id END DESC,
  c.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountConversations :one
SELECT COUNT(*) FROM conversations c
WHERE (sqlc.narg(channel)::text IS NULL OR c.channel = sqlc.narg(channel)::text)
  AND (COALESCE(cardinality(sqlc.narg(statuses)::text[]), 0) = 0 OR c.status = ANY (sqlc.narg(statuses)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(identity_kinds)::text[]), 0) = 0 OR c.identity_kind = ANY (sqlc.narg(identity_kinds)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(ai_modes)::text[]), 0) = 0 OR c.ai_mode = ANY (sqlc.narg(ai_modes)::text[]))
  AND (COALESCE(cardinality(sqlc.narg(assigned_user_ids)::bigint[]), 0) = 0 OR c.assigned_user_id = ANY (sqlc.narg(assigned_user_ids)::bigint[]))
  AND (sqlc.narg(last_message_from)::timestamptz IS NULL OR c.last_message_at >= sqlc.narg(last_message_from)::timestamptz)
  AND (sqlc.narg(last_message_before)::timestamptz IS NULL OR c.last_message_at < sqlc.narg(last_message_before)::timestamptz)
  AND (sqlc.narg(unread)::bool IS NULL OR (c.unread_count > 0) = sqlc.narg(unread)::bool)
  AND (sqlc.narg(q)::text IS NULL
       OR c.contact_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR (regexp_replace(sqlc.narg(q)::text, '[^0-9]', '', 'g') <> ''
           AND c.contact_e164 LIKE '%' || regexp_replace(sqlc.narg(q)::text, '[^0-9]', '', 'g') || '%')
       OR EXISTS (
           SELECT 1 FROM users u
           WHERE u.id = c.identity_user_id
             AND (u.name || ' ' || COALESCE(u.surname, '')) ILIKE '%' || sqlc.narg(q)::text || '%'));

-- name: TouchConversationInbound :one
-- Counters of a newly stored inbound message: unread + 1, last inbound and
-- last message time. A closed conversation reopens on a new inbound message.
UPDATE conversations
SET unread_count = unread_count + 1,
    last_inbound_at = GREATEST(last_inbound_at, sqlc.arg(at)::timestamptz),
    last_message_at = GREATEST(last_message_at, sqlc.arg(at)::timestamptz),
    status = CASE WHEN status = 'closed' THEN 'open' ELSE status END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: TouchConversationOutbound :one
-- Counters of a newly stored outbound (staff, AI, system) message.
UPDATE conversations
SET last_message_at = GREATEST(last_message_at, sqlc.arg(at)::timestamptz)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkConversationRead :one
UPDATE conversations SET unread_count = 0
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationStatus :one
UPDATE conversations SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationAIMode :one
-- paused_until is only kept for ai_mode = paused (CHECK).
UPDATE conversations
SET ai_mode = sqlc.arg(ai_mode)::text,
    ai_paused_until = CASE WHEN sqlc.arg(ai_mode)::text = 'paused' THEN sqlc.narg(ai_paused_until)::timestamptz END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ResumeExpiredAIPauses :execrows
-- Paused conversations whose pause ended return to auto.
UPDATE conversations
SET ai_mode = 'auto', ai_paused_until = NULL
WHERE ai_mode = 'paused'
  AND ai_paused_until IS NOT NULL
  AND ai_paused_until <= sqlc.arg(now)::timestamptz;

-- name: AssignConversation :one
-- assigned_org_id NULL = center.
UPDATE conversations
SET assigned_user_id = sqlc.narg(assigned_user_id),
    assigned_org_id = sqlc.narg(assigned_org_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationIdentity :one
UPDATE conversations
SET identity_kind = sqlc.arg(identity_kind),
    identity_user_id = sqlc.narg(identity_user_id),
    identity_org_id = sqlc.narg(identity_org_id),
    identity_resolved_at = sqlc.narg(identity_resolved_at)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationLocale :one
UPDATE conversations SET locale = sqlc.narg(locale)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationAIConsent :one
UPDATE conversations SET ai_consent_at = sqlc.narg(ai_consent_at)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationVisitorLead :one
UPDATE conversations SET visitor_lead_id = sqlc.narg(visitor_lead_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConversationReferredDealer :one
-- TEC-468: #dealer-code in the first public WhatsApp message routes the
-- visitor lead to that dealer while the conversation stays system-owned.
UPDATE conversations SET referred_dealer_org_id = sqlc.narg(referred_dealer_org_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- Messages (timeline) ----------------------------------------------------------

-- name: GetMessageByUUID :one
SELECT * FROM messages WHERE uuid = sqlc.arg(uuid);

-- name: ListConversationMessagesBefore :many
-- Timeline page, newest first. The cursor is the (created_at, id) of the
-- oldest message already shown; no cursor = the newest page.
SELECT * FROM messages m
WHERE m.conversation_id = sqlc.arg(conversation_id)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::bigint))
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListConversationMessagesAfter :many
-- Messages newer than the cursor (created_at, id) of the newest message
-- already shown, oldest first (catch-up after a reconnect).
SELECT * FROM messages m
WHERE m.conversation_id = sqlc.arg(conversation_id)
  AND (m.created_at, m.id) > (sqlc.arg(cursor_at)::timestamptz, sqlc.arg(cursor_id)::bigint)
ORDER BY m.created_at ASC, m.id ASC
LIMIT sqlc.arg(limit_count);

-- name: SetMessageAIRun :one
UPDATE messages SET ai_run_id = sqlc.narg(ai_run_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetMessageMedia :one
-- Inbound media after it was copied to object storage.
UPDATE messages
SET media_storage_key = sqlc.narg(media_storage_key),
    media_mime = sqlc.narg(media_mime),
    media_size = sqlc.narg(media_size)
WHERE id = sqlc.arg(id)
RETURNING *;

-- AI runs ----------------------------------------------------------------------

-- name: CreateConversationAIRun :one
-- One run per triggering message: a second insert for the same message
-- returns no row (pgx.ErrNoRows).
INSERT INTO conversation_ai_runs (conversation_id, trigger_message_id, organization_id, brand_id, model)
VALUES (sqlc.arg(conversation_id), sqlc.arg(trigger_message_id), sqlc.narg(organization_id),
        sqlc.narg(brand_id), sqlc.arg(model))
ON CONFLICT (trigger_message_id) DO NOTHING
RETURNING *;

-- name: GetConversationAIRunByUUID :one
SELECT * FROM conversation_ai_runs WHERE uuid = sqlc.arg(uuid);

-- name: GetConversationAIRunByTrigger :one
SELECT * FROM conversation_ai_runs WHERE trigger_message_id = sqlc.arg(trigger_message_id);

-- name: UpdateConversationAIRunProgress :one
-- Stages and tool calls of a running run (whole arrays are replaced).
UPDATE conversation_ai_runs
SET stages = sqlc.arg(stages), tool_calls = sqlc.arg(tool_calls)
WHERE id = sqlc.arg(id) AND status = 'running'
RETURNING *;

-- name: FinishConversationAIRun :one
-- running → completed | failed | skipped, once.
UPDATE conversation_ai_runs
SET status = sqlc.arg(status),
    stages = sqlc.arg(stages),
    tool_calls = sqlc.arg(tool_calls),
    model = sqlc.arg(model),
    input_tokens = sqlc.arg(input_tokens),
    output_tokens = sqlc.arg(output_tokens),
    cache_read_tokens = sqlc.arg(cache_read_tokens),
    cache_write_tokens = sqlc.arg(cache_write_tokens),
    error = sqlc.narg(error),
    finished_at = sqlc.arg(finished_at)::timestamptz,
    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (sqlc.arg(finished_at)::timestamptz - started_at)) * 1000)::int)
WHERE id = sqlc.arg(id) AND status = 'running'
RETURNING *;

-- name: ListConversationAIRuns :many
SELECT * FROM conversation_ai_runs
WHERE conversation_id = sqlc.arg(conversation_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: ConversationAIRunStatsBetween :one
-- AI pipeline health (TEC-409): run counts by status in [since, until).
SELECT COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE status = 'completed')::bigint AS completed,
       COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed,
       COUNT(*) FILTER (WHERE status = 'skipped')::bigint AS skipped,
       COUNT(*) FILTER (WHERE status = 'running')::bigint AS running,
       MAX(created_at)::timestamptz AS last_run_at,
       MAX(created_at) FILTER (WHERE status = 'failed')::timestamptz AS last_failed_at
FROM conversation_ai_runs
WHERE created_at >= sqlc.arg(since)::timestamptz
  AND created_at < sqlc.arg(until)::timestamptz;

-- name: PurgeConversationAIRunsBefore :execrows
-- Retention (90 days, QUESTIONS #15): deletes one batch of old runs.
DELETE FROM conversation_ai_runs
WHERE id IN (
    SELECT old.id FROM conversation_ai_runs old
    WHERE old.created_at < sqlc.arg(cutoff)::timestamptz
    ORDER BY old.id
    LIMIT sqlc.arg(batch_size)::int
);

-- Opt-outs ---------------------------------------------------------------------

-- name: InsertContactOptOut :one
-- Append-only; the AFTER INSERT trigger updates contact_opt_out_state.
INSERT INTO contact_opt_outs (contact_e164, scope, action, source, conversation_id, created_by_user_id, note)
VALUES (sqlc.arg(contact_e164), sqlc.arg(scope), sqlc.arg(action), sqlc.arg(source),
        sqlc.narg(conversation_id), sqlc.narg(created_by_user_id), sqlc.narg(note))
RETURNING *;

-- name: GetContactOptOutState :one
SELECT * FROM contact_opt_out_state
WHERE contact_e164 = sqlc.arg(contact_e164) AND scope = sqlc.arg(scope);

-- name: ListContactOptOutStates :many
SELECT * FROM contact_opt_out_state
WHERE contact_e164 = sqlc.arg(contact_e164)
ORDER BY scope;

-- name: ListContactOptOutHistory :many
SELECT * FROM contact_opt_outs
WHERE contact_e164 = sqlc.arg(contact_e164)
ORDER BY id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListOptedOutContacts :many
-- The subset of contacts that are currently opted out of a scope (campaign
-- audience and pipeline guard).
SELECT contact_e164 FROM contact_opt_out_state
WHERE scope = sqlc.arg(scope)
  AND opted_out
  AND contact_e164 = ANY (sqlc.arg(contacts)::text[])
ORDER BY contact_e164;

-- Identity resolution (TEC-394, F4-02b) ---------------------------------------

-- name: ListWhatsAppIdentityMemberships :many
-- Panel memberships of a contact's user with the organization state the
-- resolver needs (access window, read_only, locale). Deleted organizations
-- are skipped; suspended / expired / outside-window ones are filtered in Go.
SELECT o.id AS organization_id, o.brand_id, o.name, o.type, o.status,
       o.access_starts_at, o.access_ends_at, o.locale, om.role
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE om.user_id = sqlc.arg(user_id)
ORDER BY o.name ASC, o.id ASC;

-- name: IsCustomerUser :one
-- A customer is a users row with a customer profile or an organization link
-- (K11).
SELECT (EXISTS (SELECT 1 FROM customer_profiles cp WHERE cp.user_id = sqlc.arg(user_id))
     OR EXISTS (SELECT 1 FROM customer_organizations co WHERE co.user_id = sqlc.arg(user_id)))::boolean AS is_customer;

-- AI pipeline (TEC-396, F4-02c) -------------------------------------------------

-- name: ListInboundMessagesAfter :many
-- Inbound contact messages of a conversation newer than after_id and since,
-- newest first (the pipeline turns them oldest first).
SELECT * FROM messages m
WHERE m.conversation_id = sqlc.arg(conversation_id)
  AND m.direction = 'in'
  AND m.sender_type = 'contact'
  AND m.id > sqlc.arg(after_id)::bigint
  AND m.created_at >= sqlc.arg(since)::timestamptz
ORDER BY m.id DESC
LIMIT sqlc.arg(limit_count);

-- name: MaxConversationAIRunTrigger :one
-- The newest message an AI run was started for (0 = none).
SELECT COALESCE(MAX(trigger_message_id), 0)::bigint
FROM conversation_ai_runs
WHERE conversation_id = sqlc.arg(conversation_id);

-- name: MaxConversationMessageIDBySender :one
-- The newest message of one sender type (0 = none).
SELECT COALESCE(MAX(id), 0)::bigint
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id) AND sender_type = sqlc.arg(sender_type);

-- name: LastConversationMessageAtBySender :one
-- Time of the newest message of one sender type (NULL = none).
SELECT MAX(created_at)::timestamptz
FROM messages
WHERE conversation_id = sqlc.arg(conversation_id) AND sender_type = sqlc.arg(sender_type);
