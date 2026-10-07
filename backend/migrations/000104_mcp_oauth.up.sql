-- TEC-400 (F4-03a): OAuth 2.1 authorization server for the MCP endpoints
-- (/mcp/dealer, /mcp/customer, /mcp/user). Adapted from technowide-ecosystem
-- 000037_mcp_oauth.
--
--   * oauth_clients: RFC 7591 dynamic registrations (public clients only,
--     token_endpoint_auth_method = none). revoked_at blocks the client
--     (platform admin, F4-03b).
--   * oauth_auth_requests: a validated /oauth/authorize request waiting for
--     the user's consent (15 minutes).
--   * oauth_codes: authorization codes (SHA-256, 5 minutes, PKCE S256).
--     used_at + family let a replayed code revoke what it issued.
--   * oauth_grants: the user's "connected apps": one row per user + client +
--     resource + organization, created on consent.
--   * oauth_tokens: access (1 hour) and refresh (30 days) tokens as SHA-256.
--     One code exchange = one family; refresh rotation keeps the family and
--     reuse of a rotated refresh token revokes the whole family.
--
-- resource is the MCP endpoint path (RFC 8707 audience); a token is only
-- valid on that endpoint. Grants, codes and tokens carry the organization
-- chosen on consent and its brand.

-- 1. Clients ------------------------------------------------------------------
CREATE TABLE oauth_clients (
    id            BIGSERIAL     PRIMARY KEY,
    uuid          UUID          NOT NULL DEFAULT gen_random_uuid(),
    client_id     VARCHAR(64)   NOT NULL,
    client_name   VARCHAR(200)  NOT NULL,
    redirect_uris TEXT[]        NOT NULL,
    created_ip    VARCHAR(64)   NULL,
    last_used_at  TIMESTAMPTZ   NULL,
    revoked_at    TIMESTAMPTZ   NULL,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_oauth_clients_uuid UNIQUE (uuid),
    CONSTRAINT uq_oauth_clients_client_id UNIQUE (client_id),
    CONSTRAINT chk_oauth_clients_redirect_uris CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10)
);

CREATE INDEX idx_oauth_clients_created_at ON oauth_clients (created_at);

-- 2. Pending authorization requests ---------------------------------------------
CREATE TABLE oauth_auth_requests (
    id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id      VARCHAR(64)   NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    redirect_uri   TEXT          NOT NULL,
    code_challenge VARCHAR(128)  NOT NULL,
    state          TEXT          NULL,
    scopes         TEXT[]        NOT NULL DEFAULT ARRAY['mcp']::text[],
    resource       VARCHAR(32)   NOT NULL,
    expires_at     TIMESTAMPTZ   NOT NULL,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_oauth_auth_requests_resource CHECK (resource IN ('/mcp/dealer', '/mcp/customer', '/mcp/user'))
);

CREATE INDEX idx_oauth_auth_requests_expires ON oauth_auth_requests (expires_at);

-- 3. Grants ("connected apps") ----------------------------------------------------
CREATE TABLE oauth_grants (
    id              BIGSERIAL     PRIMARY KEY,
    uuid            UUID          NOT NULL DEFAULT gen_random_uuid(),
    user_id         BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id       VARCHAR(64)   NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    organization_id BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id        BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    resource        VARCHAR(32)   NOT NULL,
    scopes          TEXT[]        NOT NULL DEFAULT ARRAY['mcp']::text[],
    last_used_at    TIMESTAMPTZ   NULL,
    revoked_at      TIMESTAMPTZ   NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_oauth_grants_uuid UNIQUE (uuid),
    CONSTRAINT uq_oauth_grants_key UNIQUE (user_id, client_id, resource, organization_id),
    CONSTRAINT chk_oauth_grants_resource CHECK (resource IN ('/mcp/dealer', '/mcp/customer', '/mcp/user'))
);

CREATE INDEX idx_oauth_grants_client ON oauth_grants (client_id);
CREATE INDEX idx_oauth_grants_organization ON oauth_grants (organization_id);

