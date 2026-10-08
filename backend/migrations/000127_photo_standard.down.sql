DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('photo_standard.manage', 'photo_standard.override')
);
DELETE FROM permissions WHERE slug IN ('photo_standard.manage', 'photo_standard.override');

DROP TABLE IF EXISTS intake_photos;
DROP TABLE IF EXISTS photo_angle_overrides;
DROP FUNCTION IF EXISTS photo_angle_overrides_check_row();
DROP TABLE IF EXISTS photo_angles;
DROP FUNCTION IF EXISTS photo_angles_check_row();
