-- TEC-393 (F4-02a): WhatsApp conversation schema for the AI pipeline and the
-- panel inbox. Extends conversations/messages (000034), adds the AI run log
-- and the contact opt-out ledger with its state projection, and seeds the
-- conversations.* permissions. Endpoints, identity resolution, the pipeline
-- worker and the outgoing queue land in TEC-394+.
--
--   * conversations: inbox state (status, assignment), AI mode, the resolved
--     identity of the contact, locale, counters and the K22 consent time.
--     Existing rows become status = open, ai_mode = auto,
--     identity_kind = unknown through the column defaults.
--     assigned_org_id is the visibility owner (NULL = center); per the F4
--     user answer S2 only the platform admin reads conversations for now.
--   * messages: staff sender, the AI run that produced the message, stored
--     media (SeaweedFS key, mime, size) and the delivery status time.
--   * conversation_ai_runs: one run per triggering inbound message (UNIQUE).
--     Stages and the tool call summary are jsonb arrays. Rows older than 90
--     days are purged by the conversation AI run purge task; messages keep
--     their text and lose the run reference (ON DELETE SET NULL).
--   * contact_opt_outs: append-only ledger per phone number and scope
--     (marketing | ai); action out = opt out, in = opt back in. The phone is
--     global (K11), so no organization/brand. contact_opt_out_state is the
--     latest row per (contact_e164, scope), maintained by a trigger in the
--     same statement, so the projection always reflects the last record.

-- 1. AI run log --------------------------------------------------------------
-- Created before the messages column that references it.
CREATE TABLE conversation_ai_runs (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    conversation_id     BIGINT        NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    trigger_message_id  BIGINT        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    organization_id     BIGINT        NULL REFERENCES organizations (id) ON DELETE SET NULL,
    brand_id            BIGINT        NULL REFERENCES brands (id) ON DELETE SET NULL,
    status              VARCHAR(16)   NOT NULL DEFAULT 'running',
    -- [{"stage": "identity", "at": "...", "ms": 12, "detail": {...}}, ...]
    stages              JSONB         NOT NULL DEFAULT '[]'::jsonb,
    -- [{"name": "create_task", "ok": true, "ms": 40}, ...] (no full payloads)
    tool_calls          JSONB         NOT NULL DEFAULT '[]'::jsonb,
    model               VARCHAR(128)  NOT NULL DEFAULT '',
    input_tokens        BIGINT        NOT NULL DEFAULT 0,
    output_tokens       BIGINT        NOT NULL DEFAULT 0,
    cache_read_tokens   BIGINT        NOT NULL DEFAULT 0,
    cache_write_tokens  BIGINT        NOT NULL DEFAULT 0,
    error               TEXT          NULL,
    started_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    finished_at         TIMESTAMPTZ   NULL,
    duration_ms         INTEGER       NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_conversation_ai_runs_uuid UNIQUE (uuid),
    CONSTRAINT uq_conversation_ai_runs_trigger UNIQUE (trigger_message_id),
    CONSTRAINT chk_conversation_ai_runs_status CHECK (
        status IN ('running', 'completed', 'failed', 'skipped')),
    CONSTRAINT chk_conversation_ai_runs_finished CHECK ((status = 'running') = (finished_at IS NULL)),
    CONSTRAINT chk_conversation_ai_runs_stages CHECK (jsonb_typeof(stages) = 'array'),
    CONSTRAINT chk_conversation_ai_runs_tool_calls CHECK (jsonb_typeof(tool_calls) = 'array'),
    CONSTRAINT chk_conversation_ai_runs_tokens CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0),
    CONSTRAINT chk_conversation_ai_runs_duration CHECK (duration_ms IS NULL OR duration_ms >= 0),
    CONSTRAINT chk_conversation_ai_runs_error CHECK (error IS NULL OR char_length(error) <= 5000)
);

CREATE INDEX idx_conversation_ai_runs_conversation ON conversation_ai_runs (conversation_id, created_at DESC);
CREATE INDEX idx_conversation_ai_runs_created ON conversation_ai_runs (created_at);
CREATE INDEX idx_conversation_ai_runs_running ON conversation_ai_runs (started_at)
    WHERE status = 'running';

CREATE TRIGGER trg_conversation_ai_runs_set_updated_at
    BEFORE UPDATE ON conversation_ai_runs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 2. Conversations -----------------------------------------------------------
