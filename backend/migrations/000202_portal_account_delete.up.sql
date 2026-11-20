-- TEC-1257: the customer deletes their own account from the portal / mobile
-- app (App Store 5.1.1(v), KVKK). The phone code that confirms it has its own
-- purpose, so a sign-in code can never delete an account.
ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type,
    ADD CONSTRAINT chk_otp_codes_type CHECK (
        type IN ('password_reset', 'email_verification', 'customer_login', 'contract_sign', 'mobile_login',
                 'account_delete')
    );

-- Public legal texts (privacy policy, KVKK notice, terms of use): admin
-- edited Markdown per language, read without a session (sign-in screen).
-- They are never asked as a consent, so consents keeps its kinds.
ALTER TABLE legal_texts DROP CONSTRAINT chk_legal_texts_kind;
ALTER TABLE legal_texts ADD CONSTRAINT chk_legal_texts_kind CHECK (
    kind IN ('ai_guidelines', 'marketing_consent', 'privacy_policy', 'kvkk_notice', 'terms_of_use'));
