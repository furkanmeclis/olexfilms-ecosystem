-- TEC-383 (F4-01a): AI assistant schema, permissions and sqlc. Simplified
-- from otopoly-go 000048/000049. The provider layer, tools, chat endpoints
-- and settings API land in TEC-384+; this migration only fixes the data
-- model and database guards.
--
--   * ai_settings: platform singleton (like whatsapp_settings, so no
--     organization/brand). The API key is never stored here (env only).
--     Quotas are tokens per calendar month (UTC); 0 = unlimited.
--   * ai_org_settings: per-organization override. No row = enabled with the
--     platform default quota; monthly_token_quota NULL = inherit.
--   * ai_conversations / ai_messages: panel and portal chats. Tool calls
--     live inside ai_messages.content (replayable API blocks); ui holds the
--     render blocks. Messages carry the conversation's organization and
--     brand through a composite foreign key.
--   * ai_pending_actions: write-tool proposals awaiting the user's
--     confirmation, from any source (panel, portal, whatsapp, mcp).
--     source_ref is the polymorphic conversation reference of the source.
--     Confirmation claims the row with a single pending → executing UPDATE.
--   * ai_usage: append-only ledger, one row per model call. pool = 'system'
--     books customer/visitor WhatsApp, portal and triage calls on the brand
--     center's separate system pool; pool = 'org' books on the organization
--     quota. quota_tokens (input + output + cache_write) is what quotas
--     count; cache reads are recorded but free.
--   * ai_usage_monthly: projection of ai_usage per (organization, pool,
--     YYYY-MM UTC), upserted in the same transaction as the ledger row.

-- 1. Platform settings -------------------------------------------------------
CREATE TABLE ai_settings (
    id                          SMALLINT      PRIMARY KEY DEFAULT 1,
    default_model               VARCHAR(128)  NOT NULL DEFAULT 'claude-sonnet-5-5',
    fast_model                  VARCHAR(128)  NOT NULL DEFAULT 'claude-haiku-4-5',
    default_monthly_token_quota BIGINT        NOT NULL DEFAULT 2000000,
    system_pool_monthly_quota   BIGINT        NOT NULL DEFAULT 5000000,
    -- {"tool_name": false} disables a tool; missing keys mean enabled.
    tool_toggles                JSONB         NOT NULL DEFAULT '{}'::jsonb,
    extra_instructions          TEXT          NOT NULL DEFAULT '',
    knowledge_text              TEXT          NOT NULL DEFAULT '',
    updated_by_user_id          BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_ai_settings_models CHECK (btrim(default_model) <> '' AND btrim(fast_model) <> ''),
    CONSTRAINT chk_ai_settings_quota CHECK (default_monthly_token_quota >= 0),
    CONSTRAINT chk_ai_settings_system_quota CHECK (system_pool_monthly_quota >= 0),
    CONSTRAINT chk_ai_settings_tool_toggles CHECK (jsonb_typeof(tool_toggles) = 'object'),
    CONSTRAINT chk_ai_settings_extra_instructions CHECK (char_length(extra_instructions) <= 20000),
    CONSTRAINT chk_ai_settings_knowledge_text CHECK (char_length(knowledge_text) <= 100000)
);

CREATE TRIGGER trg_ai_settings_set_updated_at
    BEFORE UPDATE ON ai_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO ai_settings (id) VALUES (1);

-- 2. Organization override ---------------------------------------------------
CREATE TABLE ai_org_settings (
    organization_id     BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id            BIGINT       NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    -- NULL = platform default; 0 = unlimited.
    monthly_token_quota BIGINT       NULL,
    updated_by_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ai_org_settings_quota CHECK (monthly_token_quota IS NULL OR monthly_token_quota >= 0)
);

CREATE INDEX idx_ai_org_settings_brand ON ai_org_settings (brand_id);

CREATE TRIGGER trg_ai_org_settings_set_updated_at
    BEFORE UPDATE ON ai_org_settings
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_ai_org_settings_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON ai_org_settings
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 3. Conversations -----------------------------------------------------------
CREATE TABLE ai_conversations (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id          BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    channel          VARCHAR(16)   NOT NULL,
    title            VARCHAR(200)  NOT NULL DEFAULT '',
    message_count    INTEGER       NOT NULL DEFAULT 0,
    last_message_at  TIMESTAMPTZ   NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ   NULL,
    CONSTRAINT uq_ai_conversations_uuid UNIQUE (uuid),
    CONSTRAINT uq_ai_conversations_id_org_brand UNIQUE (id, organization_id, brand_id),
    CONSTRAINT chk_ai_conversations_channel CHECK (channel IN ('panel', 'portal')),
    CONSTRAINT chk_ai_conversations_message_count CHECK (message_count >= 0)
);