ALTER TABLE conversations
    ADD COLUMN status               VARCHAR(16)  NOT NULL DEFAULT 'open',
    ADD COLUMN ai_mode              VARCHAR(16)  NOT NULL DEFAULT 'auto',
    ADD COLUMN ai_paused_until      TIMESTAMPTZ  NULL,
    ADD COLUMN assigned_user_id     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN assigned_org_id      BIGINT       NULL REFERENCES organizations (id) ON DELETE SET NULL,
    ADD COLUMN identity_kind        VARCHAR(16)  NOT NULL DEFAULT 'unknown',
    ADD COLUMN identity_user_id     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN identity_org_id      BIGINT       NULL REFERENCES organizations (id) ON DELETE SET NULL,
    ADD COLUMN identity_resolved_at TIMESTAMPTZ  NULL,
    ADD COLUMN locale               VARCHAR(8)   NULL,
    ADD COLUMN last_inbound_at      TIMESTAMPTZ  NULL,
    ADD COLUMN unread_count         INTEGER      NOT NULL DEFAULT 0,
    ADD COLUMN ai_consent_at        TIMESTAMPTZ  NULL,
    ADD COLUMN visitor_lead_id      BIGINT       NULL REFERENCES leads (id) ON DELETE SET NULL,
    ADD CONSTRAINT chk_conversations_status CHECK (status IN ('open', 'pending', 'closed')),
    ADD CONSTRAINT chk_conversations_ai_mode CHECK (ai_mode IN ('auto', 'paused', 'off')),
    ADD CONSTRAINT chk_conversations_ai_paused_until CHECK (ai_paused_until IS NULL OR ai_mode = 'paused'),
    ADD CONSTRAINT chk_conversations_identity_kind CHECK (
        identity_kind IN ('panel_user', 'customer', 'visitor', 'unknown')),
    -- Only a panel user or customer names a user (the reference may later be
    -- cleared by ON DELETE SET NULL).
    ADD CONSTRAINT chk_conversations_identity_user CHECK (
        identity_user_id IS NULL OR identity_kind IN ('panel_user', 'customer')),
    ADD CONSTRAINT chk_conversations_identity_org CHECK (
        identity_org_id IS NULL OR identity_kind = 'panel_user'),
    ADD CONSTRAINT chk_conversations_identity_resolved CHECK (
        identity_kind = 'unknown' OR identity_resolved_at IS NOT NULL),
    ADD CONSTRAINT chk_conversations_unread_count CHECK (unread_count >= 0),
    ADD CONSTRAINT chk_conversations_locale CHECK (locale IS NULL OR locale ~ '^[a-z]{2}(_[A-Z]{2})?$');

-- Existing rows: last inbound time from the stored messages.
UPDATE conversations c
SET last_inbound_at = m.last_in
FROM (
    SELECT conversation_id, MAX(COALESCE(sent_at, created_at)) AS last_in
    FROM messages
    WHERE direction = 'in'
    GROUP BY conversation_id
) m
WHERE m.conversation_id = c.id;

CREATE INDEX idx_conversations_last_message ON conversations (last_message_at DESC NULLS LAST, id DESC);
CREATE INDEX idx_conversations_status_last_message ON conversations (status, last_message_at DESC);
CREATE INDEX idx_conversations_assigned_user ON conversations (assigned_user_id) WHERE assigned_user_id IS NOT NULL;
CREATE INDEX idx_conversations_assigned_org ON conversations (assigned_org_id) WHERE assigned_org_id IS NOT NULL;
CREATE INDEX idx_conversations_identity_user ON conversations (identity_user_id) WHERE identity_user_id IS NOT NULL;
CREATE INDEX idx_conversations_identity_org ON conversations (identity_org_id) WHERE identity_org_id IS NOT NULL;
CREATE INDEX idx_conversations_visitor_lead ON conversations (visitor_lead_id) WHERE visitor_lead_id IS NOT NULL;
CREATE INDEX idx_conversations_ai_paused_until ON conversations (ai_paused_until) WHERE ai_mode = 'paused';

-- 3. Messages ----------------------------------------------------------------
ALTER TABLE messages
    ADD COLUMN sender_user_id     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN ai_run_id          BIGINT       NULL REFERENCES conversation_ai_runs (id) ON DELETE SET NULL,
    ADD COLUMN media_storage_key  TEXT         NULL,
    ADD COLUMN media_mime         VARCHAR(128) NULL,
    ADD COLUMN media_size         BIGINT       NULL,
    ADD COLUMN delivery_status_at TIMESTAMPTZ  NULL,
    ADD CONSTRAINT chk_messages_sender_user CHECK (sender_user_id IS NULL OR sender_type = 'staff'),
    ADD CONSTRAINT chk_messages_ai_run CHECK (ai_run_id IS NULL OR sender_type IN ('ai', 'system')),
    ADD CONSTRAINT chk_messages_media_size CHECK (media_size IS NULL OR media_size >= 0),
    ADD CONSTRAINT chk_messages_media_key CHECK (media_storage_key IS NULL OR btrim(media_storage_key) <> '');

