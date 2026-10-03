DROP INDEX IF EXISTS idx_tasks_due_scan;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS overdue_notified_at,
    DROP COLUMN IF EXISTS due_soon_notified_at;
