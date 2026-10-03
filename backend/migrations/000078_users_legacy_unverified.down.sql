-- TEC-255: revert the legacy_unverified exception. A contactless legacy row
-- cannot satisfy the original CHECK; the down migration refuses to run while
-- one exists instead of deleting or rewriting it.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE email IS NULL AND phone_e164 IS NULL) THEN
        RAISE EXCEPTION '000078 down: contactless legacy users exist; give them an e-mail or a phone first';
    END IF;
END;
$$;

ALTER TABLE users DROP CONSTRAINT chk_users_email_or_phone;
ALTER TABLE users ADD CONSTRAINT chk_users_email_or_phone CHECK (email IS NOT NULL OR phone_e164 IS NOT NULL);

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_legacy_phone_raw,
    DROP COLUMN legacy_phone_raw,
    DROP COLUMN legacy_unverified;