-- Cursor pagination of the timeline: (created_at, id) per conversation.
DROP INDEX idx_messages_conversation_created;
CREATE INDEX idx_messages_conversation_cursor ON messages (conversation_id, created_at DESC, id DESC);
CREATE INDEX idx_messages_sender_user ON messages (sender_user_id) WHERE sender_user_id IS NOT NULL;
CREATE INDEX idx_messages_ai_run ON messages (ai_run_id) WHERE ai_run_id IS NOT NULL;

-- 4. Opt-out ledger (append-only) and state projection -----------------------
CREATE TABLE contact_opt_outs (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    contact_e164        VARCHAR(20)   NOT NULL,
    scope               VARCHAR(16)   NOT NULL,
    action              VARCHAR(8)    NOT NULL DEFAULT 'out',
    source              VARCHAR(16)   NOT NULL,
    conversation_id     BIGINT        NULL REFERENCES conversations (id) ON DELETE SET NULL,
    created_by_user_id  BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    note                TEXT          NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contact_opt_outs_uuid UNIQUE (uuid),
    CONSTRAINT chk_contact_opt_outs_e164 CHECK (contact_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT chk_contact_opt_outs_scope CHECK (scope IN ('marketing', 'ai')),
    CONSTRAINT chk_contact_opt_outs_action CHECK (action IN ('out', 'in')),
    CONSTRAINT chk_contact_opt_outs_source CHECK (
        source IN ('whatsapp', 'panel', 'portal', 'campaign', 'import', 'system')),
    CONSTRAINT chk_contact_opt_outs_note CHECK (note IS NULL OR char_length(note) <= 1000)
);

CREATE INDEX idx_contact_opt_outs_contact ON contact_opt_outs (contact_e164, scope, id DESC);
CREATE INDEX idx_contact_opt_outs_conversation ON contact_opt_outs (conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX idx_contact_opt_outs_user ON contact_opt_outs (created_by_user_id) WHERE created_by_user_id IS NOT NULL;

CREATE FUNCTION contact_opt_outs_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'contact_opt_outs is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_contact_opt_outs_append_only
    BEFORE UPDATE OR DELETE ON contact_opt_outs
    FOR EACH ROW
    EXECUTE FUNCTION contact_opt_outs_append_only();

CREATE TRIGGER trg_contact_opt_outs_no_truncate
    BEFORE TRUNCATE ON contact_opt_outs
    FOR EACH STATEMENT
    EXECUTE FUNCTION contact_opt_outs_append_only();

CREATE TABLE contact_opt_out_state (
    contact_e164  VARCHAR(20)   NOT NULL,
    scope         VARCHAR(16)   NOT NULL,
    opted_out     BOOLEAN       NOT NULL,
    last_entry_id BIGINT        NOT NULL REFERENCES contact_opt_outs (id) ON DELETE RESTRICT,
    source        VARCHAR(16)   NOT NULL,
    changed_at    TIMESTAMPTZ   NOT NULL,
    PRIMARY KEY (contact_e164, scope),
    CONSTRAINT chk_contact_opt_out_state_scope CHECK (scope IN ('marketing', 'ai'))
);

CREATE INDEX idx_contact_opt_out_state_opted_out ON contact_opt_out_state (scope, contact_e164) WHERE opted_out;
CREATE INDEX idx_contact_opt_out_state_last_entry ON contact_opt_out_state (last_entry_id);

-- The projection follows the ledger row with the highest id, so concurrent
-- inserts converge on the last record whatever order they commit in.
CREATE FUNCTION contact_opt_outs_project() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO contact_opt_out_state (contact_e164, scope, opted_out, last_entry_id, source, changed_at)
    VALUES (NEW.contact_e164, NEW.scope, NEW.action = 'out', NEW.id, NEW.source, NEW.created_at)
    ON CONFLICT (contact_e164, scope) DO UPDATE
    SET opted_out = EXCLUDED.opted_out,
        last_entry_id = EXCLUDED.last_entry_id,
        source = EXCLUDED.source,
        changed_at = EXCLUDED.changed_at
    WHERE contact_opt_out_state.last_entry_id < EXCLUDED.last_entry_id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_contact_opt_outs_project
    AFTER INSERT ON contact_opt_outs
    FOR EACH ROW
    EXECUTE FUNCTION contact_opt_outs_project();

-- 5. Permissions (platform admin only, F4 user answer S2) ---------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read conversations', 'conversations.read', 'conversations',
       ARRAY['all']::text[], false, true,
       'Read WhatsApp conversations, their messages and AI runs.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Reply to conversations', 'conversations.reply', 'conversations',
       ARRAY['all']::text[], false, true,
       'Send staff replies and attachments in WhatsApp conversations.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage conversations', 'conversations.manage', 'conversations',
       ARRAY['all']::text[], false, true,
       'Assign, close and reopen WhatsApp conversations and change their AI mode.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, 'all'
FROM roles r
JOIN permissions p ON p.slug IN ('conversations.read', 'conversations.reply', 'conversations.manage')
WHERE r.slug = 'super_admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
