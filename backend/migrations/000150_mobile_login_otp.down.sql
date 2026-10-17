DELETE FROM otp_codes WHERE type = 'mobile_login';
ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification', 'customer_login', 'contract_sign')
    );
