ALTER TABLE integration_connections
    DROP CONSTRAINT IF EXISTS chk_integration_connections_package_window,
    DROP COLUMN IF EXISTS package_expires_at,
    DROP COLUMN IF EXISTS package_issued_at,
    DROP COLUMN IF EXISTS pending_key_issued_at,
    DROP COLUMN IF EXISTS pending_api_key_enc;
