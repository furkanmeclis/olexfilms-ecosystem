-- Data-loss: drops QR sign-in challenges and the device columns of
-- refresh_tokens; mobile sessions are revoked rows afterwards (realm reset to
-- panel so the restored check constraint holds).

DROP TABLE IF EXISTS qr_login_challenges;

DROP INDEX IF EXISTS idx_refresh_tokens_user_device;
DROP INDEX IF EXISTS idx_refresh_tokens_family;

UPDATE refresh_tokens
SET realm = 'panel', revoked_at = COALESCE(revoked_at, NOW())
WHERE realm = 'mobile';

ALTER TABLE refresh_tokens
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_mobile_device,
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_platform,
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_client,
    DROP COLUMN rotated_at,
    DROP COLUMN family_id,
    DROP COLUMN app_version,
    DROP COLUMN platform,
    DROP COLUMN device_name,
    DROP COLUMN device_id,
    DROP COLUMN client,
    DROP CONSTRAINT chk_refresh_tokens_realm,
    ADD CONSTRAINT chk_refresh_tokens_realm CHECK (realm IN ('panel', 'portal'));
