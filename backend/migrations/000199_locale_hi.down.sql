-- Reverts 000199_locale_hi. Hindi data cannot be kept under the 14 locale
-- constraints: the seeded hi rows and hi child rows are removed, hi parent
-- values fall back to tr (users inherit again), and showcase JSON loses its
-- hi keys. Legal texts, KVKK notices and document templates have no locale
-- constraint; their hi rows are only removed when nothing references them.

DELETE FROM notification_templates WHERE language = 'hi';

DELETE FROM legal_texts l
WHERE l.locale = 'hi'
  AND NOT EXISTS (SELECT 1 FROM consents c WHERE c.legal_text_id = l.id);
DELETE FROM kvkk_notices WHERE locale = 'hi' AND version = 1;
DELETE FROM document_templates t
WHERE t.language = 'hi' AND t.brand_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM document_renders r WHERE r.template_id = t.id);

UPDATE users SET locale = NULL WHERE locale = 'hi';
UPDATE organizations SET locale = 'tr' WHERE locale = 'hi';
DELETE FROM contract_template_locales WHERE locale = 'hi';
UPDATE contract_instances SET locale = 'tr' WHERE locale = 'hi';
UPDATE announcements SET default_locale = 'tr' WHERE default_locale = 'hi';
DELETE FROM announcement_locales WHERE locale = 'hi';
DELETE FROM library_item_versions WHERE locale = 'hi';
DELETE FROM review_question_locales WHERE locale = 'hi';
DELETE FROM campaign_contents WHERE locale = 'hi';
UPDATE campaign_recipients SET locale = 'tr' WHERE locale = 'hi';
UPDATE fleet_profiles SET report_locale = 'tr' WHERE report_locale = 'hi';
UPDATE fleet_reports SET locale = 'tr' WHERE locale = 'hi';

UPDATE dealer_showcases SET content = content - 'hi' WHERE content ? 'hi';
UPDATE dealer_showcase_services SET title = title - 'hi' WHERE title ? 'hi';
UPDATE dealer_showcase_services SET description = description - 'hi' WHERE description ? 'hi';
UPDATE dealer_showcase_photos SET caption = caption - 'hi' WHERE caption ? 'hi';

ALTER TABLE users DROP CONSTRAINT chk_users_locale;
ALTER TABLE users ADD CONSTRAINT chk_users_locale CHECK (
    locale IS NULL OR locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE organizations DROP CONSTRAINT chk_organizations_locale;
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE notification_templates DROP CONSTRAINT notification_templates_language_chk;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_language_chk CHECK (
    language IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE contract_template_locales DROP CONSTRAINT chk_contract_template_locales_locale;
ALTER TABLE contract_template_locales ADD CONSTRAINT chk_contract_template_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE contract_instances DROP CONSTRAINT chk_contract_instances_locale;
ALTER TABLE contract_instances ADD CONSTRAINT chk_contract_instances_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE announcements DROP CONSTRAINT chk_announcements_default_locale;
ALTER TABLE announcements ADD CONSTRAINT chk_announcements_default_locale CHECK (
    default_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE announcement_locales DROP CONSTRAINT chk_announcement_locales_locale;
ALTER TABLE announcement_locales ADD CONSTRAINT chk_announcement_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE library_item_versions DROP CONSTRAINT chk_library_item_versions_locale;
ALTER TABLE library_item_versions ADD CONSTRAINT chk_library_item_versions_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE review_question_locales DROP CONSTRAINT chk_review_question_locales_locale;
ALTER TABLE review_question_locales ADD CONSTRAINT chk_review_question_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE campaign_contents DROP CONSTRAINT chk_campaign_contents_locale;
ALTER TABLE campaign_contents ADD CONSTRAINT chk_campaign_contents_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE campaign_recipients DROP CONSTRAINT chk_campaign_recipients_locale;
ALTER TABLE campaign_recipients ADD CONSTRAINT chk_campaign_recipients_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE fleet_profiles DROP CONSTRAINT chk_fleet_profiles_report_locale;
ALTER TABLE fleet_profiles ADD CONSTRAINT chk_fleet_profiles_report_locale CHECK (
    report_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE fleet_reports DROP CONSTRAINT chk_fleet_reports_locale;
ALTER TABLE fleet_reports ADD CONSTRAINT chk_fleet_reports_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);

CREATE OR REPLACE FUNCTION dealer_showcase_locale_texts_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
              OR jsonb_typeof(e.value) <> 'string'
       )
$$;

CREATE OR REPLACE FUNCTION dealer_showcase_content_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
              OR jsonb_typeof(e.value) <> 'object'
              OR EXISTS (
                  SELECT 1 FROM jsonb_each(e.value) f
                  WHERE f.key NOT IN ('headline', 'about') OR jsonb_typeof(f.value) <> 'string'
              )
       )
$$;