CREATE TRIGGER trg_oauth_grants_set_updated_at
    BEFORE UPDATE ON oauth_grants
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_oauth_grants_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON oauth_grants
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 4. Authorization codes ----------------------------------------------------------
CREATE TABLE oauth_codes (
    code_hash       VARCHAR(64)   PRIMARY KEY,
    grant_id        BIGINT        NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    client_id       VARCHAR(64)   NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    user_id         BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id        BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    redirect_uri    TEXT          NOT NULL,
    code_challenge  VARCHAR(128)  NOT NULL,
    resource        VARCHAR(32)   NOT NULL,
    scopes          TEXT[]        NOT NULL,
    expires_at      TIMESTAMPTZ   NOT NULL,
    used_at         TIMESTAMPTZ   NULL,
    -- The token family issued from this code; a replayed code revokes it.
    family          UUID          NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_oauth_codes_resource CHECK (resource IN ('/mcp/dealer', '/mcp/customer', '/mcp/user')),
    CONSTRAINT chk_oauth_codes_used CHECK ((used_at IS NULL) = (family IS NULL))
);

CREATE INDEX idx_oauth_codes_expires ON oauth_codes (expires_at);
CREATE INDEX idx_oauth_codes_grant ON oauth_codes (grant_id);

CREATE TRIGGER trg_oauth_codes_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON oauth_codes
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 5. Tokens ---------------------------------------------------------------------
CREATE TABLE oauth_tokens (
    id              BIGSERIAL     PRIMARY KEY,
    token_hash      VARCHAR(64)   NOT NULL,
    kind            VARCHAR(10)   NOT NULL,
    family          UUID          NOT NULL,
    grant_id        BIGINT        NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    client_id       VARCHAR(64)   NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    user_id         BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id        BIGINT        NOT NULL REFERENCES brands (id) ON DELETE RESTRICT,
    resource        VARCHAR(32)   NOT NULL,
    scopes          TEXT[]        NOT NULL,
    expires_at      TIMESTAMPTZ   NOT NULL,
    revoked_at      TIMESTAMPTZ   NULL,
    last_used_at    TIMESTAMPTZ   NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_oauth_tokens_hash UNIQUE (token_hash),
    CONSTRAINT chk_oauth_tokens_kind CHECK (kind IN ('access', 'refresh')),
    CONSTRAINT chk_oauth_tokens_resource CHECK (resource IN ('/mcp/dealer', '/mcp/customer', '/mcp/user'))
);

CREATE INDEX idx_oauth_tokens_family ON oauth_tokens (family);
CREATE INDEX idx_oauth_tokens_grant ON oauth_tokens (grant_id);
CREATE INDEX idx_oauth_tokens_client ON oauth_tokens (client_id);
CREATE INDEX idx_oauth_tokens_expires ON oauth_tokens (expires_at);

CREATE TRIGGER trg_oauth_tokens_check_org
    BEFORE INSERT OR UPDATE OF organization_id, brand_id ON oauth_tokens
    FOR EACH ROW
    EXECUTE FUNCTION customer_scope_check_org();

-- 6. Permissions ----------------------------------------------------------------
INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Connect MCP clients', 'mcp.connect', 'mcp',
       ARRAY['own']::text[], false, false,
       'Connect AI agents (MCP clients) to the panel MCP endpoints with one''s own permissions.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO permissions (name, slug, module, scopes, is_sensitive, super_admin_only, description, sort_order)
SELECT 'Manage MCP clients', 'mcp.clients.manage', 'mcp',
       ARRAY['all']::text[], false, true,
       'List and revoke registered MCP (OAuth) clients platform wide.',
       COALESCE(MAX(sort_order), 0) + 10
FROM permissions
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, scope)
SELECT r.id, p.id, g.scope
FROM (VALUES
    ('super_admin', 'mcp.connect', 'own'),
    ('super_admin', 'mcp.clients.manage', 'all'),
    ('center_staff', 'mcp.connect', 'own'),
    ('center_warehouse', 'mcp.connect', 'own'),
    ('center_accounting', 'mcp.connect', 'own'),
    ('center_social', 'mcp.connect', 'own'),
    ('distributor_owner', 'mcp.connect', 'own'),
    ('dealer_owner', 'mcp.connect', 'own')
) AS g (role_slug, perm_slug, scope)
JOIN roles r ON r.slug = g.role_slug
JOIN permissions p ON p.slug = g.perm_slug
ON CONFLICT (role_id, permission_id) DO UPDATE SET scope = EXCLUDED.scope;
