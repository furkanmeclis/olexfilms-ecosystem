DELETE FROM legal_texts WHERE kind IN ('privacy_policy', 'kvkk_notice', 'terms_of_use');
ALTER TABLE legal_texts DROP CONSTRAINT chk_legal_texts_kind;
ALTER TABLE legal_texts ADD CONSTRAINT chk_legal_texts_kind CHECK (
    kind IN ('ai_guidelines', 'marketing_consent'));

DELETE FROM otp_codes WHERE type = 'account_delete';
ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification', 'customer_login', 'contract_sign', 'mobile_login')
    );
