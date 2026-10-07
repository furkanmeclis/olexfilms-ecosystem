-- Reverts TEC-464. Removes only the locale seed rows introduced by
-- 000110_consent_text_locales; Turkish and English seed rows are untouched.

DELETE FROM legal_texts
WHERE version = 1
  AND kind IN ('ai_guidelines', 'marketing_consent')
  AND locale IN ('ar', 'az', 'bg', 'de', 'el', 'es', 'fr', 'it', 'ru', 'uk', 'zh-CN');

DELETE FROM kvkk_notices
WHERE version = 1
  AND locale IN ('ar', 'az', 'bg', 'de', 'el', 'es', 'fr', 'it', 'ru', 'uk', 'zh-CN');