CREATE INDEX idx_ai_conversations_user ON ai_conversations (organization_id, user_id, channel, updated_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_ai_conversations_brand ON ai_conversations (brand_id, created_at DESC);
CREATE INDEX idx_ai_conversations_user_id ON ai_conversations (user_id);

CREATE TRIGGER trg_ai_conversations_set_updated_at
    BEFORE UPDATE ON ai_conversations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_ai_conversations_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON ai_conversations
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. Messages ----------------------------------------------------------------
-- One row per user input and one per assistant turn. content holds the API
-- messages replayed to the model ([{"role": ..., "content": [blocks]}]); an
-- assistant row holds the whole tool loop of its turn.
CREATE TABLE ai_messages (
    id                  BIGSERIAL     PRIMARY KEY,
    uuid                UUID          NOT NULL DEFAULT gen_random_uuid(),
    conversation_id     BIGINT        NOT NULL,
    organization_id     BIGINT        NOT NULL,
    brand_id            BIGINT        NOT NULL,
    role                VARCHAR(16)   NOT NULL,
    status              VARCHAR(16)   NOT NULL DEFAULT 'complete',
    content             JSONB         NOT NULL DEFAULT '[]'::jsonb,
    ui                  JSONB         NOT NULL DEFAULT '[]'::jsonb,
    model               VARCHAR(128)  NOT NULL DEFAULT '',
    input_tokens        BIGINT        NOT NULL DEFAULT 0,
    output_tokens       BIGINT        NOT NULL DEFAULT 0,
    cache_read_tokens   BIGINT        NOT NULL DEFAULT 0,
    cache_write_tokens  BIGINT        NOT NULL DEFAULT 0,
    error               TEXT          NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ai_messages_uuid UNIQUE (uuid),
    CONSTRAINT fk_ai_messages_conversation FOREIGN KEY (conversation_id, organization_id, brand_id)
        REFERENCES ai_conversations (id, organization_id, brand_id) ON DELETE CASCADE,
    CONSTRAINT chk_ai_messages_role CHECK (role IN ('user', 'assistant')),
    CONSTRAINT chk_ai_messages_status CHECK (status IN ('pending', 'complete', 'error', 'cancelled')),
    CONSTRAINT chk_ai_messages_content CHECK (jsonb_typeof(content) = 'array'),
    CONSTRAINT chk_ai_messages_ui CHECK (jsonb_typeof(ui) = 'array'),
    CONSTRAINT chk_ai_messages_tokens CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0),
    CONSTRAINT chk_ai_messages_error CHECK (error IS NULL OR char_length(error) <= 5000)
);

CREATE INDEX idx_ai_messages_conversation ON ai_messages (conversation_id, id);

