-- Data-loss: drops WhatsApp conversations/messages, connection history, KVKK
-- notice versions and phone-only OTP rows / users (rows without an e-mail
-- cannot satisfy the restored NOT NULL constraints).

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'whatsapp.manage');
DELETE FROM permissions WHERE slug = 'whatsapp.manage';

DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS whatsapp_connection_events;
DROP TABLE IF EXISTS whatsapp_settings;
DROP TABLE IF EXISTS kvkk_notices;

DELETE FROM otp_codes WHERE email IS NULL OR type IN ('customer_login', 'contract_sign');
DROP INDEX IF EXISTS idx_otp_codes_phone_created;
DROP INDEX IF EXISTS idx_otp_codes_phone_type_active;
ALTER TABLE otp_codes
    DROP CONSTRAINT IF EXISTS chk_otp_codes_email_or_phone,
    DROP CONSTRAINT IF EXISTS chk_otp_codes_channel,
    DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (type IN ('password_reset', 'email_verification')),
    DROP COLUMN delivery_error,
    DROP COLUMN delivered_at,
    DROP COLUMN kvkk_version,
    DROP COLUMN kvkk_locale,
    DROP COLUMN user_agent,
    DROP COLUMN ip,
    DROP COLUMN message_sha256,
    DROP COLUMN provider_ref,
    DROP COLUMN channel,
    DROP COLUMN phone_e164,
    ALTER COLUMN email SET NOT NULL;

DELETE FROM users WHERE email IS NULL;
DROP INDEX IF EXISTS uq_users_phone_e164_active;
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_email_or_phone,
    DROP CONSTRAINT IF EXISTS chk_users_phone_e164,
    DROP COLUMN phone_verified_at,
    DROP COLUMN phone_e164,
    ALTER COLUMN email SET NOT NULL;
