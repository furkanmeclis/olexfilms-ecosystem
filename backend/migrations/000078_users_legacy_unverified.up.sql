-- TEC-255 (F2-01d): legacy customers without a usable contact (K26, K29).
--
-- The migrator turns every legacy hub customer into a users row. A customer
-- whose phone is empty or cannot be parsed into E.164, and who has no e-mail,
-- has nothing chk_users_email_or_phone accepts. The row is still needed:
-- services and warranties of that customer point at it.
--
-- Conservative change: chk_users_email_or_phone keeps its meaning for every
-- account and gains one narrow exception, a row explicitly marked
-- legacy_unverified (only the migrator sets it) that has verified nothing.
-- No other path can create a contactless account.
--
-- legacy_phone_raw keeps the unresolved legacy phone text for an operator
-- (like organizations.phone_raw); it exists only on marked rows.
--
-- Claim (K26): once the account has a phone and a successful OTP on that
-- E.164 stamps phone_verified_at, MarkUserPhoneVerified clears both columns.
-- From then on the plain e-mail-or-phone rule applies again.
ALTER TABLE users
    ADD COLUMN legacy_unverified BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN legacy_phone_raw  VARCHAR(64) NULL,
    ADD CONSTRAINT chk_users_legacy_phone_raw CHECK (legacy_phone_raw IS NULL OR legacy_unverified);

ALTER TABLE users DROP CONSTRAINT chk_users_email_or_phone;
ALTER TABLE users ADD CONSTRAINT chk_users_email_or_phone CHECK (
    email IS NOT NULL
    OR phone_e164 IS NOT NULL
    OR (legacy_unverified AND phone_verified_at IS NULL AND email_verified_at IS NULL)
);
