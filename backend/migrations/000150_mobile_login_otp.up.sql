-- Isolate staff mobile codes from portal login and contract signing codes.
ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification', 'customer_login', 'contract_sign', 'mobile_login')
    );
