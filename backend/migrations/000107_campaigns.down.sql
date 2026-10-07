-- Reverts TEC-404. Data loss: campaigns with their contents, media, events
-- and recipient snapshots, every marketing_consent decision and text version,
-- and the campaigns.approve permission plus the default campaign grants of
-- distributor_owner and dealer_owner are removed.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'campaigns.approve');
DELETE FROM permissions WHERE slug = 'campaigns.approve';
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND r.slug IN ('distributor_owner', 'dealer_owner')
  AND p.slug IN ('campaigns.read', 'campaigns.write');

DROP INDEX IF EXISTS idx_consents_kind_user;
DELETE FROM consents WHERE kind = 'marketing_consent';
DELETE FROM legal_texts WHERE kind = 'marketing_consent';
ALTER TABLE consents DROP CONSTRAINT IF EXISTS chk_consents_kind;
ALTER TABLE consents ADD CONSTRAINT chk_consents_kind CHECK (kind IN ('ai_guidelines'));
ALTER TABLE legal_texts DROP CONSTRAINT IF EXISTS chk_legal_texts_kind;
ALTER TABLE legal_texts ADD CONSTRAINT chk_legal_texts_kind CHECK (kind IN ('ai_guidelines'));

DROP TABLE IF EXISTS campaign_recipients;
DROP FUNCTION IF EXISTS campaign_recipients_project_insert();
DROP FUNCTION IF EXISTS campaign_recipients_project_update();
DROP FUNCTION IF EXISTS campaign_recipients_project_delete();
DROP FUNCTION IF EXISTS campaign_recipients_apply_counts(BIGINT, BIGINT, BIGINT, BIGINT, BIGINT);
DROP TABLE IF EXISTS campaign_events;
DROP FUNCTION IF EXISTS campaign_events_append_only();
DROP TABLE IF EXISTS campaign_media;
DROP TABLE IF EXISTS campaign_contents;
DROP TABLE IF EXISTS campaigns;
DROP FUNCTION IF EXISTS campaign_channels_valid(TEXT[]);
