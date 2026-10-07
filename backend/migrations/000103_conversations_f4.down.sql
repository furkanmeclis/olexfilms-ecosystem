-- Reverts TEC-393. Data loss: conversation inbox state, identity, counters,
-- staff/AI message attribution, stored media references, AI run logs and the
-- opt-out ledger with its projection are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'conversations.read', 'conversations.reply', 'conversations.manage')
);
DELETE FROM permissions WHERE slug IN (
    'conversations.read', 'conversations.reply', 'conversations.manage');

DROP TABLE IF EXISTS contact_opt_out_state;
DROP TABLE IF EXISTS contact_opt_outs;
DROP FUNCTION IF EXISTS contact_opt_outs_project();
DROP FUNCTION IF EXISTS contact_opt_outs_append_only();

DROP INDEX IF EXISTS idx_messages_conversation_cursor;
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages (conversation_id, created_at DESC);

ALTER TABLE messages
    DROP COLUMN IF EXISTS sender_user_id,
    DROP COLUMN IF EXISTS ai_run_id,
    DROP COLUMN IF EXISTS media_storage_key,
    DROP COLUMN IF EXISTS media_mime,
    DROP COLUMN IF EXISTS media_size,
    DROP COLUMN IF EXISTS delivery_status_at;

ALTER TABLE conversations
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS ai_mode,
    DROP COLUMN IF EXISTS ai_paused_until,
    DROP COLUMN IF EXISTS assigned_user_id,
    DROP COLUMN IF EXISTS assigned_org_id,
    DROP COLUMN IF EXISTS identity_kind,
    DROP COLUMN IF EXISTS identity_user_id,
    DROP COLUMN IF EXISTS identity_org_id,
    DROP COLUMN IF EXISTS identity_resolved_at,
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS last_inbound_at,
    DROP COLUMN IF EXISTS unread_count,
    DROP COLUMN IF EXISTS ai_consent_at,
    DROP COLUMN IF EXISTS visitor_lead_id;

DROP INDEX IF EXISTS idx_conversations_last_message;

DROP TABLE IF EXISTS conversation_ai_runs;
