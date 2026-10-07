-- Reverts TEC-383. Data loss: AI settings, conversations, messages, pending
-- actions and the usage ledger with its monthly projection are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'ai.use', 'ai.usage.read', 'ai.settings.manage', 'ai.actions.confirm')
);
DELETE FROM permissions WHERE slug IN (
    'ai.use', 'ai.usage.read', 'ai.settings.manage', 'ai.actions.confirm');

DROP TABLE IF EXISTS ai_usage_monthly;
DROP TABLE IF EXISTS ai_usage;
DROP FUNCTION IF EXISTS ai_usage_append_only();
DROP TABLE IF EXISTS ai_pending_actions;
DROP TABLE IF EXISTS ai_messages;
DROP TABLE IF EXISTS ai_conversations;
DROP TABLE IF EXISTS ai_org_settings;
DROP TABLE IF EXISTS ai_settings;
