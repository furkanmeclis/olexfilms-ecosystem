DELETE FROM notification_templates
WHERE code IN ('exports.ready', 'imports.applied', 'imports.failed');

DROP TABLE IF EXISTS activity_events;
DROP TABLE IF EXISTS import_changes;
DROP TABLE IF EXISTS import_jobs;
DROP TABLE IF EXISTS export_jobs;
DROP TABLE IF EXISTS app_settings;
