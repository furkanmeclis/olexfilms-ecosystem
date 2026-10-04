-- Reverts TEC-322. Data loss: appointment settings, closure days and
-- appointments are removed.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'appointments.read', 'appointments.write', 'appointment_settings.manage')
);
DELETE FROM permissions WHERE slug IN (
    'appointments.read', 'appointments.write', 'appointment_settings.manage');

DROP TABLE IF EXISTS appointments;
DROP FUNCTION IF EXISTS appointments_check_row();
DROP TABLE IF EXISTS appointment_closures;
DROP TABLE IF EXISTS appointment_settings;
DROP FUNCTION IF EXISTS appointment_settings_check_row();