CREATE TRIGGER trg_ai_messages_set_updated_at
    BEFORE UPDATE ON ai_messages
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- 5. Pending actions ---------------------------------------------------------
CREATE TABLE ai_pending_actions (
    id               BIGSERIAL     PRIMARY KEY,
    uuid             UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id         BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    user_id          BIGINT        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    source           VARCHAR(16)   NOT NULL,
    -- Polymorphic conversation reference of the source (ai_conversations
    -- uuid for panel/portal, the WhatsApp conversation, the MCP session).
    source_ref       VARCHAR(128)  NULL,
    tool_use_id      VARCHAR(128)  NOT NULL,
    tool_name        VARCHAR(64)   NOT NULL,
    input            JSONB         NOT NULL DEFAULT '{}'::jsonb,
    preview          JSONB         NOT NULL DEFAULT '{}'::jsonb,
    status           VARCHAR(16)   NOT NULL DEFAULT 'pending',
    result           JSONB         NULL,
    error            TEXT          NULL,
    idempotency_key  VARCHAR(200)  NOT NULL,
    expires_at       TIMESTAMPTZ   NOT NULL,
    resolved_at      TIMESTAMPTZ   NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ai_pending_actions_uuid UNIQUE (uuid),
    CONSTRAINT uq_ai_pending_actions_idempotency UNIQUE (idempotency_key),
    CONSTRAINT chk_ai_pending_actions_source CHECK (source IN ('panel', 'portal', 'whatsapp', 'mcp')),
    CONSTRAINT chk_ai_pending_actions_status CHECK (
        status IN ('pending', 'executing', 'confirmed', 'failed', 'cancelled', 'expired')),
    CONSTRAINT chk_ai_pending_actions_resolved CHECK (
        (status IN ('pending', 'executing')) = (resolved_at IS NULL)),
    CONSTRAINT chk_ai_pending_actions_names CHECK (
        btrim(tool_use_id) <> '' AND btrim(tool_name) <> '' AND btrim(idempotency_key) <> ''),
    CONSTRAINT chk_ai_pending_actions_input CHECK (jsonb_typeof(input) = 'object'),
    CONSTRAINT chk_ai_pending_actions_preview CHECK (jsonb_typeof(preview) = 'object'),
    CONSTRAINT chk_ai_pending_actions_error CHECK (error IS NULL OR char_length(error) <= 5000),
    CONSTRAINT chk_ai_pending_actions_expires CHECK (expires_at > created_at)
);

CREATE INDEX idx_ai_pending_actions_user ON ai_pending_actions (organization_id, user_id, status, created_at DESC);
CREATE INDEX idx_ai_pending_actions_source ON ai_pending_actions (source, source_ref)
    WHERE status = 'pending';
CREATE INDEX idx_ai_pending_actions_expiry ON ai_pending_actions (expires_at)
    WHERE status = 'pending';
CREATE INDEX idx_ai_pending_actions_brand ON ai_pending_actions (brand_id, created_at DESC);
CREATE INDEX idx_ai_pending_actions_user_id ON ai_pending_actions (user_id);

CREATE TRIGGER trg_ai_pending_actions_set_updated_at
    BEFORE UPDATE ON ai_pending_actions
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_ai_pending_actions_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON ai_pending_actions
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 6. Usage ledger (append-only) ----------------------------------------------
CREATE TABLE ai_usage (
    id                  BIGSERIAL     PRIMARY KEY,
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    pool                VARCHAR(16)   NOT NULL DEFAULT 'org',
    user_id             BIGINT        NULL REFERENCES users (id) ON DELETE RESTRICT,
    channel             VARCHAR(16)   NOT NULL,
    purpose             VARCHAR(16)   NOT NULL,
    model               VARCHAR(128)  NOT NULL,
    input_tokens        BIGINT        NOT NULL DEFAULT 0,
    output_tokens       BIGINT        NOT NULL DEFAULT 0,
    cache_read_tokens   BIGINT        NOT NULL DEFAULT 0,
    cache_write_tokens  BIGINT        NOT NULL DEFAULT 0,
    quota_tokens        BIGINT        NOT NULL GENERATED ALWAYS AS (input_tokens + output_tokens + cache_write_tokens) STORED,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ai_usage_pool CHECK (pool IN ('org', 'system')),
    CONSTRAINT chk_ai_usage_channel CHECK (channel IN ('panel', 'portal', 'whatsapp', 'mcp', 'triage')),
    CONSTRAINT chk_ai_usage_purpose CHECK (purpose IN ('chat', 'title', 'triage', 'locale')),
    CONSTRAINT chk_ai_usage_model CHECK (btrim(model) <> ''),
    CONSTRAINT chk_ai_usage_tokens CHECK (
        input_tokens >= 0 AND output_tokens >= 0 AND cache_read_tokens >= 0 AND cache_write_tokens >= 0)
);

CREATE INDEX idx_ai_usage_org_created ON ai_usage (organization_id, created_at);
CREATE INDEX idx_ai_usage_brand_created ON ai_usage (brand_id, created_at);
CREATE INDEX idx_ai_usage_user_created ON ai_usage (user_id, created_at) WHERE user_id IS NOT NULL;

CREATE TRIGGER trg_ai_usage_check_org
    BEFORE INSERT ON ai_usage
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

