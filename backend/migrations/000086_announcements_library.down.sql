-- TEC-329: drops announcements, the document library and their
-- permissions. Data loss: every announcement, read receipt and library
-- file record is removed (the stored objects stay in object storage).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN ('announcements.read', 'announcements.write', 'library.read', 'library.manage')
);
DELETE FROM permissions
WHERE slug IN ('announcements.read', 'announcements.write', 'library.read', 'library.manage');

DROP TABLE IF EXISTS library_item_versions;
DROP FUNCTION IF EXISTS library_item_versions_append_only();
DROP TABLE IF EXISTS library_items;
DROP TABLE IF EXISTS library_folders;
DROP FUNCTION IF EXISTS library_folders_check_cycle();

DROP TABLE IF EXISTS announcement_reads;
DROP TABLE IF EXISTS announcement_audiences;
DROP FUNCTION IF EXISTS announcement_audiences_check_target();
DROP TABLE IF EXISTS announcement_locales;
DROP TABLE IF EXISTS announcements;
DROP FUNCTION IF EXISTS announcements_check_author();
