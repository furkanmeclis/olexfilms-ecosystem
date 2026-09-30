DELETE FROM notification_templates
WHERE code IN ('bulk.completed', 'bulk.failed');

DROP TABLE IF EXISTS bulk_changes;
DROP TABLE IF EXISTS bulk_jobs;