CREATE FUNCTION ai_usage_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ai_usage is append-only: % rejected', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_ai_usage_append_only
    BEFORE UPDATE OR DELETE ON ai_usage
    FOR EACH ROW
    EXECUTE FUNCTION ai_usage_append_only();

CREATE TRIGGER trg_ai_usage_no_truncate
    BEFORE TRUNCATE ON ai_usage
    FOR EACH STATEMENT
    EXECUTE FUNCTION ai_usage_append_only();

-- 7. Monthly projection ------------------------------------------------------
CREATE TABLE ai_usage_monthly (
    organization_id     BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    brand_id            BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    pool                VARCHAR(16)   NOT NULL,
    period              CHAR(7)       NOT NULL,
    quota_tokens        BIGINT        NOT NULL DEFAULT 0,
    input_tokens        BIGINT        NOT NULL DEFAULT 0,
    output_tokens       BIGINT        NOT NULL DEFAULT 0,
    cache_read_tokens   BIGINT        NOT NULL DEFAULT 0,
    cache_write_tokens  BIGINT        NOT NULL DEFAULT 0,
    request_count       BIGINT        NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, pool, period),
    CONSTRAINT chk_ai_usage_monthly_pool CHECK (pool IN ('org', 'system')),
    CONSTRAINT chk_ai_usage_monthly_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_ai_usage_monthly_tokens CHECK (
        quota_tokens >= 0 AND input_tokens >= 0 AND output_tokens >= 0
        AND cache_read_tokens >= 0 AND cache_write_tokens >= 0 AND request_count >= 0)
);

CREATE INDEX idx_ai_usage_monthly_period ON ai_usage_monthly (period, brand_id);

CREATE TRIGGER trg_ai_usage_monthly_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON ai_usage_monthly
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 8. Permissions ---------------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Use AI assistant', 'ai.use', 'ai',
       ARRAY['own']::text[], false, false,
       'Chat with the AI assistant in the panel; own conversations only.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Read AI usage', 'ai.usage.read', 'ai',
       ARRAY['managed', 'subtree', 'brand', 'all']::text[], false, false,
       'Read AI token usage and quotas of the organizations in scope.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage AI settings', 'ai.settings.manage', 'ai',
       ARRAY['all']::text[], false, true,
       'Manage platform AI settings, models, quotas and the knowledge text.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Confirm AI actions', 'ai.actions.confirm', 'ai',
       ARRAY['own']::text[], false, false,
       'Confirm or cancel write actions proposed by the AI assistant for oneself.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'ai.use', 'own'),
    ('super_admin', 'ai.usage.read', 'all'),
    ('super_admin', 'ai.settings.manage', 'all'),
    ('super_admin', 'ai.actions.confirm', 'own'),
    ('center_staff', 'ai.use', 'own'),
    ('center_staff', 'ai.actions.confirm', 'own'),
    ('center_staff', 'ai.usage.read', 'brand'),
    ('center_warehouse', 'ai.use', 'own'),
    ('center_warehouse', 'ai.actions.confirm', 'own'),
    ('center_accounting', 'ai.use', 'own'),
    ('center_accounting', 'ai.actions.confirm', 'own'),
    ('center_accounting', 'ai.usage.read', 'brand'),
    ('center_social', 'ai.use', 'own'),
    ('center_social', 'ai.actions.confirm', 'own'),
    ('distributor_owner', 'ai.use', 'own'),
    ('distributor_owner', 'ai.actions.confirm', 'own'),
    ('distributor_owner', 'ai.usage.read', 'managed'),
    ('distributor_staff', 'ai.use', 'own'),
    ('distributor_staff', 'ai.actions.confirm', 'own'),
    ('distributor_warehouse_staff', 'ai.use', 'own'),
    ('distributor_warehouse_staff', 'ai.actions.confirm', 'own'),
    ('distributor_accounting', 'ai.use', 'own'),
    ('distributor_accounting', 'ai.actions.confirm', 'own'),
    ('dealer_owner', 'ai.use', 'own'),
    ('dealer_owner', 'ai.actions.confirm', 'own'),
    ('dealer_owner', 'ai.usage.read', 'managed'),
    ('dealer_staff', 'ai.use', 'own'),
    ('dealer_staff', 'ai.actions.confirm', 'own'),
    ('dealer_accounting', 'ai.use', 'own'),
    ('dealer_accounting', 'ai.actions.confirm', 'own')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
