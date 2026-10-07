-- Reverts TEC-400. Data loss: MCP OAuth clients, pending requests, codes,
-- grants (connected apps) and tokens are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('mcp.connect', 'mcp.clients.manage')
);
DELETE FROM permissions WHERE slug IN ('mcp.connect', 'mcp.clients.manage');

DROP TABLE IF EXISTS oauth_tokens;
DROP TABLE IF EXISTS oauth_codes;
DROP TABLE IF EXISTS oauth_grants;
DROP TABLE IF EXISTS oauth_auth_requests;
DROP TABLE IF EXISTS oauth_clients;
