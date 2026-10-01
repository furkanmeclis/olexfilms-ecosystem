DROP TABLE IF EXISTS document_renders;
DROP TABLE IF EXISTS document_templates;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('platform.documents.templates.read', 'platform.documents.templates.write')
);

DELETE FROM permissions
WHERE slug IN ('platform.documents.templates.read', 'platform.documents.templates.write');
