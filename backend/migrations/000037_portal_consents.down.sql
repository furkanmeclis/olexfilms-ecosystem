DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'platform.legal_texts.write');
DELETE FROM permissions WHERE slug = 'platform.legal_texts.write';

DROP TABLE IF EXISTS consents;
DROP TABLE IF EXISTS legal_texts;
