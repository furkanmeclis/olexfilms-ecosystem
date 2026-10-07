-- Reverts TEC-466. Data loss: every dealer showcase with its services,
-- photo rows (the stored objects stay in the bucket) and published
-- snapshot, plus the showcase permissions and their grants.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('showcase.read', 'showcase.write', 'platform.showcase.review')
);
DELETE FROM permissions WHERE slug IN ('showcase.read', 'showcase.write', 'platform.showcase.review');

DROP TABLE IF EXISTS dealer_showcase_photos;
DROP TABLE IF EXISTS dealer_showcase_services;
DROP TABLE IF EXISTS dealer_showcases;
DROP FUNCTION IF EXISTS dealer_showcases_check_org();
DROP FUNCTION IF EXISTS dealer_showcase_keywords_valid(TEXT[]);
DROP FUNCTION IF EXISTS dealer_showcase_social_links_valid(JSONB);
DROP FUNCTION IF EXISTS dealer_showcase_content_valid(JSONB);
DROP FUNCTION IF EXISTS dealer_showcase_locale_texts_valid(JSONB);
